package session

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/llm"
)

func TestFileStoreRoundTripUILog(t *testing.T) {
	root := t.TempDir()
	fs := &FileStore{Root: root}

	id := "sess_ulog"
	dir, err := fs.EnsureLayout(id)
	if err != nil {
		t.Fatal(err)
	}

	st := &State{
		ID:         id,
		CWD:        "/tmp/unit",
		Mode:       ModeAgent,
		SessionDir: dir,
	}
	st.AppendUILogError(1, "context exceeded")

	if err := fs.Save(st); err != nil {
		t.Fatal(err)
	}
	snap, err := fs.ReadSnapshot(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.UILog) != 1 {
		t.Fatalf("ui log len=%d", len(snap.UILog))
	}
	if snap.UILog[0].Message != "context exceeded" || snap.UILog[0].UserTurnIndex != 1 {
		t.Fatalf("entry %+v", snap.UILog[0])
	}
}

func TestFileStoreRoundTripMessages(t *testing.T) {
	root := t.TempDir()
	fs := &FileStore{Root: root}

	id := "sess_unit"
	dir, err := fs.EnsureLayout(id)
	if err != nil {
		t.Fatal(err)
	}

	st := &State{
		ID:         id,
		CWD:        "/tmp/unit",
		Mode:       ModeAgent,
		SessionDir: dir,
	}
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "hi"})
	st.AddMessage(llm.Message{
		Role:                llm.RoleAssistant,
		Content:             "hello",
		Reasoning:           "step one",
		ReasoningDurationMs: 42,
	})

	if err := fs.Save(st); err != nil {
		t.Fatal(err)
	}

	snap, err := fs.ReadSnapshot(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Messages) != 2 {
		t.Fatalf("messages roundtrip len=%d", len(snap.Messages))
	}
	if snap.Messages[1].Role != llm.RoleAssistant {
		t.Fatalf("second role %+v", snap.Messages[1].Role)
	}
	if snap.Messages[1].Reasoning != "step one" || snap.Messages[1].ReasoningDurationMs != 42 {
		t.Fatalf("reasoning roundtrip %+v", snap.Messages[1])
	}
}

func TestActiveTodoPersistence(t *testing.T) {
	root := t.TempDir()
	fs := &FileStore{Root: root}

	id := "sess_td"
	dir, err := fs.EnsureLayout(id)
	if err != nil {
		t.Fatal(err)
	}

	st := &State{
		ID:         id,
		CWD:        "/tmp",
		Mode:       ModeAgent,
		SessionDir: dir,
	}
	st.SetPlanWithoutPersist([]acp.PlanEntry{
		{Content: "a", Status: "pending"},
		{Content: "b", Status: "completed"},
	})

	if err := fs.Save(st); err != nil {
		t.Fatal(err)
	}
	snap, err := fs.ReadSnapshot(st.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Plan) != 2 {
		t.Fatalf("plan len=%d", len(snap.Plan))
	}
}

func TestListSnapshotsSkipsMarkedSchedulerRunsByMeta(t *testing.T) {
	root := t.TempDir()
	fs := &FileStore{Root: root}

	visibleID := "conv_normal"
	dirV, err := fs.EnsureLayout(visibleID)
	if err != nil {
		t.Fatal(err)
	}
	v := &State{ID: visibleID, CWD: "/tmp", Mode: ModeAgent, SessionDir: dirV}
	v.AddMessage(llm.Message{Role: llm.RoleUser, Content: "hi"})
	if err := fs.Save(v); err != nil {
		t.Fatal(err)
	}

	metaOnlyID := "custom_no_prefix"
	dirM, err := fs.EnsureLayout(metaOnlyID)
	if err != nil {
		t.Fatal(err)
	}
	m := &State{ID: metaOnlyID, CWD: "/tmp", Mode: ModeAgent, SessionDir: dirM}
	m.SetSchedulerRunMeta("job_a", time.Now().UTC().Format(time.RFC3339))
	m.AddMessage(llm.Message{Role: llm.RoleUser, Content: "cron"})
	if err := fs.Save(m); err != nil {
		t.Fatal(err)
	}

	rowsDefault, err := fs.ListSnapshots("", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rowsDefault) != 1 || rowsDefault[0].SessionID != visibleID {
		t.Fatalf("composer list: %+v", rowsDefault)
	}
	all, err := fs.ListSnapshots("", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("want 2 sessions with include scheduler, got %d", len(all))
	}
}

func TestListSnapshotsSkipsSchedulerSessions(t *testing.T) {
	root := t.TempDir()
	fs := &FileStore{Root: root}

	// Normal persisted session.
	normalID := "sess_normal"
	normalDir, err := fs.EnsureLayout(normalID)
	if err != nil {
		t.Fatal(err)
	}
	normal := &State{
		ID:         normalID,
		CWD:        "/tmp/unit",
		Mode:       ModeAgent,
		SessionDir: normalDir,
	}
	normal.AddMessage(llm.Message{Role: llm.RoleUser, Content: "hello"})
	if err := fs.Save(normal); err != nil {
		t.Fatal(err)
	}

	// Scheduler-like session id prefix.
	schedID := "sched_deadbeef"
	schedDir, err := fs.EnsureLayout(schedID)
	if err != nil {
		t.Fatal(err)
	}
	sched := &State{
		ID:         schedID,
		CWD:        "/tmp/unit",
		Mode:       ModeAgent,
		SessionDir: schedDir,
	}
	sched.AddMessage(llm.Message{Role: llm.RoleUser, Content: "ignore me"})
	if err := fs.Save(sched); err != nil {
		t.Fatal(err)
	}

	rows, err := fs.ListSnapshots("", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 visible snapshot, got %d", len(rows))
	}
	if rows[0].SessionID != normalID {
		t.Fatalf("expected %s, got %s", normalID, rows[0].SessionID)
	}
}

func TestTitleAutoPersistenceAndPrecedence(t *testing.T) {
	root := t.TempDir()
	fs := &FileStore{Root: root}

	id := "sess_title_auto"
	dir, err := fs.EnsureLayout(id)
	if err != nil {
		t.Fatal(err)
	}
	st := &State{ID: id, CWD: "/tmp", Mode: ModeAgent, SessionDir: dir}
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "first user message text here"})
	st.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: "assistant reply"})

	// With no pin, the auto-title takes precedence over the first-message derived title.
	st.SetTitleAuto("Refactoring the parser")
	if err := fs.Save(st); err != nil {
		t.Fatal(err)
	}
	snap, err := fs.ReadSnapshot(id)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Meta.TitleAuto != "Refactoring the parser" {
		t.Errorf("TitleAuto not persisted: %q", snap.Meta.TitleAuto)
	}
	if snap.Meta.Title != "Refactoring the parser" {
		t.Errorf("resolved Title should equal auto-title, got %q", snap.Meta.Title)
	}

	// A user pin overrides the auto-title in the resolved Title.
	st.SetTitlePinned("My pinned title")
	if err := fs.Save(st); err != nil {
		t.Fatal(err)
	}
	snap, err = fs.ReadSnapshot(id)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Meta.Title != "My pinned title" {
		t.Errorf("pin should win over auto-title, got %q", snap.Meta.Title)
	}
	if snap.Meta.TitleAuto != "Refactoring the parser" {
		t.Errorf("auto-title should still round-trip alongside pin, got %q", snap.Meta.TitleAuto)
	}
}

func TestDerivedTitleStripsInjectedContextBlocks(t *testing.T) {
	root := t.TempDir()
	fs := &FileStore{Root: root}

	id := "sess_derived_title"
	dir, err := fs.EnsureLayout(id)
	if err != nil {
		t.Fatal(err)
	}
	// No pin and no auto-title: the resolved title falls back to the first user message, which
	// carries the agent-injected environment blocks. Those must be stripped, leaving only "тест".
	st := &State{ID: id, CWD: "/tmp", Mode: ModeAgent, SessionDir: dir}
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "тест\n\n<foxxycode_ide_context>\n# Active File\nsites/all/modules/foo.php\n</foxxycode_ide_context>"})
	st.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: "ok"})

	if err := fs.Save(st); err != nil {
		t.Fatal(err)
	}
	snap, err := fs.ReadSnapshot(id)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Meta.Title != "тест" {
		t.Errorf("derived title should strip injected context, got %q", snap.Meta.Title)
	}
}

func TestDerivedTitleStripsAttachmentBlocks(t *testing.T) {
	root := t.TempDir()
	fs := &FileStore{Root: root}

	id := "sess_derived_title_att"
	dir, err := fs.EnsureLayout(id)
	if err != nil {
		t.Fatal(err)
	}
	// A hydrated @-mention turn carries a <foxxycode_attachment> block with the file body;
	// the derived title must show only the user's text.
	st := &State{ID: id, CWD: "/tmp", Mode: ModeAgent, SessionDir: dir}
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "@Dockerfile:21-31 why slow?\n\n<foxxycode_attachment path=\"Dockerfile\" name=\"Dockerfile\" lines=\"21-31\">\n<![CDATA[FROM x]]>\n</foxxycode_attachment>"})
	st.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: "ok"})

	if err := fs.Save(st); err != nil {
		t.Fatal(err)
	}
	snap, err := fs.ReadSnapshot(id)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Meta.Title != "@Dockerfile:21-31 why slow?" {
		t.Errorf("derived title should strip attachment blocks, got %q", snap.Meta.Title)
	}
}

func TestStripInjectedContextBlocks(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "just a title", "just a title"},
		{"ide_context", "тест\n\n<foxxycode_ide_context>\n# Active File\nfoo.php\n</foxxycode_ide_context>", "тест\n\n"},
		{"terminal_output attrs", "run this <foxxycode_terminal_output name=\"zsh\">$ ls\n</foxxycode_terminal_output> please", "run this  please"},
		{"unterminated", "hello <foxxycode_ide_context>\n# Active File", "hello "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := StripInjectedContextBlocks(tc.in); got != tc.want {
				t.Fatalf("StripInjectedContextBlocks(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// A transcript rendered for a human drops the ambient editor state but keeps the
// blocks that record something the user did, so the stripper has to be able to
// remove a subset rather than all wrappers.
func TestStripContextBlocksRemovesOnlyTheNamedTags(t *testing.T) {
	in := "вопрос" +
		"\n<foxxycode_ide_context>\n# Active File\nfoo.go\n</foxxycode_ide_context>" +
		"\n<foxxycode_terminal_context>\n# Active Terminal: zsh\n</foxxycode_terminal_context>" +
		"\n<foxxycode_session_assets>\n- /tmp/a.png (a.png)\n</foxxycode_session_assets>" +
		"\n<foxxycode_terminal_output name=\"zsh\">$ ls\n</foxxycode_terminal_output>"

	got := StripContextBlocks(in, TagIDEContext, TagTerminalContext)

	for _, gone := range []string{"Active File", "Active Terminal", TagIDEContext, TagTerminalContext} {
		if strings.Contains(got, gone) {
			t.Errorf("StripContextBlocks kept %q: %q", gone, got)
		}
	}
	for _, kept := range []string{"вопрос", TagSessionAssets, "a.png", TagTerminalOutput, "$ ls"} {
		if !strings.Contains(got, kept) {
			t.Errorf("StripContextBlocks dropped %q: %q", kept, got)
		}
	}
}

func TestStripContextBlocksWithNoTagsIsIdentity(t *testing.T) {
	in := "text <foxxycode_ide_context>x</foxxycode_ide_context>"
	if got := StripContextBlocks(in); got != in {
		t.Fatalf("StripContextBlocks(%q) = %q, want it unchanged", in, got)
	}
}

func TestFilterSnapshotListForSearchMatchesFirstUserNotTitle(t *testing.T) {
	root := t.TempDir()
	fs := &FileStore{Root: root}

	makeSess := func(id, title, firstUserContent string, prefixRoles []llm.Message) error {
		dir, err := fs.EnsureLayout(id)
		if err != nil {
			return err
		}
		st := &State{
			ID:         id,
			CWD:        "/tmp",
			Mode:       ModeAgent,
			SessionDir: dir,
		}
		st.SetTitlePinned(title)
		msgs := append(append([]llm.Message{}, prefixRoles...), llm.Message{Role: llm.RoleUser, Content: firstUserContent})
		for _, m := range msgs {
			st.AddMessage(m)
		}
		return fs.Save(st)
	}

	if err := makeSess("sess_a", "Alpha topic", "unique zebra finder", nil); err != nil {
		t.Fatal(err)
	}
	if err := makeSess("sess_b", "Other", "nothing special", nil); err != nil {
		t.Fatal(err)
	}

	rows, err := fs.ListSnapshots("", false)
	if err != nil {
		t.Fatal(err)
	}
	filtered, err := fs.FilterSnapshotListForSearch(rows, "zebra")
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || filtered[0].SessionID != "sess_a" {
		t.Fatalf("want sess_a only, got %+v", filtered)
	}
}

func TestFilterSnapshotListAssistantPrefixNoFirstUserSkippedForMessageMatch(t *testing.T) {
	root := t.TempDir()
	fs := &FileStore{Root: root}

	id := "sess_assist_only"
	dir, err := fs.EnsureLayout(id)
	if err != nil {
		t.Fatal(err)
	}
	st := &State{
		ID:         id,
		CWD:        "/tmp",
		Mode:       ModeAgent,
		SessionDir: dir,
	}
	st.SetTitlePinned("gamma title plain")
	st.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: "hidden needle in assistant"})
	if err := fs.Save(st); err != nil {
		t.Fatal(err)
	}

	rows, err := fs.ListSnapshots("", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{"needle", "hidden"} {
		filtered, err := fs.FilterSnapshotListForSearch(rows, needle)
		if err != nil {
			t.Fatal(err)
		}
		if len(filtered) != 0 {
			t.Fatalf("q=%q expected no rows (no user message); got %+v", needle, filtered)
		}
	}
	filteredGamma, err := fs.FilterSnapshotListForSearch(rows, "gamma")
	if err != nil {
		t.Fatal(err)
	}
	if len(filteredGamma) != 1 || filteredGamma[0].SessionID != id {
		t.Fatalf("title match gamma: %+v", filteredGamma)
	}
}

func TestFilterSnapshotFirstUserAfterSystemIgnoredForSecondUser(t *testing.T) {
	root := t.TempDir()
	fs := &FileStore{Root: root}

	id := "sess_order"
	dir, err := fs.EnsureLayout(id)
	if err != nil {
		t.Fatal(err)
	}
	st := &State{
		ID:         id,
		CWD:        "/tmp",
		Mode:       ModeAgent,
		SessionDir: dir,
	}
	st.SetTitlePinned("")
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "first hello"})
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "second unique xyzzy"})
	if err := fs.Save(st); err != nil {
		t.Fatal(err)
	}
	rows, err := fs.ListSnapshots("", false)
	if err != nil {
		t.Fatal(err)
	}
	got, err := fs.FilterSnapshotListForSearch(rows, "xyzzy")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("second user must not match, got %+v", got)
	}
	got2, err := fs.FilterSnapshotListForSearch(rows, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if len(got2) != 1 {
		t.Fatalf("first user hello: %+v", got2)
	}
}

func TestSavePreservesUpdatedAtWhenMessagesAndActivitySeqUnchanged(t *testing.T) {
	root := t.TempDir()
	fs := &FileStore{Root: root}
	id := "sess_preserve_ut"
	dir, err := fs.EnsureLayout(id)
	if err != nil {
		t.Fatal(err)
	}
	st := &State{
		ID:         id,
		CWD:        "/tmp",
		Mode:       ModeAgent,
		SessionDir: dir,
	}
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "hi"})
	st.RestoreActivityFromSnapshot(1, 0)
	if err := fs.Save(st); err != nil {
		t.Fatal(err)
	}
	snap1, err := fs.ReadSnapshot(id)
	if err != nil {
		t.Fatal(err)
	}
	ut1 := snap1.Meta.UpdatedAt
	if ut1 == "" {
		t.Fatal("expected updatedAt after first save")
	}
	if snap1.Meta.ActivitySeq != 1 || snap1.Meta.ReadActivitySeq != 0 {
		t.Fatalf("meta %+v", snap1.Meta)
	}
	st.MarkActivityReadSynced()
	if err := fs.PatchSessionMetaActivitySync(st); err != nil {
		t.Fatal(err)
	}
	snap2, err := fs.ReadSnapshot(id)
	if err != nil {
		t.Fatal(err)
	}
	if snap2.Meta.ReadActivitySeq != 1 {
		t.Fatalf("read seq %+v", snap2.Meta)
	}
	if snap2.Meta.UpdatedAt != ut1 {
		t.Fatalf("mark read only: updatedAt changed from %q to %q", ut1, snap2.Meta.UpdatedAt)
	}
	time.Sleep(1200 * time.Millisecond)
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "bye"})
	if err := fs.Save(st); err != nil {
		t.Fatal(err)
	}
	snap3, err := fs.ReadSnapshot(id)
	if err != nil {
		t.Fatal(err)
	}
	if snap3.Meta.UpdatedAt == ut1 {
		t.Fatal("expected updatedAt to change after new user message")
	}
}

func TestConcurrentPatchSessionMetaActivitySync(t *testing.T) {
	root := t.TempDir()
	fs := &FileStore{Root: root}
	id := "sess_concur_meta"
	dir, err := fs.EnsureLayout(id)
	if err != nil {
		t.Fatal(err)
	}
	st := &State{
		ID:         id,
		CWD:        "/tmp",
		Mode:       ModeAgent,
		SessionDir: dir,
	}
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "hi"})
	st.RestoreActivityFromSnapshot(10, 0)
	if err := fs.Save(st); err != nil {
		t.Fatal(err)
	}
	const n = 64
	var wg sync.WaitGroup
	wg.Add(n)
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		go func(k int) {
			defer wg.Done()
			stLocal := &State{
				ID:         id,
				CWD:        "/tmp",
				Mode:       ModeAgent,
				SessionDir: dir,
			}
			stLocal.RestoreActivityFromSnapshot(uint64(10+k), uint64(k))
			if err := fs.PatchSessionMetaActivitySync(stLocal); err != nil {
				errCh <- fmt.Errorf("k=%d: %w", k, err)
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
	snap, err := fs.ReadSnapshot(id)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Meta.ActivitySeq < 10 {
		t.Fatalf("activitySeq=%d", snap.Meta.ActivitySeq)
	}
}

func TestDeriveSessionTitleStripsAttachmentBlocks(t *testing.T) {
	st := &State{ID: "sess_title_att", CWD: "/tmp", Mode: ModeAgent}
	st.AddMessage(llm.Message{
		Role: llm.RoleUser,
		Content: "@Dockerfile:21-31 почему медленно?\n\n" +
			"<foxxycode_attachment path=\"Dockerfile\" name=\"Dockerfile\" lines=\"21-31\">\n" +
			"<![CDATA[RUN go mod download]]>\n</foxxycode_attachment>",
	})
	if got := deriveSessionTitle(st); got != "@Dockerfile:21-31 почему медленно?" {
		t.Fatalf("got %q", got)
	}
}

// A file body may itself contain the closing tag; the CDATA-aware scan must not
// cut the block short there and leak the rest of the body into the title.
func TestStripContextBlocksIgnoresTagsInsideCDATA(t *testing.T) {
	raw := "ask\n\n<foxxycode_attachment path=\"trap.txt\" name=\"trap.txt\">\n" +
		"<![CDATA[first ]]]]><![CDATA[> then </foxxycode_attachment> SECRET]]>\n</foxxycode_attachment>\ntail"
	if got := StripContextBlocks(raw, TagAttachment); got != "ask\n\n\ntail" {
		t.Fatalf("got %q", got)
	}
}

// TestListSnapshotsMatchesWorkspaceSpelledDifferently covers the ACP
// session/list filter: the console stores the logical $PWD spelling of a
// symlinked checkout while an editor sends the physical one, and both name the
// same folder (coddy-project/coddy-agent VS Code "sees no sessions" report).
func TestListSnapshotsMatchesWorkspaceSpelledDifferently(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real", "project")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(filepath.Join(root, "real"), link); err != nil {
		t.Skipf("symlink: %v", err)
	}
	fs := &FileStore{Root: filepath.Join(root, "sessions")}
	dir, err := fs.EnsureLayout("sess_link")
	if err != nil {
		t.Fatal(err)
	}
	st := &State{ID: "sess_link", CWD: filepath.Join(link, "project"), Mode: ModeAgent, SessionDir: dir}
	if err := fs.Save(st); err != nil {
		t.Fatal(err)
	}
	for _, filter := range []string{
		real,                                 // the physical path
		real + string(filepath.Separator),    // a trailing separator
		filepath.Join(real, "..", "project"), // an unclean path
		filepath.Join(link, "project"),       // the stored spelling
	} {
		rows, err := fs.ListSnapshots(filter, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || rows[0].SessionID != "sess_link" {
			t.Fatalf("filter %q: rows = %+v, want the session stored under %q", filter, rows, st.CWD)
		}
	}
	other := filepath.Join(root, "real", "other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	rows, err := fs.ListSnapshots(other, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("filter %q: rows = %+v, want none", other, rows)
	}
}

// The creation stamp is written once, when the bundle directory appears, and is
// carried forward by every later save so the session management table can sort
// by age.
func TestCreatedAtIsStampedOnceAndSurvivesSaves(t *testing.T) {
	root := t.TempDir()
	fs := &FileStore{Root: root}
	id := "sess_created_ut"
	dir, err := fs.EnsureLayout(id)
	if err != nil {
		t.Fatal(err)
	}
	first, err := fs.ReadSnapshot(id)
	if err != nil {
		t.Fatal(err)
	}
	created := first.Meta.CreatedAt
	if created == "" {
		t.Fatal("EnsureLayout left the bundle without a createdAt")
	}

	st := &State{ID: id, CWD: "/tmp", Mode: ModeAgent, SessionDir: dir}
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "hi"})
	if err := fs.Save(st); err != nil {
		t.Fatal(err)
	}
	st.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: "hello"})
	if err := fs.Save(st); err != nil {
		t.Fatal(err)
	}
	snap, err := fs.ReadSnapshot(id)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Meta.CreatedAt != created {
		t.Fatalf("createdAt moved from %q to %q", created, snap.Meta.CreatedAt)
	}

	// Patching only the activity counters rewrites session.json; the stamp must
	// come through that path too.
	st.RestoreActivityFromSnapshot(1, 0)
	st.MarkActivityReadSynced()
	if err := fs.PatchSessionMetaActivitySync(st); err != nil {
		t.Fatal(err)
	}
	snap, err = fs.ReadSnapshot(id)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Meta.CreatedAt != created {
		t.Fatalf("activity patch dropped createdAt: %q", snap.Meta.CreatedAt)
	}
}

// A bundle written by an older build has a session.json without createdAt. The
// moment it was started is not recoverable, so a later save must leave the
// field empty rather than backdate the session to that save.
func TestSaveDoesNotInventCreatedAtForALegacyBundle(t *testing.T) {
	root := t.TempDir()
	fs := &FileStore{Root: root}
	id := "sess_legacy_ut"
	dir, err := fs.EnsureLayout(id)
	if err != nil {
		t.Fatal(err)
	}
	// Rewrite session.json the way an older build left it.
	legacy := SessionMeta{Version: sessionFileLayout, ID: id, UpdatedAt: "2020-01-01T00:00:00Z"}
	if err := writeJSONAtomic(filepath.Join(dir, sessionMetaFile), legacy); err != nil {
		t.Fatal(err)
	}

	st := &State{ID: id, CWD: "/tmp", Mode: ModeAgent, SessionDir: dir}
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "hi"})
	if err := fs.Save(st); err != nil {
		t.Fatal(err)
	}
	snap, err := fs.ReadSnapshot(id)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Meta.CreatedAt != "" {
		t.Fatalf("createdAt = %q, want empty for a legacy bundle", snap.Meta.CreatedAt)
	}
}

// The listing carries what the management table renders per row from
// session.json alone: the model override and the stamps. The transcript size
// is asked per rendered row, because the listing never parses a transcript
// (TestListSnapshotsDoesNotReadTranscripts).
func TestListSnapshotsCarriesRowStatistics(t *testing.T) {
	root := t.TempDir()
	fs := &FileStore{Root: root}
	id := "sess_rowstats_ut"
	dir, err := fs.EnsureLayout(id)
	if err != nil {
		t.Fatal(err)
	}
	st := &State{ID: id, CWD: t.TempDir(), Mode: ModeAgent, SessionDir: dir}
	st.SetSelectedModelID("openai/gpt-4o")
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "ask"})
	st.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: "answer"})
	if err := fs.Save(st); err != nil {
		t.Fatal(err)
	}
	rows, err := fs.ListSnapshotsWith(ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.Model != "openai/gpt-4o" {
		t.Fatalf("model = %q", row.Model)
	}
	if row.CreatedAt == "" {
		t.Fatal("row carries no createdAt")
	}
	count, err := fs.MessageCount(row.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("messageCount = %d, want 2", count)
	}
}

// MessageCount answers from the file as it is on disk: zero for a bundle that
// never stored a transcript or stored an empty one, an error for a file that is
// not the transcript shape, and every row counted whatever its role.
func TestMessageCountReadsTheStoredTranscript(t *testing.T) {
	root := t.TempDir()
	fs := &FileStore{Root: root}
	cases := []struct {
		name    string
		file    string // messages.json content; empty writes no file
		want    int
		wantErr bool
	}{
		{name: "no transcript", want: 0},
		{name: "null messages", file: `{"version":1,"messages":null}`, want: 0},
		{name: "empty messages", file: `{"version":1,"messages":[]}`, want: 0},
		{name: "every role", file: `{"messages":[{"role":"system","content":"s"},{"role":"user","content":"u"},{"role":"assistant","content":"a","tool_calls":[{"id":"c1"}]},{"role":"tool","content":"t"}],"version":1}`, want: 4},
		{name: "not an object", file: `[1,2]`, wantErr: true},
		{name: "truncated", file: `{"version":1,"messages":[{"role":"user"`, wantErr: true},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := fmt.Sprintf("sess_count_%02d", i)
			dir := filepath.Join(root, id)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if tc.file != "" {
				if err := os.WriteFile(filepath.Join(dir, messagesFile), []byte(tc.file), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			got, err := fs.MessageCount(id)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("count = %d, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("count = %d, want %d", got, tc.want)
			}
		})
	}
}

// Persisting a session rewrote the whole history on every state change, so the
// cost of a save followed the length of the conversation rather than what had
// moved in it. These tests hold a save to the size of the change.

// buildHistory returns n messages of a realistic shape.
func buildHistory(n int) []llm.Message {
	msgs := make([]llm.Message, 0, n)
	for i := 0; i < n; i++ {
		switch i % 3 {
		case 0:
			msgs = append(msgs, llm.Message{Role: llm.RoleUser, Content: fmt.Sprintf("question %d about the repository layout", i)})
		case 1:
			msgs = append(msgs, llm.Message{
				Role:      llm.RoleAssistant,
				Content:   fmt.Sprintf("answer %d with some detail worth persisting", i),
				ToolCalls: []llm.ToolCall{{ID: fmt.Sprintf("call_%d", i), Name: "read_file", InputJSON: `{"path":"internal/session/filesystem.go"}`}},
			})
		default:
			msgs = append(msgs, llm.Message{Role: llm.RoleTool, ToolCallID: fmt.Sprintf("call_%d", i-1), Content: strings.Repeat("file line\n", 40)})
		}
	}
	return msgs
}

func savedState(t *testing.T, id string, n int) (*FileStore, *State) {
	t.Helper()
	fs := &FileStore{Root: t.TempDir()}
	dir, err := fs.EnsureLayout(id)
	if err != nil {
		t.Fatal(err)
	}
	st := &State{ID: id, CWD: "/tmp", Mode: ModeAgent, SessionDir: dir}
	st.ReplaceMessagesWithoutPersist(buildHistory(n))
	if err := fs.Save(st); err != nil {
		t.Fatal(err)
	}
	return fs, st
}

// Appending a message must not re-encode the conversation behind it. Encoding
// a long history is slow rather than allocation-heavy (1.27 ms against 0.03 ms
// for 400 messages), so the work is asserted by how many messages the encoder
// had to touch rather than by time or allocations.
func TestAppendingEncodesOnlyTheNewMessages(t *testing.T) {
	msgs := buildHistory(400)

	whole, encoded, err := encodeMessagesFile(msgs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if encoded != 400 {
		t.Fatalf("encoding from nothing touched %d messages, want the whole history", encoded)
	}

	base, _, err := encodeMessagesFile(msgs[:398], nil)
	if err != nil {
		t.Fatal(err)
	}
	grown, encoded, err := encodeMessagesFile(msgs, &persistedMessages{count: 398, bytes: base})
	if err != nil {
		t.Fatal(err)
	}
	if encoded != 2 {
		t.Fatalf("appending two messages to a 398-message history encoded %d of them", encoded)
	}
	if !bytes.Equal(whole, grown) {
		t.Fatalf("the incremental encoding differs from the full one")
	}
}

// A history that was edited rather than extended cannot be built on, and the
// encoder must notice rather than splice onto stale bytes.
func TestAnEditedHistoryIsEncodedInFull(t *testing.T) {
	msgs := buildHistory(20)
	base, _, err := encodeMessagesFile(msgs, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Same length: there is no new tail to append, so the whole history is
	// encoded again.
	edited := append([]llm.Message(nil), msgs...)
	edited[3].Content = "rewritten"
	out, encoded, err := encodeMessagesFile(edited, &persistedMessages{count: 20, bytes: base})
	if err != nil {
		t.Fatal(err)
	}
	if encoded != 20 {
		t.Fatalf("an edited history encoded %d messages, want all of them", encoded)
	}
	want, _, err := encodeMessagesFile(edited, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, want) {
		t.Fatalf("an edited history was not encoded as a full encoding")
	}
}

// Content the previous bytes cannot be built on is refused rather than spliced
// into something malformed.
func TestSpliceRefusesContentItDoesNotRecognise(t *testing.T) {
	if _, ok := spliceMessages([]byte("{\n  \"version\": 1,\n  \"messages\": []\n}\n"), buildHistory(1)); ok {
		t.Fatalf("spliced onto an empty history instead of refusing")
	}
	if _, ok := spliceMessages([]byte("garbage"), buildHistory(1)); ok {
		t.Fatalf("spliced onto unrecognised content instead of refusing")
	}
}

// A save that changes nothing must not rewrite the history at all, and must
// leave updatedAt alone. The byte comparison this used to rely on compared a
// compact encoding against the indented file and so never matched.
func TestSaveThatChangesNothingKeepsUpdatedAtAndTheFile(t *testing.T) {
	fs, st := savedState(t, "sess_nochange", 12)
	first, err := fs.ReadSnapshot("sess_nochange")
	if err != nil {
		t.Fatal(err)
	}
	msgPath := filepath.Join(st.SessionDir, messagesFile)
	before, err := os.Stat(msgPath)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(1100 * time.Millisecond)
	if err := fs.Save(st); err != nil {
		t.Fatal(err)
	}

	second, err := fs.ReadSnapshot("sess_nochange")
	if err != nil {
		t.Fatal(err)
	}
	if second.Meta.UpdatedAt != first.Meta.UpdatedAt {
		t.Fatalf("updatedAt moved on a save that changed nothing: %q -> %q", first.Meta.UpdatedAt, second.Meta.UpdatedAt)
	}
	after, err := os.Stat(msgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("messages.json was rewritten by a save that changed nothing")
	}
	if len(second.Messages) != 12 {
		t.Fatalf("history came back with %d messages", len(second.Messages))
	}
}

// However the file is produced, it must be byte-for-byte what a plain full
// encoding would have written: the incremental path may not drift from the
// format every other reader expects.
func TestPersistedHistoryMatchesAFullEncoding(t *testing.T) {
	fs, st := savedState(t, "sess_format", 5)
	msgPath := filepath.Join(st.SessionDir, messagesFile)

	check := func(stage string) {
		t.Helper()
		onDisk, err := os.ReadFile(msgPath)
		if err != nil {
			t.Fatal(err)
		}
		want, err := json.MarshalIndent(messagesFileData{Version: messagesLayout, Messages: st.GetMessages()}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		want = append(want, '\n')
		if !bytes.Equal(onDisk, want) {
			t.Fatalf("%s: persisted history is not a full encoding\n--- on disk ---\n%s\n--- want ---\n%s", stage, onDisk, want)
		}
	}
	check("initial")

	for i := 0; i < 3; i++ {
		st.AddMessage(llm.Message{Role: llm.RoleUser, Content: fmt.Sprintf("appended %d", i)})
		if err := fs.Save(st); err != nil {
			t.Fatal(err)
		}
		check(fmt.Sprintf("append %d", i))
	}

	// An edit in the middle of the history is not an append and must still
	// land correctly.
	st.AddMessage(llm.Message{Role: llm.RoleAssistant, PlanDocument: &llm.PlanDocumentSnapshot{Slug: "plan", Name: "before"}})
	if err := fs.Save(st); err != nil {
		t.Fatal(err)
	}
	st.MarkPlanDocumentDiscarded("plan")
	if err := fs.Save(st); err != nil {
		t.Fatal(err)
	}
	check("after an in-place edit")

	// A wholesale replacement (compaction, restore) too.
	st.ReplaceMessagesWithoutPersist(buildHistory(3))
	if err := fs.Save(st); err != nil {
		t.Fatal(err)
	}
	check("after a replacement")

	// A store that never wrote this session - the next process - has nothing to
	// build on and must fall back to encoding the history in full.
	fresh := &FileStore{Root: fs.Root}
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "after a restart"})
	if err := fresh.Save(st); err != nil {
		t.Fatal(err)
	}
	check("from a store with no memory of the session")
}

// The store skips rewriting a history it believes is already on disk, so it
// must notice when that file is no longer there.
func TestSaveRewritesAHistoryThatVanishedFromDisk(t *testing.T) {
	fs, st := savedState(t, "sess_vanished", 6)
	msgPath := filepath.Join(st.SessionDir, messagesFile)
	if err := os.Remove(msgPath); err != nil {
		t.Fatal(err)
	}
	if err := fs.Save(st); err != nil {
		t.Fatal(err)
	}
	snap, err := fs.ReadSnapshot("sess_vanished")
	if err != nil {
		t.Fatalf("history was not written back: %v", err)
	}
	if len(snap.Messages) != 6 {
		t.Fatalf("history came back with %d messages", len(snap.Messages))
	}
}

// Compaction inserts a summary into the middle of the history, which rewrites
// everything after it. Persistence must encode the history afresh rather than
// treat it as a history that only grew.
func TestCompactionSummaryInsertIsPersistedInFull(t *testing.T) {
	fs, st := savedState(t, "sess_compacted", 8)
	st.InsertCompactionSummary(3, NewCompactionSummaryMessage("a summary of what came before", "test-model"))
	if err := fs.Save(st); err != nil {
		t.Fatal(err)
	}
	onDisk, err := os.ReadFile(filepath.Join(st.SessionDir, messagesFile))
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.MarshalIndent(messagesFileData{Version: messagesLayout, Messages: st.GetMessages()}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, '\n')
	if !bytes.Equal(onDisk, want) {
		t.Fatalf("a compacted history was not persisted as a full encoding")
	}
	snap, err := fs.ReadSnapshot("sess_compacted")
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Messages) != 9 || !snap.Messages[3].CompactionSummary {
		t.Fatalf("summary did not land at index 3 of %d messages", len(snap.Messages))
	}
}

// The compaction engine of this fork swaps the older turns for a summary
// through ReplaceMessagesAndPersist (internal/agent/compaction.go). The shorter
// history is what has to be on disk afterwards, and the next append builds on
// it rather than on the history the store remembered from before the swap.
func TestReplaceMessagesAndPersistWritesTheCompactedHistory(t *testing.T) {
	fs, st := savedState(t, "sess_engine_compacted", 8)
	kept := st.GetMessages()[6:]
	compacted := append([]llm.Message{NewCompactionSummaryMessage("what came before", "test-model")}, kept...)
	st.ReplaceMessagesAndPersist(compacted)
	if err := fs.Save(st); err != nil {
		t.Fatal(err)
	}
	snap, err := fs.ReadSnapshot("sess_engine_compacted")
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Messages) != 3 || !snap.Messages[0].CompactionSummary {
		t.Fatalf("disk holds %d messages after the swap, want the summary and the two kept ones", len(snap.Messages))
	}

	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "after the summary"})
	if err := fs.Save(st); err != nil {
		t.Fatal(err)
	}
	onDisk, err := os.ReadFile(filepath.Join(st.SessionDir, messagesFile))
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.MarshalIndent(messagesFileData{Version: messagesLayout, Messages: st.GetMessages()}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(onDisk, append(want, '\n')) {
		t.Fatalf("the append after a compaction did not build on the compacted history")
	}
}

// A long-lived server opens many sessions; the copy of each history the store
// keeps to splice appends onto must leave with the session, and a reopened
// session still persists correctly after it did.
func TestForgettingASessionDropsItsRememberedHistory(t *testing.T) {
	fs, st := savedState(t, "sess_forgotten", 5)
	msgPath := filepath.Join(st.SessionDir, messagesFile)
	if fs.cachedMessages(msgPath) == nil {
		t.Fatal("a save left nothing remembered to splice onto")
	}

	m := NewManager(reloadTestConfig(), nil, nil, slog.Default(), "", fs)
	m.mu.Lock()
	m.sessions[st.ID] = st
	m.mu.Unlock()
	m.ForgetLiveSession(st.ID)
	if fs.cachedMessages(msgPath) != nil {
		t.Fatal("forgetting the session kept its whole history in the store")
	}

	reopened := &State{ID: st.ID, CWD: "/tmp", Mode: ModeAgent, SessionDir: st.SessionDir}
	reopened.ReplaceMessagesWithoutPersist(st.GetMessages())
	reopened.AddMessage(llm.Message{Role: llm.RoleUser, Content: "back again"})
	if err := fs.Save(reopened); err != nil {
		t.Fatal(err)
	}
	snap, err := fs.ReadSnapshot(st.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Messages) != 6 || snap.Messages[5].Content != "back again" {
		t.Fatalf("the reopened session persisted %d messages", len(snap.Messages))
	}
}

// A session deleted while nobody had it open is not in the live map, but the
// store may still remember the history it wrote for it.
func TestDeletingASessionTreeDropsItsRememberedHistory(t *testing.T) {
	fs, st := savedState(t, "sess_deleted", 3)
	msgPath := filepath.Join(st.SessionDir, messagesFile)
	m := NewManager(reloadTestConfig(), nil, nil, slog.Default(), "", fs)
	if err := m.DeleteSessionTree(st.ID, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(st.SessionDir); !os.IsNotExist(err) {
		t.Fatalf("the bundle is still there: %v", err)
	}
	if fs.cachedMessages(msgPath) != nil {
		t.Fatal("deleting the session kept its whole history in the store")
	}
}

// The incremental encoding puts a message into the array by hand, so it has to
// agree with a full encoding on every shape a message can take: characters the
// encoder escapes, content that looks like the file's own tail, empty and
// absent fields.
func TestSplicedMessagesMatchAFullEncodingForAwkwardContent(t *testing.T) {
	cases := []struct {
		name string
		msg  llm.Message
	}{
		{"plain", llm.Message{Role: llm.RoleUser, Content: "ordinary text"}},
		{"empty content", llm.Message{Role: llm.RoleUser}},
		{"html characters", llm.Message{Role: llm.RoleUser, Content: `<script>a && b > c</script>`}},
		{"unicode", llm.Message{Role: llm.RoleUser, Content: "привет 🌍 日本語  "}},
		{"looks like the file tail", llm.Message{Role: llm.RoleUser, Content: "\n  ]\n}\n"}},
		{"looks like a whole file", llm.Message{Role: llm.RoleUser, Content: "{\n  \"version\": 1,\n  \"messages\": [\n  ]\n}\n"}},
		{"control characters", llm.Message{Role: llm.RoleUser, Content: "tab\there\r\nand a \x00 nul"}},
		{"quotes and backslashes", llm.Message{Role: llm.RoleUser, Content: `he said "\" and \\ then "x"`}},
		{"tool call", llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "c1", Name: "run", InputJSON: `{"cmd":"echo <&>"}`}}}},
		{"plan document", llm.Message{Role: llm.RoleAssistant, PlanDocument: &llm.PlanDocumentSnapshot{Slug: "s", Name: "n", Body: "b"}}},
		{"long content", llm.Message{Role: llm.RoleTool, ToolCallID: "c1", Content: strings.Repeat("a long tool result line\n", 500)}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := buildHistory(3)
			full := append(append([]llm.Message(nil), base...), tc.msg)

			prev, _, err := encodeMessagesFile(base, nil)
			if err != nil {
				t.Fatal(err)
			}
			spliced, encoded, err := encodeMessagesFile(full, &persistedMessages{count: len(base), bytes: prev})
			if err != nil {
				t.Fatal(err)
			}
			if encoded != 1 {
				t.Fatalf("encoded %d messages, want only the appended one", encoded)
			}
			want, _, err := encodeMessagesFile(full, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(spliced, want) {
				t.Fatalf("spliced output differs from a full encoding\n--- spliced ---\n%s\n--- full ---\n%s", spliced, want)
			}
			// And it must still parse back to the same history.
			var back messagesFileData
			if err := json.Unmarshal(spliced, &back); err != nil {
				t.Fatalf("spliced output does not parse: %v", err)
			}
			if len(back.Messages) != len(full) {
				t.Fatalf("parsed %d messages, want %d", len(back.Messages), len(full))
			}
			if back.Messages[len(back.Messages)-1].Content != tc.msg.Content {
				t.Fatalf("content did not survive the round trip")
			}
		})
	}
}

// Saves of one session can overlap, and they now decide what to write from a
// cache of what was written last. Whatever order they land in, the file must
// end up a valid encoding of the history, never a splice onto bytes another
// save had already replaced.
func TestConcurrentSavesLeaveAValidHistory(t *testing.T) {
	fs, st := savedState(t, "sess_concurrent", 20)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 15; j++ {
				st.AddMessage(llm.Message{Role: llm.RoleUser, Content: fmt.Sprintf("writer %d turn %d", n, j)})
				if err := fs.Save(st); err != nil {
					t.Errorf("save: %v", err)
					return
				}
			}
		}(i)
	}
	wg.Wait()

	// One last save settles the file against the final history.
	if err := fs.Save(st); err != nil {
		t.Fatal(err)
	}
	onDisk, err := os.ReadFile(filepath.Join(st.SessionDir, messagesFile))
	if err != nil {
		t.Fatal(err)
	}
	var back messagesFileData
	if err := json.Unmarshal(onDisk, &back); err != nil {
		t.Fatalf("history on disk does not parse: %v", err)
	}
	want := st.GetMessages()
	if len(back.Messages) != len(want) {
		t.Fatalf("history on disk has %d messages, state has %d", len(back.Messages), len(want))
	}
	for i := range want {
		if back.Messages[i].Content != want[i].Content {
			t.Fatalf("message %d on disk is %q, state has %q", i, back.Messages[i].Content, want[i].Content)
		}
	}
}

// Revisions start at zero in every State, so a session closed and reopened can
// reach a number its predecessor already wrote. Without an identity on the
// cache entry the store reads that as "nothing moved" and silently leaves the
// wrong history on disk (found in cross-review).
func TestASecondStateOverTheSameBundleIsNotMistakenForTheFirst(t *testing.T) {
	fs := &FileStore{Root: t.TempDir()}
	dir, err := fs.EnsureLayout("sess_collide")
	if err != nil {
		t.Fatal(err)
	}

	first := &State{ID: "sess_collide", CWD: "/tmp", Mode: ModeAgent, SessionDir: dir}
	for i := 0; i < 5; i++ {
		first.AddMessage(llm.Message{Role: llm.RoleUser, Content: "from the first state"})
	}
	if err := fs.Save(first); err != nil {
		t.Fatal(err)
	}

	// Reopened: a new State over the same bundle, counting from zero again, and
	// landing on the same revision with a different history.
	second := &State{ID: "sess_collide", CWD: "/tmp", Mode: ModeAgent, SessionDir: dir}
	second.ReplaceMessagesWithoutPersist([]llm.Message{{Role: llm.RoleUser, Content: "from the second state"}})
	for i := 0; i < 4; i++ {
		second.AddMessage(llm.Message{Role: llm.RoleUser, Content: "from the second state"})
	}
	if _, rev, _, _ := second.MessagesForPersist(); rev != 5 {
		t.Fatalf("the second state reached revision %d, the test needs the collision at 5", rev)
	}
	if err := fs.Save(second); err != nil {
		t.Fatal(err)
	}

	snap, err := fs.ReadSnapshot("sess_collide")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Messages[0].Content != "from the second state" {
		t.Fatalf("the second state's history was never written; disk holds %q", snap.Messages[0].Content)
	}
}

// A save takes its snapshot after it has the file's lock, so what it writes is
// the history as of that moment. Taking it earlier let a save that started
// first write an older history over a newer one (found in cross-review).
func TestASaveWritesTheHistoryAsOfTakingTheLock(t *testing.T) {
	fs, st := savedState(t, "sess_ordering", 4)
	msgPath := filepath.Join(st.SessionDir, messagesFile)

	// Hold the file's lock so the save below cannot get past it.
	mu := fs.pathMutex(msgPath)
	mu.Lock()

	saved := make(chan error, 1)
	go func() { saved <- fs.Save(st) }()

	// Let the save block on the lock, then move the history on underneath it.
	time.Sleep(50 * time.Millisecond)
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "arrived while the save was waiting"})
	mu.Unlock()

	if err := <-saved; err != nil {
		t.Fatal(err)
	}
	snap, err := fs.ReadSnapshot("sess_ordering")
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Messages) != 5 {
		t.Fatalf("history on disk has %d messages, want the 5 present when the lock was taken", len(snap.Messages))
	}
	if snap.Messages[4].Content != "arrived while the save was waiting" {
		t.Fatalf("the save wrote a history from before it held the lock")
	}
}

// Another writer over the same bundle replaces the file without this store
// hearing about it. A later save that believes nothing moved must notice that
// the file is no longer the one it wrote (found in cross-review).
func TestSaveNoticesTheHistoryWasReplacedUnderneathIt(t *testing.T) {
	fs, st := savedState(t, "sess_replaced", 7)
	msgPath := filepath.Join(st.SessionDir, messagesFile)

	// Somebody else writes a different history to the same path.
	other, _, err := encodeMessagesFile(buildHistory(2), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeBytesAtomic(msgPath, other); err != nil {
		t.Fatal(err)
	}

	// This state has not changed, so the store would otherwise write nothing.
	if err := fs.Save(st); err != nil {
		t.Fatal(err)
	}
	snap, err := fs.ReadSnapshot("sess_replaced")
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Messages) != 7 {
		t.Fatalf("history on disk has %d messages, want this session's 7 written back", len(snap.Messages))
	}
}

// Two sessions must not share the plan documents of one history: an edit in
// one would change the other without moving the revisions its persistence
// reads (found in cross-review).
func TestAdoptingAHistoryDoesNotShareItsPlanDocuments(t *testing.T) {
	source := &State{ID: "src", CWD: "/tmp", Mode: ModeAgent}
	source.ReplaceMessagesWithoutPersist([]llm.Message{{
		Role:         llm.RoleAssistant,
		PlanDocument: &llm.PlanDocumentSnapshot{Slug: "plan", Name: "original"},
	}})

	fs, _ := savedState(t, "sess_adopted", 0)
	adopted := &State{ID: "sess_adopted", CWD: "/tmp", Mode: ModeAgent, SessionDir: filepath.Join(fs.Root, "sess_adopted")}
	adopted.ReplaceMessagesWithoutPersist(source.GetMessages())
	if err := fs.Save(adopted); err != nil {
		t.Fatal(err)
	}

	source.MarkPlanDocumentDiscarded("plan")

	if got := adopted.GetMessages()[0].PlanDocument.Discarded; got {
		t.Fatalf("an edit in one session reached the other's history")
	}
	snap, err := fs.ReadSnapshot("sess_adopted")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Messages[0].PlanDocument.Discarded {
		t.Fatalf("the persisted history followed an edit made in another session")
	}
}

// A change that leaves the history exactly as it was on disk is not a change:
// updatedAt follows the content, not the bookkeeping that tracks it.
func TestAnEditThatChangesNothingLeavesUpdatedAtAlone(t *testing.T) {
	fs, st := savedState(t, "sess_idempotent", 6)
	first, err := fs.ReadSnapshot("sess_idempotent")
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(1100 * time.Millisecond)
	// An edit that rewrites the history to exactly what it already was.
	st.ReplaceMessagesWithoutPersist(st.GetMessages())
	if err := fs.Save(st); err != nil {
		t.Fatal(err)
	}

	second, err := fs.ReadSnapshot("sess_idempotent")
	if err != nil {
		t.Fatal(err)
	}
	if second.Meta.UpdatedAt != first.Meta.UpdatedAt {
		t.Fatalf("updatedAt moved for an edit that changed nothing: %q -> %q", first.Meta.UpdatedAt, second.Meta.UpdatedAt)
	}
	if len(second.Messages) != 6 {
		t.Fatalf("history came back with %d messages", len(second.Messages))
	}
}

// docs/features/sessions.md: the stamp moves when something is persisted - a
// turn, a pinned title - and listings sort by it. Preserving it must therefore
// mean "this save wrote nothing new anywhere", not merely "the history did not
// move": pinning a title changes only the meta, and the session still has to
// rise in the listing.
func TestPersistedMetaChangesMoveUpdatedAt(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*State)
	}{
		{"a pinned title", func(s *State) { s.SetTitlePinned("pinned by the operator") }},
		{"the mode", func(s *State) { s.SetMode(string(ModePlan)) }},
		{"the model override", func(s *State) { s.SetSelectedModelID("some/model") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs, st := savedState(t, "sess_meta_"+strings.ReplaceAll(tc.name, " ", "_"), 4)
			before, err := fs.ReadSnapshot(st.ID)
			if err != nil {
				t.Fatal(err)
			}
			time.Sleep(1100 * time.Millisecond)

			tc.change(st)
			if err := fs.Save(st); err != nil {
				t.Fatal(err)
			}
			after, err := fs.ReadSnapshot(st.ID)
			if err != nil {
				t.Fatal(err)
			}
			if after.Meta.UpdatedAt == before.Meta.UpdatedAt {
				t.Fatalf("%s was persisted but the session did not move in the listing", tc.name)
			}
		})
	}
}

// Opening an unread chat in History marks it read through
// PatchSessionMetaActivitySync, which leaves the stamp alone. The read counter
// is part of the meta a save now compares, so the next save that persists
// nothing else must find it already on disk and keep the chat in its place.
func TestSavingAfterMarkingReadKeepsUpdatedAt(t *testing.T) {
	fs, st := savedState(t, "sess_marked_read", 2)
	st.RestoreActivityFromSnapshot(3, 1)
	if err := fs.Save(st); err != nil {
		t.Fatal(err)
	}
	before, err := fs.ReadSnapshot(st.ID)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)

	st.MarkActivityReadSynced()
	if err := fs.PatchSessionMetaActivitySync(st); err != nil {
		t.Fatal(err)
	}
	if err := fs.Save(st); err != nil {
		t.Fatal(err)
	}
	after, err := fs.ReadSnapshot(st.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Meta.ReadActivitySeq != 3 {
		t.Fatalf("read counter on disk is %d, want 3", after.Meta.ReadActivitySeq)
	}
	if after.Meta.UpdatedAt != before.Meta.UpdatedAt {
		t.Fatalf("marking a chat read moved it in the listing: %q -> %q", before.Meta.UpdatedAt, after.Meta.UpdatedAt)
	}
}

// withHistoryCacheBudget shrinks the history cache budget for one test.
func withHistoryCacheBudget(t *testing.T, budget int64) {
	t.Helper()
	old := msgCacheBudget
	msgCacheBudget = budget
	t.Cleanup(func() { msgCacheBudget = old })
}

func saveHistory(t *testing.T, fs *FileStore, id string, n int) *State {
	t.Helper()
	dir, err := fs.EnsureLayout(id)
	if err != nil {
		t.Fatal(err)
	}
	st := &State{ID: id, CWD: "/tmp", Mode: ModeAgent, SessionDir: dir}
	st.ReplaceMessagesWithoutPersist(buildHistory(n))
	if err := fs.Save(st); err != nil {
		t.Fatal(err)
	}
	return st
}

func historyCacheUsage(fs *FileStore) (int64, int) {
	fs.msgCacheMu.Lock()
	defer fs.msgCacheMu.Unlock()
	var sum int64
	for _, e := range fs.msgCache {
		sum += int64(len(e.bytes))
	}
	if sum != fs.msgCacheBytes {
		return -1, len(fs.msgCache)
	}
	return sum, len(fs.msgCache)
}

// The store keeps the encoded history of every session it saved so an append
// can be spliced on. The HTTP server behind an editor panel never forgets a
// session, so without a bound that copy grew with every conversation opened
// over the life of the process. The cache now evicts the least recently used
// histories past its budget, and an evicted session is simply encoded in full
// on its next save.
func TestHistoryCacheStaysWithinItsBudget(t *testing.T) {
	fs := &FileStore{Root: t.TempDir()}
	probe := saveHistory(t, fs, "sess_probe", 30)
	one, err := os.ReadFile(filepath.Join(probe.SessionDir, messagesFile))
	if err != nil {
		t.Fatal(err)
	}
	fs.ForgetPersistedMessages(probe.SessionDir)
	withHistoryCacheBudget(t, int64(len(one))*3)

	var states []*State
	for i := 0; i < 6; i++ {
		states = append(states, saveHistory(t, fs, fmt.Sprintf("sess_cache_%d", i), 30))
	}
	used, entries := historyCacheUsage(fs)
	if used < 0 {
		t.Fatal("msgCacheBytes does not match the bytes the cache holds")
	}
	if used > msgCacheBudget {
		t.Fatalf("history cache holds %d bytes, budget is %d", used, msgCacheBudget)
	}
	if entries == 0 || entries >= len(states) {
		t.Fatalf("history cache has %d entries after saving %d sessions over a 3-history budget", entries, len(states))
	}
	last := states[len(states)-1]
	if fs.cachedMessages(filepath.Join(last.SessionDir, messagesFile)) == nil {
		t.Error("the most recently saved history was evicted")
	}

	evicted := states[0]
	if fs.cachedMessages(filepath.Join(evicted.SessionDir, messagesFile)) != nil {
		t.Fatal("the least recently used history is still cached")
	}
	evicted.AddMessage(llm.Message{Role: llm.RoleUser, Content: "after eviction"})
	if err := fs.Save(evicted); err != nil {
		t.Fatal(err)
	}
	onDisk, err := os.ReadFile(filepath.Join(evicted.SessionDir, messagesFile))
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.MarshalIndent(messagesFileData{Version: messagesLayout, Messages: evicted.GetMessages()}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(onDisk, append(want, '\n')) {
		t.Fatal("an evicted session did not persist as a full encoding")
	}
	if used, _ := historyCacheUsage(fs); used < 0 || used > msgCacheBudget {
		t.Fatalf("history cache holds %d bytes after re-saving an evicted session, budget is %d", used, msgCacheBudget)
	}
}

// A single history larger than the whole budget is not cached at all, and
// forgetting a session gives its bytes back.
func TestHistoryCacheAccountingOnOversizeAndForget(t *testing.T) {
	fs := &FileStore{Root: t.TempDir()}
	withHistoryCacheBudget(t, 64)
	big := saveHistory(t, fs, "sess_big", 30)
	if used, entries := historyCacheUsage(fs); used != 0 || entries != 0 {
		t.Fatalf("a history over the budget was cached: %d bytes, %d entries", used, entries)
	}
	_ = big

	withHistoryCacheBudget(t, 1<<30)
	a := saveHistory(t, fs, "sess_a", 10)
	b := saveHistory(t, fs, "sess_b", 10)
	fs.ForgetPersistedMessages(a.SessionDir)
	bBytes, err := os.ReadFile(filepath.Join(b.SessionDir, messagesFile))
	if err != nil {
		t.Fatal(err)
	}
	if used, entries := historyCacheUsage(fs); used != int64(len(bBytes)) || entries != 1 {
		t.Fatalf("after forgetting one of two histories: %d bytes, %d entries; want %d bytes, 1 entry", used, entries, len(bBytes))
	}
}
