//go:build !linux && !darwin && !freebsd && !dragonfly && !netbsd && !openbsd && !solaris && !windows
// +build !linux,!darwin,!freebsd,!dragonfly,!netbsd,!openbsd,!solaris,!windows

package ssh

import (
	"os/exec"

	"golang.org/x/crypto/ssh"
)

type impl struct{}

func (i *impl) IsZero() bool {
	return true
}

func (i *impl) Name() string {
	return ""
}

func (i *impl) Read(p []byte) (n int, err error) {
	return 0, ErrUnsupported
}

func (i *impl) Write(p []byte) (n int, err error) {
	return 0, ErrUnsupported
}

func (i *impl) Resize(w int, h int) error {
	return ErrUnsupported
}

func (i *impl) Close() error {
	return nil
}

func (*impl) start(*exec.Cmd) error {
	return ErrUnsupported
}

func newPty(Context, string, Window, ssh.TerminalModes) (impl, error) {
	return impl{}, ErrUnsupported
}
