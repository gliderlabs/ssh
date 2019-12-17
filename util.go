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
		return
	}

	modes := []byte(reqMsg.Modelist)
	terminalModes, ok := parseTerminalModes(modes)
	if !ok {
		return
	}

	pty = Pty{
		Term: reqMsg.Term,
		Window: Window{
			Width:  int(reqMsg.Columns),
			Height: int(reqMsg.Rows),
		},
		TerminalModes: terminalModes,
	}
	return
}

func makeTerminalModes(terminalModes ssh.TerminalModes) string {
	var tm []byte
	for k, v := range terminalModes {
		kv := struct {
			Key byte
			Val uint32
		}{k, v}

		tm = append(tm, ssh.Marshal(&kv)...)
	}
	tm = append(tm, ttyOPEND)
	return string(tm)
}

func parseTerminalModes(s []byte) (terminalModes ssh.TerminalModes, ok bool) {
	mode := struct {
		Key uint8
		Val uint32
	}{}

	terminalModes = make(ssh.TerminalModes, 0)
	for {
		if len(s) < 1 {
			ok = true
			return
		}

		opcode := s[0]
		switch opcode {
		case ttyOPEND:
			ok = true
			return
		default:
			/*
			 * SSH2:
			 * Opcodes 1 to 159 are defined to have a uint32
			 * argument.
			 * Opcodes 160 to 255 are undefined and cause parsing
			 * to stop.
			 */
			if opcode > 0 && opcode < 160 {
				if len(s) < 5 {
					// parse failed
					return
				}

				b := s[:5]
				if err := ssh.Unmarshal(b, &mode); err != nil {
					return
				}

				terminalModes[mode.Key] = mode.Val
				s = s[6:]

			} else {
				ok = true
				return
			}
		}
	}
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
