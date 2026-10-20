//go:build http && miniapps

package httpserver

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/cmdprofile/cmdtest"
)

// installTestManager builds a recording fake named like this platform's
// package manager, puts it on PATH, and returns its id.
func installTestManager(t *testing.T) (cmdtest.Fake, string) {
	t.Helper()
	binary, managerID := "apt-get", "apt"
	switch runtime.GOOS {
	case "windows":
		binary, managerID = "scoop", "scoop"
	case "darwin":
		binary, managerID = "brew", "brew"
	}
	fake, err := cmdtest.Build(t.TempDir(), binary)
	if err != nil {
		t.Fatal(err)
	}
	fake.Setenv(t.Setenv)
	t.Setenv("PATH", filepath.Dir(fake.Binary)+string(os.PathListSeparator)+os.Getenv("PATH"))
	return fake, managerID
}

// installTestDocument declares every manager coordinate, so the platform's
// detected manager always has one.
func installTestDocument() map[string]any {
	doc := commandsTestDocument()
	requirements := doc["requirements"].(map[string]any)
	profile := requirements["commands"].([]any)[0].(map[string]any)
	profile["install"] = map[string]any{
		"winget": "fakeenc", "scoop": "fakeenc", "brew": "fakeenc", "apt": "fakeenc", "dnf": "fakeenc",
	}
	return doc
}

func TestMiniAppsCommandInstallEndpointRunsTheManager(t *testing.T) {
	fake, managerID := installTestManager(t)
	ts, _ := newMiniAppsHTTPTestServer(t)
	body, _ := json.Marshal(installTestDocument())
	created, err := http.Post(ts.URL+"/foxxycode/miniapps", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	_ = created.Body.Close()

	start, err := http.Post(ts.URL+"/foxxycode/miniapps/greeting-app/commands/fakeenc_convert/install",
		"application/json", strings.NewReader(`{"manager":"`+managerID+`","approved":true}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(start.Body)
	_ = start.Body.Close()
	if start.StatusCode != http.StatusAccepted {
		t.Fatalf("install status %d: %s", start.StatusCode, raw)
	}
	var job map[string]any
	if err := json.Unmarshal(raw, &job); err != nil {
		t.Fatal(err)
	}
	jobID, _ := job["id"].(string)

	deadline := time.Now().Add(10 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("install job did not finish")
		}
		response, err := http.Get(ts.URL + "/foxxycode/miniapp-command-installs/" + jobID)
		if err != nil {
			t.Fatal(err)
		}
		var current map[string]any
		decodeErr := json.NewDecoder(response.Body).Decode(&current)
		_ = response.Body.Close()
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		status, _ := current["status"].(string)
		if status == "succeeded" {
			break
		}
		if status == "failed" || status == "cancelled" || status == "interrupted" {
			t.Fatalf("install job = %v", current)
		}
		time.Sleep(25 * time.Millisecond)
	}
	calls, err := fake.Calls()
	if err != nil || len(calls) != 1 {
		t.Fatalf("manager calls = %#v, err %v", calls, err)
	}
	if !strings.Contains(strings.Join(calls[0].Args, " "), "fakeenc") {
		t.Fatalf("manager argv = %#v", calls[0].Args)
	}
}

func TestMiniAppsCommandInstallRejectsAnUndetectedManager(t *testing.T) {
	ts, _ := newMiniAppsHTTPTestServer(t)
	body, _ := json.Marshal(installTestDocument())
	created, err := http.Post(ts.URL+"/foxxycode/miniapps", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	_ = created.Body.Close()

	start, err := http.Post(ts.URL+"/foxxycode/miniapps/greeting-app/commands/fakeenc_convert/install",
		"application/json", strings.NewReader(`{"manager":"no-such-manager","approved":true}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(start.Body)
	_ = start.Body.Close()
	if start.StatusCode != http.StatusConflict || !strings.Contains(string(raw), "manager_unavailable") {
		t.Fatalf("install status %d: %s", start.StatusCode, raw)
	}
}

func TestMiniAppsCommandInstallRequiresApproval(t *testing.T) {
	_, managerID := installTestManager(t)
	ts, _ := newMiniAppsHTTPTestServer(t)
	body, _ := json.Marshal(installTestDocument())
	created, err := http.Post(ts.URL+"/foxxycode/miniapps", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	_ = created.Body.Close()

	start, err := http.Post(ts.URL+"/foxxycode/miniapps/greeting-app/commands/fakeenc_convert/install",
		"application/json", strings.NewReader(`{"manager":"`+managerID+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(start.Body)
	_ = start.Body.Close()
	if start.StatusCode != http.StatusBadRequest || !strings.Contains(string(raw), "approval_required") {
		t.Fatalf("install status %d: %s", start.StatusCode, raw)
	}
}
