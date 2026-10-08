package main

import (
	"bytes"
	"fmt"
	"log"
	"os/exec"

	"github.com/gliderlabs/ssh"
)

func main() {
	ssh.Handle(func(s ssh.Session) {
		cmd := exec.Command("ssh-add", "-l")
		if ssh.AgentRequested(s) {
			l, err := ssh.NewAgentListener()
			if err != nil {
				log.Fatal(err)
			}
			defer l.Close()
			go ssh.ForwardAgentConnections(l, s)
			cmd.Env = append(s.Environ(), fmt.Sprintf("%s=%s", "SSH_AUTH_SOCK", l.Addr().String()))
		} else {
			cmd.Env = s.Environ()
		}
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		s.Write(bytes.ReplaceAll(out, []byte("\n"), []byte("\r\n")))
		s.Stderr().Write(bytes.ReplaceAll(stderr.Bytes(), []byte("\n"), []byte("\r\n")))
		if err != nil {
			log.Println(err)
			return
		}
	})

	log.Println("starting ssh server on port 2222...")
	log.Fatal(ssh.ListenAndServe(":2222", nil))
}
