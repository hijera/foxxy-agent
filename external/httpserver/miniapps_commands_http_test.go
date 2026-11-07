//go:build http && miniapps

package httpserver

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/external/miniapps"
	"github.com/hijera/foxxycode-agent/internal/cmdprofile/cmdtest"
)

// commandsTestDocument is miniAppsTestDocument with a command-profile step and
// its embedded declaration.
func commandsTestDocument() map[string]any {
	doc := miniAppsTestDocument()
	doc["permissions"] = map[string]any{"tools": []string{"cmd_fakeenc_convert"}}
	doc["workflow"] = []any{map[string]any{
		"id": "convert", "kind": "tool", "title": "Convert", "tool": "cmd_fakeenc_convert",
		"arguments": map[string]any{"input_path": "in.mp4", "output_path": "out.mp3"},
	}}
	doc["success"] = map[string]any{"mode": "all", "checks": []any{map[string]any{
		"kind": "step", "step": "convert", "status": "succeeded",
	}}}
	doc["requirements"] = map[string]any{"commands": []any{map[string]any{
		"name": "fakeenc_convert", "binary": "fakeenc", "permission": "allow",
		"template": []any{"-i", "{input_path}", "{output_path}"},
		"params": []any{
			map[string]any{"name": "input_path", "type": "file"},
			map[string]any{"name": "output_path", "type": "file"},
		},
	}}}
	return doc
}

func TestMiniAppsCommandsStatusAndTrustEndpoints(t *testing.T) {
	fake, err := cmdtest.Build(t.TempDir(), "fakeenc")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(fake.Binary)+string(os.PathListSeparator)+os.Getenv("PATH"))
	ts, srv := newMiniAppsHTTPTestServer(t)

	body, _ := json.Marshal(commandsTestDocument())
	created, err := http.Post(ts.URL+"/foxxycode/miniapps", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	createdBody, _ := io.ReadAll(created.Body)
	_ = created.Body.Close()
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("create status %d: %s", created.StatusCode, createdBody)
	}

	statusOf := func() map[string]any {
		t.Helper()
		response, err := http.Get(ts.URL + "/foxxycode/miniapps/greeting-app/commands")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusOK {
			raw, _ := io.ReadAll(response.Body)
			t.Fatalf("commands status %d: %s", response.StatusCode, raw)
		}
		var payload struct {
			Items []map[string]any `json:"items"`
		}
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if len(payload.Items) != 1 {
			t.Fatalf("items = %#v", payload.Items)
		}
		return payload.Items[0]
	}

	before := statusOf()
	if before["name"] != "fakeenc_convert" || before["source"] != "document" {
		t.Fatalf("row = %#v", before)
	}
	if before["installed"] != true {
		t.Fatalf("row = %#v, want installed via PATH", before)
	}
	if before["trusted"] == true {
		t.Fatal("a fresh document profile must not be trusted")
	}
	if _, hasHash := before["hash"].(string); !hasHash {
		t.Fatalf("row lacks the hash: %#v", before)
	}

	trust, err := http.Post(ts.URL+"/foxxycode/miniapps/greeting-app/commands/fakeenc_convert/trust",
		"application/json", strings.NewReader(`{"approved":true}`))
	if err != nil {
		t.Fatal(err)
	}
	trustBody, _ := io.ReadAll(trust.Body)
	_ = trust.Body.Close()
	if trust.StatusCode != http.StatusOK {
		t.Fatalf("trust status %d: %s", trust.StatusCode, trustBody)
	}
	after := statusOf()
	if after["trusted"] != true {
		t.Fatalf("row after trust = %#v", after)
	}
	_ = srv
}

func TestMiniAppsCommandConfirmationAcceptsPersistedRunID(t *testing.T) {
	fake, err := cmdtest.Build(t.TempDir(), "fakeenc")
	if err != nil {
		t.Fatal(err)
	}
	fake.Setenv(t.Setenv)
	t.Setenv("PATH", filepath.Dir(fake.Binary)+string(os.PathListSeparator)+os.Getenv("PATH"))
	ts, srv := newMiniAppsHTTPTestServer(t)
	raw, _ := json.Marshal(commandsTestDocument())
	var app miniapps.MiniApp
	if err := json.Unmarshal(raw, &app); err != nil {
		t.Fatal(err)
	}
	state := srv.miniAppsHTTPState()
	if err := state.store.CreateDraft(app, nil); err != nil {
		t.Fatal(err)
	}
	draft, err := state.store.GetDraft(app.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.store.RecordPassingTest(app.ID, miniapps.Run{
		ID: "test-before-release", AppID: app.ID, Revision: draft.Revision, Test: true, Status: miniapps.RunSucceeded,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := state.store.Release(app.ID, "1.0.0", miniapps.ReleaseOptions{Approved: true, ExpectedRevision: draft.Revision}); err != nil {
		t.Fatal(err)
	}

	response, err := http.Post(ts.URL+"/foxxycode/miniapps/greeting-app/versions/1.0.0/runs", "application/json", strings.NewReader(`{"inputs":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	var job miniapps.AsyncJob
	if err := json.NewDecoder(response.Body).Decode(&job); err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		job, err = state.service.GetJob(job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if job.Status == miniapps.JobWaitingForConfirm {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if job.Status != miniapps.JobWaitingForConfirm || job.RunID == "" {
		t.Fatalf("run did not wait for trust: %+v", job)
	}

	confirmed, err := http.Post(ts.URL+"/foxxycode/miniapp-runs/"+job.RunID+"/confirmation", "application/json", strings.NewReader(`{"approved":true}`))
	if err != nil {
		t.Fatal(err)
	}
	confirmationBody, _ := io.ReadAll(confirmed.Body)
	_ = confirmed.Body.Close()
	if confirmed.StatusCode != http.StatusAccepted {
		t.Fatalf("confirmation status %d: %s", confirmed.StatusCode, confirmationBody)
	}
	for time.Now().Before(deadline) {
		job, err = state.service.GetJob(job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if job.Status == miniapps.JobSucceeded || job.Status == miniapps.JobFailed || job.Status == miniapps.JobCancelled || job.Status == miniapps.JobInterrupted {
			break
		}
		if job.Status == miniapps.JobWaitingForConfirm {
			t.Fatalf("run waited for the same trust again: %+v", job)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if job.Status != miniapps.JobSucceeded {
		t.Fatalf("confirmed run = %+v", job)
	}
	if calls, err := fake.Calls(); err != nil || len(calls) != 1 {
		t.Fatalf("fake calls = %#v, error %v", calls, err)
	}
}

func TestMiniAppsCommandsTrustRequiresAnInstalledBinary(t *testing.T) {
	ts, _ := newMiniAppsHTTPTestServer(t)
	doc := commandsTestDocument()
	requirements := doc["requirements"].(map[string]any)
	profile := requirements["commands"].([]any)[0].(map[string]any)
	profile["binary"] = "definitely-not-installed-xyz"
	body, _ := json.Marshal(doc)
	created, err := http.Post(ts.URL+"/foxxycode/miniapps", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	_ = created.Body.Close()

	trust, err := http.Post(ts.URL+"/foxxycode/miniapps/greeting-app/commands/fakeenc_convert/trust",
		"application/json", strings.NewReader(`{"approved":true}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(trust.Body)
	_ = trust.Body.Close()
	if trust.StatusCode != http.StatusConflict || !strings.Contains(string(raw), "binary_missing") {
		t.Fatalf("trust status %d: %s", trust.StatusCode, raw)
	}
}
