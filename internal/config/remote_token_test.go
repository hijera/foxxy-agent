package config_test

import (
	"encoding/json"
	"github.com/hijera/foxxycode-agent/internal/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHTTPRemoteTokenLoadsFromTheEnvironmentAndRoundTrips(t *testing.T) {
	t.Setenv("FOXXYCODE_TEST_RELAY_TOKEN", "relay-client")
	dir := t.TempDir()
	f := filepath.Join(dir, "config.yaml")
	yaml := httpAuthBaseYAML +
		"httpserver:\n" +
		"  remotes:\n" +
		"    - name: office-relay\n" +
		"      url: http://relay.lan:12346\n" +
		"      token: \"  ${FOXXYCODE_TEST_RELAY_TOKEN}  \"\n" +
		"    - name: nas02\n" +
		"      url: https://nas02:12345\n"
	if err := os.WriteFile(f, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(f)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.HTTPServer.Remotes[0].Token; got != "relay-client" {
		t.Fatalf("remote token = %q, want the environment's value, trimmed", got)
	}
	if got := cfg.HTTPServer.Remotes[1].Token; got != "" {
		t.Fatalf("a remote without a token got %q", got)
	}
	raw, err := json.Marshal(config.ConfigToJSONDTO(cfg))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"token":"relay-client"`) {
		t.Fatalf("the config document does not carry the remote's token: %s", raw)
	}
	if strings.Count(string(raw), `"token"`) != 1 {
		t.Fatalf("a remote without a token gained an empty one: %s", raw)
	}
	back, err := config.ParseConfigJSONPreservingSecrets(raw, cfg.Paths, cfg)
	if err != nil {
		t.Fatalf("round-trip parse: %v", err)
	}
	if got := back.HTTPServer.Remotes[0].Token; got != "relay-client" {
		t.Fatalf("remote token lost in round-trip: %q", got)
	}
}
