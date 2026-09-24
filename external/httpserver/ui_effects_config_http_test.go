//go:build http

package httpserver

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// The "Animations and translucency" switch in Settings -> Appearance saves ui.effects through
// PUT /foxxycode/config, and every client reads it back from GET at startup. Turning the
// effects off must survive both the write to config.yaml and a later footer Save, which PUTs
// the whole document it fetched.
func TestUIEffectsConfigRoundTripsThroughTheSettingsAPI(t *testing.T) {
	home := t.TempDir()
	cfgPath := filepath.Join(home, "config.yaml")
	yml := `
providers:
  - name: openai
    type: openai
    api_key: "k"

models:
  - model: "openai/gpt-4o"
    max_tokens: 4096

agent:
  model: "openai/gpt-4o"
`
	if err := os.WriteFile(cfgPath, []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	runner := func(context.Context, *session.State, []acp.ContentBlock, acp.UpdateSender) (string, error) {
		return "", nil
	}
	mgr := session.NewManager(cfg, noopSender{}, runner, slog.Default(), home, nil)
	srv := New(cfg, mgr, slog.Default(), home)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	getUI := func() map[string]interface{} {
		t.Helper()
		res, err := http.Get(ts.URL + "/foxxycode/config")
		if err != nil {
			t.Fatal(err)
		}
		b, _ := ioReadAllClose(res.Body)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("GET /foxxycode/config status %d %s", res.StatusCode, b)
		}
		var doc map[string]interface{}
		if err := json.Unmarshal(b, &doc); err != nil {
			t.Fatal(err)
		}
		ui, _ := doc["ui"].(map[string]interface{})
		return ui
	}
	put := func(body string) {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPut, ts.URL+"/foxxycode/config", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := ioReadAllClose(res.Body)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("PUT /foxxycode/config status %d %s", res.StatusCode, b)
		}
	}

	// Unset: every client falls back to its own default.
	if _, ok := getUI()["effects"]; ok {
		t.Fatalf("ui.effects present before anything set it: %v", getUI())
	}

	// The switch turned off: reduced effects everywhere.
	put(`{"providers":[{"name":"openai","type":"openai","api_key":"k"}],` +
		`"models":[{"model":"openai/gpt-4o","max_tokens":4096}],` +
		`"agent":{"model":"openai/gpt-4o"},"ui":{"effects":false}}`)
	if got, ok := getUI()["effects"]; !ok || got != false {
		t.Fatalf("GET after PUT: ui.effects = %v (present %v), want false", got, ok)
	}
	onDisk, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if onDisk.UI.Effects == nil || *onDisk.UI.Effects {
		t.Fatalf("config.yaml ui.effects = %v, want false", onDisk.UI.Effects)
	}

	// A footer Save sends back the whole document it fetched; the value must stay.
	res, err := http.Get(ts.URL + "/foxxycode/config")
	if err != nil {
		t.Fatal(err)
	}
	whole, _ := ioReadAllClose(res.Body)
	put(string(whole))
	if got := getUI()["effects"]; got != false {
		t.Fatalf("ui.effects after a whole-document save = %v, want false", got)
	}

	// And back on.
	var doc map[string]interface{}
	if err := json.Unmarshal(whole, &doc); err != nil {
		t.Fatal(err)
	}
	doc["ui"].(map[string]interface{})["effects"] = true
	on, _ := json.Marshal(doc)
	put(string(on))
	if got := getUI()["effects"]; got != true {
		t.Fatalf("ui.effects after turning the switch on = %v, want true", got)
	}
}
