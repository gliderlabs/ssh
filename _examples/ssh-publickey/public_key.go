package main

import (
	"fmt"
	"io"
	"log"

	"github.com/charmbracelet/ssh"
	gossh "golang.org/x/crypto/ssh"
)

func main() {
	ssh.Handle(func(s ssh.Session) {
		authorizedKey := gossh.MarshalAuthorizedKey(s.PublicKey())
		io.WriteString(s, fmt.Sprintf("public key used by %s:\n", s.User()))
		s.Write(authorizedKey)
	})

	publicKeyOption := ssh.PublicKeyAuth(func(ctx ssh.Context, key ssh.PublicKey) bool {
		return true // allow all keys, or use ssh.KeysEqual() to compare against known keys
	})

	log.Println("starting ssh server on port 2222...")
	log.Fatal(ssh.ListenAndServe("localhost:2222", nil, publicKeyOption))
}
