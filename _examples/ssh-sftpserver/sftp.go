package main

import (
	"fmt"
	"io"
	"log"

	"github.com/gliderlabs/ssh"
	"github.com/pkg/sftp"
)

// SftpHandler handler for SFTP subsystem
func SftpHandler(sess ssh.Session) {
	debugStream := io.Discard
	serverOptions := []sftp.ServerOption{
		sftp.WithDebug(debugStream),
	}
	server, err := sftp.NewServer(
		sess,
		serverOptions...,
	)
	if err != nil {
		log.Printf("sftp server init error: %s\n", err)
		return
	}
	if err := server.Serve(); err == io.EOF || err == nil {
		server.Close()
		fmt.Println("sftp client exited session.")
	} else {
		fmt.Println("sftp server completed with error:", err)
	}
}

func main() {
	ssh_server := ssh.Server{
		Addr: "127.0.0.1:2222",
		SubsystemHandlers: map[string]ssh.SubsystemHandler{
			"sftp": SftpHandler,
		},
	}
	log.Fatal(ssh_server.ListenAndServe())
}
