package session_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// twoProcesses is one sessions root served by two Managers with stores of their
// own: an editor panel's `foxxycode http` and the Telegram gateway that
// /resume'd the same session, as far as the bundle on disk can tell.
type twoProcesses struct {
	root, cwd string
	a, b      *session.Manager
	mu        sync.Mutex
	// seen is how many messages each turn found in its session when it began.
	seen map[string]int
}

func newTwoProcesses(t *testing.T) *twoProcesses {
	t.Helper()
	p := &twoProcesses{root: t.TempDir(), cwd: t.TempDir(), seen: map[string]int{}}
	runner := func(name string) session.AgentRunner {
		return func(_ context.Context, st *session.State, prompt []acp.ContentBlock, _ acp.UpdateSender) (string, error) {
			text := prompt[0].Text
			p.mu.Lock()
			p.seen[text] = len(st.GetMessages())
			p.mu.Unlock()
			st.AddMessage(llm.Message{Role: llm.RoleUser, Content: text})
			st.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: name + ": " + text})
			return string(acp.StopReasonEndTurn), nil
		}
	}
	p.a = session.NewManager(testConfig(), noopSender{}, runner("a"), slog.Default(), p.cwd, &session.FileStore{Root: p.root})
	p.b = session.NewManager(testConfig(), noopSender{}, runner("b"), slog.Default(), p.cwd, &session.FileStore{Root: p.root})
	return p
}

func (p *twoProcesses) prompt(t *testing.T, m *session.Manager, id, text string) {
	t.Helper()
	_, err := m.HandleSessionPromptWithSender(context.Background(), acp.SessionPromptParams{
		SessionID: id,
		Prompt:    []acp.ContentBlock{{Type: acp.ContentTypeText, Text: text}},
	}, noopSender{}, nil)
	if err != nil {
		t.Fatalf("prompt %q: %v", text, err)
	}
}

func (p *twoProcesses) seenAtStart(text string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.seen[text]
}

// onDisk returns the contents of the messages.json of a session, in order.
func onDisk(t *testing.T, root, id string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, id, session.MessagesFileName))
	if err != nil {
		t.Fatal(err)
	}
	var wrap struct {
		Messages []llm.Message `json:"messages"`
	}
	if err := json.Unmarshal(b, &wrap); err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(wrap.Messages))
	for _, m := range wrap.Messages {
		out = append(out, m.Content)
	}
	return out
}

// startShared opens a session in a, runs one turn there, and loads it into b.
func (p *twoProcesses) startShared(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	res, err := p.a.HandleSessionNew(ctx, acp.SessionNewParams{CWD: p.cwd})
	if err != nil {
		t.Fatal(err)
	}
	id := res.SessionID
	p.prompt(t, p.a, id, "one")
	if _, err := p.b.EnsureHTTPSession(ctx, id, p.cwd); err != nil {
		t.Fatal(err)
	}
	return id
}

// A turn in one process re-reads the history the other process added since it
// loaded the session, instead of answering on its stale copy and writing that
// back over the other process's turn.
func TestTurnRereadsASessionAnotherProcessChanged(t *testing.T) {
	p := newTwoProcesses(t)
	id := p.startShared(t)
	liveB, err := p.b.EnsureHTTPSession(context.Background(), id, p.cwd)
	if err != nil {
		t.Fatal(err)
	}

	p.prompt(t, p.a, id, "two") // a: one, a: one, two, a: two
	p.prompt(t, p.b, id, "three")

	if got := p.seenAtStart("three"); got != 4 {
		t.Fatalf("b began its turn on %d messages, want the 4 a had written", got)
	}
	want := []string{"one", "a: one", "two", "a: two", "three", "b: three"}
	if got := onDisk(t, p.root, id); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("messages.json = %q, want %q", got, want)
	}
	again, err := p.b.EnsureHTTPSession(context.Background(), id, p.cwd)
	if err != nil {
		t.Fatal(err)
	}
	if again != liveB {
		t.Fatal("the refresh replaced the live State; the turn, its MCP clients and its sender hold the old one")
	}

	// And back the other way: a's next turn sees b's.
	p.prompt(t, p.a, id, "four")
	if got := p.seenAtStart("four"); got != 6 {
		t.Fatalf("a began its turn on %d messages, want the 6 on disk", got)
	}
}

// A save of a session that changed nothing in its history - a mode switch in
// the panel that has not run a turn since the other process did - must not
// put its older history back on disk.
func TestStaleSaveLeavesTheNewerHistoryOnDisk(t *testing.T) {
	p := newTwoProcesses(t)
	id := p.startShared(t)

	p.prompt(t, p.a, id, "two")
	if err := p.b.HandleSessionSetMode(context.Background(), acp.SessionSetModeParams{SessionID: id, ModeID: "plan"}); err != nil {
		t.Fatal(err)
	}

	want := []string{"one", "a: one", "two", "a: two"}
	if got := onDisk(t, p.root, id); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("a stale save rewrote messages.json to %q, want %q", got, want)
	}
	meta, err := (&session.FileStore{Root: p.root}).ReadMeta(id)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Mode != "plan" {
		t.Fatalf("the stale save still owes the meta its change: mode = %q", meta.Mode)
	}
}

// A history the caller replaced in memory before the turn - the direct
// /v1/chat/completions path hands the client's conversation over that way - is
// the one the turn must run on, even when the file moved underneath it.
func TestTurnKeepsAHistoryReplacedInMemory(t *testing.T) {
	p := newTwoProcesses(t)
	id := p.startShared(t)
	liveB, err := p.b.EnsureHTTPSession(context.Background(), id, p.cwd)
	if err != nil {
		t.Fatal(err)
	}

	p.prompt(t, p.a, id, "two")
	liveB.ReplaceMessagesWithoutPersist([]llm.Message{{Role: llm.RoleUser, Content: "client prefix"}})
	p.prompt(t, p.b, id, "three")

	if got := p.seenAtStart("three"); got != 1 {
		t.Fatalf("b began its turn on %d messages, want the 1 its caller put there", got)
	}
}

// An older build without these guards writes its stale copy back over the
// bundle. That copy is a prefix of the history this process holds, so the turn
// keeps its own history - and its save puts it back - instead of rolling the
// conversation back to what the older writer knew.
func TestTurnKeepsItsHistoryOverAnOlderCopyOnDisk(t *testing.T) {
	p := newTwoProcesses(t)
	id := p.startShared(t)
	p.prompt(t, p.a, id, "two")

	msgPath := filepath.Join(p.root, id, session.MessagesFileName)
	older, err := json.Marshal(map[string]any{
		"version":  1,
		"messages": []llm.Message{{Role: llm.RoleUser, Content: "one"}, {Role: llm.RoleAssistant, Content: "a: one"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(msgPath, older, 0o644); err != nil {
		t.Fatal(err)
	}

	p.prompt(t, p.a, id, "three")
	if got := p.seenAtStart("three"); got != 4 {
		t.Fatalf("a began its turn on %d messages, want its own 4 over the older copy", got)
	}
	want := []string{"one", "a: one", "two", "a: two", "three", "a: three"}
	if got := onDisk(t, p.root, id); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("messages.json = %q, want %q", got, want)
	}
}
