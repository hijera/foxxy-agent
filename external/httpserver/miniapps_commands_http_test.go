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
