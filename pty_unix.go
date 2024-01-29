//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris
// +build darwin dragonfly freebsd linux netbsd openbsd solaris

package ssh

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/creack/pty"
	"github.com/u-root/u-root/pkg/termios"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sys/unix"
)

type impl struct {
	// Master is the master PTY file descriptor.
	Master *os.File

	// Slave is the slave PTY file descriptor.
	Slave *os.File
}

func (i *impl) IsZero() bool {
	return i.Master == nil && i.Slave == nil
}

// Name returns the name of the slave PTY.
func (i *impl) Name() string {
	return i.Slave.Name()
}

// Read implements ptyInterface.
func (i *impl) Read(p []byte) (n int, err error) {
	return i.Master.Read(p)
}

// Write implements ptyInterface.
func (i *impl) Write(p []byte) (n int, err error) {
	return i.Master.Write(p)
}

func (i *impl) Close() error {
	if err := i.Master.Close(); err != nil {
		return err
	}
	return i.Slave.Close()
}

func (i *impl) Resize(w int, h int) (rErr error) {
	conn, err := i.Master.SyscallConn()
	if err != nil {
		return err
	}

	return conn.Control(func(fd uintptr) {
		rErr = termios.SetWinSize(fd, &termios.Winsize{
			Winsize: unix.Winsize{
				Row: uint16(h),
				Col: uint16(w),
			},
		})
	})
}

func (i *impl) start(c *exec.Cmd) error {
	c.Stdin, c.Stdout, c.Stderr = i.Slave, i.Slave, i.Slave
	return c.Start()
}

func newPty(_ Context, _ string, win Window, modes ssh.TerminalModes) (_ impl, rErr error) {
	ptm, pts, err := pty.Open()
	if err != nil {
		return impl{}, err
	}

	conn, err := ptm.SyscallConn()
	if err != nil {
		return impl{}, err
	}

	if err := conn.Control(func(fd uintptr) {
		rErr = applyTerminalModesToFd(fd, win.Width, win.Height, modes)
	}); err != nil {
		return impl{}, err
	}

	return impl{Master: ptm, Slave: pts}, rErr
}

func applyTerminalModesToFd(fd uintptr, width int, height int, modes ssh.TerminalModes) error {
	// Get the current TTY configuration.
	tios, err := termios.GTTY(int(fd))
	if err != nil {
		return fmt.Errorf("GTTY: %w", err)
	}

	// Apply the modes from the SSH request.
	tios.Row = height
	tios.Col = width

	for c, v := range modes {
		if c == ssh.TTY_OP_ISPEED {
			tios.Ispeed = int(v)
			continue
		}
		if c == ssh.TTY_OP_OSPEED {
			tios.Ospeed = int(v)
			continue
		}
		k, ok := terminalModeFlagNames[c]
		if !ok {
			continue
		}
		if _, ok := tios.CC[k]; ok {
			tios.CC[k] = uint8(v)
			continue
		}
		if _, ok := tios.Opts[k]; ok {
			tios.Opts[k] = v > 0
			continue
		}
	}

	// Save the new TTY configuration.
	if _, err := tios.STTY(int(fd)); err != nil {
		return fmt.Errorf("STTY: %w", err)
	}

	return nil
}

// terminalModeFlagNames maps the SSH terminal mode flags to mnemonic
// names used by the termios package.
var terminalModeFlagNames = map[uint8]string{
	ssh.VINTR:         "intr",
	ssh.VQUIT:         "quit",
	ssh.VERASE:        "erase",
	ssh.VKILL:         "kill",
	ssh.VEOF:          "eof",
	ssh.VEOL:          "eol",
	ssh.VEOL2:         "eol2",
	ssh.VSTART:        "start",
	ssh.VSTOP:         "stop",
	ssh.VSUSP:         "susp",
	ssh.VDSUSP:        "dsusp",
	ssh.VREPRINT:      "rprnt",
	ssh.VWERASE:       "werase",
	ssh.VLNEXT:        "lnext",
	ssh.VFLUSH:        "flush",
	ssh.VSWTCH:        "swtch",
	ssh.VSTATUS:       "status",
	ssh.VDISCARD:      "discard",
	ssh.IGNPAR:        "ignpar",
	ssh.PARMRK:        "parmrk",
	ssh.INPCK:         "inpck",
	ssh.ISTRIP:        "istrip",
	ssh.INLCR:         "inlcr",
	ssh.IGNCR:         "igncr",
	ssh.ICRNL:         "icrnl",
	ssh.IUCLC:         "iuclc",
	ssh.IXON:          "ixon",
	ssh.IXANY:         "ixany",
	ssh.IXOFF:         "ixoff",
	ssh.IMAXBEL:       "imaxbel",
	ssh.IUTF8:         "iutf8",
	ssh.ISIG:          "isig",
	ssh.ICANON:        "icanon",
	ssh.XCASE:         "xcase",
	ssh.ECHO:          "echo",
	ssh.ECHOE:         "echoe",
	ssh.ECHOK:         "echok",
	ssh.ECHONL:        "echonl",
	ssh.NOFLSH:        "noflsh",
	ssh.TOSTOP:        "tostop",
	ssh.IEXTEN:        "iexten",
	ssh.ECHOCTL:       "echoctl",
	ssh.ECHOKE:        "echoke",
	ssh.PENDIN:        "pendin",
	ssh.OPOST:         "opost",
	ssh.OLCUC:         "olcuc",
	ssh.ONLCR:         "onlcr",
	ssh.OCRNL:         "ocrnl",
	ssh.ONOCR:         "onocr",
	ssh.ONLRET:        "onlret",
	ssh.CS7:           "cs7",
	ssh.CS8:           "cs8",
	ssh.PARENB:        "parenb",
	ssh.PARODD:        "parodd",
	ssh.TTY_OP_ISPEED: "tty_op_ispeed",
	ssh.TTY_OP_OSPEED: "tty_op_ospeed",
}
