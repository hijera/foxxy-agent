package session_test

// Fork edges of the nested child layout: bundles earlier releases stored in
// the sessions root under sub_ ids, and two backends sharing one home.

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/bgtask"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// writeLegacyChild stores a bundle the way releases before the nested layout
// did: in the sessions root, under a sub_ id, linked to its parent only by
// session.json.
func writeLegacyChild(t *testing.T, store *session.FileStore, id, parentID, cwd string) {
	t.Helper()
	dir := filepath.Join(store.Root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta, err := json.Marshal(map[string]interface{}{
		"version": 1, "id": id, "cwd": cwd, "mode": "agent", "updatedAt": "2026-09-01T10:00:00Z",
		"subagentRun": true, "parentSessionId": parentID, "subagentName": "explore", "subagentTaskId": "bg_legacy",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "session.json"), meta, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "messages.json"), []byte(`{"version":1,"messages":[{"role":"user","content":"look around"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Nothing migrates a child an earlier release stored in the sessions root.
// It stays out of the default listing, shows under its parent when children
// are asked for, stays read-only, and goes with its parent's tree instead of
// being left behind as an orphan nobody can open from History.
func TestLegacyRootLevelChildStaysPartOfItsParentsTree(t *testing.T) {
	m, store, root := newSubagentTestManager(t)
	parent := newParent(t, m, root)
	legacy := "sub_0123456789abcdef01234567"
	writeLegacyChild(t, store, legacy, parent.ID, root)

	rows, err := store.ListSnapshotsWith(session.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.SessionID == legacy {
			t.Fatal("the legacy child is in the default listing")
		}
	}
	rows, err = store.ListSnapshotsWith(session.ListOptions{IncludeSubagents: true})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range rows {
		if row.SessionID == legacy {
			found = true
			if !row.SubagentRun || row.ParentSessionID != parent.ID {
				t.Fatalf("legacy row = %+v, want a subagent run of %s", row, parent.ID)
			}
		}
	}
	if !found {
		t.Fatal("the legacy child is missing from the listing that includes children")
	}

	// Read from disk alone: the child is not live in this process yet.
	tree, err := m.SessionTree(parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree) != 2 || tree[1].ID != legacy || tree[1].ParentSessionID != parent.ID {
		t.Fatalf("tree = %+v, want the parent and its legacy child", tree)
	}

	st, err := m.EnsureHTTPSession(context.Background(), legacy, root)
	if err != nil {
		t.Fatal(err)
	}
	if !st.IsSubagentRun() {
		t.Fatal("a reopened legacy child must stay a read-only subagent run")
	}
	if _, err := m.HandleSessionPrompt(context.Background(), acp.SessionPromptParams{SessionID: legacy, Prompt: []acp.ContentBlock{{Type: "text", Text: "x"}}}); !errors.Is(err, session.ErrSubagentReadOnly) {
		t.Fatalf("prompt on a legacy child = %v, want ErrSubagentReadOnly", err)
	}

	pool := bgtask.NewWithRunner(bgtask.Config{}, nopRunner{})
	if err := m.DeleteSessionTree(parent.ID, pool); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{parent.ID, legacy} {
		if store.HasPersistedSnapshot(id) {
			t.Fatalf("bundle %s survived the tree delete", id)
		}
	}
}

// A root-level sub_ folder that names no parent is not adopted by anyone.
func TestLegacyRootLevelBundleWithoutAParentIsLeftAlone(t *testing.T) {
	m, store, root := newSubagentTestManager(t)
	parent := newParent(t, m, root)
	stray := "sub_ffffffffffffffffffffffff"
	writeLegacyChild(t, store, stray, "", root)

	tree, err := m.SessionTree(parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree) != 1 {
		t.Fatalf("tree = %+v, want only the parent", tree)
	}
	pool := bgtask.NewWithRunner(bgtask.Config{}, nopRunner{})
	if err := m.DeleteSessionTree(parent.ID, pool); err != nil {
		t.Fatal(err)
	}
	if !store.HasPersistedSnapshot(stray) {
		t.Fatal("a delete removed a bundle that names no parent")
	}
}

// Two backends on one home (an editor panel and the desktop window) see each
// other's bundles only through the filesystem. A child the first one spawned
// just after the second walked the sessions root is still reopened by the
// second as the read-only transcript it is, never answered with a fresh,
// writable session built over its bundle.
func TestSecondBackendReopensAChildSpawnedAfterItsLastWalk(t *testing.T) {
	root := t.TempDir()
	sessions := filepath.Join(root, "sessions")
	if err := os.MkdirAll(sessions, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := testConfig()
	cfg.Paths.Home = filepath.Join(root, "home")
	runner := func(_ context.Context, st *session.State, prompt []acp.ContentBlock, _ acp.UpdateSender) (string, error) {
		st.AddMessage(acpToLLM(prompt))
		return string(acp.StopReasonEndTurn), nil
	}
	storeA := &session.FileStore{Root: sessions}
	mA := session.NewManager(cfg, noopSender{}, runner, slog.Default(), root, storeA)
	storeB := &session.FileStore{Root: sessions}
	mB := session.NewManager(cfg, noopSender{}, runner, slog.Default(), root, storeB)

	parent := newParent(t, mA, root)
	// B looks an unknown id up, which walks the root and starts its rate
	// limit before the child exists.
	_ = storeB.SessionPath(newTestSessionID(t))

	childID := newTestSessionID(t)
	if _, err := mA.CreateSubagentSession(context.Background(), session.SubagentSpec{
		ID: childID, ParentSessionID: parent.ID, Name: "explore", TaskID: "bg_1", CWD: root, Depth: 1,
	}); err != nil {
		t.Fatal(err)
	}

	st, err := mB.EnsureHTTPSession(context.Background(), childID, root)
	if err != nil {
		t.Fatal(err)
	}
	if !st.IsSubagentRun() {
		t.Fatal("the second backend built a writable session over the child's bundle")
	}
	meta, err := storeA.ReadMeta(childID)
	if err != nil {
		t.Fatal(err)
	}
	if !meta.SubagentRun || meta.ParentSessionID != parent.ID {
		t.Fatalf("the child's session.json lost its link: %+v", meta)
	}
	if dir := storeB.SessionPath(childID); filepath.Dir(filepath.Dir(dir)) != storeA.SessionPath(parent.ID) {
		t.Fatalf("the second backend resolved the child to %s, want inside the parent's bundle", dir)
	}
	if _, err := os.Stat(filepath.Join(sessions, childID)); !os.IsNotExist(err) {
		t.Fatalf("a second, top-level bundle appeared for the child: %v", err)
	}
}
