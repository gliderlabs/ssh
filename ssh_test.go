package ssh

import (
	"testing"

	"github.com/gliderlabs/ssh"
)

func TestKeysEqual(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("The code did panic")
		}
	}()

	if ssh.KeysEqual(nil, nil) {
		t.Error("two nil keys should not return true")
	}
}
