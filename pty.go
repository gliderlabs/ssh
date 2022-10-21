package ssh

import (
	"bytes"
	"io"
)

// NewPtyWriter creates a writer that handles when the session has a active
// PTY, replacing the \n with \r\n.
func NewPtyWriter(w io.Writer) io.Writer {
	return ptyWriter{
		w: w,
	}
}

type ptyWriter struct {
	w io.Writer
}

func (w ptyWriter) Write(p []byte) (int, error) {
	m := len(p)
	// normalize \n to \r\n when pty is accepted.
	// this is a hardcoded shortcut since we don't support terminal modes.
	p = bytes.Replace(p, []byte{'\n'}, []byte{'\r', '\n'}, -1)
	p = bytes.Replace(p, []byte{'\r', '\r', '\n'}, []byte{'\r', '\n'}, -1)
	n, err := w.w.Write(p)
	if n > m {
		n = m
	}
	return n, err
}
