package ssh

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

// ErrServerClosed is returned by the Server's Serve, ListenAndServe,
// and ListenAndServeTLS methods after a call to Shutdown or Close.
var ErrServerClosed = errors.New("ssh: Server closed")

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
	ConnCallback                ConnCallback                // optional callback for wrapping net.Conn before handling
	LocalPortForwardingCallback LocalPortForwardingCallback // callback for allowing local port forwarding, denies all if nil

	IdleTimeout time.Duration // connection timeout when no activity, none if empty
	MaxTimeout  time.Duration // absolute connection timeout, none if empty

	channelHandlers map[string]channelHandler

	mu        sync.Mutex
	listeners map[net.Listener]struct{}
	conns     map[*gossh.ServerConn]struct{}
	doneChan  chan struct{}
}

// internal for now
type channelHandler func(srv *Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx Context)

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

func (srv *Server) config(ctx Context) *gossh.ServerConfig {
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
			applyConnMetadata(ctx, conn)
			if ok := srv.PasswordHandler(ctx, string(password)); !ok {
				return ctx.Permissions().Permissions, fmt.Errorf("permission denied")
			}
			return ctx.Permissions().Permissions, nil
		}
	}
	if srv.PublicKeyHandler != nil {
		config.PublicKeyCallback = func(conn gossh.ConnMetadata, key gossh.PublicKey) (*gossh.Permissions, error) {
			applyConnMetadata(ctx, conn)
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

// Close immediately closes all active listeners and all active
// connections.
//
// Close returns any error returned from closing the Server's
// underlying Listener(s).
func (srv *Server) Close() error {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	srv.closeDoneChanLocked()
	err := srv.closeListenersLocked()
	for c := range srv.conns {
		c.Close()
		delete(srv.conns, c)
	}
	return err
}

// shutdownPollInterval is how often we poll for quiescence
// during Server.Shutdown. This is lower during tests, to
// speed up tests.
// Ideally we could find a solution that doesn't involve polling,
// but which also doesn't have a high runtime cost (and doesn't
// involve any contentious mutexes), but that is left as an
// exercise for the reader.
var shutdownPollInterval = 500 * time.Millisecond

// Shutdown gracefully shuts down the server without interrupting any
// active connections. Shutdown works by first closing all open
// listeners, and then waiting indefinitely for connections to close.
// If the provided context expires before the shutdown is complete,
// then the context's error is returned.
func (srv *Server) Shutdown(ctx context.Context) error {
	srv.mu.Lock()
	lnerr := srv.closeListenersLocked()
	srv.closeDoneChanLocked()
	srv.mu.Unlock()
	ticker := time.NewTicker(shutdownPollInterval)
	defer ticker.Stop()
	for {
		srv.mu.Lock()
		conns := len(srv.conns)
		srv.mu.Unlock()
		if conns == 0 {
			return lnerr
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}

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
	if srv.channelHandlers == nil {
		srv.channelHandlers = map[string]channelHandler{
			"session":      sessionHandler,
			"direct-tcpip": directTcpipHandler,
		}
	}
	var tempDelay time.Duration

	srv.trackListener(l, true)
	defer srv.trackListener(l, false)
	for {
		conn, e := l.Accept()
		if e != nil {
			select {
			case <-srv.getDoneChan():
				return ErrServerClosed
			default:
			}
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

func (srv *Server) handleConn(newConn net.Conn) {
	if srv.ConnCallback != nil {
		cbConn := srv.ConnCallback(newConn)
		if cbConn == nil {
			newConn.Close()
			return
		}
		newConn = cbConn
	}
	ctx, cancel := newContext(srv)
	conn := &serverConn{
		Conn:          newConn,
		idleTimeout:   srv.IdleTimeout,
		closeCanceler: cancel,
	}
	if srv.MaxTimeout > 0 {
		conn.maxDeadline = time.Now().Add(srv.MaxTimeout)
	}
	defer conn.Close()
	sshConn, chans, reqs, err := gossh.NewServerConn(conn, srv.config(ctx))
	if err != nil {
		// TODO: trigger event callback
		return
	}

	srv.trackConn(sshConn, true)
	defer srv.trackConn(sshConn, false)

	ctx.SetValue(ContextKeyConn, sshConn)
	applyConnMetadata(ctx, sshConn)
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

func (srv *Server) getDoneChan() <-chan struct{} {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	return srv.getDoneChanLocked()
}

func (srv *Server) getDoneChanLocked() chan struct{} {
	if srv.doneChan == nil {
		srv.doneChan = make(chan struct{})
	}
	return srv.doneChan
}

func (srv *Server) closeDoneChanLocked() {
	ch := srv.getDoneChanLocked()
	select {
	case <-ch:
		// Already closed. Don't close again.
	default:
		// Safe to close here. We're the only closer, guarded
		// by srv.mu.
		close(ch)
	}
}

func (srv *Server) closeListenersLocked() error {
	var err error
	for ln := range srv.listeners {
		if cerr := ln.Close(); cerr != nil && err == nil {
			err = cerr
		}
		delete(srv.listeners, ln)
	}
	return err
}

func (srv *Server) trackListener(ln net.Listener, add bool) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if srv.listeners == nil {
		srv.listeners = make(map[net.Listener]struct{})
	}
	if add {
		// If the *Server is being reused after a previous
		// Close or Shutdown, reset its doneChan:
		if len(srv.listeners) == 0 && len(srv.conns) == 0 {
			srv.doneChan = nil
		}
		srv.listeners[ln] = struct{}{}
	} else {
		delete(srv.listeners, ln)
	}
}

func (srv *Server) trackConn(c *gossh.ServerConn, add bool) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if srv.conns == nil {
		srv.conns = make(map[*gossh.ServerConn]struct{})
	}
	if add {
		srv.conns[c] = struct{}{}
	} else {
		delete(srv.conns, c)
	}
}
