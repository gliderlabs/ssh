package ssh

import (
	"context"
	"net"

	gossh "golang.org/x/crypto/ssh"
)

// contextKey is a value for use with context.WithValue. It's used as
// a pointer so it fits in an interface{} without allocation.
type contextKey struct {
	name string
}

var (
	ContextKeyUser          = &contextKey{"user"}
	ContextKeySessionID     = &contextKey{"session-id"}
	ContextKeyPermissions   = &contextKey{"permissions"}
	ContextKeyClientVersion = &contextKey{"client-version"}
	ContextKeyServerVersion = &contextKey{"server-version"}
	ContextKeyLocalAddr     = &contextKey{"local-addr"}
	ContextKeyRemoteAddr    = &contextKey{"remote-addr"}
	ContextKeyServer        = &contextKey{"ssh-server"}
	ContextKeyPublicKey     = &contextKey{"public-key"}
)

type Context interface {
	context.Context
	User() string
	SessionID() string
	ClientVersion() string
	ServerVersion() string
	RemoteAddr() net.Addr
	LocalAddr() net.Addr
	Permissions() *Permissions
	SetValue(key, value interface{})
}

type sshContext struct {
	context.Context
}

func newContext(srv *Server) *sshContext {
	ctx := &sshContext{context.Background()}
	ctx.SetValue(ContextKeyServer, srv)
	perms := &Permissions{&gossh.Permissions{}}
	ctx.SetValue(ContextKeyPermissions, perms)
	return ctx
}

// this is separate from newContext because we will get ConnMetadata
// at different points so it needs to be applied separately
func (ctx *sshContext) applyConnMetadata(conn gossh.ConnMetadata) {
	if ctx.Value(ContextKeySessionID) != nil {
		return
	}
	// for most of these, instead of converting to strings now, storing the byte
	// slices means allocations only happen when accessing, not when contexts
	// are being copied around
	ctx.SetValue(ContextKeySessionID, conn.SessionID())
	ctx.SetValue(ContextKeyClientVersion, conn.ClientVersion())
	ctx.SetValue(ContextKeyServerVersion, conn.ServerVersion())
	ctx.SetValue(ContextKeyUser, conn.User())
	ctx.SetValue(ContextKeyLocalAddr, conn.LocalAddr())
	ctx.SetValue(ContextKeyRemoteAddr, conn.RemoteAddr())
}

func (ctx *sshContext) SetValue(key, value interface{}) {
	ctx.Context = context.WithValue(ctx.Context, key, value)
}

func (ctx *sshContext) User() string {
	return ctx.Value(ContextKeyUser).(string)
}

func (ctx *sshContext) SessionID() string {
	id, _ := ctx.Value(ContextKeySessionID).([]byte)
	return string(id)
}

func (ctx *sshContext) ClientVersion() string {
	version, _ := ctx.Value(ContextKeyClientVersion).([]byte)
	return string(version)
}

func (ctx *sshContext) ServerVersion() string {
	version, _ := ctx.Value(ContextKeyServerVersion).([]byte)
	return string(version)
}

func (ctx *sshContext) RemoteAddr() net.Addr {
	return ctx.Value(ContextKeyRemoteAddr).(net.Addr)
}

func (ctx *sshContext) LocalAddr() net.Addr {
	return ctx.Value(ContextKeyLocalAddr).(net.Addr)
}

func (ctx *sshContext) Permissions() *Permissions {
	return ctx.Value(ContextKeyPermissions).(*Permissions)
}
