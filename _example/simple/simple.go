package main

import (
	"io"
	"log"

	"github.com/gliderlabs/ssh"
)

func main() {

	ssh.Handle(func(s ssh.Session) {
		user := s.User()
		io.WriteString(s, "Hello "+user+"\n")
	})

	log.Println("starting ssh server on port: 2222")
	log.Fatal(ssh.ListenAndServe(":2222", nil))
}
