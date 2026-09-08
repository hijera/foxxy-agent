//go:build http

package httpserver

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/project"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// subagentRoutesFixture stands up a server whose process cwd is one workspace
// while a second one holds a different project definition, so a test can tell
// which workspace a route answered for.
type subagentRoutesFixture struct {
	ts       *httptest.Server
	srv      *Server
	home     string
	procWS   string
	otherWS  string
	projects *project.Store
}

func newSubagentRoutesFixture(t *testing.T) *subagentRoutesFixture {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	sessRoot := filepath.Join(root, "sessions")
	procWS := filepath.Join(root, "proc-ws")
	otherWS := filepath.Join(root, "other-ws")
	for _, dir := range []string{filepath.Join(home, "agents"), sessRoot, filepath.Join(procWS, ".foxxycode", "agents"), filepath.Join(otherWS, ".foxxycode", "agents")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Two project definitions with different names: whichever shows up names
	// the workspace the route resolved.
	write := func(dir, name, body string) {
		if err := os.WriteFile(filepath.Join(dir, ".foxxycode", "agents", name+".md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(procWS, "proc-only", "---\ndescription: lives in the process workspace\n---\nGo.\n")
	write(otherWS, "bounded", "---\ndescription: lives in the project workspace\n"+
		"tools: read, grep\ndisallowed_tools: run_command\npermission_mode: ask\ntimeout_seconds: 120\nmax_turns: 7\nbackground: true\n---\n"+
		"You review code and report findings.\n")

	runner := func(context.Context, *session.State, []acp.ContentBlock, acp.UpdateSender) (string, error) {
		return string(acp.StopReasonEndTurn), nil
	}
	cfg := &config.Config{
		Paths:     config.Paths{Home: home, CWD: procWS},
		Models:    []config.ModelEntry{{Model: "openai/gpt-4o", MaxTokens: 100, Temperature: 0.2}},
		Agent:     config.Agent{Model: "openai/gpt-4o"},
		Subagents: config.Subagents{Dirs: config.DefaultSubagentDirs()},
	}
	mgr := session.NewManager(cfg, noopSender{}, runner, slog.Default(), procWS, &session.FileStore{Root: sessRoot})
	srv := New(cfg, mgr, slog.Default(), procWS)
	t.Cleanup(srv.Drain)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	ps, err := project.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	return &subagentRoutesFixture{ts: ts, srv: srv, home: home, procWS: procWS, otherWS: otherWS, projects: ps}
}

func (f *subagentRoutesFixture) names(t *testing.T, body map[string]interface{}) []string {
	t.Helper()
	items, _ := body["items"].([]interface{})
	out := make([]string, 0, len(items))
	for _, raw := range items {
		if row, ok := raw.(map[string]interface{}); ok {
			name, _ := row["name"].(string)
			out = append(out, name)
		}
	}
	return out
}

func (f *subagentRoutesFixture) row(t *testing.T, body map[string]interface{}, name string) map[string]interface{} {
	t.Helper()
	items, _ := body["items"].([]interface{})
	for _, raw := range items {
		row, _ := raw.(map[string]interface{})
		if row["name"] == name {
			return row
		}
	}
	t.Fatalf("catalog does not list %q: %v", name, f.names(t, body))
	return nil
}

// A request without cwd must answer for the workspace a new session would get.
// spawn_agent decides trust against the session cwd, so answering for the
// process cwd while a project is set would list one workspace and file the
// receipt under another - an approval the runtime never sees.
func TestSubagentRoutesFollowTheSessionDefaultWorkspace(t *testing.T) {
	f := newSubagentRoutesFixture(t)

	status, body := httpJSON(t, f.ts, http.MethodGet, "/foxxycode/subagents", "", nil)
	if status != http.StatusOK {
		t.Fatalf("catalog status %d", status)
	}
	if got, _ := body["workspace"].(string); filepath.Base(got) != "proc-ws" {
		t.Fatalf("with no project set the catalog answers for the process cwd, got %q", got)
	}

	if err := f.projects.SetCurrent(f.otherWS); err != nil {
		t.Fatal(err)
	}
	f.srv.AttachProjectStore(f.projects)

	status, body = httpJSON(t, f.ts, http.MethodGet, "/foxxycode/subagents", "", nil)
	if status != http.StatusOK {
		t.Fatalf("catalog status %d", status)
	}
	if got, _ := body["workspace"].(string); filepath.Base(got) != "other-ws" {
		t.Fatalf("with a project set the catalog must answer for it, got %q (names %v)", got, f.names(t, body))
	}
	if strings.Join(f.names(t, body), ",") == "" || !strings.Contains(strings.Join(f.names(t, body), ","), "bounded") {
		t.Fatalf("the project workspace's definition is missing: %v", f.names(t, body))
	}
	for _, name := range f.names(t, body) {
		if name == "proc-only" {
			t.Fatalf("the process workspace's definition must not leak into the project catalog: %v", f.names(t, body))
		}
	}

	// The receipt has to land in the same bucket the catalog named.
	status, trusted := httpJSON(t, f.ts, http.MethodPost, "/foxxycode/subagents/bounded/trust", "", nil)
	if status != http.StatusOK {
		t.Fatalf("trust status %d body %v", status, trusted)
	}
	raw, err := os.ReadFile(filepath.Join(f.home, "subagents-trust.json"))
	if err != nil {
		t.Fatal(err)
	}
	var receipts struct {
		Workspaces map[string][]struct {
			Name string `json:"name"`
		} `json:"workspaces"`
	}
	if err := json.Unmarshal(raw, &receipts); err != nil {
		t.Fatal(err)
	}
	for ws, recs := range receipts.Workspaces {
		if filepath.Base(ws) != "other-ws" {
			t.Fatalf("receipt filed under %q, not the project workspace", ws)
		}
		if len(recs) != 1 || recs[0].Name != "bounded" {
			t.Fatalf("receipt records = %+v", recs)
		}
	}
	if len(receipts.Workspaces) != 1 {
		t.Fatalf("receipts = %+v", receipts.Workspaces)
	}
}

// The catalog is what an approval surface reasons about, so it has to name the
// bounds the definition declares - not just its name and description.
func TestSubagentCatalogServesTheDeclaredBounds(t *testing.T) {
	f := newSubagentRoutesFixture(t)
	status, body := httpJSON(t, f.ts, http.MethodGet, "/foxxycode/subagents?cwd="+f.otherWS, "", nil)
	if status != http.StatusOK {
		t.Fatalf("catalog status %d body %v", status, body)
	}
	row := f.row(t, body, "bounded")
	tools, _ := row["tools"].([]interface{})
	if len(tools) != 2 || tools[0] != "read" || tools[1] != "grep" {
		t.Fatalf("tools = %v", row["tools"])
	}
	denied, _ := row["disallowed_tools"].([]interface{})
	if len(denied) != 1 || denied[0] != "run_command" {
		t.Fatalf("disallowed_tools = %v", row["disallowed_tools"])
	}
	if row["permission_mode"] != "ask" || row["background"] != true {
		t.Fatalf("bounds = %v", row)
	}
	if secs, _ := row["timeout_seconds"].(float64); secs != 120 {
		t.Fatalf("timeout_seconds = %v", row["timeout_seconds"])
	}
	if turns, _ := row["max_turns"].(float64); turns != 7 {
		t.Fatalf("max_turns = %v", row["max_turns"])
	}
	if size, _ := row["role_bytes"].(float64); size == 0 {
		t.Fatalf("role_bytes = %v", row["role_bytes"])
	}
	// The role body itself must never be served: this row belongs to a file
	// nobody has approved yet.
	encoded, _ := json.Marshal(row)
	if strings.Contains(string(encoded), "You review code") {
		t.Fatalf("the role body reached the client: %s", encoded)
	}
	// A built-in restricting nothing must not claim an empty allowlist.
	if _, ok := f.row(t, body, "general")["tools"]; ok {
		t.Fatalf("general restricts no tools, so the row must omit the key: %v", f.row(t, body, "general"))
	}
}

// Every error on these routes is read by the SPA with res.json(), so the body
// has to be JSON and say so.
func TestSubagentRouteErrorsAreJSON(t *testing.T) {
	f := newSubagentRoutesFixture(t)
	for _, tc := range []struct {
		name, method, path string
		want               int
	}{
		{"relative cwd", http.MethodGet, "/foxxycode/subagents?cwd=relative/path", http.StatusBadRequest},
		{"unknown name", http.MethodPost, "/foxxycode/subagents/nope/trust", http.StatusNotFound},
		{"builtin", http.MethodPost, "/foxxycode/subagents/general/trust", http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, f.ts.URL+tc.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = res.Body.Close() }()
			if res.StatusCode != tc.want {
				t.Fatalf("status %d, want %d", res.StatusCode, tc.want)
			}
			if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Fatalf("Content-Type %q", ct)
			}
			raw, err := io.ReadAll(res.Body)
			if err != nil {
				t.Fatal(err)
			}
			var parsed struct {
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(raw, &parsed); err != nil {
				t.Fatalf("body is not JSON (%v): %s", err, raw)
			}
			if strings.TrimSpace(parsed.Error.Message) == "" {
				t.Fatalf("error body carries no message: %s", raw)
			}
		})
	}
}
