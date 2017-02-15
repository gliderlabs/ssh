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

var (
	// ErrInvalidState will be returned from some functions when asked to do
	// something but the server is either already running and shouldn't be or
	// vice versa.
	ErrInvalidState = errors.New("Invalid server state")

	// ErrServerClosed will be returned from Serve if Close or Shutdown were
	// called.
	ErrServerClosed = errors.New("Server has been closed")
)

type serverState int

const (
	stateStopped serverState = iota
	stateStarted
	stateDraining
)

// Server defines parameters for running an SSH server. The zero value for
// Server is a valid configuration. When both PasswordHandler and
// PublicKeyHandler are nil, no client authentication is performed.
type Server struct {
	Addr        string   // TCP address to listen on, ":22" if empty
	Handler     Handler  // handler to invoke, ssh.DefaultHandler if nil
	HostSigners []Signer // private keys for the host key, must have at least one
	Version     string   // server version to be sent before the initial handshake

	PasswordHandler     PasswordHandler     // password authentication handler
	PublicKeyHandler    PublicKeyHandler    // public key authentication handler
	PtyCallback         PtyCallback         // callback for allowing PTY sessions, allows all if nil
	PermissionsCallback PermissionsCallback // optional callback for setting up permissions

	// Internal fields. Note that the zero value for these should be a state we
	// can detect so the Server can still be instantiated using &Server{}.

	listener net.Listener    // listener currently being used by Serve, nil otherwise
	mu       sync.Mutex      // general lock around the Server state
	wg       *sync.WaitGroup // WaitGroup to track the number of connections still running
	state    serverState     // tracks if the server is running or not
	doneChan chan struct{}   // tracks if we want to exit
	killChan chan struct{}   // tracks if we're blindly killing connections
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
	if srv.Version != "" {
		config.ServerVersion = "SSH-2.0-" + srv.Version
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

// Handle sets the Handler for the server.
func (srv *Server) Handle(fn Handler) error {
	srv.mu.Lock()
	defer srv.mu.Unlock()

	if srv.state != stateStopped {
		return ErrInvalidState
	}

	srv.Handler = fn

	return nil
}

// Serve accepts incoming connections on the Listener l, creating a new
// connection goroutine for each. The connection goroutines read requests and then
// calls srv.Handler to handle sessions. Note that this method will wait for all
// the connections to exit before returning.
//
// Serve always returns a non-nil error.
func (srv *Server) Serve(l net.Listener) error {
	// Ensure we're just starting the server and set up any values which need to
	// be set up.
	srv.mu.Lock()
	if srv.state != stateStopped {
		l.Close()
		srv.mu.Unlock()
		return ErrInvalidState
	}

	// Server struct initialization can go here. We know we're coming from the
	// stopped state, so this is safe to do.
	srv.state = stateStarted
	srv.listener = l
	srv.doneChan = make(chan struct{})
	srv.killChan = make(chan struct{})
	srv.wg = &sync.WaitGroup{}

	// Store values for the done channel, kill channels and wait group so we
	// can still access the values even after they've been closed and set to
	// nil.
	var (
		srvDoneChan = srv.doneChan
		srvKillChan = srv.killChan
		wg          = srv.wg
	)

	srv.mu.Unlock()

	// We want to ensure this exits before srv.Close/Shutdown
	wg.Add(1)
	defer wg.Done()

	// This nasty piece of work cleans up everything we can before this function
	// exits
	defer func() {
		srv.mu.Lock()
		defer srv.mu.Unlock()

		srv.state = stateStopped

		// If there's still a listener around, we need to close it
		if srv.listener != nil {
			srv.listener.Close()
			srv.listener = nil
		}

		// Clean up any leftover variables
		srv.wg = nil
		if srv.doneChan != nil {
			close(srv.doneChan)
			srv.doneChan = nil
		}
		if srv.killChan != nil {
			close(srv.killChan)
			srv.killChan = nil
		}
	}()

	config, err := srv.makeConfig()
	if err != nil {
		return err
	}
	if srv.Handler == nil {
		srv.Handler = DefaultHandler
	}

	var tempDelay time.Duration
	for {
		conn, e := l.Accept()
		if e != nil {
			// If we got an error while accepting (meaning the listener was
			// closed) but we're already listed as trying to close the server,
			// return ErrServerClosed rather than whatever error was returned
			// from Accept
			select {
			case <-srvDoneChan:
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

		// Add one to the wg and start up the connection
		wg.Add(1)
		go func() {
			defer wg.Done()

			doneChan := make(chan struct{}, 1)
			go func() {
				srv.handleConn(conn, config)
				doneChan <- struct{}{}
			}()

			// If doneChan gets a value first, we can just be on our merry way,
			// but if we get notified from killChan, we need to nuke the
			// connection.
			select {
			case <-doneChan:
			case <-srvKillChan:
				conn.Close()
			}
		}()
	}
}

func (srv *Server) Close() error {
	srv.mu.Lock()

	// We only want to be able to do this if the server is running and not
	// draining. Waiting on the wg at the end will ensure that when this
	// function returns, the state will be stateStopped.
	if srv.state != stateStarted {
		srv.mu.Unlock()
		return ErrInvalidState
	}

	// Close the doneChan, killChan and listeners so we stop accepting new
	// connections and ensure existing connections start getting shut down.
	close(srv.doneChan)
	close(srv.killChan)
	lerr := srv.listener.Close()
	srv.listener = nil

	// Grab the waitgroup then unlock
	wg := srv.wg
	srv.mu.Unlock()

	// Now that we've asked everything to close, we wait for Serve to exit
	wg.Wait()

	return lerr
}

func (srv *Server) Shutdown(ctx context.Context) error {
	srv.mu.Lock()

	// Similar to Close, we only want to do this if the server is running and
	// not already draining.
	if srv.state != stateStarted {
		srv.mu.Unlock()
		return ErrInvalidState
	}

	srv.state = stateDraining

	// Close the listeners so everyone knows we're done.
	close(srv.doneChan)
	lerr := srv.listener.Close()
	srv.listener = nil

	// Grab the waitgroup then unlock
	wg := srv.wg
	srv.mu.Unlock()

	// We need a chan from this waitgroup because we need to select on this and
	// the ctx.Done() channel in case they asked for a timeout.
	wgDoneChan := make(chan struct{}, 1)
	go func() {
		wg.Wait()
		wgDoneChan <- struct{}{}
	}()

	select {
	case <-ctx.Done():
	case <-wgDoneChan:
	}

	return lerr
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
	srv.mu.Lock()
	defer srv.mu.Unlock()

	if srv.state != stateStopped {
		return ErrInvalidState
	}

	return option(srv)
}
