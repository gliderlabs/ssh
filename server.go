package ssh

import (
	"fmt"
	"net"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

// Server defines parameters for running an SSH server. The zero value for
// Server is a valid configuration. When both PasswordHandler and
// PublicKeyHandler are nil, no client authentication is performed.
type Server struct {
	Addr        string   // TCP address to listen on, ":22" if empty
	Handler     Handler  // handler to invoke, ssh.DefaultHandler if nil
	HostSigners []Signer // private keys for the host key, must have at least one
	Version     string   // server version to be sent before the initial handshake

	PasswordHandler             PasswordHandler             // password authentication handler
	PublicKeyHandler            PublicKeyHandler            // public key authentication handler
	PtyCallback                 PtyCallback                 // callback for allowing PTY sessions, allows all if nil
	LocalPortForwardingCallback LocalPortForwardingCallback // callback for allowing local port forwarding, denies all if nil

	channelHandlers map[string]channelHandler
}

// internal for now
type channelHandler func(srv *Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx *sshContext)

func (srv *Server) ensureHostSigner() error {
	if len(srv.HostSigners) == 0 {
		signer, err := generateSigner()
		if err != nil {
			return err
		}
		srv.HostSigners = append(srv.HostSigners, signer)
	}
	return nil
}

func (srv *Server) config(ctx *sshContext) *gossh.ServerConfig {
	srv.channelHandlers = map[string]channelHandler{
		"session":      sessionHandler,
		"direct-tcpip": directTcpipHandler,
	}
	config := &gossh.ServerConfig{}
	for _, signer := range srv.HostSigners {
		config.AddHostKey(signer)
	}
	if srv.PasswordHandler == nil && srv.PublicKeyHandler == nil {
		config.NoClientAuth = true
	}
	if srv.Version != "" {
		config.ServerVersion = "SSH-2.0-" + srv.Version
	}
	if srv.PasswordHandler != nil {
		config.PasswordCallback = func(conn gossh.ConnMetadata, password []byte) (*gossh.Permissions, error) {
			ctx.applyConnMetadata(conn)
			if ok := srv.PasswordHandler(ctx, string(password)); !ok {
				return ctx.Permissions().Permissions, fmt.Errorf("permission denied")
			}
			return ctx.Permissions().Permissions, nil
		}
	}
	if srv.PublicKeyHandler != nil {
		config.PublicKeyCallback = func(conn gossh.ConnMetadata, key gossh.PublicKey) (*gossh.Permissions, error) {
			ctx.applyConnMetadata(conn)
			if ok := srv.PublicKeyHandler(ctx, key); !ok {
				return ctx.Permissions().Permissions, fmt.Errorf("permission denied")
			}
			ctx.SetValue(ContextKeyPublicKey, key)
			return ctx.Permissions().Permissions, nil
		}
	}
	return config
}

// Handle sets the Handler for the server.
func (srv *Server) Handle(fn Handler) {
	srv.Handler = fn
}

// Serve accepts incoming connections on the Listener l, creating a new
// connection goroutine for each. The connection goroutines read requests and then
// calls srv.Handler to handle sessions.
//
// Serve always returns a non-nil error.
func (srv *Server) Serve(l net.Listener) error {
	defer l.Close()
	if err := srv.ensureHostSigner(); err != nil {
		return err
	}
	if srv.Handler == nil {
		srv.Handler = DefaultHandler
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
				time.Sleep(tempDelay)
				continue
			}
			return e
		}
		go srv.handleConn(conn)
	}
}

func (srv *Server) handleConn(conn net.Conn) {
	defer conn.Close()
	ctx := newContext(srv)
	sshConn, chans, reqs, err := gossh.NewServerConn(conn, srv.config(ctx))
	if err != nil {
		// TODO: trigger event callback
		return
	}
	ctx.SetValue(ContextKeyConn, sshConn)
	ctx.applyConnMetadata(sshConn)
	go gossh.DiscardRequests(reqs)
	for ch := range chans {
		handler, found := srv.channelHandlers[ch.ChannelType()]
		if !found {
			ch.Reject(gossh.UnknownChannelType, "unsupported channel type")
			continue
		}
		go handler(srv, sshConn, ch, ctx)
	}
}

// ListenAndServe listens on the TCP network address srv.Addr and then calls
// Serve to handle incoming connections. If srv.Addr is blank, ":22" is used.
// ListenAndServe always returns a non-nil error.
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

// AddHostKey adds a private key as a host key. If an existing host key exists
// with the same algorithm, it is overwritten. Each server config must have at
// least one host key.
func (srv *Server) AddHostKey(key Signer) {
	// these are later added via AddHostKey on ServerConfig, which performs the
	// check for one of every algorithm.
	srv.HostSigners = append(srv.HostSigners, key)
}

// SetOption runs a functional option against the server.
func (srv *Server) SetOption(option Option) error {
	return option(srv)
}
