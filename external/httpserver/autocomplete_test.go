//go:build http

package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/ideenv"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// ---- pure text rules -------------------------------------------------------------------------

func TestDecideMultiLine(t *testing.T) {
	cases := []struct {
		name           string
		prefix, suffix string
		want           bool
	}{
		{"code right of the caret forces a single line", "foo(", ")\n}", false},
		{"end of a line that opened a block", "func f() {", "\n}", true},
		{"python block opener", "def f():", "\n", true},
		{"empty line inside a body", "func f() {\n\t", "\n}", true},
		{"middle of a statement", "\treturn ", "\n}", false},
		{"bare else keyword", "} else", "\n", true},
		{"trailing comma in a literal", "x := []int{1,", "\n}", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := decideMultiLine(tc.prefix, tc.suffix); got != tc.want {
				t.Fatalf("decideMultiLine() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLineStops(t *testing.T) {
	if got := lineStops(false, "\n}"); len(got) != 1 || got[0] != "\n" {
		t.Fatalf("single-line stops = %q", got)
	}
	got := lineStops(true, "\n\treturn nil\n}\n")
	if len(got) != 2 || got[0] != "\n\treturn nil" || got[1] != "\n\n\n" {
		t.Fatalf("block stops = %q, want the exact next suffix line then a blank-line run", got)
	}
	if got := lineStops(true, "   \n"); len(got) != 1 || got[0] != "\n\n\n" {
		t.Fatalf("block stops without a suffix line = %q", got)
	}
	// A lone closing bracket would end an unindented block on its own "}".
	if got := lineStops(true, "\n}\n\nfunc g() {}"); len(got) != 1 || got[0] != "\n\n\n" {
		t.Fatalf("block stops with a bare closer next = %q", got)
	}
	if got := capStops([]string{"a", "", "a", "b", "c", "d", "e"}, 4); strings.Join(got, ",") != "a,b,c,d" {
		t.Fatalf("capStops() = %q", got)
	}
}

func TestTrimSuffixOverlap(t *testing.T) {
	cases := []struct {
		name, out, suffix, want string
	}{
		{"closing brace already present", "a + b\n}", "\n}", "a + b"},
		{"closing paren already present", "arg)", ")", "arg"},
		{"multi-char overlap", "foo(bar", "bar)", "foo("},
		{"no overlap", "a + b", "\nreturn x", "a + b"},
		{"single letter is not an overlap", "return a", "a\n}", "return a"},
		{"differently indented closer", "x\n\t}", "\n}", "x\n\t"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := trimSuffixOverlap(tc.out, tc.suffix); got != tc.want {
				t.Fatalf("trimSuffixOverlap(%q, %q) = %q, want %q", tc.out, tc.suffix, got, tc.want)
			}
		})
	}
}

func TestTruncateBlockStopsAtDedent(t *testing.T) {
	lines := []string{"", "\t\tdo()", "\t}", "}", "", "func next() {"}
	got := truncateBlock(lines, "\t")
	if strings.Join(got, "|") != "|\t\tdo()|\t}" {
		t.Fatalf("truncateBlock() = %q", got)
	}
	long := make([]string, 30)
	for i := range long {
		long[i] = "x"
	}
	if got := truncateBlock(long, ""); len(got) != autocompleteMaxLines {
		t.Fatalf("cap = %d lines", len(got))
	}
}

func TestCutoffReached(t *testing.T) {
	if cutoffReached("a + ", false, "") {
		t.Fatal("single-line: still on the first line")
	}
	if !cutoffReached("a + b\nfmt.Pr", false, "") {
		t.Fatal("single-line: a line break after content ends it")
	}
	if cutoffReached("\n\t\tdo()\n\t}\n", true, "\t") {
		t.Fatal("block: still inside the caret's scope")
	}
	if !cutoffReached("\n\t\tdo()\n\t}\n}\n", true, "\t") {
		t.Fatal("block: a dedented complete line ends it")
	}
	if cutoffReached("```go\n\t\tdo()\n", true, "\t") {
		t.Fatal("block: an opening fence must not count as a dedent")
	}
}

func TestCleanCompletion(t *testing.T) {
	cases := []struct {
		name           string
		raw            string
		prefix, suffix string
		multiLine      bool
		want           string
	}{
		{"plain continuation", "a + b", "\treturn ", "\n}", false, "a + b"},
		{"fence stripped", "```go\nreturn a + b\n```", "\t", "\n}", false, "return a + b"},
		{"caret-line echo dropped", "if err != nil {", "\tif err ", "\n", false, "!= nil {"},
		{"indentation preserved on an empty caret line", "\treturn x", "func f() {\n", "\n}", true, "\treturn x"},
		{"typed indentation is not doubled", "\t\treturn x", "func f() {\n\t\t", "\n}", true, "return x"},
		{"leading space after a trailing space is dropped", " a + b", "\treturn ", "\n}", false, "a + b"},
		{"closing brace in the suffix is not re-typed", "a + b\n}", "\treturn ", "\n}", true, "a + b"},
		{"single-line mode keeps the first line", "a + b\nfmt.Println(a)", "\treturn ", "\n}", false, "a + b"},
		{"single-line mode yields nothing when the model starts on the next line", "\nfoo()", "bar()", "\n", false, ""},
		{"block cut at the enclosing scope", "\n\t\tdo()\n\t}\n}\n\nfunc g() {}", "\tif x {", "\n}", true, "\n\t\tdo()\n\t}"},
		{"closing punctuation only is nothing", ")\n}", "foo(a", ")\n}", true, ""},
		{"a repeat of the next line is nothing", "return nil", "\tx := 1\n\t", "\n\treturn nil\n}", true, ""},
		{"trailing blank lines trimmed", "return a\n\n\n", "\t", "", true, "return a"},
		{"empty reply", "   \n  ", "\t", "", true, ""},
		{"CRLF normalised", "return a\r\n\treturn b", "func f() {\n\t", "", true, "return a\n\treturn b"},
		{"a second line at column zero is outside the caret's scope", "return a\nfunc g() {}", "func f() {\n\t", "", true, "return a"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := cleanCompletion(tc.raw, tc.prefix, tc.suffix, tc.multiLine); got != tc.want {
				t.Fatalf("cleanCompletion() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSmallHelpers(t *testing.T) {
	if !startsMidIdentifier("bar()") || startsMidIdentifier(")") || startsMidIdentifier("") || startsMidIdentifier(" x") {
		t.Fatal("startsMidIdentifier")
	}
	if !isClosingOnly(" )\n}") || isClosingOnly("x)") || isClosingOnly("") {
		t.Fatal("isClosingOnly")
	}
	if commentLeader("python") != "#" || commentLeader("go") != "//" || commentLeader("sql") != "--" || commentLeader("") != "//" {
		t.Fatal("commentLeader")
	}
	if caretLine("func f() {\n\tif err ") != "if err " || caretIndent("a\n\t\tx") != "\t\t" {
		t.Fatal("caretLine / caretIndent")
	}
}

func TestTailBytesKeepsWholeLines(t *testing.T) {
	if got := tailBytes("aaa one\ntwo\nthree", 12); got != "two\nthree" {
		t.Fatalf("tailBytes() = %q", got)
	}
	if got := tailBytes(strings.Repeat("b", 20)+"\nc", 12); got != "bbbbbbbbbb\nc" {
		t.Fatalf("tailBytes() over-trimmed a long fragment: %q", got)
	}
	if got := tailBytes("short", 100); got != "short" {
		t.Fatalf("tailBytes() under the cap = %q", got)
	}
	if got := tailBytes("ЖЖЖЖ", 5); !strings.HasSuffix("ЖЖЖЖ", got) {
		t.Fatalf("tailBytes() mangled a rune: %q", got)
	}
}

func TestHeadBytesKeepsWholeLines(t *testing.T) {
	if got := headBytes("one\ntwo\nthree\nfour", 16); got != "one\ntwo\nthree" {
		t.Fatalf("headBytes() = %q", got)
	}
	if got := headBytes("a\n"+strings.Repeat("b", 20), 12); got != "a\nbbbbbbbbbb" {
		t.Fatalf("headBytes() over-trimmed a long fragment: %q", got)
	}
	if got := headBytes("ЖЖЖЖ", 5); !strings.HasPrefix("ЖЖЖЖ", got) {
		t.Fatalf("headBytes() mangled a rune: %q", got)
	}
}

// ---- fake model ------------------------------------------------------------------------------

// fakeLLM answers chat completions (streamed or not) and raw completions, recording every request
// body so a test can pin what reached the wire.
type fakeLLM struct {
	chatReply string
	rawReply  string // "" answers 404 on /v1/completions, like a gateway without the endpoint
	// reasoning, when set, is streamed as reasoning_content before the answer. A request that
	// carries stop sequences then gets no answer at all, the way a server that matches stops
	// against the reasoning text behaves.
	reasoning string
	// chatStatus, when non-zero, makes every chat call fail with that HTTP status (429 with
	// retryAfter as the Retry-After header, say). chatDelay holds the answer back first.
	chatStatus int
	retryAfter string
	chatDelay  time.Duration
	srv        *httptest.Server

	mu         sync.Mutex
	chatBodies []map[string]interface{}
	rawBodies  []map[string]interface{}
}

func newFakeLLM(t *testing.T, chatReply, rawReply string) *fakeLLM {
	t.Helper()
	f := &fakeLLM{chatReply: chatReply, rawReply: rawReply}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeLLM) serve(w http.ResponseWriter, r *http.Request) {
	body := map[string]interface{}{}
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.mu.Lock()
	switch r.URL.Path {
	case "/v1/chat/completions":
		f.chatBodies = append(f.chatBodies, body)
	case "/v1/completions":
		f.rawBodies = append(f.rawBodies, body)
	}
	f.mu.Unlock()

	switch r.URL.Path {
	case "/v1/completions":
		if f.rawReply == "" {
			http.Error(w, `{"error":{"message":"no such endpoint"}}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		text, _ := json.Marshal(f.rawReply)
		_, _ = fmt.Fprintf(w, `{"id":"1","object":"text_completion","choices":[{"index":0,"text":%s,"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":3}}`, text)
	case "/v1/chat/completions":
		if f.chatDelay > 0 {
			select {
			case <-time.After(f.chatDelay):
			case <-r.Context().Done():
				return
			}
		}
		if f.chatStatus != 0 {
			if f.retryAfter != "" {
				w.Header().Set("Retry-After", f.retryAfter)
			}
			http.Error(w, `{"error":{"message":"rate limit 20/min reached","code":"429"}}`, f.chatStatus)
			return
		}
		reply := f.chatReply
		if f.reasoning != "" {
			if stops, _ := body["stop"].([]interface{}); len(stops) > 0 {
				reply = ""
			}
		}
		if stream, _ := body["stream"].(bool); stream {
			w.Header().Set("Content-Type", "text/event-stream")
			if f.reasoning != "" {
				think, _ := json.Marshal(f.reasoning)
				_, _ = fmt.Fprintf(w, "data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"reasoning_content\":%s},\"finish_reason\":null}]}\n\n", think)
			}
			half := len(reply) / 2
			first, _ := json.Marshal(reply[:half])
			second, _ := json.Marshal(reply[half:])
			_, _ = fmt.Fprintf(w, "data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":%s},\"finish_reason\":null}]}\n\n", first)
			_, _ = fmt.Fprintf(w, "data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"content\":%s},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":3,\"total_tokens\":15}}\n\n", second)
			_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		content, _ := json.Marshal(reply)
		think, _ := json.Marshal(f.reasoning)
		_, _ = fmt.Fprintf(w, `{"id":"1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":%s,"reasoning_content":%s},"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":3}}`, content, think)
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeLLM) chatCalls() []map[string]interface{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[string]interface{}{}, f.chatBodies...)
}

func (f *fakeLLM) rawCalls() []map[string]interface{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[string]interface{}{}, f.rawBodies...)
}

func userMessage(body map[string]interface{}) string {
	msgs, _ := body["messages"].([]interface{})
	for _, m := range msgs {
		mm, _ := m.(map[string]interface{})
		if mm["role"] == "user" {
			s, _ := mm["content"].(string)
			return s
		}
	}
	return ""
}

// ---- server harness --------------------------------------------------------------------------

// newAutocompleteServer wires a Server whose only LLM is the fake at llmURL, with root as its
// workspace (the directory related-file excerpts are allowed to come from).
func newAutocompleteServer(t *testing.T, llmURL, model string, ac config.AutocompleteConfig) (*httptest.Server, string) {
	t.Helper()
	root := t.TempDir()
	cfg := &config.Config{
		Paths: config.Paths{Home: root, CWD: root},
		Providers: []config.ProviderConfig{
			{Name: "openai", Type: "openai", APIBase: llmURL + "/v1", APIKey: "sk-test"},
		},
		Models: []config.ModelEntry{
			{Model: model, MaxTokens: 4096, Temperature: 0.2},
		},
		Agent:        config.Agent{Model: model},
		Autocomplete: ac,
	}
	cfg.Autocomplete.ApplyDefaults()
	runner := func(context.Context, *session.State, []acp.ContentBlock, acp.UpdateSender) (string, error) {
		return "", nil
	}
	mgr := session.NewManager(cfg, noopSender{}, runner, slog.Default(), root, nil)
	srv := New(cfg, mgr, slog.Default(), root)
	t.Cleanup(srv.Drain)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, root
}

func enabledAutocomplete(mode string) config.AutocompleteConfig {
	enabled := true
	return config.AutocompleteConfig{Enabled: &enabled, Mode: mode}
}

type completionReply struct {
	Completion string `json:"completion"`
	Model      string `json:"model"`
	Mode       string `json:"mode"`
	Enabled    bool   `json:"enabled"`
}

func postCompletion(t *testing.T, base, body string) (*http.Response, completionReply) {
	t.Helper()
	res, err := http.Post(base+"/foxxycode/completion", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	var out completionReply
	if res.StatusCode == http.StatusOK {
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("decode %s: %v", raw, err)
		}
	}
	return res, out
}

const addFuncRequest = `{"prefix":"func add(a, b int) int {\n\treturn ","suffix":"\n}","path":"main.go","language":"go"}`

// ---- chat mode -------------------------------------------------------------------------------

// TestCompletionChatWireRequest drives the real provider stack against the fake and pins what
// reaches the wire in chat mode: the caret marker, the language header, the suffix after the
// caret, autocomplete.max_tokens overriding the model entry's budget, greedy sampling, thinking
// pinned off for Qwen, and the single-line stop sequence.
func TestCompletionChatWireRequest(t *testing.T) {
	fake := newFakeLLM(t, "```go\nreturn a + b\n```", "")
	ts, _ := newAutocompleteServer(t, fake.srv.URL, "openai/qwen3.6-35b", enabledAutocomplete(config.AutocompleteModeChat))

	res, out := postCompletion(t, ts.URL, addFuncRequest)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	if out.Completion != "a + b" || out.Mode != "chat" || out.Model != "openai/qwen3.6-35b" || !out.Enabled {
		t.Fatalf("reply = %+v", out)
	}

	calls := fake.chatCalls()
	if len(calls) != 1 {
		t.Fatalf("chat calls = %d", len(calls))
	}
	body := calls[0]
	if mt, _ := body["max_tokens"].(float64); int(mt) != config.AutocompleteDefaultMaxTokens {
		t.Fatalf("max_tokens = %v, want %d", body["max_tokens"], config.AutocompleteDefaultMaxTokens)
	}
	if temp, ok := body["temperature"].(float64); !ok || temp != 0 {
		t.Fatalf("temperature = %v, want an explicit 0", body["temperature"])
	}
	kw, _ := body["chat_template_kwargs"].(map[string]interface{})
	if think, ok := kw["enable_thinking"].(bool); !ok || think {
		t.Fatalf("enable_thinking = %v, want false", kw)
	}
	stops, _ := body["stop"].([]interface{})
	if len(stops) != 1 || stops[0] != "\n" {
		t.Fatalf("stop = %v, want the single-line stop", body["stop"])
	}
	content := userMessage(body)
	for _, want := range []string{autocompleteCursor, "Language: go", autocompleteCursor + "\n}"} {
		if !strings.Contains(content, want) {
			t.Fatalf("user message lacks %q: %q", want, content)
		}
	}
	if _, raw := body["prompt"]; raw {
		t.Fatal("chat mode must not send a raw prompt")
	}
}

func TestCompletionBlockStopsCarryTheNextLine(t *testing.T) {
	fake := newFakeLLM(t, "\n\tif err != nil {\n\t\treturn err\n\t}", "")
	ts, _ := newAutocompleteServer(t, fake.srv.URL, "openai/gpt-4o", enabledAutocomplete(config.AutocompleteModeChat))

	res, out := postCompletion(t, ts.URL, `{"prefix":"func f() error {","suffix":"\n\treturn nil\n}","language":"go"}`)
	if res.StatusCode != http.StatusOK || out.Completion != "\n\tif err != nil {\n\t\treturn err\n\t}" {
		t.Fatalf("status %d reply %+v", res.StatusCode, out)
	}
	stops, _ := fake.chatCalls()[0]["stop"].([]interface{})
	if len(stops) != 2 || stops[0] != "\n\treturn nil" || stops[1] != "\n\n\n" {
		t.Fatalf("stop = %v", stops)
	}
}

// A model that reasons before answering loses its answer to the stop sequences (servers match
// them against the reasoning text too). The first empty answer teaches the server: it retries
// once without stops and never sends them to that model again.
func TestCompletionThinkingModelGetsNoStopSequences(t *testing.T) {
	fake := newFakeLLM(t, "a + b", "")
	fake.reasoning = "Let me think.\nThe sum is wanted."
	ts, _ := newAutocompleteServer(t, fake.srv.URL, "openai/kimi-k2.6", enabledAutocomplete(config.AutocompleteModeChat))

	res, out := postCompletion(t, ts.URL, addFuncRequest)
	if res.StatusCode != http.StatusOK || out.Completion != "a + b" {
		t.Fatalf("first request: status %d reply %+v", res.StatusCode, out)
	}
	calls := fake.chatCalls()
	if len(calls) != 2 {
		t.Fatalf("first request should cost two calls (with stops, then without), got %d", len(calls))
	}
	if _, withStops := calls[0]["stop"]; !withStops {
		t.Fatal("the first attempt must carry the stop sequences")
	}
	if _, retriedWithStops := calls[1]["stop"]; retriedWithStops {
		t.Fatal("the retry must not carry stop sequences")
	}
	if mt, _ := calls[1]["max_tokens"].(float64); int(mt) != autocompleteThinkingBudget {
		t.Fatalf("retry max_tokens = %v, want the thinking floor %d", calls[1]["max_tokens"], autocompleteThinkingBudget)
	}

	res, out = postCompletion(t, ts.URL, addFuncRequest)
	if res.StatusCode != http.StatusOK || out.Completion != "a + b" {
		t.Fatalf("second request: status %d reply %+v", res.StatusCode, out)
	}
	calls = fake.chatCalls()
	if len(calls) != 3 {
		t.Fatalf("second request should cost one call, got %d total", len(calls))
	}
	if _, withStops := calls[2]["stop"]; withStops {
		t.Fatal("a model known to reason must not be sent stop sequences again")
	}
}

// A provider rate limit reaches the editor as a 429 with Retry-After, so it can pause its
// automatic requests instead of paying for refusals; the counter shows it happened.
func TestCompletionPassesRateLimitsOnAsRetryAfter(t *testing.T) {
	fake := newFakeLLM(t, "a + b", "")
	fake.chatStatus = http.StatusTooManyRequests
	fake.retryAfter = "6"
	ts, _ := newAutocompleteServer(t, fake.srv.URL, "openai/gpt-4o", enabledAutocomplete(config.AutocompleteModeChat))

	res, err := http.Post(ts.URL+"/foxxycode/completion", "application/json", strings.NewReader(addFuncRequest))
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status %d, want 429", res.StatusCode)
	}
	if ra := res.Header.Get("Retry-After"); ra != "6" {
		t.Fatalf("Retry-After = %q, want the provider's 6 s", ra)
	}
	stats := e2eStats(t, ts.URL)
	if rl, _ := stats["rate_limited"].(float64); rl != 1 {
		t.Fatalf("rate_limited = %v", stats["rate_limited"])
	}
}

// A model slower than autocomplete.timeout_ms yields an empty suggestion flagged timed_out, not
// an error and not an empty body.
func TestCompletionTimeoutAnswersEmpty(t *testing.T) {
	fake := newFakeLLM(t, "a + b", "")
	fake.chatDelay = 2 * time.Second
	ac := enabledAutocomplete(config.AutocompleteModeChat)
	ac.TimeoutMS = 200
	ts, _ := newAutocompleteServer(t, fake.srv.URL, "openai/gpt-4o", ac)

	res, err := http.Post(ts.URL+"/foxxycode/completion", "application/json", strings.NewReader(addFuncRequest))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", res.StatusCode)
	}
	var out map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["completion"] != "" || out["timed_out"] != true {
		t.Fatalf("reply = %v", out)
	}
	if to, _ := e2eStats(t, ts.URL)["timeouts"].(float64); to != 1 {
		t.Fatalf("timeouts counter = %v", to)
	}
}

func TestCompletionDebugEchoesTheRawModelText(t *testing.T) {
	fake := newFakeLLM(t, "```go\nreturn a + b\n```", "")
	ts, _ := newAutocompleteServer(t, fake.srv.URL, "openai/gpt-4o", enabledAutocomplete(config.AutocompleteModeChat))
	res, err := http.Post(ts.URL+"/foxxycode/completion", "application/json",
		strings.NewReader(`{"prefix":"func add(a, b int) int {\n\treturn ","suffix":"\n}","language":"go","debug":true}`))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	var out map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out["completion"] != "a + b" || out["raw"] != "```go\nreturn a + b\n```" || out["multi_line"] != false {
		t.Fatalf("debug reply = %v", out)
	}
}

// ---- FIM mode --------------------------------------------------------------------------------

func TestCompletionFIMUsesRawCompletionForCodeModels(t *testing.T) {
	fake := newFakeLLM(t, "chat should not be used", "a + b")
	ts, _ := newAutocompleteServer(t, fake.srv.URL, "openai/qwen2.5-coder-7b", enabledAutocomplete(config.AutocompleteModeAuto))

	res, out := postCompletion(t, ts.URL, addFuncRequest)
	if res.StatusCode != http.StatusOK || out.Completion != "a + b" || out.Mode != "fim" {
		t.Fatalf("status %d reply %+v", res.StatusCode, out)
	}
	if len(fake.chatCalls()) != 0 {
		t.Fatal("a FIM-capable model must not fall back to chat when raw completion works")
	}
	raw := fake.rawCalls()
	if len(raw) != 1 {
		t.Fatalf("raw calls = %d", len(raw))
	}
	prompt, _ := raw[0]["prompt"].(string)
	// With no related files the prompt is the plain single-file FIM form: the repository-level
	// separators only appear once there are other files to separate.
	if prompt != "<|fim_prefix|>func add(a, b int) int {\n\treturn <|fim_suffix|>\n}<|fim_middle|>" {
		t.Fatalf("raw prompt = %q", prompt)
	}
	stops, _ := raw[0]["stop"].([]interface{})
	if len(stops) == 0 || stops[0] != "\n" || len(stops) > autocompleteMaxStops {
		t.Fatalf("stop = %v, want the line stop first and at most %d entries", stops, autocompleteMaxStops)
	}
	if temp, ok := raw[0]["temperature"].(float64); !ok || temp != 0 {
		t.Fatalf("temperature = %v", raw[0]["temperature"])
	}
}

// A gateway without /v1/completions answers 404: auto mode re-issues the request as chat and
// remembers the model as chat-only, so the second keystroke never pays for the failed call again.
func TestCompletionAutoFallsBackToChatOnce(t *testing.T) {
	fake := newFakeLLM(t, "a + b", "")
	ts, _ := newAutocompleteServer(t, fake.srv.URL, "openai/qwen2.5-coder-7b", enabledAutocomplete(config.AutocompleteModeAuto))

	for i := 0; i < 2; i++ {
		res, out := postCompletion(t, ts.URL, addFuncRequest)
		if res.StatusCode != http.StatusOK || out.Completion != "a + b" || out.Mode != "chat" {
			t.Fatalf("request %d: status %d reply %+v", i, res.StatusCode, out)
		}
	}
	if raw, chat := len(fake.rawCalls()), len(fake.chatCalls()); raw != 1 || chat != 2 {
		t.Fatalf("raw calls = %d, chat calls = %d; want one failed raw attempt and two chat answers", raw, chat)
	}
}

// A gateway can serve /v1/completions and still hand the FIM tokens to a model that ignores them,
// which shows as 200 with an empty text. Auto mode answers such a keystroke through chat and,
// after a streak of empties, stops trying FIM for that model.
func TestCompletionAutoRetriesEmptyFIMAsChatAndRetiresIt(t *testing.T) {
	fake := newFakeLLM(t, "a + b", "   ")
	ts, _ := newAutocompleteServer(t, fake.srv.URL, "openai/qwen2.5-coder-7b", enabledAutocomplete(config.AutocompleteModeAuto))

	for i := 0; i < fimEmptyStreakLimit+2; i++ {
		res, out := postCompletion(t, ts.URL, addFuncRequest)
		if res.StatusCode != http.StatusOK || out.Completion != "a + b" || out.Mode != "chat" {
			t.Fatalf("request %d: status %d reply %+v", i, res.StatusCode, out)
		}
	}
	if raw, chat := len(fake.rawCalls()), len(fake.chatCalls()); raw != fimEmptyStreakLimit || chat != fimEmptyStreakLimit+2 {
		t.Fatalf("raw calls = %d, chat calls = %d; want %d empty raw attempts then chat only", raw, chat, fimEmptyStreakLimit)
	}
}

func TestCompletionForcedFIMFailsLoudlyWithoutTokens(t *testing.T) {
	fake := newFakeLLM(t, "a + b", "a + b")
	ts, _ := newAutocompleteServer(t, fake.srv.URL, "openai/gpt-4o", enabledAutocomplete(config.AutocompleteModeFIM))
	res, _ := postCompletion(t, ts.URL, addFuncRequest)
	if res.StatusCode != http.StatusBadGateway {
		t.Fatalf("status %d, want 502 for a model with no FIM convention", res.StatusCode)
	}
	if len(fake.chatCalls()) != 0 || len(fake.rawCalls()) != 0 {
		t.Fatal("no model call should be made")
	}
}

// ---- related files ---------------------------------------------------------------------------

func TestCompletionExcerptsRelatedOpenFiles(t *testing.T) {
	fake := newFakeLLM(t, "a + b", "")
	ts, root := newAutocompleteServer(t, fake.srv.URL, "openai/gpt-4o", enabledAutocomplete(config.AutocompleteModeChat))

	current := filepath.Join(root, "main.go")
	helper := filepath.Join(root, "pkg", "util.go")
	outside := filepath.Join(t.TempDir(), "secret.go")
	for path, content := range map[string]string{
		current: "package main\n",
		helper:  "package pkg\n\nfunc Helper(x int) int { return x }\n",
		outside: "package secret\n\nconst Token = \"nope\"\n",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ideenv.Set([]string{current, helper, outside}, current, nil)
	t.Cleanup(ideenv.Reset)

	body, _ := json.Marshal(map[string]string{"prefix": "func main() {\n\tx := ", "suffix": "\n}", "path": current, "language": "go"})
	res, _ := postCompletion(t, ts.URL, string(body))
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	content := userMessage(fake.chatCalls()[0])
	if !strings.Contains(content, "```pkg/util.go\npackage pkg\n\nfunc Helper(x int) int { return x }\n```") {
		t.Fatalf("related file excerpt missing: %q", content)
	}
	if strings.Contains(content, "secret") {
		t.Fatalf("a file outside the workspace was sent to the model: %q", content)
	}
	if strings.Contains(content, "```main.go") {
		t.Fatalf("the file being edited must not be excerpted as a related file: %q", content)
	}
	if !strings.Contains(content, "File: main.go\n") {
		t.Fatalf("current file should be named relative to the workspace: %q", content)
	}
}

func TestCompletionRelatedFilesCanBeDisabled(t *testing.T) {
	fake := newFakeLLM(t, "a + b", "")
	zero := 0
	ac := enabledAutocomplete(config.AutocompleteModeChat)
	ac.RelatedFiles = &zero
	ts, root := newAutocompleteServer(t, fake.srv.URL, "openai/gpt-4o", ac)

	helper := filepath.Join(root, "util.go")
	if err := os.WriteFile(helper, []byte("package main\n\nfunc Helper() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ideenv.Set([]string{helper}, "", nil)
	t.Cleanup(ideenv.Reset)

	postCompletion(t, ts.URL, addFuncRequest)
	if strings.Contains(userMessage(fake.chatCalls()[0]), "Helper") {
		t.Fatal("related_files: 0 must not excerpt anything")
	}
}

// ---- gating and bookkeeping ------------------------------------------------------------------

func TestCompletionDisabledIsNotAnError(t *testing.T) {
	fake := newFakeLLM(t, "must not be called", "")
	ts, _ := newAutocompleteServer(t, fake.srv.URL, "openai/gpt-4o", config.AutocompleteConfig{})
	res, out := postCompletion(t, ts.URL, `{"prefix":"x := ","suffix":""}`)
	if res.StatusCode != http.StatusOK || out.Enabled || out.Completion != "" {
		t.Fatalf("status %d reply %+v", res.StatusCode, out)
	}
	if len(fake.chatCalls()) != 0 {
		t.Fatal("the model must not be called while autocomplete is disabled")
	}
}

func TestCompletionSkipsMidWordAndEmptyContext(t *testing.T) {
	fake := newFakeLLM(t, "must not be called", "")
	ts, _ := newAutocompleteServer(t, fake.srv.URL, "openai/gpt-4o", enabledAutocomplete(config.AutocompleteModeChat))
	for _, body := range []string{`{"prefix":"   ","suffix":"\n"}`, `{"prefix":"fo","suffix":"oBar()"}`} {
		res, out := postCompletion(t, ts.URL, body)
		if res.StatusCode != http.StatusOK || out.Completion != "" {
			t.Fatalf("%s: status %d reply %+v", body, res.StatusCode, out)
		}
	}
	if len(fake.chatCalls()) != 0 {
		t.Fatal("neither request should reach the model")
	}
}

func TestCompletionRejectsInvalidJSON(t *testing.T) {
	ts, _ := newAutocompleteServer(t, "http://127.0.0.1:1", "openai/gpt-4o", enabledAutocomplete(config.AutocompleteModeChat))
	res, _ := postCompletion(t, ts.URL, "not json")
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", res.StatusCode)
	}
}

func TestCompletionConfigEndpoint(t *testing.T) {
	ac := enabledAutocomplete(config.AutocompleteModeAuto)
	ac.Trigger = config.AutocompleteTriggerManual
	ac.DebounceMS = 700
	ts, _ := newAutocompleteServer(t, "http://127.0.0.1:1", "openai/gpt-4o", ac)

	res, err := http.Get(ts.URL + "/foxxycode/completion/config")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	var out struct {
		Enabled        bool   `json:"enabled"`
		Trigger        string `json:"trigger"`
		DebounceMS     int    `json:"debounce_ms"`
		MultiLine      bool   `json:"multi_line"`
		MaxPrefixBytes int    `json:"max_prefix_bytes"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if !out.Enabled || out.Trigger != config.AutocompleteTriggerManual || out.DebounceMS != 700 {
		t.Fatalf("config = %+v", out)
	}
	if !out.MultiLine || out.MaxPrefixBytes != config.AutocompleteDefaultMaxPrefixBytes {
		t.Fatalf("defaults not applied: %+v", out)
	}
}

func TestCompletionStatsAndFeedback(t *testing.T) {
	fake := newFakeLLM(t, "a + b", "")
	ts, _ := newAutocompleteServer(t, fake.srv.URL, "openai/gpt-4o", enabledAutocomplete(config.AutocompleteModeChat))
	postCompletion(t, ts.URL, addFuncRequest)

	for _, ev := range []string{"shown", "accepted", "cache_hit", "cache_hit"} {
		res, err := http.Post(ts.URL+"/foxxycode/completion/feedback", "application/json", strings.NewReader(`{"event":"`+ev+`"}`))
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if res.StatusCode != http.StatusNoContent {
			t.Fatalf("feedback %s: status %d", ev, res.StatusCode)
		}
	}
	res, err := http.Post(ts.URL+"/foxxycode/completion/feedback", "application/json", strings.NewReader(`{"event":"bogus"}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("bogus feedback: status %d", res.StatusCode)
	}

	res, err = http.Get(ts.URL + "/foxxycode/completion/stats")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	var stats map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&stats); err != nil {
		t.Fatal(err)
	}
	want := map[string]float64{"requests": 1, "served": 1, "chat": 1, "fim": 0, "shown": 1, "accepted": 1, "cache_hits": 2, "acceptance_rate": 1}
	for k, v := range want {
		if got, _ := stats[k].(float64); got != v {
			t.Fatalf("stats[%s] = %v, want %v (all: %v)", k, stats[k], v, stats)
		}
	}
	if pt, _ := stats["prompt_tokens"].(float64); pt <= 0 {
		t.Fatalf("prompt tokens not counted: %v", stats)
	}
}
