package ssh

import (
	"strings"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

func newTestSessionWithOptions(t *testing.T, srv *Server, cfg *gossh.ClientConfig, options ...Option) (*gossh.Session, *gossh.Client, func()) {
	for _, option := range options {
		if err := srv.SetOption(option); err != nil {
			t.Fatal(err)
		}
	}
	return newTestSession(t, srv, cfg)
}

func TestPasswordAuth(t *testing.T) {
	t.Parallel()
	testUser := "testuser"
	testPass := "testpass"
	session, _, cleanup := newTestSessionWithOptions(t, &Server{
		Handler: func(s Session) {
			// noop
		},
	}, &gossh.ClientConfig{
		User: testUser,
		Auth: []gossh.AuthMethod{
			gossh.Password(testPass),
		},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
	}, PasswordAuth(func(ctx Context, password string) bool {
		if ctx.User() != testUser {
			t.Fatalf("user = %#v; want %#v", ctx.User(), testUser)
		}
		if password != testPass {
			t.Fatalf("user = %#v; want %#v", password, testPass)
		}
		return true
	}))
	defer cleanup()
	if err := session.Run(""); err != nil {
		t.Fatal(err)
	}
}

func TestPasswordAuthBadPass(t *testing.T) {
	t.Parallel()
	l := newLocalListener()
	srv := &Server{Handler: func(s Session) {}}
	srv.SetOption(PasswordAuth(func(ctx Context, password string) bool {
		return false
	}))
	go srv.serveOnce(l)
	_, err := gossh.Dial("tcp", l.Addr().String(), &gossh.ClientConfig{
		User: "testuser",
		Auth: []gossh.AuthMethod{
			gossh.Password("testpass"),
		},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
	})
	if err != nil {
		if !strings.Contains(err.Error(), "unable to authenticate") {
			t.Fatal(err)
		}
	}
}
