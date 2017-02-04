package ssh

import (
	"bytes"
	"errors"
	"fmt"
	"net"

	"github.com/anmitsu/go-shlex"
	gossh "golang.org/x/crypto/ssh"
)

// Session provides access to information about an SSH session and methods
// to read and write to the SSH channel with an embedded Channel interface from
// cypto/ssh.
//
// When Command() returns an empty slice, the user requested a shell. Otherwise
// the user is performing an exec with those command arguments.
//
// TODO: Signals
type Session interface {
	gossh.Channel

	// User returns the username used when establishing the SSH connection.
	User() string

	// RemoteAddr returns the net.Addr of the client side of the connection.
	RemoteAddr() net.Addr

	// Environ returns a copy of strings representing the environment set by the
	// user for this session, in the form "key=value".
	Environ() []string

	// Exit sends an exit status and then closes the session.
	Exit(code int) error

	// Command returns a shell parsed slice of arguments that were provided by the
	// user. Shell parsing splits the command string according to POSIX shell rules,
	// which considers quoting not just whitespace.
	Command() []string

	// PublicKey returns the PublicKey used to authenticate. If a public key was not
	// used it will return nil.
	PublicKey() PublicKey

	// Pty returns PTY information, a channel of window size changes, and a boolean
	// of whether or not a PTY was accepted for this session.
	Pty() (Pty, <-chan Window, bool)

	// TODO: Signals(c chan<- Signal)
}

type session struct {
	gossh.Channel
	conn    *gossh.ServerConn
	handler Handler
	handled bool
	exited  bool
	pty     *Pty
	winch   chan Window
	env     []string
	ptyCb   PtyCallback
	cmd     []string
}

func (sess *session) Write(p []byte) (n int, err error) {
	if sess.pty != nil {
		// normalize \n to \r\n when pty is accepted
		p = bytes.Replace(p, []byte{'\n'}, []byte{'\r', '\n'}, -1)
		p = bytes.Replace(p, []byte{'\r', '\r', '\n'}, []byte{'\r', '\n'}, -1)
	}
	return sess.Channel.Write(p)
}

func (sess *session) PublicKey() PublicKey {
	if sess.conn.Permissions == nil {
		return nil
	}
	s, ok := sess.conn.Permissions.Extensions["_publickey"]
	if !ok {
		return nil
	}
	key, err := ParsePublicKey([]byte(s))
	if err != nil {
		return nil
	}
	return key
}

func (sess *session) Exit(code int) error {
	if sess.exited {
		return errors.New("Session.Exit called multiple times")
	}
	sess.exited = true

	status := struct{ Status uint32 }{uint32(code)}
	_, err := sess.SendRequest("exit-status", false, gossh.Marshal(&status))
	if err != nil {
		return err
	}
	return sess.Close()
}

func (sess *session) User() string {
	return sess.conn.User()
}

func (sess *session) RemoteAddr() net.Addr {
	return sess.conn.RemoteAddr()
}

func (sess *session) Environ() []string {
	return append([]string(nil), sess.env...)
}

func (sess *session) Command() []string {
	return append([]string(nil), sess.cmd...)
}

func (sess *session) Pty() (Pty, <-chan Window, bool) {
	if sess.pty != nil {
		return *sess.pty, sess.winch, true
	}
	return Pty{}, sess.winch, false
}

func (sess *session) handleRequests(reqs <-chan *gossh.Request) {
	for req := range reqs {
		var width, height int
		var ok bool
		switch req.Type {
		case "shell", "exec":
			if sess.handled {
				req.Reply(false, nil)
				continue
			}
			sess.handled = true
			req.Reply(true, nil)

			var payload = struct{ Value string }{}
			gossh.Unmarshal(req.Payload, &payload)
			sess.cmd, _ = shlex.Split(payload.Value, true)
			go func() {
				sess.handler(sess)
				sess.Exit(0)
			}()
		case "env":
			if sess.handled {
				req.Reply(false, nil)
				continue
			}
			var kv = struct{ Key, Value string }{}
			gossh.Unmarshal(req.Payload, &kv)
			sess.env = append(sess.env, fmt.Sprintf("%s=%s", kv.Key, kv.Value))
		case "pty-req":
			if sess.handled {
				req.Reply(false, nil)
				continue
			}
			if sess.ptyCb != nil {
				ok := sess.ptyCb(sess.conn.User(), &Permissions{sess.conn.Permissions})
				if !ok {
					req.Reply(false, nil)
					continue
				}
			}
			width, height, ok = parsePtyRequest(req.Payload)
			if ok {
				sess.pty = &Pty{Window{width, height}}
				sess.winch = make(chan Window)
			}

			req.Reply(ok, nil)
		case "window-change":
			if sess.pty == nil {
				req.Reply(false, nil)
				continue
			}
			width, height, ok = parseWinchRequest(req.Payload)
			if ok {
				sess.pty.Window = Window{width, height}
				sess.winch <- sess.pty.Window
			}
		}
	}
}
