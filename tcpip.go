package ssh

import (
	"fmt"
	"io"
	"net"

	gossh "golang.org/x/crypto/ssh"
)

// direct-tcpip data struct as specified in RFC4254, Section 7.2
type localForwardData struct {
	DestinationHost string
	DestinationPort uint32

	OriginatorHost string
	OriginatorPort uint32
}

type directTCPHandler struct{}

func (_ directTCPHandler) HandleChannel(ctx *sshContext, newChan gossh.NewChannel) {
	srv := ctx.Value(ContextKeyServer).(*Server)
	d := localForwardData{}
	if err := gossh.Unmarshal(newChan.ExtraData(), &d); err != nil {
		newChan.Reject(gossh.ConnectionFailed, "error parsing forward data: "+err.Error())
		return
	}

	if srv.LocalPortForwardingCallback == nil || !srv.LocalPortForwardingCallback(ctx, d.DestinationHost, d.DestinationPort) {
		newChan.Reject(gossh.Prohibited, "port forwarding is disabled")
		return
	}

	dest := fmt.Sprintf("%s:%d", d.DestinationHost, d.DestinationPort)

	var dialer net.Dialer
	dconn, err := dialer.DialContext(ctx, "tcp", dest)
	if err != nil {
		newChan.Reject(gossh.ConnectionFailed, err.Error())
		return
	}

	ch, reqs, err := newChan.Accept()
	if err != nil {
		dconn.Close()
		return
	}
	go gossh.DiscardRequests(reqs)

	go func() {
		defer ch.Close()
		defer dconn.Close()
		io.Copy(ch, dconn)
	}()
	go func() {
		defer ch.Close()
		defer dconn.Close()
		io.Copy(dconn, ch)
	}()
}

type forwardedTCPHandler struct{}

func (_ forwardedTCPHandler) HandleRequest(ctx *sshContext, req *gossh.Request) (bool, []byte) {
	switch req.Type {
	case "cancel-tcpip-forward":
		return true, nil
	case "tcpip-forward":
		return true, nil
	default:
		return false, nil
	}
}

func (_ forwardedTCPHandler) HandleChannel(ctx *sshContext, newChan gossh.NewChannel) {

}
