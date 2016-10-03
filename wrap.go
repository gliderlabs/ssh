package ssh

import gossh "golang.org/x/crypto/ssh"

type PublicKey interface {
	gossh.PublicKey
}

type Permissions struct {
	*gossh.Permissions
}

type Signer interface {
	gossh.Signer
}

func ParseAuthorizedKey(in []byte) (out PublicKey, comment string, options []string, rest []byte, err error) {
	return gossh.ParseAuthorizedKey(in)
}

func ParsePublicKey(in []byte) (out PublicKey, err error) {
	return gossh.ParsePublicKey(in)
}
