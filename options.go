package ssh

import "io/ioutil"

func PasswordAuth(fn PasswordHandler) Option {
	return func(srv *Server) error {
		srv.PasswordHandler = fn
		return nil
	}
}

func PublicKeyAuth(fn PublicKeyHandler) Option {
	return func(srv *Server) error {
		srv.PublicKeyHandler = fn
		return nil
	}
}

func HostKeyFile(filepath string) Option {
	return func(srv *Server) error {
		pemBytes, err := ioutil.ReadFile(filepath)
		if err != nil {
			return err
		}
		for _, block := range decodePemBlocks(pemBytes) {
			signer, err := signerFromBlock(block)
			if err != nil {
				return err
			}
			srv.HostSigners = append(srv.HostSigners, signer)
		}
		return nil
	}
}

func HostKeyPEM(bytes []byte) Option {
	return func(srv *Server) error {
		for _, block := range decodePemBlocks(bytes) {
			signer, err := signerFromBlock(block)
			if err != nil {
				return err
			}
			srv.HostSigners = append(srv.HostSigners, signer)
		}
		return nil
	}
}

func NoPty() Option {
	return func(srv *Server) error {
		srv.PtyCallback = func(user string, permissions *Permissions) bool {
			return false
		}
		return nil
	}
}
