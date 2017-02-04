package main

import (
	"io"
	"log"

	b64 "encoding/base64"

	"github.com/gliderlabs/ssh"
)

func main() {

	ssh.Handle(func(s ssh.Session) {
		user := s.User()
		keyType := s.PublicKey().Type()

		publicKeyString := keyType + " " + b64.StdEncoding.EncodeToString(s.PublicKey().Marshal())

		io.WriteString(s, "Hello "+user+"\n\n")
		io.WriteString(s, "your public key:\n")
		io.WriteString(s, publicKeyString+"\n\n")
	})

	publicKeyHandler := ssh.PublicKeyAuth(func(user string, key ssh.PublicKey) bool {
		// allow all keys
		// use ssh.KeysEqual() to compare agains know keys
		return true
	})

	log.Println("starting ssh server on port: 2222")
	log.Fatal(ssh.ListenAndServe(":2222", nil, publicKeyHandler))
}
