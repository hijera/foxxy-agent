package config_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/config"
)

// uiSchemaFixturePath is the committed snapshot the SPA reads to guard that every
// schema title/description/enum has a Russian translation (schemaStrings.test.ts).
var uiSchemaFixturePath = filepath.Join(
	"..", "..", "external", "ui", "src", "ui", "i18n", "__fixtures__", "ui-schema.json",
)

// TestUISchemaFixtureMatches keeps external/ui/.../ui-schema.json in sync with
// UISchemaMap(). When the config UI schema changes, regenerate the fixture with:
//
//	UPDATE_UI_SCHEMA_FIXTURE=1 go test ./internal/config -run TestUISchemaFixtureMatches
//
// then add the new/changed strings to messages/schema.ru.ts so the SPA coverage
// test stays green.
func TestUISchemaFixtureMatches(t *testing.T) {
	got, err := json.MarshalIndent(config.UISchemaMap(), "", "  ")
	if err != nil {
		t.Fatalf("marshal ui schema: %v", err)
	}
	got = append(got, '\n')

	if os.Getenv("UPDATE_UI_SCHEMA_FIXTURE") == "1" {
		if err := os.MkdirAll(filepath.Dir(uiSchemaFixturePath), 0o755); err != nil {
			t.Fatalf("mkdir fixture dir: %v", err)
		}
		if err := os.WriteFile(uiSchemaFixturePath, got, 0o644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		return
	}

	want, err := os.ReadFile(uiSchemaFixturePath)
	if err != nil {
		t.Fatalf("read fixture (regenerate with UPDATE_UI_SCHEMA_FIXTURE=1): %v", err)
	}
	if !bytes.Equal(bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n")), got) {
		t.Fatalf("ui-schema.json is stale; regenerate with UPDATE_UI_SCHEMA_FIXTURE=1 " +
			"and update messages/schema.ru.ts")
	}
}
