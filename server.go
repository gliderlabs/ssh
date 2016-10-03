package ssh

import (
	"fmt"
	"net"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

type Server struct {
	Addr                string
	Handler             Handler
	HostSigners         []Signer
	PasswordHandler     PasswordHandler
	PublicKeyHandler    PublicKeyHandler
	PermissionsCallback PermissionsCallback
	PtyCallback         PtyCallback
}

func (srv *Server) makeConfig() (*gossh.ServerConfig, error) {
	config := &gossh.ServerConfig{}
	if len(srv.HostSigners) == 0 {
		signer, err := generateSigner()
		if err != nil {
			return nil, err
		}
		srv.HostSigners = append(srv.HostSigners, signer)
	}
	for _, signer := range srv.HostSigners {
		config.AddHostKey(signer)
	}
	if srv.PasswordHandler == nil && srv.PublicKeyHandler == nil {
		config.NoClientAuth = true
	}
	if srv.PasswordHandler != nil {
		config.PasswordCallback = func(conn gossh.ConnMetadata, password []byte) (*gossh.Permissions, error) {
			perms := &gossh.Permissions{}
			if ok := srv.PasswordHandler(conn.User(), string(password)); !ok {
				return perms, fmt.Errorf("permission denied")
			}
			if srv.PermissionsCallback != nil {
				srv.PermissionsCallback(conn.User(), &Permissions{perms})
			}
			return perms, nil
		}
	}
	if srv.PublicKeyHandler != nil {
		config.PublicKeyCallback = func(conn gossh.ConnMetadata, key gossh.PublicKey) (*gossh.Permissions, error) {
			perms := &gossh.Permissions{}
			if ok := srv.PublicKeyHandler(conn.User(), key); !ok {
				return perms, fmt.Errorf("permission denied")
			}
			perms.Extensions = map[string]string{
				"_publickey": string(key.Marshal()),
			}
			if srv.PermissionsCallback != nil {
				srv.PermissionsCallback(conn.User(), &Permissions{perms})
			}
			return perms, nil
		}
	}
	return config, nil
}

func (srv *Server) Handle(fn Handler) {
	srv.Handler = fn
}

func (srv *Server) Serve(l net.Listener) error {
	defer l.Close()
	config, err := srv.makeConfig()
	if err != nil {
		return err
	}
	if srv.Handler == nil {
		srv.Handler = defaultHandler
	}
	var tempDelay time.Duration
	for {
		conn, e := l.Accept()
		if e != nil {
			if ne, ok := e.(net.Error); ok && ne.Temporary() {
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}
				if max := 1 * time.Second; tempDelay > max {
					tempDelay = max
				}
				//srv.logf("http: Accept error: %v; retrying in %v", e, tempDelay)
				time.Sleep(tempDelay)
				continue
			}
			return e
		}
		go srv.handleConn(conn, config)
	}
}

func (srv *Server) handleConn(conn net.Conn, conf *gossh.ServerConfig) {
	defer conn.Close()
	sshConn, chans, reqs, err := gossh.NewServerConn(conn, conf)
	if err != nil {
		return
	}
	go gossh.DiscardRequests(reqs)
	for ch := range chans {
		if ch.ChannelType() != "session" {
			ch.Reject(gossh.UnknownChannelType, "unsupported channel type")
			continue
		}
		go srv.handleChannel(sshConn, ch)
	}
}

func (srv *Server) handleChannel(conn *gossh.ServerConn, newChan gossh.NewChannel) {
	ch, reqs, err := newChan.Accept()
	if err != nil {
		return
	}
	sess := srv.newSession(conn, ch)
	sess.handleRequests(reqs)
}

func (srv *Server) newSession(conn *gossh.ServerConn, ch gossh.Channel) *session {
	sess := &session{
		Channel: ch,
		conn:    conn,
		handler: srv.Handler,
		ptyCb:   srv.PtyCallback,
	}
	return sess
}

func (srv *Server) ListenAndServe() error {
	addr := srv.Addr
	if addr == "" {
		addr = ":22"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return srv.Serve(ln)
}

func (srv *Server) SetOption(option Option) error {
	return option(srv)
}
