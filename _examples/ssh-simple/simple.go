package main

import (
	"fmt"
	"io"
	"log"

	"github.com/charmbracelet/ssh"
)

func main() {
	ssh.Handle(func(s ssh.Session) {
		io.WriteString(s, fmt.Sprintf("Hello %s\n", s.User()))
	})

	log.Println("starting ssh server on port 2222...")
	log.Fatal(ssh.ListenAndServe("localhost:2222", nil))
}
