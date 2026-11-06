package remote

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// The server pins a session on its first prompt, so a change asked for before
// that is held here and rides in as commands with that prompt. When another
// surface creates the session in the meantime and the snapshot that arrives
// already arms that change, the held copy is obsolete: SessionSettings must
// list the override once, and the first prompt must not ask for it again.
func TestSessionSettingsListsAnOverrideTheSnapshotCarriesOnce(t *testing.T) {
	mini := "remote/mini"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			// The session does not exist until its first prompt.
			http.Error(w, `{"error":{"message":"session not found"}}`, http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"object":"list","data":[
			{"id":"agent","owned_by":"foxxycode"},
			{"id":"remote/mini","owned_by":"remote"}]}`))
	}))
	defer srv.Close()

	h, err := NewHandler(Options{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	h.SetServer(&collectSender{})
	if _, err := h.ApplySessionSettings(context.Background(), "sess_x", session.SettingsChange{
		Model: &mini, Turns: 2, Source: "remote",
	}); err != nil {
		t.Fatalf("ApplySessionSettings: %v", err)
	}

	// Held for a session the server has not created yet: listed once.
	snap, err := h.SessionSettings("sess_x")
	if err != nil {
		t.Fatal(err)
	}
	assertArmedOverrides(t, snap.Overrides, []acp.TurnOverride{
		{Setting: session.SettingModel, Value: mini, TurnsLeft: 2},
	})

	// Another surface creates the session; the snapshot that comes back
	// already carries the change, now one turn further along.
	h.mirrorSettings("sess_x", acp.SessionSettings{
		SessionID: "sess_x", Version: 3, Model: mini, Mode: "agent", PermissionMode: "ask",
		Overrides: []acp.TurnOverride{{Setting: session.SettingModel, Value: mini, TurnsLeft: 1}},
	})
	snap, err = h.SessionSettings("sess_x")
	if err != nil {
		t.Fatal(err)
	}
	assertArmedOverrides(t, snap.Overrides, []acp.TurnOverride{
		{Setting: session.SettingModel, Value: mini, TurnsLeft: 1},
	})

	// The held copy is gone, so the first prompt does not arm it a second
	// time (which would put its turns back to two).
	if pending := h.takePendingSettings(h.session("sess_x")); pending != "" {
		t.Fatalf("first prompt would still send %q", pending)
	}
}

// A setting the snapshot does not arm stays held and listed: that is the
// whole point of the pending copy, and nothing may swallow it.
func TestSessionSettingsKeepsAnOverrideTheSnapshotDoesNotCarry(t *testing.T) {
	mini, high := "remote/mini", "high"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			http.Error(w, `{"error":{"message":"session not found"}}`, http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"agent","owned_by":"foxxycode"}]}`))
	}))
	defer srv.Close()

	h, err := NewHandler(Options{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	h.SetServer(&collectSender{})
	if _, err := h.ApplySessionSettings(context.Background(), "sess_x", session.SettingsChange{
		Model: &mini, Reasoning: &high, Turns: 2, Source: "remote",
	}); err != nil {
		t.Fatalf("ApplySessionSettings: %v", err)
	}

	// The snapshot arms only the model; the level of the same change is
	// still ours to send.
	h.mirrorSettings("sess_x", acp.SessionSettings{
		SessionID: "sess_x", Version: 4, Model: mini, PermissionMode: "ask",
		Overrides: []acp.TurnOverride{{Setting: session.SettingModel, Value: mini, TurnsLeft: 2}},
	})
	snap, err := h.SessionSettings("sess_x")
	if err != nil {
		t.Fatal(err)
	}
	assertArmedOverrides(t, snap.Overrides, []acp.TurnOverride{
		{Setting: session.SettingModel, Value: mini, TurnsLeft: 2},
		{Setting: session.SettingReasoning, Value: high, TurnsLeft: 2},
	})
	if pending := h.takePendingSettings(h.session("sess_x")); pending != "/reasoning high --count=2" {
		t.Fatalf("first prompt would send %q, want the reasoning command only", pending)
	}

	// An active-only override is the running turn's own value, not an armed
	// one: it covers nothing and both stay listed.
	h2, err := NewHandler(Options{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	h2.SetServer(&collectSender{})
	if _, err := h2.ApplySessionSettings(context.Background(), "sess_y", session.SettingsChange{
		Model: &mini, Turns: 2, Source: "remote",
	}); err != nil {
		t.Fatalf("ApplySessionSettings: %v", err)
	}
	h2.mirrorSettings("sess_y", acp.SessionSettings{
		SessionID: "sess_y", Version: 2, Model: "remote/other", PermissionMode: "ask",
		Overrides: []acp.TurnOverride{{Setting: session.SettingModel, Value: "remote/other", Active: true}},
	})
	snap, err = h2.SessionSettings("sess_y")
	if err != nil {
		t.Fatal(err)
	}
	assertArmedOverrides(t, snap.Overrides, []acp.TurnOverride{
		{Setting: session.SettingModel, Value: "remote/other", Active: true},
		{Setting: session.SettingModel, Value: mini, TurnsLeft: 2},
	})
}

// assertArmedOverrides checks the setting, value and turns of each override
// in order, treating an entry as active when want says so.
func assertArmedOverrides(t *testing.T, got, want []acp.TurnOverride) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("overrides = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i].Setting != want[i].Setting || got[i].Value != want[i].Value ||
			got[i].TurnsLeft != want[i].TurnsLeft || got[i].Active != want[i].Active {
			t.Fatalf("overrides[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}
