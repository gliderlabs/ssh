package ssh

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	gossh "golang.org/x/crypto/ssh"
)

const (
	forwardedUnixChannelType = "forwarded-streamlocal@openssh.com"
)

// directStreamLocalChannelData data struct as specified in OpenSSH's protocol
// extensions document, Section 2.4.
// https://cvsweb.openbsd.org/src/usr.bin/ssh/PROTOCOL?annotate=HEAD
type directStreamLocalChannelData struct {
	SocketPath string

	Reserved1 string
	Reserved2 uint32
}

// DirectStreamLocalHandler provides Unix forwarding from client -> server. It
// can be enabled by adding it to the server's ChannelHandlers under
// `direct-streamlocal@openssh.com`.
//
// Unix socket support on Windows is not widely available, so this handler may
// not work on all Windows installations and is not tested on Windows.
func DirectStreamLocalHandler(srv *Server, _ *gossh.ServerConn, newChan gossh.NewChannel, ctx Context) {
	var d directStreamLocalChannelData
	err := gossh.Unmarshal(newChan.ExtraData(), &d)
	if err != nil {
		_ = newChan.Reject(gossh.ConnectionFailed, "error parsing direct-streamlocal data: "+err.Error())
		return
	}

	if srv.LocalUnixForwardingCallback == nil {
		_ = newChan.Reject(gossh.Prohibited, "unix forwarding is disabled")
		return
	}
	dconn, err := srv.LocalUnixForwardingCallback(ctx, d.SocketPath)
	if err != nil {
		if errors.Is(err, ErrRejected) {
			_ = newChan.Reject(gossh.Prohibited, "unix forwarding is disabled")
			return
		}
		_ = newChan.Reject(gossh.ConnectionFailed, fmt.Sprintf("dial unix socket %q: %+v", d.SocketPath, err.Error()))
		return
	}

	ch, reqs, err := newChan.Accept()
	if err != nil {
		_ = dconn.Close()
		return
	}
	go gossh.DiscardRequests(reqs)

	bicopy(ctx, ch, dconn)
}

// remoteUnixForwardRequest describes the extra data sent in a
// streamlocal-forward@openssh.com containing the socket path to bind to.
type remoteUnixForwardRequest struct {
	SocketPath string
}

// remoteUnixForwardChannelData describes the data sent as the payload in the new
// channel request when a Unix connection is accepted by the listener.
type remoteUnixForwardChannelData struct {
	SocketPath string
	Reserved   uint32
}

// ForwardedUnixHandler can be enabled by creating a ForwardedUnixHandler and
// adding the HandleSSHRequest callback to the server's RequestHandlers under
// `streamlocal-forward@openssh.com` and
// `cancel-streamlocal-forward@openssh.com`
//
// Unix socket support on Windows is not widely available, so this handler may
// not work on all Windows installations and is not tested on Windows.
type ForwardedUnixHandler struct {
	sync.Mutex
	forwards map[string]net.Listener
}

func (h *ForwardedUnixHandler) HandleSSHRequest(ctx Context, srv *Server, req *gossh.Request) (bool, []byte) {
	h.Lock()
	if h.forwards == nil {
		h.forwards = make(map[string]net.Listener)
	}
	h.Unlock()
	conn, ok := ctx.Value(ContextKeyConn).(*gossh.ServerConn)
	if !ok {
		// TODO: log cast failure
		return false, nil
	}

	switch req.Type {
	case "streamlocal-forward@openssh.com":
		var reqPayload remoteUnixForwardRequest
		err := gossh.Unmarshal(req.Payload, &reqPayload)
		if err != nil {
			// TODO: log parse failure
			return false, nil
		}

		if srv.ReverseUnixForwardingCallback == nil {
			return false, []byte("unix forwarding is disabled")
		}

		addr := reqPayload.SocketPath
		h.Lock()
		_, ok := h.forwards[addr]
		h.Unlock()
		if ok {
			// TODO: log failure
			return false, nil
		}

		ln, err := srv.ReverseUnixForwardingCallback(ctx, addr)
		if err != nil {
			if errors.Is(err, ErrRejected) {
				return false, []byte("unix forwarding is disabled")
			}
			// TODO: log unix listen failure
			return false, nil
		}

		// The listener needs to successfully start before it can be added to
		// the map, so we don't have to worry about checking for an existing
		// listener as you can't listen on the same socket twice.
		//
		// This is also what the TCP version of this code does.
		h.Lock()
		h.forwards[addr] = ln
		h.Unlock()

		ctx, cancel := context.WithCancel(ctx)
		go func() {
			<-ctx.Done()
			_ = ln.Close()
		}()
		go func() {
			defer cancel()

			for {
				c, err := ln.Accept()
				if err != nil {
					// closed below
					break
				}
				payload := gossh.Marshal(&remoteUnixForwardChannelData{
					SocketPath: addr,
				})

				go func() {
					ch, reqs, err := conn.OpenChannel(forwardedUnixChannelType, payload)
					if err != nil {
						_ = c.Close()
						return
					}
					go gossh.DiscardRequests(reqs)
					bicopy(ctx, ch, c)
				}()
			}

			h.Lock()
			ln2, ok := h.forwards[addr]
			if ok && ln2 == ln {
				delete(h.forwards, addr)
			}
			h.Unlock()
			_ = ln.Close()
		}()

		return true, nil

	case "cancel-streamlocal-forward@openssh.com":
		var reqPayload remoteUnixForwardRequest
		err := gossh.Unmarshal(req.Payload, &reqPayload)
		if err != nil {
			// TODO: log parse failure
			return false, nil
		}
		h.Lock()
		ln, ok := h.forwards[reqPayload.SocketPath]
		h.Unlock()
		if ok {
			_ = ln.Close()
		}
		return true, nil

	default:
		return false, nil
	}
}

// unlink removes files and unlike os.Remove, directories are kept.
func unlink(path string) error {
	// Ignore EINTR like os.Remove, see ignoringEINTR in os/file_posix.go
	// for more details.
	for {
		err := syscall.Unlink(path)
		if !errors.Is(err, syscall.EINTR) {
			return err
		}
	}
}

// SimpleUnixLocalForwardingCallback provides a basic implementation for
// LocalUnixForwardingCallback. It will simply dial the requested socket using
// a context-aware dialer.
func SimpleUnixLocalForwardingCallback(ctx Context, socketPath string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "unix", socketPath)
}

// SimpleUnixReverseForwardingCallback provides a basic implementation for
// ReverseUnixForwardingCallback. The parent directory will be created (with
// os.MkdirAll), and existing files with the same name will be removed.
func SimpleUnixReverseForwardingCallback(_ Context, socketPath string) (net.Listener, error) {
	// Create socket parent dir if not exists.
	parentDir := filepath.Dir(socketPath)
	err := os.MkdirAll(parentDir, 0700)
	if err != nil {
		return nil, fmt.Errorf("failed to create parent directory %q for socket %q: %w", parentDir, socketPath, err)
	}

	// Remove existing socket if it exists. We do not use os.Remove() here
	// so that directories are kept. Note that it's possible that we will
	// overwrite a regular file here. Both of these behaviors match OpenSSH,
	// however, which is why we unlink.
	err = unlink(socketPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("failed to remove existing file in socket path %q: %w", socketPath, err)
	}

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on unix socket %q: %w", socketPath, err)
	}

	return ln, err
}
