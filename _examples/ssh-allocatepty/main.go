package main

import (
	"fmt"
	"log"

	"github.com/charmbracelet/ssh"
)

func main() {
	ssh.Handle(func(s ssh.Session) {
		log.Printf("connected %s %s %q", s.User(), s.RemoteAddr(), s.RawCommand())
		defer log.Printf("disconnected %s %s", s.User(), s.RemoteAddr())

		pty, _, ok := s.Pty()
		if !ok {
			_, _ = fmt.Fprintln(s, "No PTY requested.")
			_ = s.Exit(1)
			return
		}

		_, _ = fmt.Fprintln(s, "Got a PTY:", pty.Term)
	})

	log.Println("starting ssh server on port 2222...")
	if err := ssh.ListenAndServe("localhost:2222", nil, ssh.AllocatePty()); err != nil && err != ssh.ErrServerClosed {
		log.Fatal(err)
	}
}
