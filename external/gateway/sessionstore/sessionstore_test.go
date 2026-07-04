//go:build gateway || gateway.telegram

package sessionstore_test

import (
	"testing"

	"github.com/hijera/foxxycode-agent/external/gateway/sessionstore"
	"github.com/hijera/foxxycode-agent/internal/config"
)

func TestSessionKey_Private(t *testing.T) {
	k := sessionstore.SessionKey("tg", -1, 42, config.IsolationShared, false)
	want := "tg:user:42"
	if k != want {
		t.Fatalf("want %q got %q", want, k)
	}
}

func TestSessionKey_GroupShared(t *testing.T) {
	k := sessionstore.SessionKey("tg", -100, 42, config.IsolationShared, true)
	want := "tg:chat:-100"
	if k != want {
		t.Fatalf("want %q got %q", want, k)
	}
}

func TestSessionKey_GroupIndividual(t *testing.T) {
	k := sessionstore.SessionKey("tg", -100, 42, config.IsolationIndividual, true)
	want := "tg:chat:-100:user:42"
	if k != want {
		t.Fatalf("want %q got %q", want, k)
	}
}

func TestSessionKey_GroupAdmin(t *testing.T) {
	k := sessionstore.SessionKey("tg", -100, 42, config.IsolationAdmin, true)
	want := "tg:chat:-100:admin"
	if k != want {
		t.Fatalf("want %q got %q", want, k)
	}
}

func TestStore_GetAndReset(t *testing.T) {
	s := sessionstore.New()
	id1 := s.Get("tg:user:1")
	if id1 == "" {
		t.Fatal("expected non-empty session ID")
	}
	if s.Get("tg:user:1") != id1 {
		t.Fatal("second Get should return same ID")
	}
	id2 := s.Reset("tg:user:1")
	if id2 == id1 {
		t.Fatal("Reset should produce a different ID")
	}
	if s.Get("tg:user:1") != id2 {
		t.Fatal("Get after Reset should return new ID")
	}
}
