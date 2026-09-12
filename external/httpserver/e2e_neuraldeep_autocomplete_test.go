//go:build http

package httpserver

// Live quality harness for inline completion against the NeuralDeep hub.
//
// This is not a unit test: it talks to api.neuraldeep.ru with a real key, and its purpose is to
// let a human judge how good the suggestions are for the models the hub actually serves, and
// whether native fill-in-the-middle or a chat prompt does better for each. It skips unless the
// key is present, so CI never runs it.
//
//	NEURALDEEP_API_KEY=sk-... make e2e-autocomplete
//
// Knobs (all optional):
//
//	FOXXYCODE_E2E_MODELS     comma-separated hub model ids (default: the -noreason variants and kimi)
//	FOXXYCODE_E2E_MODES      comma-separated autocomplete.mode values to compare (default: auto,chat)
//	FOXXYCODE_E2E_MIN_SCORE  fail when any model/mode scores below this fraction (default 0: report only)
//	FOXXYCODE_E2E_REPORT     also write the markdown report to this file
//	FOXXYCODE_E2E_SCENARIO   run only scenarios whose name contains this text
//	FOXXYCODE_E2E_DEBUG=1    log the server at debug level and add the model's raw text to the report
//
// Each scenario is a caret position with a loose acceptance check - a substring the answer must
// contain, plus rules every suggestion must obey (no fence, no caret marker, no re-typed suffix).
// LLM output varies, so the check is deliberately about the idea, not the exact text.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/session"
)

type e2eScenario struct {
	name     string
	language string
	path     string
	prefix   string
	suffix   string
	// anyOf: the cleaned completion must contain at least one of these (case-insensitive).
	anyOf []string
	// allOf: the cleaned completion must contain every one of these (case-insensitive).
	allOf []string
	// forbid: none of these may appear (case-sensitive) - typically a re-typed suffix.
	forbid []string
	// singleLine: the completion must stay on the caret line.
	singleLine bool
}

var e2eScenarios = []e2eScenario{
	{
		name: "go: finish a return expression", language: "go", path: "main.go",
		prefix: "package main\n\nfunc add(a, b int) int {\n\treturn ", suffix: "\n}",
		anyOf: []string{"a + b", "a+b"}, singleLine: true,
	},
	{
		name: "go: the error-return idiom", language: "go", path: "load.go",
		prefix: "package main\n\nimport \"os\"\n\nfunc load(path string) ([]byte, error) {\n\tdata, err := os.ReadFile(path)\n\tif err != nil {\n\t\t",
		suffix: "\n\t}\n\treturn data, nil\n}",
		allOf:  []string{"return nil, err"},
	},
	{
		name: "go: use a struct field from the receiver", language: "go", path: "user.go",
		prefix: "package main\n\nimport \"fmt\"\n\ntype User struct {\n\tName string\n\tAge  int\n}\n\nfunc (u User) Greeting() string {\n\treturn fmt.Sprintf(\"Hello, %s\", ",
		suffix: ")\n}",
		allOf:  []string{"u.Name"}, forbid: []string{")\n}"}, singleLine: true,
	},
	{
		name: "go: a whole small block", language: "go", path: "max.go",
		prefix: "package main\n\nfunc max(a, b int) int {", suffix: "\n}",
		anyOf: []string{"a > b", "a < b", "b > a", "a >= b"}, allOf: []string{"return"},
	},
	{
		name: "go: code to the right of the caret stays single-line", language: "go", path: "parts.go",
		prefix: "package main\n\nimport \"strings\"\n\nfunc parts(s string) []string {\n\treturn strings.",
		suffix: "(s, \",\")\n}",
		allOf:  []string{"Split"}, forbid: []string{"(s"}, singleLine: true,
	},
	{
		name: "go: the closing brace is not re-typed", language: "go", path: "f.go",
		prefix: "package main\n\nfunc f() error {\n\tif err := g(); err != nil {\n\t\treturn ",
		suffix: "\n\t}\n\treturn nil\n}",
		allOf:  []string{"err"}, forbid: []string{"}"}, singleLine: true,
	},
	{
		name: "python: parity check", language: "python", path: "num.py",
		prefix: "def is_even(n: int) -> bool:\n    return ", suffix: "\n",
		anyOf: []string{"% 2 == 0", "%2==0", "% 2 == 0"}, singleLine: true,
	},
	{
		name: "python: recursive step", language: "python", path: "fib.py",
		prefix: "def fib(n: int) -> int:\n    if n < 2:\n        return n\n    return ", suffix: "\n",
		anyOf: []string{"fib(n - 1)", "fib(n-1)"}, singleLine: true,
	},
	{
		name: "python: method body from the class around it", language: "python", path: "stack.py",
		prefix: "class Stack:\n    def __init__(self):\n        self.items = []\n\n    def push(self, item):\n        ",
		suffix: "\n\n    def pop(self):\n        return self.items.pop()\n",
		allOf:  []string{"self.items.append(item)"},
	},
	{
		name: "typescript: clamp with the semicolon already there", language: "typescript", path: "clamp.ts",
		prefix: "export function clamp(value: number, min: number, max: number): number {\n  return ",
		suffix: ";\n}",
		allOf:  []string{"Math.min", "Math.max"}, forbid: []string{";\n}"}, singleLine: true,
	},
	{
		name: "javascript: reducer callback", language: "javascript", path: "total.js",
		prefix: "const total = items.reduce((sum, item) => ", suffix: ", 0);",
		anyOf: []string{"sum + item"}, forbid: []string{", 0)"}, singleLine: true,
	},
	{
		name: "kotlin: palindrome check", language: "kotlin", path: "Pal.kt",
		prefix: "fun isPalindrome(s: String): Boolean {\n    val clean = s.lowercase().filter { it.isLetterOrDigit() }\n    return ",
		suffix: "\n}",
		allOf:  []string{"reversed()"}, singleLine: true,
	},
}

type e2eResult struct {
	scenario   string
	ok         bool
	reason     string
	mode       string
	latency    time.Duration
	completion string
	raw        string
}

func e2eDebug() bool { return strings.TrimSpace(os.Getenv("FOXXYCODE_E2E_DEBUG")) != "" }

func TestE2ENeuralDeepAutocomplete(t *testing.T) {
	key := strings.TrimSpace(os.Getenv("NEURALDEEP_API_KEY"))
	if key == "" {
		t.Skip("NEURALDEEP_API_KEY not set; live NeuralDeep quality harness skipped")
	}
	models := e2eList("FOXXYCODE_E2E_MODELS", []string{
		"qwen3.6-35b-a3b-noreason", "qwen3.8-27b-noreason", "gemma-4-31b-noreason", "kimi-k2.6",
	})
	modes := e2eList("FOXXYCODE_E2E_MODES", []string{config.AutocompleteModeAuto, config.AutocompleteModeChat})
	minScore, _ := strconv.ParseFloat(strings.TrimSpace(os.Getenv("FOXXYCODE_E2E_MIN_SCORE")), 64)
	only := strings.TrimSpace(os.Getenv("FOXXYCODE_E2E_SCENARIO"))
	scenarios := make([]e2eScenario, 0, len(e2eScenarios))
	for _, sc := range e2eScenarios {
		if only == "" || strings.Contains(sc.name, only) {
			scenarios = append(scenarios, sc)
		}
	}

	var report strings.Builder
	fmt.Fprintf(&report, "# Inline completion quality on NeuralDeep (%s)\n\n", time.Now().Format("2006-01-02 15:04"))
	summary := []string{"| model | mode | score | avg ms | max ms | fim/chat/empty |", "|---|---|---|---|---|---|"}
	failed := false

	for _, model := range models {
		for _, mode := range modes {
			ts := e2eServer(t, key, model, mode)
			results := make([]e2eResult, 0, len(scenarios))
			for _, sc := range scenarios {
				results = append(results, e2eRun(t, ts.URL, sc))
			}
			stats := e2eStats(t, ts.URL)
			ts.Close()

			passed, total := 0, len(results)
			var sum, worst time.Duration
			if e2eDebug() {
				fmt.Fprintf(&report, "## %s — mode %s\n\n| scenario | ok | mode | ms | completion | raw model text |\n|---|---|---|---|---|---|\n", model, mode)
			} else {
				fmt.Fprintf(&report, "## %s — mode %s\n\n| scenario | ok | mode | ms | completion |\n|---|---|---|---|---|\n", model, mode)
			}
			for _, r := range results {
				if r.ok {
					passed++
				}
				sum += r.latency
				if r.latency > worst {
					worst = r.latency
				}
				mark := "✅"
				if !r.ok {
					mark = "❌ " + r.reason
				} else if r.reason != "" {
					mark = "✅ " + r.reason
				}
				if e2eDebug() {
					fmt.Fprintf(&report, "| %s | %s | %s | %d | `%s` | `%s` |\n", r.scenario, mark, r.mode, r.latency.Milliseconds(), e2eEscape(r.completion), e2eEscape(r.raw))
				} else {
					fmt.Fprintf(&report, "| %s | %s | %s | %d | `%s` |\n", r.scenario, mark, r.mode, r.latency.Milliseconds(), e2eEscape(r.completion))
				}
			}
			score := float64(passed) / float64(total)
			avg := time.Duration(0)
			if total > 0 {
				avg = sum / time.Duration(total)
			}
			fmt.Fprintf(&report, "\nscore %d/%d, avg %d ms, max %d ms; server counters: fim=%v chat=%v fim_fallback=%v fim_empty=%v reasoning_retries=%v empty=%v errors=%v timeouts=%v rate_limited=%v\n\n",
				passed, total, avg.Milliseconds(), worst.Milliseconds(), stats["fim"], stats["chat"], stats["fim_fallback"], stats["fim_empty"], stats["reasoning_retries"], stats["empty"], stats["errors"], stats["timeouts"], stats["rate_limited"])
			summary = append(summary, fmt.Sprintf("| %s | %s | %d/%d | %d | %d | %v/%v/%v |",
				model, mode, passed, total, avg.Milliseconds(), worst.Milliseconds(), stats["fim"], stats["chat"], stats["fim_empty"]))
			if minScore > 0 && score < minScore {
				failed = true
				t.Errorf("%s (%s): score %.2f below FOXXYCODE_E2E_MIN_SCORE %.2f", model, mode, score, minScore)
			}
		}
	}

	fmt.Fprintf(&report, "## Summary\n\n%s\n", strings.Join(summary, "\n"))
	t.Log("\n" + report.String())
	if path := strings.TrimSpace(os.Getenv("FOXXYCODE_E2E_REPORT")); path != "" {
		if err := os.WriteFile(path, []byte(report.String()), 0o644); err != nil {
			t.Errorf("write report: %v", err)
		} else {
			t.Logf("report written to %s", path)
		}
	}
	if failed {
		t.Fail()
	}
}

func e2eList(env string, fallback []string) []string {
	raw := strings.TrimSpace(os.Getenv(env))
	if raw == "" {
		return fallback
	}
	var out []string
	for _, s := range strings.Split(raw, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// e2eServer wires a Server whose autocomplete model is one hub model, in one prompt mode. The
// generous timeout is on purpose: the hub can be slow, and a slow answer is a finding, not a
// reason to abort the run.
func e2eServer(t *testing.T, key, model, mode string) *httptest.Server {
	t.Helper()
	root := t.TempDir()
	enabled := true
	related := 0
	id := "neuraldeep/" + model
	cfg := &config.Config{
		Paths:     config.Paths{Home: root, CWD: root},
		Providers: []config.ProviderConfig{{Name: "neuraldeep", Type: "neuraldeep", APIKey: key}},
		Models:    []config.ModelEntry{{Model: id, MaxTokens: 512}},
		Agent:     config.Agent{Model: id},
		Autocomplete: config.AutocompleteConfig{
			Enabled: &enabled, Mode: mode, TimeoutMS: 30000, RelatedFiles: &related,
		},
	}
	cfg.Autocomplete.ApplyDefaults()
	runner := func(context.Context, *session.State, []acp.ContentBlock, acp.UpdateSender) (string, error) {
		return "", nil
	}
	logger := slog.Default()
	if e2eDebug() {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}
	mgr := session.NewManager(cfg, noopSender{}, runner, logger, root, nil)
	srv := New(cfg, mgr, logger, root)
	t.Cleanup(srv.Drain)
	return httptest.NewServer(srv.Handler())
}

// e2eRun asks for one suggestion. A 429 from the hub (it caps each model at a few requests per
// minute) is waited out per Retry-After and retried, so a rate limit shows up as a longer wait
// in the notes, never as a quality failure.
func e2eRun(t *testing.T, base string, sc e2eScenario) e2eResult {
	t.Helper()
	body, _ := json.Marshal(map[string]interface{}{"prefix": sc.prefix, "suffix": sc.suffix, "path": sc.path, "language": sc.language, "debug": e2eDebug()})
	client := &http.Client{Timeout: 60 * time.Second}

	var res *http.Response
	var raw []byte
	var latency time.Duration
	var waited time.Duration
	for attempt := 0; ; attempt++ {
		started := time.Now()
		r, err := client.Post(base+"/foxxycode/completion", "application/json", strings.NewReader(string(body)))
		latency = time.Since(started)
		if err != nil {
			return e2eResult{scenario: sc.name, reason: "transport: " + err.Error(), latency: latency}
		}
		raw, _ = io.ReadAll(r.Body)
		_ = r.Body.Close()
		res = r
		if r.StatusCode != http.StatusTooManyRequests || attempt >= 4 {
			break
		}
		pause := 6 * time.Second
		if secs, err := strconv.Atoi(strings.TrimSpace(r.Header.Get("Retry-After"))); err == nil && secs > 0 {
			pause = time.Duration(secs) * time.Second
		}
		if pause > 30*time.Second {
			pause = 30 * time.Second
		}
		time.Sleep(pause)
		waited += pause
	}
	if res.StatusCode != http.StatusOK {
		return e2eResult{scenario: sc.name, reason: fmt.Sprintf("HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(raw))), latency: latency}
	}
	var out struct {
		Completion string `json:"completion"`
		Mode       string `json:"mode"`
		Raw        string `json:"raw"`
		TimedOut   bool   `json:"timed_out"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return e2eResult{scenario: sc.name, reason: "bad JSON: " + err.Error(), latency: latency}
	}
	r := e2eResult{scenario: sc.name, mode: out.Mode, latency: latency, completion: out.Completion, raw: out.Raw}
	if out.TimedOut {
		r.reason = "timed out on the server"
		return r
	}
	r.ok, r.reason = e2eJudge(sc, out.Completion)
	if waited > 0 {
		r.reason = strings.TrimSpace(r.reason + fmt.Sprintf(" (waited %ds on 429)", int(waited.Seconds())))
	}
	return r
}

// e2eJudge applies the scenario's acceptance rules plus the ones every suggestion must obey.
func e2eJudge(sc e2eScenario, completion string) (bool, string) {
	if strings.TrimSpace(completion) == "" {
		return false, "empty"
	}
	if strings.Contains(completion, "```") {
		return false, "markdown fence"
	}
	if strings.Contains(completion, "<CURSOR>") || strings.Contains(completion, "<|fim") {
		return false, "prompt marker leaked"
	}
	lower := strings.ToLower(completion)
	for _, word := range []string{"here is", "here's", "sure", "certainly"} {
		if strings.HasPrefix(strings.TrimSpace(lower), word) {
			return false, "explanation instead of code"
		}
	}
	if sc.singleLine && strings.Contains(strings.TrimRight(completion, "\n"), "\n") {
		return false, "grew past the caret line"
	}
	for _, f := range sc.forbid {
		if strings.Contains(completion, f) {
			return false, "re-typed the suffix " + strconv.Quote(f)
		}
	}
	for _, want := range sc.allOf {
		if !strings.Contains(lower, strings.ToLower(want)) {
			return false, "missing " + strconv.Quote(want)
		}
	}
	if len(sc.anyOf) > 0 {
		hit := false
		for _, want := range sc.anyOf {
			if strings.Contains(lower, strings.ToLower(want)) {
				hit = true
				break
			}
		}
		if !hit {
			return false, "none of " + strconv.Quote(strings.Join(sc.anyOf, " | "))
		}
	}
	return true, ""
}

func e2eStats(t *testing.T, base string) map[string]interface{} {
	t.Helper()
	res, err := http.Get(base + "/foxxycode/completion/stats")
	if err != nil {
		return map[string]interface{}{}
	}
	defer func() { _ = res.Body.Close() }()
	out := map[string]interface{}{}
	_ = json.NewDecoder(res.Body).Decode(&out)
	return out
}

func e2eEscape(s string) string {
	s = strings.ReplaceAll(s, "\n", "⏎")
	s = strings.ReplaceAll(s, "\t", "→")
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "`", "'")
	if len(s) > 90 {
		s = s[:90] + "…"
	}
	return s
}
