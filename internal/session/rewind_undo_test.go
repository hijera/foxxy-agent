package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/llm"
)

func contents(msgs []llm.Message) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		out[i] = string(m.Role) + ":" + m.Content
	}
	return out
}

func sameContents(t *testing.T, got, want []llm.Message) {
	t.Helper()
	g, w := contents(got), contents(want)
	if len(g) != len(w) {
		t.Fatalf("history length: got %v, want %v", g, w)
	}
	for i := range g {
		if g[i] != w[i] {
			t.Fatalf("history[%d]: got %v, want %v", i, g, w)
		}
	}
}

func undoCurrentRewind(mgr *Manager, id string) (uint64, error) {
	dir := mgr.SessionByID(id).GetPersistedSessionDir()
	if snap := readRewindUndoSnapshot(dir); snap != nil {
		return mgr.UndoRewind(id, snap.snapshotID())
	}
	return mgr.UndoRewind(id, "")
}

func TestRewindUndoRestoresTheCutTail(t *testing.T) {
	mgr, fs := newTestManager(t)
	original := userMsgs("hello", "world", "again")
	st := newRewindState(t, mgr, fs, "s1", original)

	if _, err := mgr.RewindSession("s1", 1); err != nil {
		t.Fatalf("RewindSession: %v", err)
	}
	if offer, ok := mgr.RewindUndoAvailable("s1"); !ok || offer.UserMessageIndex != 1 {
		t.Fatalf("undo right after the rewind: offer=%+v ok=%v, want index 1", offer, ok)
	}
	// The edited turn the client sends after the rewind.
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "edited world"})
	st.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: "edited answer"})
	if offer, ok := mgr.RewindUndoAvailable("s1"); !ok || offer.UserMessageIndex != 1 {
		t.Fatalf("undo during the edited turn: offer=%+v ok=%v, want index 1", offer, ok)
	}
	before := st.MessagesRev()

	rev, err := undoCurrentRewind(mgr, "s1")
	if err != nil {
		t.Fatalf("UndoRewind: %v", err)
	}
	if rev <= before {
		t.Fatalf("messagesRev did not advance: before=%d after=%d", before, rev)
	}
	sameContents(t, st.GetMessages(), original)
	if _, ok := mgr.RewindUndoAvailable("s1"); ok {
		t.Fatalf("undo must be spent once applied")
	}
	if _, err := os.Stat(filepath.Join(st.GetPersistedSessionDir(), rewindUndoDirName)); !os.IsNotExist(err) {
		t.Fatalf("the snapshot folder should be gone after an undo: %v", err)
	}
}

func TestRewindUndoBeforeTheEditIsSent(t *testing.T) {
	mgr, fs := newTestManager(t)
	original := userMsgs("hello", "world")
	st := newRewindState(t, mgr, fs, "s1", original)

	if _, err := mgr.RewindSession("s1", 1); err != nil {
		t.Fatalf("RewindSession: %v", err)
	}
	if _, err := undoCurrentRewind(mgr, "s1"); err != nil {
		t.Fatalf("UndoRewind: %v", err)
	}
	sameContents(t, st.GetMessages(), original)
}

func TestRewindUndoIsGoneOnceTheNextPromptIsSent(t *testing.T) {
	mgr, fs := newTestManager(t)
	st := newRewindState(t, mgr, fs, "s1", userMsgs("hello", "world"))

	if _, err := mgr.RewindSession("s1", 1); err != nil {
		t.Fatalf("RewindSession: %v", err)
	}
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "edited world"})
	st.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: "edited answer"})
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "follow-up"})
	st.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: "follow-up answer"})

	if _, ok := mgr.RewindUndoAvailable("s1"); ok {
		t.Fatalf("a prompt after the edited turn must end the undo")
	}
	if _, err := undoCurrentRewind(mgr, "s1"); !errors.Is(err, ErrRewindUndoUnavailable) {
		t.Fatalf("want ErrRewindUndoUnavailable, got %v", err)
	}
	if got := st.GetMessages(); len(got) != 6 {
		t.Fatalf("a refused undo must leave the history alone, got %v", contents(got))
	}
}

func TestRewindUndoIsGoneWhenThePrefixChanged(t *testing.T) {
	mgr, fs := newTestManager(t)
	st := newRewindState(t, mgr, fs, "s1", userMsgs("hello", "world", "again"))

	if _, err := mgr.RewindSession("s1", 2); err != nil {
		t.Fatalf("RewindSession: %v", err)
	}
	// A compaction (or a rewind from another surface) rewrites the kept prefix:
	// the saved tail no longer continues it.
	st.ReplaceMessagesWithoutPersist([]llm.Message{
		{Role: llm.RoleUser, Content: "summary", CompactionSummary: true},
		{Role: llm.RoleAssistant, Content: "ok"},
	})
	if _, ok := mgr.RewindUndoAvailable("s1"); ok {
		t.Fatalf("a rewritten prefix must end the undo")
	}
	if _, err := undoCurrentRewind(mgr, "s1"); !errors.Is(err, ErrRewindUndoUnavailable) {
		t.Fatalf("want ErrRewindUndoUnavailable, got %v", err)
	}
}

func TestRewindUndoNamesOnlyTheLatestRewind(t *testing.T) {
	mgr, fs := newTestManager(t)
	st := newRewindState(t, mgr, fs, "s1", userMsgs("one", "two", "three"))

	if _, err := mgr.RewindSession("s1", 2); err != nil {
		t.Fatalf("first RewindSession: %v", err)
	}
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "three edited"})
	st.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: "ok"})
	afterFirstEdit := st.GetMessages()

	if _, err := mgr.RewindSession("s1", 1); err != nil {
		t.Fatalf("second RewindSession: %v", err)
	}
	if offer, ok := mgr.RewindUndoAvailable("s1"); !ok || offer.UserMessageIndex != 1 {
		t.Fatalf("the second rewind owns the undo: offer=%+v ok=%v", offer, ok)
	}
	if _, err := undoCurrentRewind(mgr, "s1"); err != nil {
		t.Fatalf("UndoRewind: %v", err)
	}
	sameContents(t, st.GetMessages(), afterFirstEdit)
}

func TestRewindUndoRestoresTheUILogOfTheTail(t *testing.T) {
	mgr, fs := newTestManager(t)
	st := newRewindState(t, mgr, fs, "s1", userMsgs("hello", "world"))
	st.AppendUILogError(1, "first turn error")
	st.AppendUILogNotice(2, "second turn notice")

	if _, err := mgr.RewindSession("s1", 1); err != nil {
		t.Fatalf("RewindSession: %v", err)
	}
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "edited world"})
	st.AppendUILogError(2, "edited turn error")

	if _, err := undoCurrentRewind(mgr, "s1"); err != nil {
		t.Fatalf("UndoRewind: %v", err)
	}
	log := st.GetUILog()
	if len(log) != 2 || log[0].Message != "first turn error" || log[1].Message != "second turn notice" {
		t.Fatalf("expected the original ui log back, got %+v", log)
	}
}

func TestRewindUndoRestoresToolCallDetail(t *testing.T) {
	mgr, fs := newTestManager(t)
	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "ask"},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "call_keep", Name: "read"}}},
		{Role: llm.RoleTool, ToolCallID: "call_keep", Content: "out"},
		{Role: llm.RoleUser, Content: "more"},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "call_cut", Name: "write"}}},
		{Role: llm.RoleTool, ToolCallID: "call_cut", Content: "out"},
	}
	st := newRewindState(t, mgr, fs, "s1", msgs)
	dir := st.GetPersistedSessionDir()
	for _, id := range []string{"call_keep", "call_cut"} {
		if err := WriteToolCallResult(dir, id, "full "+id); err != nil {
			t.Fatalf("tool call %s: %v", id, err)
		}
	}

	if _, err := mgr.RewindSession("s1", 1); err != nil {
		t.Fatalf("RewindSession: %v", err)
	}
	if dirs, _ := ListToolCalls(dir); len(dirs) != 1 || dirs[0] != ToolCallDirName("call_keep") {
		t.Fatalf("the rewind still prunes the cut calls from tool_calls/, got %v", dirs)
	}
	// The edited turn runs a tool of its own.
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "edited"})
	st.AddMessage(llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "call_edit", Name: "read"}}})
	if err := WriteToolCallResult(dir, "call_edit", "full call_edit"); err != nil {
		t.Fatalf("tool call edit: %v", err)
	}

	if _, err := undoCurrentRewind(mgr, "s1"); err != nil {
		t.Fatalf("UndoRewind: %v", err)
	}
	dirs, err := ListToolCalls(dir)
	if err != nil {
		t.Fatalf("ListToolCalls: %v", err)
	}
	want := map[string]bool{ToolCallDirName("call_keep"): true, ToolCallDirName("call_cut"): true}
	if len(dirs) != len(want) {
		t.Fatalf("expected call_keep and call_cut back, got %v", dirs)
	}
	for _, d := range dirs {
		if !want[d] {
			t.Fatalf("unexpected tool_calls dir %q after undo (all: %v)", d, dirs)
		}
	}
}

func TestRewindUndoRefusesActiveTurn(t *testing.T) {
	mgr, fs := newTestManager(t)
	newRewindState(t, mgr, fs, "s1", userMsgs("hello", "world"))
	if _, err := mgr.RewindSession("s1", 1); err != nil {
		t.Fatalf("RewindSession: %v", err)
	}

	release := mgr.markTurnActive("s1")
	defer release()

	if _, err := undoCurrentRewind(mgr, "s1"); !errors.Is(err, ErrSessionTurnActive) {
		t.Fatalf("want ErrSessionTurnActive, got %v", err)
	}
}

func TestRewindUndoWithoutRewind(t *testing.T) {
	mgr, fs := newTestManager(t)
	newRewindState(t, mgr, fs, "s1", userMsgs("hello"))

	if _, ok := mgr.RewindUndoAvailable("s1"); ok {
		t.Fatalf("no rewind, no undo")
	}
	if _, err := undoCurrentRewind(mgr, "s1"); !errors.Is(err, ErrRewindUndoUnavailable) {
		t.Fatalf("want ErrRewindUndoUnavailable, got %v", err)
	}
}

func TestRewindUndoPersistsTheRestoredHistory(t *testing.T) {
	mgr, fs := newTestManager(t)
	original := userMsgs("hello", "world")
	newRewindState(t, mgr, fs, "s1", original)

	if _, err := mgr.RewindSession("s1", 1); err != nil {
		t.Fatalf("RewindSession: %v", err)
	}
	if _, err := undoCurrentRewind(mgr, "s1"); err != nil {
		t.Fatalf("UndoRewind: %v", err)
	}
	snap, err := fs.ReadSnapshot("s1")
	if err != nil {
		t.Fatalf("ReadSnapshot: %v", err)
	}
	sameContents(t, snap.Messages, original)
}

func TestRewindUndoClearsThePermissionPromptOfTheEditedTurn(t *testing.T) {
	mgr, fs := newTestManager(t)
	st := newRewindState(t, mgr, fs, "s1", userMsgs("hello", "world"))
	dir := st.GetPersistedSessionDir()

	if _, err := mgr.RewindSession("s1", 1); err != nil {
		t.Fatalf("RewindSession: %v", err)
	}
	// The edited turn stopped on a permission prompt and was cancelled there.
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "edited"})
	st.AddMessage(llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "call_wait", Name: "write"}}})
	if err := WritePendingPermission(dir, acp.PermissionRequestParams{
		SessionID: "s1",
		ToolCall:  acp.PermissionToolCall{ToolCallID: "call_wait"},
	}, "write", "{}"); err != nil {
		t.Fatalf("pending permission: %v", err)
	}

	if _, err := undoCurrentRewind(mgr, "s1"); err != nil {
		t.Fatalf("UndoRewind: %v", err)
	}
	if PendingPermissionHeld(dir) {
		t.Fatalf("the prompt of a call that left the history must be cleared")
	}
}

func TestStaleRewindUndoIsRetired(t *testing.T) {
	mgr, fs := newTestManager(t)
	st := newRewindState(t, mgr, fs, "s1", userMsgs("hello", "world"))
	dir := st.GetPersistedSessionDir()
	undoDir := filepath.Join(dir, rewindUndoDirName)

	if _, err := mgr.RewindSession("s1", 1); err != nil {
		t.Fatalf("RewindSession: %v", err)
	}
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "edited"})
	st.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: "ok"})
	// The edited turn alone keeps the snapshot.
	retireStaleRewindUndo(st)
	if _, err := os.Stat(undoDir); err != nil {
		t.Fatalf("a valid snapshot must stay: %v", err)
	}
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "follow-up"})
	retireStaleRewindUndo(st)
	if _, err := os.Stat(undoDir); !os.IsNotExist(err) {
		t.Fatalf("a snapshot a prompt moved past must leave the bundle: %v", err)
	}
}

func TestRejectedRewindPreservesPreviousUndo(t *testing.T) {
	mgr, store := newTestManager(t)
	original := userMsgs("one", "two", "three")
	state := newRewindState(t, mgr, store, "s1", original)
	if _, err := mgr.RewindSession("s1", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.RewindSession("s1", 2); !errors.Is(err, ErrRewindOutOfRange) {
		t.Fatalf("expected out-of-range error, got %v", err)
	}
	if _, err := undoCurrentRewind(mgr, "s1"); err != nil {
		t.Fatalf("rejected rewind destroyed the prior undo: %v", err)
	}
	sameContents(t, state.GetMessages(), original)
}

// A tail that is gone or cannot be decoded is a spent record: keeping it would
// only make every later read refuse again.
func TestRewindUndoRetiresABrokenTail(t *testing.T) {
	breakTail := map[string]func(t *testing.T, tail string){
		"missing": func(t *testing.T, tail string) {
			t.Helper()
			if err := os.Remove(tail); err != nil {
				t.Fatalf("remove tail: %v", err)
			}
		},
		"corrupt": func(t *testing.T, tail string) {
			t.Helper()
			if err := os.WriteFile(tail, []byte("{not json"), 0o644); err != nil {
				t.Fatalf("write a corrupt tail: %v", err)
			}
		},
	}
	for name, breakIt := range breakTail {
		t.Run(name, func(t *testing.T) {
			mgr, fs := newTestManager(t)
			st := newRewindState(t, mgr, fs, "s1", userMsgs("hello", "world"))
			undoDir := filepath.Join(st.GetPersistedSessionDir(), rewindUndoDirName)
			if _, err := mgr.RewindSession("s1", 1); err != nil {
				t.Fatalf("RewindSession: %v", err)
			}
			breakIt(t, filepath.Join(undoDir, "tail.json"))

			if _, err := undoCurrentRewind(mgr, "s1"); !errors.Is(err, ErrRewindUndoUnavailable) {
				t.Fatalf("want ErrRewindUndoUnavailable, got %v", err)
			}
			if _, err := os.Stat(undoDir); !os.IsNotExist(err) {
				t.Fatalf("a tail that is gone or corrupt must retire the snapshot: %v", err)
			}
		})
	}
}

// A tail that exists but cannot be read right now - a sharing violation or an
// antivirus lock on Windows - is not a spent record. Retiring the snapshot over
// it would throw away the only copy of the cut history over a hiccup.
func TestRewindUndoReadFailureKeepsTheSnapshot(t *testing.T) {
	mgr, fs := newTestManager(t)
	st := newRewindState(t, mgr, fs, "s1", userMsgs("hello", "world"))
	undoDir := filepath.Join(st.GetPersistedSessionDir(), rewindUndoDirName)
	if _, err := mgr.RewindSession("s1", 1); err != nil {
		t.Fatalf("RewindSession: %v", err)
	}
	tail := filepath.Join(undoDir, "tail.json")
	if err := os.Remove(tail); err != nil {
		t.Fatalf("remove tail: %v", err)
	}
	if err := os.Mkdir(tail, 0o755); err != nil {
		t.Fatalf("stand in the way of the read: %v", err)
	}

	_, err := undoCurrentRewind(mgr, "s1")
	if errors.Is(err, ErrRewindUndoUnavailable) {
		t.Fatalf("a tail that only failed to read must stay on offer, got %v", err)
	}
	if err == nil {
		t.Fatal("an unreadable tail cannot be restored")
	}
	if _, statErr := os.Stat(undoDir); statErr != nil {
		t.Fatalf("an unreadable tail must not retire the snapshot: %v", statErr)
	}
}

// The cut detail is parked for the undo, never deleted on the way. An extra
// folder under tool_calls/ is harmless; losing the only copy is not.
func TestRewindCleanupKeepsCutDetailWhenTheParkFails(t *testing.T) {
	mgr, fs := newTestManager(t)
	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "ask"},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "call_cut", Name: "write"}}},
		{Role: llm.RoleTool, ToolCallID: "call_cut", Content: "out"},
	}
	st := newRewindState(t, mgr, fs, "s1", msgs)
	dir := st.GetPersistedSessionDir()
	if err := WriteToolCallResult(dir, "call_cut", "full call_cut"); err != nil {
		t.Fatalf("tool call: %v", err)
	}
	// A park the move cannot reach: the parent of the keep folder is a file.
	blocked := filepath.Join(dir, "blocked")
	if err := os.WriteFile(blocked, []byte("x"), 0o644); err != nil {
		t.Fatalf("blocker: %v", err)
	}

	rewindCleanupArtifacts(dir, []llm.Message{{Role: llm.RoleUser, Content: "ask"}}, filepath.Join(blocked, "tool_calls"))

	dirs, err := ListToolCalls(dir)
	if err != nil {
		t.Fatalf("ListToolCalls: %v", err)
	}
	if len(dirs) != 1 || dirs[0] != ToolCallDirName("call_cut") {
		t.Fatalf("a park that failed must leave the cut detail in tool_calls/, got %v", dirs)
	}
}

// A turn marker only proves that some watcher wrote one. When the turn runs in
// this process and no watcher of ours holds that marker, the process that wrote
// it died mid-turn and nothing will ever read a cancel: refusing now is the
// answer the caller would get only after polling for a stop that never comes.
func TestRewindUndoCancelRefusesAStaleTurnMarker(t *testing.T) {
	mgr, fs := newTestManager(t)
	st := newRewindState(t, mgr, fs, "s1", userMsgs("hello", "world"))
	dir := st.GetPersistedSessionDir()
	undoDir := filepath.Join(dir, rewindUndoDirName)
	if _, err := mgr.RewindSession("s1", 1); err != nil {
		t.Fatalf("RewindSession: %v", err)
	}
	snap := readRewindUndoSnapshot(dir)
	if snap == nil {
		t.Fatal("no snapshot after the rewind")
	}
	if err := writeJSONAtomic(filepath.Join(undoDir, "turn.json"), rewindUndoTurn{SnapshotID: snap.snapshotID(), TurnID: "gone"}); err != nil {
		t.Fatalf("stale marker: %v", err)
	}
	release := mgr.markTurnActive("s1")
	defer release()

	if err := mgr.RequestRewindUndoCancel("s1", snap.snapshotID()); !errors.Is(err, ErrSessionTurnActive) {
		t.Fatalf("a marker nobody answers must refuse at once, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(undoDir, "turn.json")); !os.IsNotExist(statErr) {
		t.Fatalf("the stale marker must be taken down, got %v", statErr)
	}
}

// The other half of the refusal: a marker its watcher is still standing behind
// is answered, and is left where the watcher can find it.
func TestRewindUndoCancelReachesALiveWatcher(t *testing.T) {
	mgr, fs := newTestManager(t)
	st := newRewindState(t, mgr, fs, "s1", userMsgs("hello", "world"))
	dir := st.GetPersistedSessionDir()
	undoDir := filepath.Join(dir, rewindUndoDirName)
	if _, err := mgr.RewindSession("s1", 1); err != nil {
		t.Fatalf("RewindSession: %v", err)
	}
	snap := readRewindUndoSnapshot(dir)
	if snap == nil {
		t.Fatal("no snapshot after the rewind")
	}
	ctx, cancel := context.WithCancel(context.Background())
	finish := watchRewindUndoCancel(ctx, st, cancel)
	t.Cleanup(func() { cancel(); finish() })

	release := mgr.markTurnActive("s1")
	defer release()

	if err := mgr.RequestRewindUndoCancel("s1", snap.snapshotID()); err != nil {
		t.Fatalf("a live watcher must be reached, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(undoDir, "turn.json")); statErr != nil {
		t.Fatalf("a live marker must stay where its watcher reads it: %v", statErr)
	}
}
