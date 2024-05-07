package ssh

import (
	"os"

	gossh "golang.org/x/crypto/ssh"
)

// PasswordAuth returns a functional option that sets PasswordHandler on the server.
func PasswordAuth(fn PasswordHandler) Option {
	return func(srv *Server) error {
		srv.PasswordHandler = fn
		return nil
	}
}

// PublicKeyAuth returns a functional option that sets PublicKeyHandler on the server.
func PublicKeyAuth(fn PublicKeyHandler) Option {
	return func(srv *Server) error {
		srv.PublicKeyHandler = fn
		return nil
	}
}

// HostKeyFile returns a functional option that adds HostSigners to the server
// from a PEM file at filepath.
func HostKeyFile(filepath string) Option {
	return func(srv *Server) error {
		pemBytes, err := os.ReadFile(filepath)
		if err != nil {
			return err
		}

		signer, err := gossh.ParsePrivateKey(pemBytes)
		if err != nil {
			return err
		}

		srv.AddHostKey(signer)

		return nil
	}
}

func KeyboardInteractiveAuth(fn KeyboardInteractiveHandler) Option {
	return func(srv *Server) error {
		srv.KeyboardInteractiveHandler = fn
		return nil
	}
}

// HostKeyPEM returns a functional option that adds HostSigners to the server
// from a PEM file as bytes.
func HostKeyPEM(bytes []byte) Option {
	return func(srv *Server) error {
		signer, err := gossh.ParsePrivateKey(bytes)
		if err != nil {
			return err
		}

		srv.AddHostKey(signer)

		return nil
	}
}

// NoPty returns a functional option that sets PtyCallback to return false,
// denying PTY requests.
func NoPty() Option {
	return func(srv *Server) error {
		srv.PtyCallback = func(Context, Pty) bool {
			return false
		}
		return nil
	}
}

// WrapConn returns a functional option that sets ConnCallback on the server.
func WrapConn(fn ConnCallback) Option {
	return func(srv *Server) error {
		srv.ConnCallback = fn
		return nil
	}
}

var contextKeyEmulatePty = &contextKey{"emulate-pty"}

func emulatePtyHandler(ctx Context, _ Session, _ Pty) (func() error, error) {
	ctx.SetValue(contextKeyEmulatePty, true)
	return func() error { return nil }, nil
}

// EmulatePty returns a functional option that fakes a PTY. It uses PtyWriter
// underneath.
func EmulatePty() Option {
	return func(s *Server) error {
		s.PtyHandler = emulatePtyHandler
		return nil
	}
}

// AllocatePty returns a functional option that allocates a PTY. Implementers
// who wish to use an actual PTY should use this along with the platform
// specific PTY implementation defined in pty_*.go.
func AllocatePty() Option {
	return func(s *Server) error {
		s.PtyHandler = func(_ Context, s Session, pty Pty) (func() error, error) {
			return s.(*session).ptyAllocate(pty.Term, pty.Window, pty.Modes)
		}
		return nil
	}
}
