package main

import (
	"fmt"
	"io"
	"log"

	"github.com/gliderlabs/ssh"
)

const (
	ADDR = "0.0.0.0:4444"
)

func main() {
	ssh.Handle(func(s ssh.Session) {
		io.WriteString(s, fmt.Sprintf("Your address is %s\n", s.RemoteAddr()))
	})

	log.Println("starting ssh server on " + ADDR)
	log.Fatal(ssh.ListenAndServe(ADDR, nil, ssh.EnableProxyProtocol()))
}
