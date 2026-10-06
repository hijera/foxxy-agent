//go:build http

package httpserver

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// browserSectionFromSchemaEndpoint fetches GET /foxxycode/config/schema and returns
// the browser section of the served document.
func browserSectionFromSchemaEndpoint(t *testing.T) map[string]interface{} {
	t.Helper()
	cfg := &config.Config{Paths: config.Paths{Home: t.TempDir()}}
	runner := func(context.Context, *session.State, []acp.ContentBlock, acp.UpdateSender) (string, error) {
		return "", nil
	}
	mgr := session.NewManager(cfg, noopSender{}, runner, slog.Default(), t.TempDir(), nil)
	srv := New(cfg, mgr, slog.Default(), t.TempDir())

	rec := httptest.NewRecorder()
	srv.foxxycodeConfigSchemaGet(rec, httptest.NewRequest(http.MethodGet, "/foxxycode/config/schema", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("schema endpoint: HTTP %d", rec.Code)
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	props, ok := doc["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("schema has no properties")
	}
	browser, ok := props["browser"].(map[string]interface{})
	if !ok {
		t.Fatal("schema has no browser section")
	}
	return browser
}

// TestConfigSchemaAlwaysNamesTheBrowserBuildTag keeps the requirement visible in the
// form regardless of how this binary was built: the section is always present and
// always says which tag it needs, so the setting never looks like a plain toggle
// that simply does not work.
func TestConfigSchemaAlwaysNamesTheBrowserBuildTag(t *testing.T) {
	browser := browserSectionFromSchemaEndpoint(t)
	if got := browser["x-foxxycode-requires-build-tag"]; got != "browser" {
		t.Errorf("x-foxxycode-requires-build-tag = %v, want \"browser\"", got)
	}
}

// TestConfigSchemaMarksBrowserUnavailableWithoutTheTag is the actual gate: the served
// document must state whether THIS binary can run the tool, so the form can disable
// the controls instead of letting the user turn on something that is not compiled in.
// The expectation follows the build tags of the test binary itself.
func TestConfigSchemaMarksBrowserUnavailableWithoutTheTag(t *testing.T) {
	browser := browserSectionFromSchemaEndpoint(t)
	missing, present := browser["x-foxxycode-build-tag-missing"]
	if config.BrowserToolCompiled {
		if present {
			t.Errorf("built WITH -tags browser, but the schema still marks it missing (%v)", missing)
		}
		return
	}
	if missing != true {
		t.Errorf("built WITHOUT -tags browser: x-foxxycode-build-tag-missing = %v (present=%v), want true",
			missing, present)
	}
}
