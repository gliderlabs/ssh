//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris
// +build darwin dragonfly freebsd linux netbsd openbsd solaris

package ssh

import (
	"os"
	"os/exec"

	"github.com/charmbracelet/x/termios"
	"github.com/creack/pty"
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
		rErr = termios.SetWinsize(int(fd), &unix.Winsize{
			Row: uint16(h),
			Col: uint16(w),
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
	var ispeed, ospeed uint32
	ccs := map[termios.CC]uint8{}
	iflag := map[termios.I]bool{}
	oflag := map[termios.O]bool{}
	cflag := map[termios.C]bool{}
	lflag := map[termios.L]bool{}

	for op, value := range modes {
		switch op {
		case ssh.TTY_OP_ISPEED:
			ispeed = value
		case ssh.TTY_OP_OSPEED:
			ospeed = value
		default:
			cc, ok := sshToCc[op]
			if ok {
				ccs[cc] = uint8(value)
				continue
			}
			i, ok := sshToIflag[op]
			if ok {
				iflag[i] = value > 0
				continue
			}
			o, ok := sshToOflag[op]
			if ok {
				oflag[o] = value > 0
				continue
			}

			c, ok := sshToCflag[op]
			if ok {
				cflag[c] = value > 0
				continue
			}
			l, ok := sshToLflag[op]
			if ok {
				lflag[l] = value > 0
				continue
			}
		}
	}
	if err := termios.SetTermios(
		int(fd),
		ispeed,
		ospeed,
		ccs,
		iflag,
		oflag,
		cflag,
		lflag,
	); err != nil {
		return err
	}
	return termios.SetWinsize(int(fd), &unix.Winsize{
		Row: uint16(height),
		Col: uint16(width),
	})
}

var sshToCc = map[uint8]termios.CC{
	ssh.VINTR:    termios.INTR,
	ssh.VQUIT:    termios.QUIT,
	ssh.VERASE:   termios.ERASE,
	ssh.VKILL:    termios.KILL,
	ssh.VEOF:     termios.EOF,
	ssh.VEOL:     termios.EOL,
	ssh.VEOL2:    termios.EOL2,
	ssh.VSTART:   termios.START,
	ssh.VSTOP:    termios.STOP,
	ssh.VSUSP:    termios.SUSP,
	ssh.VWERASE:  termios.WERASE,
	ssh.VREPRINT: termios.RPRNT,
	ssh.VLNEXT:   termios.LNEXT,
	ssh.VDISCARD: termios.DISCARD,
	ssh.VSTATUS:  termios.STATUS,
	ssh.VSWTCH:   termios.SWTCH,
	ssh.VFLUSH:   termios.FLUSH,
	ssh.VDSUSP:   termios.DSUSP,
}

var sshToIflag = map[uint8]termios.I{
	ssh.IGNPAR:  termios.IGNPAR,
	ssh.PARMRK:  termios.PARMRK,
	ssh.INPCK:   termios.INPCK,
	ssh.ISTRIP:  termios.ISTRIP,
	ssh.INLCR:   termios.INLCR,
	ssh.IGNCR:   termios.IGNCR,
	ssh.ICRNL:   termios.ICRNL,
	ssh.IUCLC:   termios.IUCLC,
	ssh.IXON:    termios.IXON,
	ssh.IXANY:   termios.IXANY,
	ssh.IXOFF:   termios.IXOFF,
	ssh.IMAXBEL: termios.IMAXBEL,
}

var sshToOflag = map[uint8]termios.O{
	ssh.OPOST:  termios.OPOST,
	ssh.OLCUC:  termios.OLCUC,
	ssh.ONLCR:  termios.ONLCR,
	ssh.OCRNL:  termios.OCRNL,
	ssh.ONOCR:  termios.ONOCR,
	ssh.ONLRET: termios.ONLRET,
}

var sshToCflag = map[uint8]termios.C{
	ssh.CS7:    termios.CS7,
	ssh.CS8:    termios.CS8,
	ssh.PARENB: termios.PARENB,
	ssh.PARODD: termios.PARODD,
}

var sshToLflag = map[uint8]termios.L{
	ssh.IUTF8:   termios.IUTF8,
	ssh.ISIG:    termios.ISIG,
	ssh.ICANON:  termios.ICANON,
	ssh.ECHO:    termios.ECHO,
	ssh.ECHOE:   termios.ECHOE,
	ssh.ECHOK:   termios.ECHOK,
	ssh.ECHONL:  termios.ECHONL,
	ssh.NOFLSH:  termios.NOFLSH,
	ssh.TOSTOP:  termios.TOSTOP,
	ssh.IEXTEN:  termios.IEXTEN,
	ssh.ECHOCTL: termios.ECHOCTL,
	ssh.ECHOKE:  termios.ECHOKE,
	ssh.PENDIN:  termios.PENDIN,
	ssh.XCASE:   termios.XCASE,
}
