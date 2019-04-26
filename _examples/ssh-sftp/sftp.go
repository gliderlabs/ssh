package main

import (
	"fmt"
	"github.com/gliderlabs/ssh"
	"github.com/pkg/sftp"
	"io"
	"io/ioutil"
	"log"
)

func SftpHandler(sess ssh.Session) {
	debugStream := ioutil.Discard
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
	if err := server.Serve(); err == io.EOF {
		server.Close()
		fmt.Println("sftp client exited session.")
	} else if err != nil {
		fmt.Println("sftp server completed with error:", err)
	}

}

func main() {
	srv := ssh.Server{
		Addr:              ":2223",
		SubsystemHandlers: map[string]ssh.SubsystemHandler{},
	}
	srv.SetSubsystemHandler("sftp", SftpHandler)
	log.Fatal(srv.ListenAndServe())
}
