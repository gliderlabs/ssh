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

var _ io.Writer = ptyWriter{}

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

// NewPtyReadWriter return an io.ReadWriter that delegates the read to the
// given io.ReadWriter, and the writes to a ptyWriter.
func NewPtyReadWriter(rw io.ReadWriter) io.ReadWriter {
	return readWriterDelegate{
		w: NewPtyWriter(rw),
		r: rw,
	}
}

var _ io.ReadWriter = readWriterDelegate{}

type readWriterDelegate struct {
	w io.Writer
	r io.Reader
}

func (rw readWriterDelegate) Read(p []byte) (n int, err error) {
	return rw.r.Read(p)
}

func (rw readWriterDelegate) Write(p []byte) (n int, err error) {
	return rw.w.Write(p)
}
