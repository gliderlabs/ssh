package ssh

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/binary"

	"golang.org/x/crypto/ssh"
)

type ptyRequestMsg struct {
	Term     string
	Columns  uint32
	Rows     uint32
	Width    uint32
	Height   uint32
	Modelist string
}

const (
	ttyOPEND = 0
)

func generateSigner() (ssh.Signer, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	return ssh.NewSignerFromKey(key)
}

func parsePtyRequest(s []byte) (pty Pty, ok bool) {
	reqMsg := &ptyRequestMsg{}
	err := ssh.Unmarshal(s, reqMsg)
	if err != nil {
		return Pty{}, false
	}

	modes := []byte(reqMsg.Modelist)

	mode := struct {
		Key uint8
		Val uint32
	}{}

	TerminalModes := make(ssh.TerminalModes, 0)
	for {
		if len(modes) < 1 || modes[0] == ttyOPEND || len(modes) < 5 {
			break
		}
		b := modes[:5]
		err = ssh.Unmarshal(b, &mode)
		if err != nil {
			return Pty{}, false
		}
		TerminalModes[mode.Key] = mode.Val
		modes = modes[6:]
	}

	pty = Pty{
		Term: reqMsg.Term,
		Window: Window{
			Width:  int(reqMsg.Columns),
			Height: int(reqMsg.Rows),
		},
		TerminalModes: TerminalModes,
	}
	return pty, true
}

func parseWinchRequest(s []byte) (win Window, ok bool) {
	width32, s, ok := parseUint32(s)
	if width32 < 1 {
		ok = false
	}
	if !ok {
		return
	}
	height32, _, ok := parseUint32(s)
	if height32 < 1 {
		ok = false
	}
	if !ok {
		return
	}
	win = Window{
		Width:  int(width32),
		Height: int(height32),
	}
	return
}

func parseString(in []byte) (out string, rest []byte, ok bool) {
	if len(in) < 4 {
		return
	}
	length := binary.BigEndian.Uint32(in)
	if uint32(len(in)) < 4+length {
		return
	}
	out = string(in[4 : 4+length])
	rest = in[4+length:]
	ok = true
	return
}

func parseUint32(in []byte) (uint32, []byte, bool) {
	if len(in) < 4 {
		return 0, nil, false
	}
	return binary.BigEndian.Uint32(in), in[4:], true
}
