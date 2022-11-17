package ssh_test

import (
	"bytes"
	"testing"

	"github.com/charmbracelet/ssh"
)

func TestNewPtyWriter(t *testing.T) {
	in := "\nfoo\r\nbar\nmore text\rmore\r\r\r\nfoo\n\n"
	out := "\r\nfoo\r\nbar\r\nmore text\rmore\r\r\r\nfoo\r\n\r\n"
	var b bytes.Buffer
	n, err := ssh.NewPtyWriter(&b).Write([]byte(in))
	if err != nil {
		t.Error("did not expect an error", err)
	}
	if out != b.String() {
		t.Errorf("outputs do not match, expected %q got %q", out, b.String())
	}
	if n != len(in) {
		t.Errorf("expected to write %d bytes, wrote %d", len(in), n)
	}
}
