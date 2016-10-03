package ssh

import (
	"crypto/subtle"
	"net"

	gossh "golang.org/x/crypto/ssh"
)

type Signal string

// POSIX signals as listed in RFC 4254 Section 6.10.
const (
	SIGABRT Signal = "ABRT"
	SIGALRM Signal = "ALRM"
	SIGFPE  Signal = "FPE"
	SIGHUP  Signal = "HUP"
	SIGILL  Signal = "ILL"
	SIGINT  Signal = "INT"
	SIGKILL Signal = "KILL"
	SIGPIPE Signal = "PIPE"
	SIGQUIT Signal = "QUIT"
	SIGSEGV Signal = "SEGV"
	SIGTERM Signal = "TERM"
	SIGUSR1 Signal = "USR1"
	SIGUSR2 Signal = "USR2"
)

var defaultHandler Handler

type Option func(*Server) error
type Handler func(Session)

type PublicKeyHandler func(user string, key PublicKey) bool
type PasswordHandler func(user, password string) bool

type PermissionsCallback func(user string, permissions *Permissions) error
type PtyCallback func(user string, permissions *Permissions) bool

type Window struct {
	Width  int
	Height int
}

type Pty struct {
	Window Window
}

func Serve(l net.Listener, handler Handler, options ...Option) error {
	srv := &Server{Handler: handler}
	for _, option := range options {
		if err := srv.SetOption(option); err != nil {
			return err
		}
	}
	return srv.Serve(l)
}

func ListenAndServe(addr string, handler Handler, options ...Option) error {
	srv := &Server{Addr: addr, Handler: handler}
	for _, option := range options {
		if err := srv.SetOption(option); err != nil {
			return err
		}
	}
	return srv.ListenAndServe()
}

func Handle(handler Handler) {
	defaultHandler = handler
}

// KeysEqual is constant time compare of the keys to avoid timing attacks
func KeysEqual(ak, bk PublicKey) bool {
	a := gossh.Marshal(ak)
	b := gossh.Marshal(bk)
	return (len(a) == len(b) && subtle.ConstantTimeCompare(a, b) == 1)
}
