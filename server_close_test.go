package ssh

import (
	"bufio"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

// A value implementing net.Conn need not be comparable.
type closeTestWrappedConn struct {
	net.Conn
	padding []byte
	closed  chan struct{}
	once    *sync.Once
}

func (c closeTestWrappedConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return c.Conn.Close()
}

type closeTestReplayConn struct {
	net.Conn
	reader io.Reader
}

func (c closeTestReplayConn) Read(p []byte) (int, error) { return c.reader.Read(p) }

type closeTestCountedConn struct {
	net.Conn
	closes *atomic.Int32
}

func (c closeTestCountedConn) Close() error {
	c.closes.Add(1)
	return c.Conn.Close()
}

func closeTestServer(t *testing.T, srv *Server, listener net.Listener) (*gossh.ClientConfig, net.Listener, <-chan struct{}) {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := gossh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	srv.AddHostKey(signer)
	if listener == nil {
		listener, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
	}
	served := make(chan struct{})
	var serveErr error
	go func() {
		serveErr = srv.Serve(listener)
		close(served)
	}()
	t.Cleanup(func() {
		srv.Close()
		listener.Close()
		select {
		case <-served:
			if !errors.Is(serveErr, ErrServerClosed) {
				t.Errorf("Serve: %v", serveErr)
			}
		case <-time.After(2 * time.Second):
			t.Error("Serve did not stop")
		}
	})
	return &gossh.ClientConfig{User: "test", HostKeyCallback: gossh.FixedHostKey(signer.PublicKey())}, listener, served
}

func closeTestDial(t *testing.T, listener net.Listener) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("tcp", listener.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	return conn
}

func closeTestWait(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("connection cleanup did not finish")
	}
}

func TestServerClosePendingHandshake(t *testing.T) {
	for _, wrapped := range []bool{false, true} {
		name := "plain"
		if wrapped {
			name = "wrapped"
		}
		t.Run(name, func(t *testing.T) {
			var handled atomic.Bool
			srv := &Server{Handler: func(Session) { handled.Store(true) }}
			closed := make(chan struct{})
			if wrapped {
				srv.ConnCallback = func(_ Context, conn net.Conn) net.Conn {
					return closeTestWrappedConn{Conn: conn, closed: closed, once: new(sync.Once)}
				}
			}
			config, listener, _ := closeTestServer(t, srv, nil)
			conn := closeTestDial(t, listener)
			reader := bufio.NewReader(conn)
			banner, err := reader.ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}
			if err := srv.Close(); err != nil {
				t.Fatal(err)
			}
			if err := conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
				t.Fatal(err)
			}
			if _, err := reader.ReadByte(); !errors.Is(err, io.EOF) {
				t.Errorf("read after Close = %v, want EOF", err)
			}
			if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
				t.Fatal(err)
			}
			// Replay the banner consumed above, then try a real SSH handshake.
			replay := closeTestReplayConn{conn, io.MultiReader(strings.NewReader(banner), reader)}
			cc, chans, reqs, err := gossh.NewClientConn(replay, listener.Addr().String(), config)
			if err == nil {
				client := gossh.NewClient(cc, chans, reqs)
				defer client.Close()
				session, err := client.NewSession()
				if err == nil {
					_ = session.Run("after-close")
					session.Close()
				}
				t.Error("SSH handshake succeeded after Close")
			}
			if handled.Load() {
				t.Error("session handler ran after Close")
			}
			if wrapped {
				closeTestWait(t, closed)
			}
		})
	}
}

func TestServerCloseDuringConnCallback(t *testing.T) {
	entered, release, wrappedClosed := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	var callbackContext Context
	srv := &Server{ConnCallback: func(ctx Context, conn net.Conn) net.Conn {
		callbackContext = ctx
		close(entered)
		<-release
		return closeTestWrappedConn{Conn: conn, closed: wrappedClosed, once: new(sync.Once)}
	}}
	_, listener, _ := closeTestServer(t, srv, nil)
	conn := closeTestDial(t, listener)
	closeTestWait(t, entered)
	if err := srv.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Errorf("read after Close = %v, want EOF", err)
	}
	closeTestWait(t, callbackContext.Done())
	once.Do(func() { close(release) })
	closeTestWait(t, wrappedClosed)
}

type closeTestListener struct {
	net.Listener
	accepted chan struct{}
	release  chan struct{}
}

func (l *closeTestListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err == nil {
		close(l.accepted)
		<-l.release
	}
	return conn, err
}

func TestServerCloseDuringAccept(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	held := &closeTestListener{listener, make(chan struct{}), make(chan struct{})}
	var release sync.Once
	defer release.Do(func() { close(held.release) })
	var callbackCalled atomic.Bool
	srv := &Server{ConnCallback: func(_ Context, conn net.Conn) net.Conn {
		callbackCalled.Store(true)
		return conn
	}}
	_, _, _ = closeTestServer(t, srv, held)
	conn := closeTestDial(t, held)
	closeTestWait(t, held.accepted)
	if err := srv.Close(); err != nil {
		t.Fatal(err)
	}
	release.Do(func() { close(held.release) })
	if _, err := conn.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Errorf("read after Close = %v, want EOF", err)
	}
	if callbackCalled.Load() {
		t.Error("connection callback called after Close")
	}
}

func TestServerHandleConnAfterClose(t *testing.T) {
	var callbackCalled atomic.Bool
	srv := &Server{ConnCallback: func(_ Context, conn net.Conn) net.Conn {
		callbackCalled.Store(true)
		return nil
	}}
	if err := srv.Close(); err != nil {
		t.Fatal(err)
	}
	server, client := net.Pipe()
	defer client.Close()
	srv.HandleConn(server)
	if _, err := client.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Errorf("read after Close = %v, want EOF", err)
	}
	if callbackCalled.Load() {
		t.Error("connection callback called after Close")
	}
}

func TestServerPendingHandshakeCleanup(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		name := "failed-handshake"
		if rejected {
			name = "rejected-callback"
		}
		t.Run(name, func(t *testing.T) {
			srv := &Server{}
			if err := srv.ensureHostSigner(); err != nil {
				t.Fatal(err)
			}
			if rejected {
				srv.ConnCallback = func(Context, net.Conn) net.Conn { return nil }
			}
			server, client := net.Pipe()
			defer client.Close()
			defer server.Close()
			if err := client.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
				t.Fatal(err)
			}
			done := make(chan struct{})
			var closes atomic.Int32
			go func() { srv.HandleConn(closeTestCountedConn{server, &closes}); close(done) }()
			if !rejected {
				if _, err := bufio.NewReader(client).ReadString('\n'); err != nil {
					t.Fatal(err)
				}
				if _, err := io.WriteString(client, "SSH-1.0-invalid\r\n"); err != nil {
					t.Fatal(err)
				}
				client.Close()
			}
			closeTestWait(t, done)
			if len(srv.pendingConns) != 0 {
				t.Error("failed connection remains in pending tracker")
			}
			if closes.Load() == 0 {
				t.Fatal("failed connection was not closed")
			}
			if err := srv.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestServerCloseEstablishedConnection(t *testing.T) {
	started, ended := make(chan struct{}), make(chan struct{})
	srv := &Server{Handler: func(s Session) {
		close(started)
		<-s.Context().Done()
		close(ended)
	}}
	config, listener, _ := closeTestServer(t, srv, nil)
	conn := closeTestDial(t, listener)
	cc, chans, reqs, err := gossh.NewClientConn(conn, listener.Addr().String(), config)
	if err != nil {
		t.Fatal(err)
	}
	client := gossh.NewClient(cc, chans, reqs)
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if err := session.Start("wait-for-close"); err != nil {
		t.Fatal(err)
	}
	closeTestWait(t, started)
	if err := srv.Close(); err != nil {
		t.Fatal(err)
	}
	if err := session.Wait(); err == nil {
		t.Error("session completed successfully after connection was closed")
	}
	closeTestWait(t, ended)
}

func TestServerReuseAfterClose(t *testing.T) {
	srv := &Server{Handler: func(s Session) {
		_, _ = io.WriteString(s, "session accepted\n")
	}}
	for i := 0; i < 2; i++ {
		config, listener, served := closeTestServer(t, srv, nil)
		conn := closeTestDial(t, listener)
		cc, chans, reqs, err := gossh.NewClientConn(conn, listener.Addr().String(), config)
		if err != nil {
			t.Fatal(err)
		}
		client := gossh.NewClient(cc, chans, reqs)
		t.Cleanup(func() { client.Close() })
		session, err := client.NewSession()
		if err != nil {
			t.Fatal(err)
		}
		output, err := session.CombinedOutput("test-reuse")
		session.Close()
		client.Close()
		if err != nil {
			t.Fatal(err)
		}
		if string(output) != "session accepted\n" {
			t.Fatalf("unexpected session output on run %d: %q", i, output)
		}
		if err := srv.Close(); err != nil {
			t.Fatal(err)
		}
		// Wait for Serve's deferred listener cleanup before reusing the server.
		closeTestWait(t, served)
	}
}
