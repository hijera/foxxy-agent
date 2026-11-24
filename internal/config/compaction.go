package config

import (
	"fmt"
	"strings"
)

const (
	// CompactionEngineCoddy is the sole compaction engine.
	CompactionEngineCoddy = "coddy"
	// CompactionEngineFoxxyCode is a spelling used by existing configurations.
	CompactionEngineFoxxyCode      = "foxxycode"
	legacyCompactionEngineOpenCode = "opencode"
	openCodeCompactionNotice       = "The OpenCode compaction engine has been replaced by the more advanced Coddy engine."
	openCodeCompactionFix          = "replace \"opencode\" with \"coddy\" or remove compaction.engine"
	legacyCompactionThreshold      = 85

	// CompactionDefaultThresholdCoddy triggers automatic compaction.
	CompactionDefaultThresholdCoddy = 80
	// CompactionDefaultKeepRecentTurns is how many recent user turns stay verbatim.
	CompactionDefaultKeepRecentTurns = 2
)

const (
	// ResultEvictionDefaultKeepRecent keeps the common read-plus-grep working
	// pair intact. A window of one causes alternating re-fetches.
	ResultEvictionDefaultKeepRecent = 2
	// ResultEvictionDefaultMinResultBytes leaves small results untouched.
	ResultEvictionDefaultMinResultBytes = 2000
	// ResultEvictionDefaultStartPercent is the share of the model's context
	// window the conversation must reach before eviction starts rewriting it.
	// Below it the history is sent untouched, because every placeholder that
	// appears mid-history is a byte the provider's prompt cache keyed the rest
	// of the conversation on: a sliding window would reprocess the whole
	// transcript on almost every step to save a few thousand tokens nobody was
	// short of yet.
	ResultEvictionDefaultStartPercent = 50
)

// CompactionConfig controls summarization of older turns as the conversation
// approaches the model's context window.
type CompactionConfig struct {
	// Engine is retained for existing YAML and settings clients. It resolves
	// to Coddy, including when a configuration used to select OpenCode.
	Engine string `yaml:"engine"`

	migratedOpenCode bool `yaml:"-"`

	// Enabled is a pointer so an unset value defaults to true while an explicit false is
	// preserved. Use CompactionEnabled to read the effective value. Auto-compaction only fires
	// when real prompt tokens approach the context window, so leaving it on is a safe default.
	Enabled *bool `yaml:"enable"`
	// AutoEnabled controls the threshold trigger independently of manual compaction.
	AutoEnabled *bool `yaml:"auto_enable"`

	// Model selects a cfg.models entry for the summarization pass. Empty uses agent.model.
	Model string `yaml:"model"`

	// FallbackModels are the models[].model ids tried, in order, when the
	// summarizer above them fails. A compaction is what a session out of room
	// has left, so one unreachable or overloaded model must not be the end of
	// it; the session's own model is always the last resort, whether or not it
	// is listed here (upstream issue #247).
	FallbackModels []string `yaml:"fallback_models"`

	// ThresholdPercent triggers compaction when context usage exceeds this percentage of the
	// model's context window. Read the effective value with EffectiveThresholdPercent.
	ThresholdPercent int `yaml:"threshold_percent"`

	// KeepRecentTurns is the number of most recent user turns preserved verbatim. A nil pointer
	// means the default (2). An explicit 0 requests a full fold for manual compaction.
	// Read the effective value with EffectiveKeepRecentTurns. When the window holds no
	// more user turns than this, a compaction keeps fewer: the automatic trigger down to the
	// prompt being answered, the manual command down to none.
	KeepRecentTurns *int `yaml:"keep_recent_turns"`

	// MaxTokens is accepted for compatibility with old YAML and settings clients.
	// Deprecated: Coddy does not use this limit.
	MaxTokens int `yaml:"max_tokens,omitempty"`

	// ResultEviction prunes superseded read/grep results only from the LLM
	// projection. Persisted transcript messages are never rewritten.
	ResultEviction ResultEviction `yaml:"result_eviction"`
}

// ResultEviction controls collapsing stale read and grep results to short
// placeholders when building an LLM request.
type ResultEviction struct {
	Enabled        *bool `yaml:"enable"`
	KeepRecent     *int  `yaml:"keep_recent"`
	MinResultBytes *int  `yaml:"min_result_bytes"`
	// StartPercent is the share of the effective context window the estimated
	// context must reach before eviction starts (default 50, valid 0..100). 0
	// evicts from the first result, which is what the projection did before
	// prompt caching was accounted for.
	StartPercent *int `yaml:"start_percent"`
}

func (r *ResultEviction) IsEnabled() bool {
	return r.Enabled == nil || *r.Enabled
}

func (r *ResultEviction) EffectiveKeepRecent() int {
	if r.KeepRecent == nil {
		return ResultEvictionDefaultKeepRecent
	}
	return *r.KeepRecent
}

func (r *ResultEviction) EffectiveMinResultBytes() int {
	if r.MinResultBytes == nil {
		return ResultEvictionDefaultMinResultBytes
	}
	return *r.MinResultBytes
}

// EffectiveStartPercent returns start_percent with the default applied.
func (r *ResultEviction) EffectiveStartPercent() int {
	if r.StartPercent == nil {
		return ResultEvictionDefaultStartPercent
	}
	return *r.StartPercent
}

func (r *ResultEviction) Validate() error {
	if r.KeepRecent != nil && *r.KeepRecent < 0 {
		return fmt.Errorf("compaction.result_eviction.keep_recent: must be >= 0")
	}
	if r.MinResultBytes != nil && *r.MinResultBytes < 0 {
		return fmt.Errorf("compaction.result_eviction.min_result_bytes: must be >= 0")
	}
	if r.StartPercent != nil && (*r.StartPercent < 0 || *r.StartPercent > 100) {
		return fmt.Errorf("compaction.result_eviction.start_percent: must be between 0 and 100")
	}
	return nil
}

// CompactionEnabled reports whether compaction is active. Unset (nil) defaults to true.
func (c *CompactionConfig) CompactionEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

// IsEnabled is an alias for CompactionEnabled used by the coddy engine call sites.
func (c *CompactionConfig) IsEnabled() bool { return c.CompactionEnabled() }

// IsAutoEnabled reports whether automatic compaction is enabled.
func (c *CompactionConfig) IsAutoEnabled() bool {
	return c.IsEnabled() && (c.AutoEnabled == nil || *c.AutoEnabled)
}

// EffectiveThresholdPercent returns threshold_percent with the default applied.
func (c *CompactionConfig) EffectiveThresholdPercent() int {
	if c.ThresholdPercent > 0 {
		return c.ThresholdPercent
	}
	return CompactionDefaultThresholdCoddy
}

// EffectiveKeepRecentTurns returns keep_recent_turns with the default applied.
func (c *CompactionConfig) EffectiveKeepRecentTurns() int {
	if c.KeepRecentTurns == nil {
		return CompactionDefaultKeepRecentTurns
	}
	return *c.KeepRecentTurns
}

// Normalize trims string fields and maps old engine selections to Coddy.
func (c *CompactionConfig) Normalize() {
	c.Model = strings.TrimSpace(c.Model)
	c.Engine = strings.ToLower(strings.TrimSpace(c.Engine))
	switch c.Engine {
	case CompactionEngineFoxxyCode:
		c.Engine = CompactionEngineCoddy
	case legacyCompactionEngineOpenCode:
		c.migratedOpenCode = true
		c.Engine = CompactionEngineCoddy
		// Keep the effective trigger of configurations that previously used
		// the old default or had their threshold clamped by the loader.
		if c.ThresholdPercent <= 0 {
			c.ThresholdPercent = legacyCompactionThreshold
		} else {
			c.ThresholdPercent = min(99, max(50, c.ThresholdPercent))
		}
	}
}

// ApplyDefaults sets omitted values for the Coddy engine.
func (c *CompactionConfig) ApplyDefaults() {
	if c.Engine == "" {
		c.Engine = CompactionEngineCoddy
	}
	if c.Enabled == nil {
		enabled := true
		c.Enabled = &enabled
	}
	if c.ThresholdPercent <= 0 {
		c.ThresholdPercent = c.EffectiveThresholdPercent()
	}
}

// Validate checks the engine value, compaction model reference, and threshold bounds.
func (c *CompactionConfig) Validate(cfg *Config) error {
	switch c.Engine {
	case "", CompactionEngineCoddy, CompactionEngineFoxxyCode:
	default:
		return fmt.Errorf("engine %q: must be %q or %q", c.Engine, CompactionEngineCoddy, CompactionEngineFoxxyCode)
	}
	if err := c.ResultEviction.Validate(); err != nil {
		return err
	}
	if !c.CompactionEnabled() {
		return nil
	}
	if c.Model != "" && cfg.FindModelEntry(c.Model) == nil {
		return fmt.Errorf("model %q: not found in models list", c.Model)
	}
	if c.ThresholdPercent < 1 || c.ThresholdPercent > 100 {
		return fmt.Errorf("threshold_percent: must be within 1..100")
	}
	if c.KeepRecentTurns != nil && *c.KeepRecentTurns < 0 {
		return fmt.Errorf("keep_recent_turns: must be >= 0")
	}
	return nil
}
