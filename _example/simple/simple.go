package main

import (
	"io"
	"log"

	"github.com/hloeffler/ssh"
)

func main() {

	ssh.Handle(func(s ssh.Session) {
		user := s.User()
		io.WriteString(s, "Hello "+user+"\n")
	})

	log.Fatal(ssh.ListenAndServe(":2222", nil))

}
