package main

import (
	"fmt"
	"log"
	"net"
	"time"

	"github.com/gliderlabs/ssh"
)

var (
	MaxLifeTimeout = 30 * time.Second
	IdleTimeout    = 5 * time.Second
)

type timeoutConn struct {
	net.Conn
	maxlife time.Time
	idle    time.Time
}

func (c *timeoutConn) Write(p []byte) (n int, err error) {
	c.updateDeadline()
	return c.Conn.Write(p)
}

func (c *timeoutConn) Read(b []byte) (n int, err error) {
	c.idle = time.Now().Add(IdleTimeout)
	c.updateDeadline()
	return c.Conn.Read(b)
}

func (c *timeoutConn) updateDeadline() {
	if c.idle.Unix() < c.maxlife.Unix() {
		c.Conn.SetDeadline(c.idle)
	} else {
		c.Conn.SetDeadline(c.maxlife)
	}
}

func main() {
	ssh.Handle(func(s ssh.Session) {
		i := 0
		for {
			i += 1
			fmt.Fprintln(s, i)
			time.Sleep(time.Second)
		}
	})

	log.Println("starting ssh server on port 2222...")
	log.Printf("connections will only last %s\n", MaxLifeTimeout)
	log.Printf("and timeout after %s of no client activity\n", IdleTimeout)
	log.Fatal(ssh.ListenAndServe(":2222", nil, ssh.WrapConn(func(conn net.Conn) net.Conn {
		return &timeoutConn{conn, time.Now().Add(MaxLifeTimeout), time.Now().Add(IdleTimeout)}
	})))
}
