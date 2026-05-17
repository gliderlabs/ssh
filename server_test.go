package ssh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pires/go-proxyproto"
	gossh "golang.org/x/crypto/ssh"
)

func TestAddHostKey(t *testing.T) {
	s := Server{}
	signer, err := generateSigner()
	if err != nil {
		t.Fatal(err)
	}
	s.AddHostKey(signer)
	if len(s.HostSigners) != 1 {
		t.Fatal("Key was not properly added")
	}
	signer, err = generateSigner()
	if err != nil {
		t.Fatal(err)
	}
	s.AddHostKey(signer)
	if len(s.HostSigners) != 1 {
		t.Fatal("Key was not properly replaced")
	}
}

func TestServerShutdown(t *testing.T) {
	l := newLocalListener()
	testBytes := []byte("Hello world\n")
	s := &Server{
		Handler: func(s Session) {
			s.Write(testBytes)
			time.Sleep(50 * time.Millisecond)
		},
	}
	go func() {
		err := s.Serve(l)
		if err != nil && err != ErrServerClosed {
			t.Fatal(err)
		}
	}()
	sessDone := make(chan struct{})
	sess, _, cleanup := newClientSession(t, l.Addr().String(), nil)
	go func() {
		defer cleanup()
		defer close(sessDone)
		var stdout bytes.Buffer
		sess.Stdout = &stdout
		if err := sess.Run(""); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(stdout.Bytes(), testBytes) {
			t.Fatalf("expected = %s; got %s", testBytes, stdout.Bytes())
		}
	}()

	srvDone := make(chan struct{})
	go func() {
		defer close(srvDone)
		err := s.Shutdown(context.Background())
		if err != nil {
			t.Fatal(err)
		}
	}()

	timeout := time.After(2 * time.Second)
	select {
	case <-timeout:
		t.Fatal("timeout")
		return
	case <-srvDone:
		// TODO: add timeout for sessDone
		<-sessDone
		return
	}
}

func TestServerClose(t *testing.T) {
	l := newLocalListener()
	s := &Server{
		Handler: func(s Session) {
			time.Sleep(5 * time.Second)
		},
	}
	go func() {
		err := s.Serve(l)
		if err != nil && err != ErrServerClosed {
			t.Fatal(err)
		}
	}()

	clientDoneChan := make(chan struct{})
	closeDoneChan := make(chan struct{})

	sess, _, cleanup := newClientSession(t, l.Addr().String(), nil)
	go func() {
		defer cleanup()
		defer close(clientDoneChan)
		<-closeDoneChan
		if err := sess.Run(""); err != nil && err != io.EOF {
			t.Fatal(err)
		}
	}()

	go func() {
		err := s.Close()
		if err != nil {
			t.Fatal(err)
		}
		close(closeDoneChan)
	}()

	timeout := time.After(100 * time.Millisecond)
	select {
	case <-timeout:
		t.Error("timeout")
		return
	case <-s.getDoneChan():
		<-clientDoneChan
		return
	}
}

func TestServerHandshakeTimeout(t *testing.T) {
	l := newLocalListener()

	s := &Server{
		HandshakeTimeout: time.Millisecond,
	}
	go func() {
		if err := s.Serve(l); err != nil {
			t.Error(err)
		}
	}()

	conn, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	ch := make(chan struct{})
	go func() {
		defer close(ch)
		io.Copy(io.Discard, conn)
	}()

	select {
	case <-ch:
		return
	case <-time.After(time.Second):
		t.Fatal("client connection was not force-closed")
		return
	}
}

func TestProxyProtocol(t *testing.T) {
	const (
		CORRECT_IP   = "1.1.1.1"
		CORRECT_PORT = 55555
	)
	handlerDone := make(chan struct{})
	var testResult error

	handler := func(sess Session) {
		defer close(handlerDone)
		sourceAddress := sess.RemoteAddr()

		index := strings.Index(sourceAddress.String(), ":")
		ip := sourceAddress.String()[:index]
		portStr := sourceAddress.String()[index+1:]

		if ip != CORRECT_IP {
			errorMsg := fmt.Sprintf("Expected source address '%s' but got '%s'", CORRECT_IP, ip)
			testResult = errors.Join(testResult, fmt.Errorf("%s", errorMsg))
		}
		port, err := strconv.Atoi(portStr)
		if err != nil {
			testResult = errors.Join(testResult, fmt.Errorf("%s", err))
		} else if port != CORRECT_PORT {
			errorMsg := fmt.Sprintf("Expected source port '%d' but got '%d'", CORRECT_PORT, port)
			testResult = errors.Join(testResult, fmt.Errorf("%s", errorMsg))
		}
	}

	// Bind the port before starting the goroutine so net.Dial never races
	// with the server not yet listening.
	l := newLocalListener()
	srv := &Server{Handler: handler}
	srv.SetOption(EnableProxyProtocol())

	serverDone := make(chan error, 1)

	go func() {
		serverDone <- srv.Serve(l)
	}()

	defer func() {
		srv.Close()
		if err := <-serverDone; err != nil && err != ErrServerClosed {
			t.Error(err)
		}
	}()

	serverIP, serverPortStr, _ := net.SplitHostPort(l.Addr().String())
	serverPort, _ := strconv.Atoi(serverPortStr)
	conn, err := net.Dial("tcp", l.Addr().String())

	if err != nil {
		t.Fatal(err)
	}

	header := &proxyproto.Header{
		Version:           1,
		Command:           proxyproto.PROXY,
		TransportProtocol: proxyproto.TCPv4,
		SourceAddr: &net.TCPAddr{
			IP:   net.ParseIP(CORRECT_IP),
			Port: CORRECT_PORT,
		},
		DestinationAddr: &net.TCPAddr{
			IP:   net.ParseIP(serverIP),
			Port: serverPort,
		},
	}

	// Writes the PROXY header to the TCP stream before SSH begins
	_, err = header.WriteTo(conn)
	if err != nil {
		t.Fatal(err)
	}

	// Hand the same conn to the SSH stack — handshake starts from here.
	clientConn, chans, reqs, err := gossh.NewClientConn(conn, l.Addr().String(), &gossh.ClientConfig{
		User:            "testuser",
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
	})
	if err != nil {
		t.Fatal(err)
	}
	client := gossh.NewClient(clientConn, chans, reqs)
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	session.Run("") // triggers the handler; ignore exec error

	<-handlerDone

	if testResult != nil {
		t.Fatal(testResult)
	}
}
