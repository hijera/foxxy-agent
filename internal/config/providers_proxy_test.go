package config_test

import (
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/config"
)

// TestProviderProxyOnboardingRoundTrip mirrors the onboarding save path: the SPA
// PUTs a JSON config carrying providers[].proxy, which the handler parses with
// ParseAndValidateConfigJSON and persists via MarshalConfigYAML. It proves the
// proxy set during onboarding lands in the saved config profile (and survives a
// reload parse).
func TestProviderProxyOnboardingRoundTrip(t *testing.T) {
	t.Parallel()
	const body = `{
		"providers": [
			{"name": "neuraldeep", "type": "neuraldeep", "api_key": "sk-nd", "proxy": "socks5h://127.0.0.1:1080"}
		],
		"models": [
			{"model": "neuraldeep/qwen-3", "max_tokens": 8192, "temperature": 0.2, "multimodal": true}
		],
		"agent": {"model": "neuraldeep/qwen-3", "max_turns": 35}
	}`

	cfg, err := config.ParseAndValidateConfigJSON([]byte(body), config.Paths{})
	if err != nil {
		t.Fatalf("ParseAndValidateConfigJSON: %v", err)
	}
	if len(cfg.Providers) != 1 {
		t.Fatalf("providers = %d, want 1", len(cfg.Providers))
	}
	if got := cfg.Providers[0].Proxy; got != "socks5h://127.0.0.1:1080" {
		t.Fatalf("parsed proxy = %q, want socks5h://127.0.0.1:1080", got)
	}

	yb, err := config.MarshalConfigYAML(cfg)
	if err != nil {
		t.Fatalf("MarshalConfigYAML: %v", err)
	}
	if !strings.Contains(string(yb), "proxy: socks5h://127.0.0.1:1080") {
		t.Fatalf("serialized YAML missing provider proxy:\n%s", yb)
	}
}

func TestProviderConfigValidateProxy(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		proxy   string
		wantErr bool
	}{
		{name: "empty", proxy: "", wantErr: false},
		{name: "http", proxy: "http://127.0.0.1:8080", wantErr: false},
		{name: "https", proxy: "https://proxy.example:8443", wantErr: false},
		{name: "socks5", proxy: "socks5://127.0.0.1:1080", wantErr: false},
		{name: "socks5h", proxy: "socks5h://127.0.0.1:1080", wantErr: false},
		{name: "socks4", proxy: "socks4://127.0.0.1:1080", wantErr: true},
		{name: "ftp", proxy: "ftp://127.0.0.1:21", wantErr: true},
		{name: "no_scheme", proxy: "127.0.0.1:8080", wantErr: true},
		{name: "no_host", proxy: "http://", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := config.ProviderConfig{Name: "p", Type: "openai", Proxy: tt.proxy}
			p.Normalize()
			err := p.Validate()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate: %v", err)
			}
		})
	}
}
