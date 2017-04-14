package main

import (
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
		cmd.Stdout = s
		cmd.Stderr = s.Stderr()
		if err := cmd.Run(); err != nil {
			log.Println(err)
			return
		}
	})

	log.Println("starting ssh server on port 2222...")
	log.Fatal(ssh.ListenAndServe(":2222", nil))
}
