package config

// BrowserDefaultTimeoutSeconds is the default per-action timeout for the interactive
// browser tool (navigate/click/...) when browser.timeout_seconds is unset.
const BrowserDefaultTimeoutSeconds = 30

// BrowserConfig is the YAML browser section (key browser). It configures the optional
// interactive browser tool (build tag "browser"), which drives a local Chrome/Chromium
// over the DevTools Protocol via chromedp.
type BrowserConfig struct {
	// Enabled turns on the interactive browser tools for eligible builds.
	Enabled bool `yaml:"enabled"`
	// Headless runs the browser without a visible window. Defaults to true when unset;
	// set explicitly to false to watch the automated session.
	Headless *bool `yaml:"headless"`
	// ExecutablePath optionally points at a specific Chrome/Chromium binary. Empty lets
	// chromedp auto-detect an installed browser.
	ExecutablePath string `yaml:"executable_path"`
	// TimeoutSeconds bounds each browser action (navigation, click, ...). Defaults to
	// BrowserDefaultTimeoutSeconds.
	TimeoutSeconds int `yaml:"timeout_seconds"`
}

// HeadlessEnabled reports whether the browser should run headless (default true).
func (c *BrowserConfig) HeadlessEnabled() bool {
	return c.Headless == nil || *c.Headless
}

// ResolvedTimeoutSeconds returns TimeoutSeconds with a safe default.
func (c *BrowserConfig) ResolvedTimeoutSeconds() int {
	if c.TimeoutSeconds <= 0 {
		return BrowserDefaultTimeoutSeconds
	}
	return c.TimeoutSeconds
}

// ApplyDefaults fills unset fields with defaults.
func (c *BrowserConfig) ApplyDefaults() {
	if c.TimeoutSeconds <= 0 {
		c.TimeoutSeconds = BrowserDefaultTimeoutSeconds
	}
}

// Validate checks the browser section (currently a no-op; timeouts are normalised in ApplyDefaults).
func (c *BrowserConfig) Validate() error {
	return nil
}
