package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/charmbracelet/ssh"
)

func main() {
	ssh.Handle(func(s ssh.Session) {
		log.Printf("connected %s %s %q", s.User(), s.RemoteAddr(), s.RawCommand())
		defer log.Printf("disconnected %s %s", s.User(), s.RemoteAddr())

		pty, _, ok := s.Pty()
		if !ok {
			io.WriteString(s, "No PTY requested.\n")
			s.Exit(1)
			return
		}

		name := "bash"
		if runtime.GOOS == "windows" {
			name = "powershell.exe"
		}
		cmd := exec.Command(name)
		cmd.Env = append(os.Environ(), "SSH_TTY="+pty.Name(), fmt.Sprintf("TERM=%s", pty.Term))
		if err := pty.Start(cmd); err != nil {
			fmt.Fprintln(s, err.Error())
			s.Exit(1)
			return
		}

		if runtime.GOOS == "windows" {
			// ProcessState gets populated by pty.Start waiting on the process
			// to exit.
			for cmd.ProcessState == nil {
				time.Sleep(100 * time.Millisecond)
			}

			s.Exit(cmd.ProcessState.ExitCode())
		} else {
			if err := cmd.Wait(); err != nil {
				fmt.Fprintln(s, err)
				s.Exit(cmd.ProcessState.ExitCode())
			}
		}
	})

	log.Println("starting ssh server on port 2222...")
	if err := ssh.ListenAndServe("localhost:2222", nil, ssh.AllocatePty()); err != nil && err != ssh.ErrServerClosed {
		log.Fatal(err)
	}
}
