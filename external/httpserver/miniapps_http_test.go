//go:build http && miniapps

package httpserver

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/external/miniapps"
	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

func newMiniAppsHTTPTestServer(t *testing.T) (*httptest.Server, *Server) {
	t.Helper()
	cfg := &config.Config{Paths: config.Paths{Home: t.TempDir(), CWD: t.TempDir()}, Agent: config.Agent{Model: ""}}
	mgr := session.NewManager(cfg, noopSender{}, func(context.Context, *session.State, []acp.ContentBlock, acp.UpdateSender) (string, error) {
		return "", nil
	}, slog.Default(), cfg.Paths.CWD, &session.FileStore{Root: t.TempDir()})
	srv := New(cfg, mgr, slog.Default(), cfg.Paths.CWD)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(func() { ts.Close(); srv.Drain() })
	return ts, srv
}

func miniAppsTestDocument() map[string]any {
	return map[string]any{
		"schema_version": "1.0.0", "kind": "foxxycode.miniapp", "id": "greeting-app",
		"metadata":    map[string]any{"name": "Greeting", "goal": "Write greeting"},
		"workflow":    []any{map[string]any{"id": "step-write", "kind": "tool", "title": "Write", "tool": "write", "arguments": map[string]any{"path": "greeting.txt", "content": "hello"}}},
		"permissions": map[string]any{"tools": []string{"write"}},
		"success":     map[string]any{"mode": "all", "checks": []any{map[string]any{"kind": "step", "step": "step-write", "status": "succeeded"}}},
		"runtime":     map[string]any{"log_scope": "global", "operator_event_level": "status", "diagnostic_tool_events": "sanitized"},
	}
}

func TestMiniAppsCapabilityAndCatalogDraftRevision(t *testing.T) {
	ts, _ := newMiniAppsHTTPTestServer(t)
	capability, err := http.Get(ts.URL + "/foxxycode/capabilities")
	if err != nil {
		t.Fatal(err)
	}
	defer capability.Body.Close()
	if capability.StatusCode != http.StatusOK {
		t.Fatalf("capability status %d", capability.StatusCode)
	}
	var capabilityBody struct {
		Capabilities map[string]bool `json:"capabilities"`
	}
	if err := json.NewDecoder(capability.Body).Decode(&capabilityBody); err != nil {
		t.Fatal(err)
	}
	if !capabilityBody.Capabilities["miniapps"] {
		t.Fatal("miniapps capability was not advertised")
	}
	body, _ := json.Marshal(miniAppsTestDocument())
	response, err := http.Post(ts.URL+"/foxxycode/miniapps", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("create status %d: %s", response.StatusCode, data)
	}
	var app struct {
		Revision string `json:"revision"`
		ID       string `json:"id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&app); err != nil {
		t.Fatal(err)
	}
	if app.ID != "greeting-app" || app.Revision == "" {
		t.Fatalf("created app = %+v", app)
	}
	getDraft, err := http.Get(ts.URL + "/foxxycode/miniapps/greeting-app/draft")
	if err != nil {
		t.Fatal(err)
	}
	var update map[string]any
	if err := json.NewDecoder(getDraft.Body).Decode(&update); err != nil {
		t.Fatal(err)
	}
	getDraft.Body.Close()
	update["revision"] = app.Revision
	update["metadata"] = map[string]any{"name": "Greeting v2", "goal": "Write greeting"}
	updateBody, _ := json.Marshal(update)
	request, _ := http.NewRequest(http.MethodPut, ts.URL+"/foxxycode/miniapps/greeting-app/draft", strings.NewReader(string(updateBody)))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("If-Match", app.Revision)
	updated, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	updated.Body.Close()
	if updated.StatusCode != http.StatusOK {
		t.Fatalf("update status %d", updated.StatusCode)
	}
	stale, _ := http.NewRequest(http.MethodPut, ts.URL+"/foxxycode/miniapps/greeting-app/draft", strings.NewReader(string(updateBody)))
	stale.Header.Set("Content-Type", "application/json")
	stale.Header.Set("If-Match", app.Revision)
	conflict, err := http.DefaultClient.Do(stale)
	if err != nil {
		t.Fatal(err)
	}
	conflict.Body.Close()
	if conflict.StatusCode != http.StatusConflict {
		t.Fatalf("stale update status %d, want 409", conflict.StatusCode)
	}
}

func TestMiniAppsAuthoringSourceOmitsFixtureBytes(t *testing.T) {
	ts, srv := newMiniAppsHTTPTestServer(t)
	var app miniapps.MiniApp
	raw, _ := json.Marshal(miniAppsTestDocument())
	if err := json.Unmarshal(raw, &app); err != nil {
		t.Fatal(err)
	}
	if err := srv.miniAppsHTTPState().store.CreateDraft(app, &miniapps.SourceEvidence{FixtureFiles: map[string][]byte{"secret.txt": []byte("secret fixture")}}); err != nil {
		t.Fatal(err)
	}
	response, err := http.Get(ts.URL + "/foxxycode/miniapps/greeting-app/authoring/source")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("source status=%d body=%s", response.StatusCode, body)
	}
	if strings.Contains(string(body), "c2VjcmV0") || strings.Contains(string(body), "secret fixture") {
		t.Fatalf("fixture bytes leaked in source response: %s", body)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	manifest, _ := decoded["fixture_files"].([]any)
	if len(manifest) != 1 {
		t.Fatalf("fixture manifest=%#v", decoded["fixture_files"])
	}
}

func TestMiniAppsRunEndpointsAcceptAsyncJobIDImmediately(t *testing.T) {
	ts, srv := newMiniAppsHTTPTestServer(t)
	state := srv.miniAppsHTTPState()
	app := miniapps.MiniApp{
		SchemaVersion: miniapps.SchemaVersion, Kind: miniapps.KindMiniApp, ID: "async-run-app", State: miniapps.StateDraft,
		Metadata:    miniapps.Metadata{Name: "Async", Goal: "Run async"},
		Permissions: miniapps.Permissions{Tools: []string{"write"}},
		Workflow:    []miniapps.Step{{ID: "write", Kind: "tool", Title: "Write", Tool: "write", Arguments: map[string]any{"path": "x.txt", "content": "x"}}},
		Success:     miniapps.SuccessSpec{Mode: "all", Checks: []miniapps.SuccessCheck{{Kind: "step", Step: "write", Status: string(miniapps.TraceActionSucceeded)}}},
		Runtime:     miniapps.RuntimePolicy{LogScope: "global", OperatorEventLevel: "status", DiagnosticToolEvents: "sanitized"},
	}
	if err := state.store.CreateDraft(app, &miniapps.SourceEvidence{AcceptedResult: "ignored"}); err != nil {
		t.Fatal(err)
	}
	body := strings.NewReader(`{"inputs":{}}`)
	response, err := http.Post(ts.URL+"/foxxycode/miniapps/async-run-app/test-runs", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("start run status %d: %s", response.StatusCode, data)
	}
	var job struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&job); err != nil {
		t.Fatal(err)
	}
	if job.ID == "" {
		t.Fatal("start run did not return async job id")
	}
	status, err := http.Get(ts.URL + "/foxxycode/miniapp-runs/" + job.ID)
	if err != nil {
		t.Fatal(err)
	}
	status.Body.Close()
	if status.StatusCode != http.StatusOK {
		t.Fatalf("GET by async job id status %d", status.StatusCode)
	}
	events, err := http.Get(ts.URL + "/foxxycode/miniapp-runs/" + job.ID + "/events")
	if err != nil {
		t.Fatal(err)
	}
	eventBody, _ := io.ReadAll(events.Body)
	events.Body.Close()
	if events.StatusCode != http.StatusOK || !strings.Contains(string(eventBody), "data: [DONE]") {
		t.Fatalf("run events status=%d body=%s", events.StatusCode, eventBody)
	}
	if !strings.Contains(string(eventBody), "id: 1\n") {
		t.Fatalf("run events omitted SSE id: %s", eventBody)
	}
}

func TestMiniAppsSSEResumeUsesLastEventID(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/foxxycode/miniapp-runs/run/events", nil)
	request.Header.Set("Last-Event-ID", "17")
	if got := parseMiniAppsAfter(request); got != 17 {
		t.Fatalf("Last-Event-ID parsed as %d", got)
	}
	request.URL.RawQuery = "after=3"
	if got := parseMiniAppsAfter(request); got != 3 {
		t.Fatalf("after query did not override header: %d", got)
	}
}

func TestMiniAppsConfirmationInfersSingleWaitingStep(t *testing.T) {
	ts, srv := newMiniAppsHTTPTestServer(t)
	state := srv.miniAppsHTTPState()
	app := miniapps.MiniApp{
		SchemaVersion: miniapps.SchemaVersion, Kind: miniapps.KindMiniApp, ID: "confirm-run-app", State: miniapps.StateDraft,
		Metadata: miniapps.Metadata{Name: "Confirmation", Goal: "Confirm an action"},
		Workflow: []miniapps.Step{{ID: "confirm", Kind: "confirm", Title: "Approve action", Message: "Approve this action?"}},
		Success:  miniapps.SuccessSpec{Mode: "all", Checks: []miniapps.SuccessCheck{{Kind: "step", Step: "confirm", Status: string(miniapps.RunSucceeded)}}},
		Outputs:  []miniapps.Output{{ID: "result", Type: "boolean", Value: miniapps.Ref{Ref: "steps.confirm.outputs.result"}}},
		Runtime:  miniapps.RuntimePolicy{LogScope: "global", OperatorEventLevel: "status", DiagnosticToolEvents: "sanitized"},
	}
	if err := state.store.CreateDraft(app, &miniapps.SourceEvidence{AcceptedResult: true}); err != nil {
		t.Fatal(err)
	}
	response, err := http.Post(ts.URL+"/foxxycode/miniapps/confirm-run-app/test-runs", "application/json", strings.NewReader(`{"inputs":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	var initial miniapps.AsyncJob
	if err := json.NewDecoder(response.Body).Decode(&initial); err != nil {
		response.Body.Close()
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusAccepted || initial.ID == "" {
		t.Fatalf("start response status=%d job=%+v", response.StatusCode, initial)
	}
	var waiting miniapps.AsyncJob
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, getErr := http.Get(ts.URL + "/foxxycode/miniapp-runs/" + initial.ID)
		if getErr == nil {
			var current miniapps.AsyncJob
			decodeErr := json.NewDecoder(status.Body).Decode(&current)
			status.Body.Close()
			if decodeErr == nil {
				if current.Status == miniapps.JobWaitingForConfirm {
					waiting = current
					break
				}
				if current.Status == miniapps.JobSucceeded || current.Status == miniapps.JobFailed || current.Status == miniapps.JobCancelled || current.Status == miniapps.JobInterrupted {
					t.Fatalf("run became terminal before confirmation: %+v", current)
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if waiting.ID == "" || waiting.RunID == "" {
		t.Fatalf("run did not reach confirmation: %+v", waiting)
	}
	confirmation, err := http.Post(ts.URL+"/foxxycode/miniapp-runs/"+initial.ID+"/confirmation", "application/json", strings.NewReader(`{"approved":true}`))
	if err != nil {
		t.Fatal(err)
	}
	var resumed miniapps.AsyncJob
	_ = json.NewDecoder(confirmation.Body).Decode(&resumed)
	confirmation.Body.Close()
	if confirmation.StatusCode != http.StatusAccepted {
		t.Fatalf("confirmation status=%d job=%+v", confirmation.StatusCode, resumed)
	}
	if resumed.Status == miniapps.JobWaitingForConfirm {
		t.Fatalf("confirmation did not resume run: %+v", resumed)
	}
}

func TestMiniAppsDiffArtifactsFollowTranscriptTurns(t *testing.T) {
	dir := t.TempDir()
	if err := session.StoreWorkspaceDiff(dir, 1, &session.WorkspaceDiff{Changes: []session.WorkspaceChange{
		{Path: "a.txt", After: &session.WorkspaceFile{Content: []byte("old")}},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := session.StoreWorkspaceDiff(dir, 2, &session.WorkspaceDiff{Changes: []session.WorkspaceChange{
		{Path: "b.txt", After: &session.WorkspaceFile{Content: []byte("new")}},
	}}); err != nil {
		t.Fatal(err)
	}
	evidence := []miniapps.TraceCallEvidence{
		{ID: "a", Status: miniapps.TraceActionSucceeded, Arguments: `{"path":"a.txt"}`},
		{ID: "b", Status: miniapps.TraceActionSucceeded, Arguments: `{"path":"b.txt"}`},
	}
	fixtures := addMiniAppsDiffArtifacts(dir, evidence, map[string]int{"a": 1, "b": 2})
	if len(evidence[0].Artifacts) != 1 || evidence[0].Artifacts[0].SizeBytes != 3 {
		t.Fatalf("a artifacts = %#v", evidence[0].Artifacts)
	}
	if len(evidence[1].Artifacts) != 1 || evidence[1].Artifacts[0].Path != "b.txt" || evidence[1].Artifacts[0].SizeBytes != 3 {
		t.Fatalf("b artifacts = %#v", evidence[1].Artifacts)
	}
	if string(fixtures["a.txt"]) != "old" || string(fixtures["b.txt"]) != "new" {
		t.Fatalf("fixture files = %#v", fixtures)
	}
}

func TestMiniAppsTraceEvidenceFallsBackToPersistedMessages(t *testing.T) {
	dir := t.TempDir()
	if err := session.StoreWorkspaceDiff(dir, 1, &session.WorkspaceDiff{Changes: []session.WorkspaceChange{{
		Path: "from-message.txt", After: &session.WorkspaceFile{Content: []byte("fixture")},
	}}}); err != nil {
		t.Fatal(err)
	}
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "write a file"},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "message-call", Name: "write", InputJSON: `{"path":"from-message.txt","content":"fixture"}`}}},
		{Role: llm.RoleTool, ToolCallID: "message-call", Content: "wrote file"},
	}
	evidence, fixtures := readMiniAppsTraceEvidence(dir, messages)
	if len(evidence) != 1 || evidence[0].Status != miniapps.TraceActionSucceeded {
		t.Fatalf("message evidence = %#v", evidence)
	}
	if len(evidence[0].Artifacts) != 1 || string(fixtures["from-message.txt"]) != "fixture" {
		t.Fatalf("message artifacts=%#v fixtures=%#v", evidence[0].Artifacts, fixtures)
	}
}
