//go:build http

package httpserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/project"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// TestHelperMCPProjectCWD is re-executed as a minimal stdio MCP server. Its
// single tool is named after the expanded MCP_PROJECT_CWD value so the parent
// test can assert which workspace the management probe used.
func TestHelperMCPProjectCWD(t *testing.T) {
	if os.Getenv("GO_WANT_MCP_PROJECT_HELPER") != "1" {
		t.Skip("helper process")
	}
	in := bufio.NewReader(os.Stdin)
	out := bufio.NewWriter(os.Stdout)
	respond := func(id interface{}, result interface{}) {
		data, _ := json.Marshal(map[string]interface{}{"jsonrpc": "2.0", "id": id, "result": result})
		_, _ = out.Write(append(data, '\n'))
		_ = out.Flush()
	}
	for {
		line, err := in.ReadBytes('\n')
		if err != nil {
			os.Exit(0)
		}
		var req struct {
			ID     interface{} `json:"id"`
			Method string      `json:"method"`
		}
		if err := json.Unmarshal(line, &req); err != nil || req.ID == nil {
			continue
		}
		switch req.Method {
		case "initialize":
			respond(req.ID, map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]interface{}{},
				"serverInfo":      map[string]interface{}{"name": "project-cwd", "version": "0.0.1"},
			})
		case "tools/list":
			respond(req.ID, map[string]interface{}{"tools": []map[string]interface{}{{
				"name":        filepath.Base(os.Getenv("MCP_PROJECT_CWD")),
				"description": "Reports the workspace used by the probe",
				"inputSchema": map[string]interface{}{"type": "object"},
			}}})
		default:
			respond(req.ID, nil)
		}
	}
}

func TestMCPManagementUsesCurrentProjectWorkspace(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	alpha := filepath.Join(root, "alpha")
	beta := filepath.Join(root, "beta")
	for _, dir := range []string{home, alpha, beta} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	configPath := filepath.Join(home, "config.yaml")
	if err := os.WriteFile(configPath, []byte("skills:\n  sources: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadWithPaths(config.Paths{Home: home, CWD: alpha, ConfigPath: configPath})
	if err != nil {
		t.Fatal(err)
	}
	projectServer := config.MCPJSONServer{
		Command: os.Args[0],
		Args:    []string{"-test.run=TestHelperMCPProjectCWD"},
		Env: map[string]string{
			"GO_WANT_MCP_PROJECT_HELPER": "1",
			"MCP_PROJECT_CWD":            "${CWD}",
		},
	}
	for _, cwd := range []string{alpha, beta} {
		if err := config.UpsertMCPJSONServer(config.MCPJSONPath(cwd), "current-project", projectServer); err != nil {
			t.Fatal(err)
		}
	}

	ps, err := project.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := ps.SetCurrent(alpha); err != nil {
		t.Fatal(err)
	}
	runner := func(context.Context, *session.State, []acp.ContentBlock, acp.UpdateSender) (string, error) {
		return string(acp.StopReasonEndTurn), nil
	}
	mgr := session.NewManager(cfg, noopSender{}, runner, slog.Default(), alpha, nil)
	srv := New(cfg, mgr, slog.Default(), alpha)
	srv.AttachProjectStore(ps)

	// A project declaration is gated per workspace, so each one is approved
	// separately: alpha's approval must not carry over to beta, even though
	// both files declare the same server name.
	assertMCPAwaitingApproval(t, srv)
	approveMCPServer(t, srv, "current-project")
	assertMCPManagementWorkspace(t, srv, alpha)

	if err := ps.SetCurrent(beta); err != nil {
		t.Fatal(err)
	}
	assertMCPAwaitingApproval(t, srv)
	approveMCPServer(t, srv, "current-project")
	assertMCPManagementWorkspace(t, srv, beta)

	putBody, _ := json.Marshal(config.MCPJSONServer{Command: "not-run", Disabled: true})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(
		http.MethodPut,
		"/foxxycode/mcp/created?scope=local",
		bytes.NewReader(putBody),
	))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT local MCP status = %d, body = %s", rec.Code, rec.Body.String())
	}
	assertMCPJSONContains(t, config.MCPJSONPath(beta), "created", true)
	assertMCPJSONContains(t, config.MCPJSONPath(alpha), "created", false)

	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(
		http.MethodPost,
		"/foxxycode/mcp/current-project/tools/"+filepath.Base(beta)+"/disable",
		nil,
	))
	if rec.Code != http.StatusOK {
		t.Fatalf("disable current-project tool status = %d, body = %s", rec.Code, rec.Body.String())
	}
	betaEntries, err := config.ReadMCPJSONFile(config.MCPJSONPath(beta))
	if err != nil {
		t.Fatal(err)
	}
	if got := betaEntries["current-project"].DisabledTools; len(got) != 1 || got[0] != filepath.Base(beta) {
		t.Fatalf("disabled tools = %v, want %q", got, filepath.Base(beta))
	}

	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/foxxycode/mcp/created", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE local MCP status = %d, body = %s", rec.Code, rec.Body.String())
	}
	assertMCPJSONContains(t, config.MCPJSONPath(beta), "created", false)
}

// assertMCPAwaitingApproval pins that an unapproved project declaration is
// reported rather than probed: the whole point of the gate is that listing the
// servers must not start the checkout's command.
func assertMCPAwaitingApproval(t *testing.T, srv *Server) {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/foxxycode/mcp", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /foxxycode/mcp status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var list struct {
		Items []struct {
			Name    string       `json:"name"`
			Status  string       `json:"status"`
			Trusted bool         `json:"trusted"`
			Gated   bool         `json:"gated"`
			Tools   []mcpToolRow `json:"tools"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("MCP rows = %+v, want current project only", list.Items)
	}
	row := list.Items[0]
	if row.Status != "needs_approval" || row.Trusted || !row.Gated {
		t.Fatalf("unapproved project row = %+v", row)
	}
	if len(row.Tools) != 0 {
		t.Fatalf("unapproved server was probed: tools = %+v", row.Tools)
	}
}

func approveMCPServer(t *testing.T, srv *Server, name string) {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/foxxycode/mcp/"+name+"/trust", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST trust %s status = %d, body = %s", name, rec.Code, rec.Body.String())
	}
}

func assertMCPManagementWorkspace(t *testing.T, srv *Server, workspace string) {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/foxxycode/mcp", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /foxxycode/mcp status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var list struct {
		Items []struct {
			Name   string       `json:"name"`
			Origin string       `json:"origin"`
			Status string       `json:"status"`
			Tools  []mcpToolRow `json:"tools"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("MCP rows = %+v, want current project only", list.Items)
	}
	row := list.Items[0]
	if row.Name != "current-project" || row.Origin != "project" || row.Status != "connected" {
		t.Fatalf("current project row = %+v", row)
	}
	wantTool := filepath.Base(workspace)
	if len(row.Tools) != 1 || row.Tools[0].Name != wantTool {
		t.Fatalf("probe tools = %+v, want workspace %q", row.Tools, wantTool)
	}
}

// TestMCPPutRoundTripsInsecureSkipVerify pins the whole path the settings
// checkbox drives: the flag survives the PUT into mcp.json, comes back on the
// row so the box stays ticked, and actually reaches the transport — the server
// here presents a certificate no root signed, so it only connects with the
// flag set.
func TestMCPPutRoundTripsInsecureSkipVerify(t *testing.T) {
	selfSigned := httptest.NewTLSServer(&fakeBetaMCPHandler{token: "tok"})
	defer selfSigned.Close()

	home := t.TempDir()
	cwd := t.TempDir()
	configPath := filepath.Join(home, "config.yaml")
	if err := os.WriteFile(configPath, []byte("skills:\n  sources: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadWithPaths(config.Paths{Home: home, CWD: cwd, ConfigPath: configPath})
	if err != nil {
		t.Fatal(err)
	}
	runner := func(context.Context, *session.State, []acp.ContentBlock, acp.UpdateSender) (string, error) {
		return string(acp.StopReasonEndTurn), nil
	}
	mgr := session.NewManager(cfg, noopSender{}, runner, slog.Default(), cwd, nil)
	srv := New(cfg, mgr, slog.Default(), cwd)

	// Global scope keeps the entry out of the workspace trust gate, so the row
	// reports the declaration instead of asking for approval first.
	putBody, _ := json.Marshal(config.MCPJSONServer{
		URL:                selfSigned.URL,
		InsecureSkipVerify: true,
	})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(
		http.MethodPut, "/foxxycode/mcp/selfsigned?scope=global", bytes.NewReader(putBody)))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", rec.Code, rec.Body.String())
	}

	entries, err := config.ReadMCPJSONFile(config.GlobalMCPJSONPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if !entries["selfsigned"].InsecureSkipVerify {
		t.Fatalf("mcp.json entry = %+v, want insecureSkipVerify true", entries["selfsigned"])
	}

	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/foxxycode/mcp", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var list struct {
		Items []struct {
			Name               string       `json:"name"`
			Status             string       `json:"status"`
			Error              string       `json:"error,omitempty"`
			InsecureSkipVerify bool         `json:"insecure_skip_verify"`
			Tools              []mcpToolRow `json:"tools"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 || list.Items[0].Name != "selfsigned" {
		t.Fatalf("rows = %+v, want the selfsigned server", list.Items)
	}
	row := list.Items[0]
	if !row.InsecureSkipVerify {
		t.Error("row must report insecure_skip_verify so the checkbox renders ticked")
	}
	if row.Status != "connected" {
		t.Fatalf("status = %q (%s), want connected through the self-signed certificate", row.Status, row.Error)
	}
	if len(row.Tools) != 1 || row.Tools[0].Name != "get_token" {
		t.Fatalf("tools = %+v, want get_token", row.Tools)
	}
}

func assertMCPJSONContains(t *testing.T, path, name string, want bool) {
	t.Helper()
	entries, err := config.ReadMCPJSONFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_, got := entries[name]
	if got != want {
		t.Fatalf("%s contains %q = %v, want %v", path, name, got, want)
	}
}
