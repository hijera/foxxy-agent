package config

import "testing"

// Telegram token follows the same "optional in config, resolved from env at runtime"
// pattern as provider api_key: the user may keep it in .env / the environment instead
// of writing it into config.yaml.

func TestTelegramConfig_TokenOptionalWhenEnabled(t *testing.T) {
	c := &TelegramGatewayConfig{Enabled: true} // no token
	c.Normalize()
	c.ApplyDefaults()
	if err := c.Validate(); err != nil {
		t.Fatalf("enabled telegram with no token must validate (token may come from env), got: %v", err)
	}
}

func TestTelegramConfig_ProxyStillValidatedWithoutToken(t *testing.T) {
	c := &TelegramGatewayConfig{Enabled: true, Proxy: "ftp://nope"}
	c.Normalize()
	c.ApplyDefaults()
	if err := c.Validate(); err == nil {
		t.Fatal("invalid proxy scheme must still fail validation")
	}
}

func TestTelegramConfig_EffectiveToken(t *testing.T) {
	// Configured token wins.
	c := &TelegramGatewayConfig{Token: "from-config"}
	t.Setenv(TelegramBotTokenEnvVar, "from-env")
	if got := c.EffectiveToken(); got != "from-config" {
		t.Fatalf("configured token should win, got %q", got)
	}

	// Empty token falls back to the environment variable.
	c2 := &TelegramGatewayConfig{}
	t.Setenv(TelegramBotTokenEnvVar, "env-token")
	if got := c2.EffectiveToken(); got != "env-token" {
		t.Fatalf("empty token should resolve from %s, got %q", TelegramBotTokenEnvVar, got)
	}

	// Empty token and no env var resolves to empty.
	c3 := &TelegramGatewayConfig{}
	t.Setenv(TelegramBotTokenEnvVar, "")
	if got := c3.EffectiveToken(); got != "" {
		t.Fatalf("no token anywhere should be empty, got %q", got)
	}
}
