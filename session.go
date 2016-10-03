package ssh

import (
	"bytes"
	"fmt"
	"net"

	"github.com/anmitsu/go-shlex"
	gossh "golang.org/x/crypto/ssh"
)

type Session interface {
	gossh.Channel
	User() string
	RemoteAddr() net.Addr
	Environ() []string
	Exit(code int) error
	Command() []string
	//Signals(c chan<- Signal)
	PublicKey() PublicKey
	Pty() (Pty, <-chan Window, bool)
}

type session struct {
	gossh.Channel
	conn    *gossh.ServerConn
	handler Handler
	handled bool
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
			var payload = struct{ Value string }{}
			gossh.Unmarshal(req.Payload, &payload)
			sess.cmd, _ = shlex.Split(payload.Value, true)
			go func() {
				sess.handler(sess)
				sess.Exit(0)
			}()
			sess.handled = true
			req.Reply(true, nil)
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
				req.Reply(true, nil)
			}
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
