package ssh

import (
	"context"
	"io"
	"testing"
	"time"
)

func TestServerShutdown(t *testing.T) {
	// We want to ensure that we wait for the connection to close before
	// Shutdown returns.
	sessionEnd := make(chan struct{}, 1)
	srvExited := make(chan error, 1)
	clientExited := make(chan error, 1)
	shutdownExited := make(chan error, 1)

	l := newLocalListener()
	srv := &Server{
		Handler: func(s Session) {
			<-sessionEnd
		},
	}

	// Start the server and the session
	go func() {
		srvExited <- srv.Serve(l)
	}()
	session, cleanup := newClientSession(t, l.Addr().String(), nil)
	go func() {
		err := session.Run("")
		cleanup()
		clientExited <- err
	}()

	// Start the Shutdown. This should make the server exit, but nothing else.
	go func() {
		shutdownExited <- srv.Shutdown(context.TODO())
	}()

	select {
	case <-time.After(10 * time.Millisecond):
		t.Fatal("Server took too long to exit")
	case err := <-clientExited:
		if err != nil {
			t.Fatal(err)
		}
		t.Fatal("Client exited early")
	case err := <-shutdownExited:
		if err != nil {
			t.Fatal(err)
		}
		t.Fatal("Shutdown exited early")
	case err := <-srvExited:
		if err != ErrServerClosed {
			t.Fatal(err)
		}
		// This is the expected case so we only fail if there was an error here.
	}

	// Tell the session it can return
	sessionEnd <- struct{}{}

	select {
	case <-time.After(10 * time.Millisecond):
		t.Fatal("Client took too long to exit")
	case err := <-clientExited:
		if err != nil && err != io.EOF {
			t.Fatal(err)
		}
		// This is the expected case so we only fail if there was an error here.
	}

	err := <-shutdownExited
	if err != nil {
		t.Fatal(err)
	}
}
