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
	// ContextKeyUser is a context key for use with Contexts in this package.
	// The associated value will be of type string.
	ContextKeyUser = &contextKey{"user"}

	// ContextKeySessionID is a context key for use with Contexts in this package.
	// The associated value will be of type []byte.
	ContextKeySessionID = &contextKey{"session-id"}

	// ContextKeyPermissions is a context key for use with Contexts in this package.
	// The associated value will be of type *Permissions.
	ContextKeyPermissions = &contextKey{"permissions"}

	// ContextKeyClientVersion is a context key for use with Contexts in this package.
	// The associated value will be of type []byte.
	ContextKeyClientVersion = &contextKey{"client-version"}

	// ContextKeyServerVersion is a context key for use with Contexts in this package.
	// The associated value will be of type []byte.
	ContextKeyServerVersion = &contextKey{"server-version"}

	// ContextKeyLocalAddr is a context key for use with Contexts in this package.
	// The associated value will be of type net.Addr.
	ContextKeyLocalAddr = &contextKey{"local-addr"}

	// ContextKeyRemoteAddr is a context key for use with Contexts in this package.
	// The associated value will be of type net.Addr.
	ContextKeyRemoteAddr = &contextKey{"remote-addr"}

	// ContextKeyServer is a context key for use with Contexts in this package.
	// The associated value will be of type *Server.
	ContextKeyServer = &contextKey{"ssh-server"}

	// ContextKeyPublicKey is a context key for use with Contexts in this package.
	// The associated value will be of type PublicKey.
	ContextKeyPublicKey = &contextKey{"public-key"}
)

// Context is a package specific context interface. It exposes connection
// metadata and allows new values to be easily written to it. It's used in
// authentication handlers and callbacks, and its underlying context.Context is
// exposed on Session in the session Handler.
type Context interface {
	context.Context

	// User returns the username used when establishing the SSH connection.
	User() string

	// SessionID returns the session hash.
	SessionID() string

	// ClientVersion returns the version reported by the client.
	ClientVersion() string

	// ServerVersion returns the version reported by the server.
	ServerVersion() string

	// RemoteAddr returns the remote address for this connection.
	RemoteAddr() net.Addr

	// LocalAddr returns the local address for this connection.
	LocalAddr() net.Addr

	// Permissions returns the Permissions object used for this connection.
	Permissions() *Permissions

	// SetValue allows you to easily write new values into the underlying context.
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
