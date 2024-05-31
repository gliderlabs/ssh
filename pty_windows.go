//go:build windows
// +build windows

package ssh

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/charmbracelet/x/conpty"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sys/windows"
)

type impl struct {
	Context
	*conpty.ConPty
}

func (i *impl) IsZero() bool {
	return i.ConPty == nil
}

func (i *impl) Name() string {
	return "windows-pty"
}

func (i *impl) Read(p []byte) (n int, err error) {
	return i.ConPty.Read(p)
}

func (i *impl) Write(p []byte) (n int, err error) {
	return i.ConPty.Write(p)
}

func (i *impl) Resize(w int, h int) error {
	return i.ConPty.Resize(w, h)
}

func (i *impl) Close() error {
	return i.ConPty.Close()
}

func (i *impl) start(c *exec.Cmd) error {
	pid, process, err := i.Spawn(c.Path, c.Args, &syscall.ProcAttr{
		Dir: c.Dir,
		Env: c.Env,
		Sys: c.SysProcAttr,
	})
	if err != nil {
		return err
	}

	c.Process, err = os.FindProcess(pid)
	if err != nil {
		// If we can't find the process via os.FindProcess, terminate the
		// process as that's what we rely on for all further operations on the
		// object.
		if tErr := windows.TerminateProcess(windows.Handle(process), 1); tErr != nil {
			return fmt.Errorf("failed to terminate process after process not found: %w", tErr)
		}
		return fmt.Errorf("failed to find process after starting: %w", err)
	}

	type result struct {
		*os.ProcessState
		error
	}
	donec := make(chan result, 1)
	go func() {
		state, err := c.Process.Wait()
		donec <- result{state, err}
	}()
	go func() {
		select {
		case <-i.Context.Done():
			c.Err = windows.TerminateProcess(windows.Handle(process), 1)
		case r := <-donec:
			c.ProcessState = r.ProcessState
			c.Err = r.error
		}
	}()

	return nil
}

func newPty(ctx Context, _ string, win Window, _ ssh.TerminalModes) (impl, error) {
	c, err := conpty.New(win.Width, win.Height, 0)
	if err != nil {
		return impl{}, err
	}

	return impl{ctx, c}, nil
}
