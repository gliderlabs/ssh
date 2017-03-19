package ssh

import (
	"io"
	"io/ioutil"
	"net"
	"path"
	"sync"

	gossh "golang.org/x/crypto/ssh"
)

const (
	agentRequestType = "auth-agent-req@openssh.com"
	agentChannelType = "auth-agent@openssh.com"
)

var contextKeyAgentRequest = &contextKey{"auth-agent-req"}

func setAgentRequested(sess *session) {
	sess.ctx.SetValue(contextKeyAgentRequest, true)
}

func AgentRequested(sess Session) bool {
	return sess.Context().Value(contextKeyAgentRequest) == true
}

func NewAgentListener() (net.Listener, error) {
	dir, err := ioutil.TempDir("", "auth-agent")
	if err != nil {
		return nil, err
	}
	l, err := net.Listen("unix", path.Join(dir, "listener.sock"))
	if err != nil {
		return nil, err
	}
	return l, nil
}

func ForwardAgentConnections(l net.Listener, s Session) {
	sshConn := s.Context().Value(ContextKeyConn).(gossh.Conn)
	for {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		go func(conn net.Conn) {
			defer conn.Close()
			channel, reqs, err := sshConn.OpenChannel(agentChannelType, nil)
			if err != nil {
				return
			}
			defer channel.Close()
			go gossh.DiscardRequests(reqs)
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				io.Copy(conn, channel)
				conn.(*net.UnixConn).CloseWrite()
				wg.Done()
			}()
			go func() {
				io.Copy(channel, conn)
				channel.CloseWrite()
				wg.Done()
			}()
			wg.Wait()
		}(conn)
	}
}
