package ssh

import (
	"encoding/binary"
	"testing"
)

func TestParseStringValid(t *testing.T) {
	in := make([]byte, 4+5)
	binary.BigEndian.PutUint32(in, 5)
	copy(in[4:], []byte("hello"))
	out, rest, ok := parseString(in)
	if !ok {
		t.Fatal("expected ok for a well-formed string")
	}
	if out != "hello" {
		t.Fatalf("got %q, want %q", out, "hello")
	}
	if len(rest) != 0 {
		t.Fatalf("got %d bytes of rest, want 0", len(rest))
	}
}

func TestParseStringLengthTooLarge(t *testing.T) {
	// length prefix of 0xFFFFFFFF. In the old code 4+length overflowed the
	// uint32 to 3, defeating the bounds check and slicing in[4:3].
	in := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0x00, 0x00}
	out, rest, ok := parseString(in)
	if ok {
		t.Fatalf("expected ok=false for oversized length, got out=%q", out)
	}
	if rest != nil {
		t.Fatalf("expected nil rest, got %v", rest)
	}
}

func TestParsePtyRequestOversizedTerm(t *testing.T) {
	// A pty-req whose term-string length is 0xFFFFFFFF must be rejected, not panic.
	payload := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0x00, 0x00, 0x00, 0x00}
	_, ok := parsePtyRequest(payload)
	if ok {
		t.Fatal("expected ok=false for malformed pty-req")
	}
}
