package ssh_test

import (
	"fmt"
	"io"
	"os"

	"github.com/gliderlabs/ssh"
)

func ExampleListenAndServe() {
	ssh.ListenAndServe(":2222", func(s ssh.Session) {
		io.WriteString(s, "Hello world\n")
	})
}

func ExamplePasswordAuth() {
	ssh.ListenAndServe(":2222", nil,
		ssh.PasswordAuth(func(ctx ssh.Context, pass string) bool {
			return pass == "secret"
		}),
	)
}

func ExamplePasswordAuthE() {
	ssh.ListenAndServe(":2222", nil,
		ssh.PasswordAuthE(func(ctx ssh.Context, pass string) error {
			if pass == "secret" {
				return nil
			}
			return fmt.Errorf("password incorrect")
		}),
	)
}

func ExampleNoPty() {
	ssh.ListenAndServe(":2222", nil, ssh.NoPty())
}

func ExamplePublicKeyAuth() {
	ssh.ListenAndServe(":2222", nil,
		ssh.PublicKeyAuth(func(ctx ssh.Context, key ssh.PublicKey) bool {
			data, _ := os.ReadFile("/path/to/allowed/key.pub")
			allowed, _, _, _, _ := ssh.ParseAuthorizedKey(data)
			return ssh.KeysEqual(key, allowed)
		}),
	)
}

func ExampleHostKeyFile() {
	ssh.ListenAndServe(":2222", nil, ssh.HostKeyFile("/path/to/host/key"))
}
