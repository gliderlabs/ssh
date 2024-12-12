package ssh

import (
	"testing"
	"time"
)

func TestSetPermissions(t *testing.T) {
	t.Parallel()
	permsExt := map[string]string{
		"foo": "bar",
	}
	session, _, cleanup := newTestSessionWithOptions(t, &Server{
		Handler: func(s Session) {
			if _, ok := s.Permissions().Extensions["foo"]; !ok {
				t.Fatalf("got %#v; want %#v", s.Permissions().Extensions, permsExt)
			}
		},
	}, nil, PasswordAuth(func(ctx Context, password string) bool {
		ctx.Permissions().Extensions = permsExt
		return true
	}))
	defer cleanup()
	if err := session.Run(""); err != nil {
		t.Fatal(err)
	}
}

func TestSetValue(t *testing.T) {
	t.Parallel()
	value := map[string]string{
		"foo": "bar",
	}
	key := "testValue"
	session, _, cleanup := newTestSessionWithOptions(t, &Server{
		Handler: func(s Session) {
			v := s.Context().Value(key).(map[string]string)
			if v["foo"] != value["foo"] {
				t.Fatalf("got %#v; want %#v", v, value)
			}
		},
	}, nil, PasswordAuth(func(ctx Context, password string) bool {
		ctx.SetValue(key, value)
		return true
	}))
	defer cleanup()
	if err := session.Run(""); err != nil {
		t.Fatal(err)
	}
}

func TestSetValueConcurrency(t *testing.T) {
	ctx, cancel := newContext(nil)
	defer cancel()

	go func() {
		for { // use a loop to access context.Context functions to make sure they are thread-safe with SetValue
			_, _ = ctx.Deadline()
			_ = ctx.Err()
			_ = ctx.Value("foo")
			select {
			case <-ctx.Done():
				return
			default:
			}
		}
	}()
	ctx.SetValue("bar", -1) // a context value which never changes
	now := time.Now()
	var cnt int64
	go func() {
		for time.Since(now) < 100*time.Millisecond {
			cnt++
			ctx.SetValue("foo", cnt) // a context value which changes a lot
		}
		cancel()
	}()
	<-ctx.Done()
	if ctx.Value("foo") != cnt {
		t.Fatal("context.Value(foo) doesn't match latest SetValue")
	}
	if ctx.Value("bar") != -1 {
		t.Fatal("context.Value(bar) doesn't match latest SetValue")
	}
}
