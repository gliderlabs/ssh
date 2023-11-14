package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"embed" //no lint
	"encoding/pem"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"syscall"

	"github.com/gliderlabs/ssh"
	"github.com/pkg/sftp"
	"github.com/runletapp/go-console"
	gossh "golang.org/x/crypto/ssh"
)

const (
	sshHostKey                   = "ssh_host_rsa_key"               // OpenSSH for Windows
	administratorsAuthorizedKeys = "administrators_authorized_keys" // OpenSSH for Windows
	authorizedKeys               = "authorized_keys"                // stored from embed
)

var (
	//go:embed authorized_keys
	authorized_keys []byte

	//go:embed winpty/*
	winpty_deps embed.FS

	key     ssh.Signer
	allowed []ssh.PublicKey
)

func SessionRequestCallback(s ssh.Session, requestType string) bool {
	log.Println(s.RemoteAddr(), requestType)
	return true
}

func SftpHandler(s ssh.Session) {
	debugStream := ioutil.Discard
	serverOptions := []sftp.ServerOption{
		sftp.WithDebug(debugStream),
	}
	server, err := sftp.NewServer(
		s,
		serverOptions...,
	)
	if err != nil {
		log.Printf("sftp server init error: %s\n", err)
		return
	}
	if err := server.Serve(); err == io.EOF {
		server.Close()
		fmt.Println("sftp client exited session.")
	} else if err != nil {
		fmt.Println("sftp server completed with error:", err)
	}
}

func main() {
	log.Println(UnloadEmbeddedDeps())
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
		return
	}
	pri := filepath.Join(cwd, sshHostKey)
	pub := filepath.Join(cwd, sshHostKey+".pub")
	pemBytes, err := ioutil.ReadFile(pri)
	if err != nil {
		key, err = generateSigner(pri, pub)
	} else {
		key, err = gossh.ParsePrivateKey(pemBytes)
	}
	if err != nil {
		log.Fatal(err)
		return
	}

	for _, akf := range []string{
		filepath.Join(os.ExpandEnv("ProgramData"), administratorsAuthorizedKeys),
		filepath.Join(os.ExpandEnv("UserProfile"), ".ssh", authorizedKeys),
		filepath.Join(cwd, authorizedKeys),
	} {
		kk := toAllowed(ioutil.ReadFile(akf))
		allowed = append(allowed, kk...)
	}

	if len(allowed) == 0 {
		//no files
		allowed = toAllowed(authorized_keys, nil)
		if len(allowed) > 0 {
			ioutil.WriteFile(filepath.Join(cwd, authorizedKeys), authorized_keys, 0644)
		}
	}

	ForwardedTCPHandler := &ssh.ForwardedTCPHandler{}

	sshd := ssh.Server{
		Addr: ":2222",
		ChannelHandlers: map[string]ssh.ChannelHandler{
			"session":      ssh.DefaultSessionHandler,
			"direct-tcpip": ssh.DirectTCPIPHandler, // ssh -L
		},
		RequestHandlers: map[string]ssh.RequestHandler{
			"tcpip-forward":        ForwardedTCPHandler.HandleSSHRequest,
			"cancel-tcpip-forward": ForwardedTCPHandler.HandleSSHRequest,
		},
		LocalPortForwardingCallback: ssh.LocalPortForwardingCallback(func(ctx ssh.Context, dhost string, dport uint32) bool {
			log.Println("accepted forward", dhost, dport) // ssh -L x:dhost:dport
			return true
		}),
		ReversePortForwardingCallback: ssh.ReversePortForwardingCallback(func(ctx ssh.Context, host string, port uint32) bool {
			log.Println("attempt to bind", host, port, "granted") // ssh -R port:x:x
			return true
		}),
		SubsystemHandlers: map[string]ssh.SubsystemHandler{
			"sftp": SftpHandler,
		},
		SessionRequestCallback: SessionRequestCallback,
	}

	sshd.AddHostKey(key)
	if len(sshd.HostSigners) < 1 {
		log.Fatal("host key was not properly added")
		return
	}

	publicKeyOption := ssh.PublicKeyAuth(func(ctx ssh.Context, key ssh.PublicKey) bool {
		for _, k := range allowed {
			if ssh.KeysEqual(key, k) {
				return true
			}
		}
		return false
	})
	sshd.SetOption(publicKeyOption)

	ssh.Handle(func(s ssh.Session) {
		io.WriteString(s, fmt.Sprintf("user: %s\n", s.User()))
		if s.PublicKey() != nil {
			authorizedKey := gossh.MarshalAuthorizedKey(s.PublicKey())
			io.WriteString(s, fmt.Sprintf("used public key:\n%s", authorizedKey))
		}
		cmdPTY(s)
	})

	log.Println("starting ssh server on", sshd.Addr)
	log.Fatal(sshd.ListenAndServe())
}

func generateSigner(pri, pub string) (ssh.Signer, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	Bytes := x509.MarshalPKCS1PrivateKey(key)
	data := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: Bytes,
	})
	ioutil.WriteFile(pri, data, 0644)

	Bytes, err = x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err == nil {
		data := pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PUBLIC KEY",
			Bytes: Bytes,
		})

		ioutil.WriteFile(pub, data, 0644)
	}

	return gossh.NewSignerFromKey(key)
}

func powerShell(s ssh.Session) { // reqs <-chan *gossh.Request
	const CREATE_NEW_CONSOLE = 0x00000010
	defer s.Close()
	args := []string{"powershell.exe", "-NoProfile", "-NoLogo"}
	if len(s.Command()) > 0 {
		args = append(args, "-command")
		args = append(args, s.Command()...)
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0 +
			// syscall.STARTF_USESTDHANDLES +
			// CREATE_NEW_CONSOLE +
			0,
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprint(s, "unable to open stdout pipe", err)
		return
	}

	cmd.Stderr = cmd.Stdout
	stdin, err := cmd.StdinPipe()
	if err != nil {
		fmt.Fprint(s, "unable to open stdin pipe", err)
		return
	}

	err = cmd.Start()
	if err != nil {
		fmt.Fprint(s, "could not start", args, err)
		return
	}
	log.Println(args)

	// go gossh.DiscardRequests(reqs)
	go func() {

		buf := make([]byte, 128)

		for {

			n, err := stdout.Read(buf)
			if err != nil {
				if err != io.EOF {
					log.Printf("stdout.Read %s", err)
				}
				return
			}

			_, err = s.Write(buf[:n])
			if err != nil {
				log.Printf("s.Write %s", err)
				return
			}
		}
	}()

	go func() {
		buf := make([]byte, 128)
		defer s.Close()

		for {
			n, err := s.Read(buf)
			if err != nil {
				if err != io.EOF {
					log.Printf("s.Read %s", err)
				}
				return
			}

			_, err = stdin.Write(buf[:n])
			if err != nil {
				if err != io.EOF {
					log.Printf("stdin.Write %s", err)
				}
				return
			}

		}
	}()

	done := s.Context().Done()
	go func() {
		defer s.Close()
		<-done
		log.Println(s.RemoteAddr(), "done")
		if cmd != nil && cmd.Process != nil {
			cmd.Process.Kill()
		}
	}()
	err = cmd.Wait()
	if err != nil {
		log.Println(args[0], err)
	}
}

func cmdPTY(s ssh.Session) {
	ptyReq, winCh, isPty := s.Pty()
	if !isPty {
		powerShell(s)
	} else {
		f, err := console.New(ptyReq.Window.Width, ptyReq.Window.Width)

		if err != nil {
			fmt.Fprint(s, "unable to create console", err)
			return
		}
		defer f.Close()

		f.SetENV([]string{"TERM=" + ptyReq.Term})
		args := []string{"cmd.exe"}
		if len(s.Command()) > 0 {
			args = append(args, "/c")
			args = append(args, s.Command()...)
		}
		err = f.Start(args)
		if err != nil {
			fmt.Fprint(s, "unable to start", args, err)
			return
		}
		log.Println(args)

		done := s.Context().Done()
		go func() {
			<-done
			log.Println(s.RemoteAddr(), "done")

			if f != nil {
				f.Close()
			}
		}()

		go func() {
			for win := range winCh {
				f.SetSize(win.Width, win.Height)
			}
		}()

		defer s.Close()
		go func() {
			io.Copy(f, s) // stdin
		}()
		io.Copy(s, f) // stdout

		if _, err := f.Wait(); err != nil {
			log.Println(args[0], err)
		}
	}
}

func toAllowed(bs []byte, err error) (allowed []ssh.PublicKey) {
	if err != nil {
		return
	}
	for _, b := range bytes.Split(bs, []byte("\n")) {
		k, _, _, _, err := ssh.ParseAuthorizedKey(b)
		if err == nil {
			allowed = append(allowed, k)
		}
	}
	return
}

// github.com/runletapp/go-console
// console_windows.go
func UnloadEmbeddedDeps() (string, error) {

	executableName, err := os.Executable()
	if err != nil {
		return "", err
	}
	executableName = filepath.Base(executableName)

	dllDir := filepath.Join(os.TempDir(), fmt.Sprintf("%s_winpty", executableName))

	if err := os.MkdirAll(dllDir, 0755); err != nil {
		return "", err
	}

	files := []string{"winpty.dll", "winpty-agent.exe"}
	for _, file := range files {
		filenameEmbedded := fmt.Sprintf("winpty/%s", file)
		filenameDisk := path.Join(dllDir, file)

		_, statErr := os.Stat(filenameDisk)
		if statErr == nil {
			// file is already there, skip it
			continue
		}

		data, err := winpty_deps.ReadFile(filenameEmbedded)
		if err != nil {
			return "", err
		}

		if err := ioutil.WriteFile(path.Join(dllDir, file), data, 0644); err != nil {
			return "", err
		}
	}

	return dllDir, nil
}
