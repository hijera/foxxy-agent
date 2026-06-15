//go:build gateway || gateway.telegram

package telegram

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/EvilFreelancer/coddy-agent/internal/acp"
)

// capturingServer records every Bot API call (endpoint + form values).
type capturingServer struct {
	mu    sync.Mutex
	calls []capturedCall
}

type capturedCall struct {
	endpoint string
	form     map[string]string
}

func (c *capturingServer) handler(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	_ = r.Body.Close()
	r.Body = io.NopCloser(strings.NewReader(string(body)))
	_ = r.ParseForm()
	form := map[string]string{}
	for k := range r.PostForm {
		form[k] = r.PostForm.Get(k)
	}
	endpoint := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	c.mu.Lock()
	c.calls = append(c.calls, capturedCall{endpoint: endpoint, form: form})
	c.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if endpoint == "sendRichMessageDraft" {
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
		return
	}
	_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"date":0,"chat":{"id":5,"type":"private"}}}`))
}

func (c *capturingServer) byEndpoint(name string) []capturedCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []capturedCall
	for _, call := range c.calls {
		if call.endpoint == name {
			out = append(out, call)
		}
	}
	return out
}

func TestSender_RichFlow_DraftsThenFinalizesWithTools(t *testing.T) {
	srvCap := &capturingServer{}
	srv := httptest.NewServer(http.HandlerFunc(srvCap.handler))
	defer srv.Close()

	s := newSender(stubBot(t, srv.URL), 5, 678, slog.Default(), richConfig{
		enabled:    true,
		allowDraft: true,
		draftID:    7,
	})

	// Stream some text, then a tool call, then more text.
	_ = s.SendSessionUpdate("sess", acp.MessageChunkUpdate{
		Content: acp.ContentBlock{Type: acp.ContentTypeText, Text: "Hello "},
	})
	_ = s.SendSessionUpdate("sess", acp.ToolCallUpdate{Title: "bash"})
	_ = s.SendSessionUpdate("sess", acp.MessageChunkUpdate{
		Content: acp.ContentBlock{Type: acp.ContentTypeText, Text: "world"},
	})
	s.Flush()

	// At least one ephemeral draft was streamed to the right draft_id.
	drafts := srvCap.byEndpoint("sendRichMessageDraft")
	if len(drafts) == 0 {
		t.Fatalf("expected at least one sendRichMessageDraft call")
	}
	if drafts[0].form["draft_id"] != "7" {
		t.Fatalf("draft_id: want 7 got %q", drafts[0].form["draft_id"])
	}

	// Exactly one persistent sendRichMessage finalized the turn.
	finals := srvCap.byEndpoint("sendRichMessage")
	if len(finals) != 1 {
		t.Fatalf("expected exactly one sendRichMessage, got %d", len(finals))
	}
	rm := finals[0].form["rich_message"]
	if !strings.Contains(rm, "Hello world") {
		t.Fatalf("final message should contain the LLM text, got: %s", rm)
	}
	if !strings.Contains(rm, "details") || !strings.Contains(rm, "bash") {
		t.Fatalf("final message should contain a tools <details> block listing bash, got: %s", rm)
	}
	// The legacy live message path must not be used in rich mode.
	if len(srvCap.byEndpoint("editMessageText")) != 0 {
		t.Fatalf("rich mode must not call editMessageText")
	}
}

func TestSender_RichGroup_NoDraftButFinalizes(t *testing.T) {
	srvCap := &capturingServer{}
	srv := httptest.NewServer(http.HandlerFunc(srvCap.handler))
	defer srv.Close()

	// Group chat: allowDraft is false (drafts are private-only).
	s := newSender(stubBot(t, srv.URL), -100, 5, slog.Default(), richConfig{
		enabled:    true,
		allowDraft: false,
		draftID:    9,
	})
	_ = s.SendSessionUpdate("sess", acp.MessageChunkUpdate{
		Content: acp.ContentBlock{Type: acp.ContentTypeText, Text: "Answer"},
	})
	s.Flush()

	if got := len(srvCap.byEndpoint("sendRichMessageDraft")); got != 0 {
		t.Fatalf("group chat must not stream drafts, got %d", got)
	}
	if got := len(srvCap.byEndpoint("sendRichMessage")); got != 1 {
		t.Fatalf("expected one sendRichMessage in group, got %d", got)
	}
}
