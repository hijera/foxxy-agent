package config

import (
	"fmt"
	"strings"
)

// Compaction engine identifiers (compaction.engine).
const (
	// CompactionEngineCoddy is the default engine ported from upstream: it inserts a
	// summary row at the compaction boundary and replays only the window from the
	// last summary onward (session.MessagesForLLM). It supports the manual /compact
	// command and the HTTP compact endpoint.
	CompactionEngineCoddy = "coddy"
	// CompactionEngineOpenCode is the fork's original engine: it flags older messages
	// Compacted and filters them from the model payload (isLLMHistoryMessage).
	CompactionEngineOpenCode = "opencode"
)

// Default values for CompactionConfig.
const (
	// CompactionDefaultThresholdCoddy triggers auto-compaction for the coddy engine.
	CompactionDefaultThresholdCoddy = 80
	// CompactionDefaultThresholdOpenCode triggers auto-compaction for the opencode engine.
	CompactionDefaultThresholdOpenCode = 85
	// CompactionDefaultKeepRecentTurns is how many recent user turns stay verbatim.
	CompactionDefaultKeepRecentTurns = 2
	// CompactionDefaultMaxTokens caps the opencode summary completion.
	CompactionDefaultMaxTokens = 4096

	compactionMinThresholdPercent = 50
	compactionMaxThresholdPercent = 99
)

const (
	// ResultEvictionDefaultKeepRecent keeps the common read-plus-grep working
	// pair intact. A window of one causes alternating re-fetches.
	ResultEvictionDefaultKeepRecent = 2
	// ResultEvictionDefaultMinResultBytes leaves small results untouched.
	ResultEvictionDefaultMinResultBytes = 2000
)

// CompactionConfig controls automatic context compaction (summarization of older turns when the
// conversation approaches the model's context window). Two engines share this section, selected
// by Engine; implementations live in internal/agent (compact.go for coddy, compaction.go for
// opencode).
type CompactionConfig struct {
	// Engine selects the compaction implementation: "coddy" (default) inserts a summary row and
	// replays only the window from the last summary onward; "opencode" flags older messages
	// Compacted and filters them from the payload. Empty defaults to coddy.
	Engine string `yaml:"engine"`

	// Enabled is a pointer so an unset value defaults to true while an explicit false is
	// preserved. Use CompactionEnabled to read the effective value. Auto-compaction only fires
	// when real prompt tokens approach the context window, so leaving it on is a safe default.
	Enabled *bool `yaml:"enabled"`

	// Model selects a cfg.models entry for the summarization pass. Empty uses agent.model.
	Model string `yaml:"model"`

	// ThresholdPercent triggers compaction when context usage exceeds this percentage of the
	// model's context window. The default depends on Engine (coddy 80, opencode 85); read the
	// effective value with EffectiveThresholdPercent.
	ThresholdPercent int `yaml:"threshold_percent"`

	// KeepRecentTurns is the number of most recent user turns preserved verbatim. A nil pointer
	// means the default (2); an explicit 0 is honored by the coddy engine (opencode clamps to at
	// least 1). Read the effective value with EffectiveKeepRecentTurns.
	KeepRecentTurns *int `yaml:"keep_recent_turns"`

	// MaxTokens caps the summary completion size for the opencode engine. The coddy engine issues
	// the summary request without an output cap and ignores this. Default 4096.
	MaxTokens int `yaml:"max_tokens"`

	// ResultEviction prunes superseded read/grep results only from the LLM
	// projection. Persisted transcript messages are never rewritten.
	ResultEviction ResultEviction `yaml:"result_eviction"`
}

// ResultEviction controls collapsing stale read and grep results to short
// placeholders when building an LLM request.
type ResultEviction struct {
	Enabled        *bool `yaml:"enabled"`
	KeepRecent     *int  `yaml:"keep_recent"`
	MinResultBytes *int  `yaml:"min_result_bytes"`
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

func (r *ResultEviction) Validate() error {
	if r.KeepRecent != nil && *r.KeepRecent < 0 {
		return fmt.Errorf("compaction.result_eviction.keep_recent: must be >= 0")
	}
	if r.MinResultBytes != nil && *r.MinResultBytes < 0 {
		return fmt.Errorf("compaction.result_eviction.min_result_bytes: must be >= 0")
	}
	return nil
}

// CompactionEnabled reports whether compaction is active. Unset (nil) defaults to true.
func (c *CompactionConfig) CompactionEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

// IsEnabled is an alias for CompactionEnabled used by the coddy engine call sites.
func (c *CompactionConfig) IsEnabled() bool { return c.CompactionEnabled() }

// EngineIsOpenCode reports whether the opencode engine is selected.
func (c *CompactionConfig) EngineIsOpenCode() bool {
	return strings.EqualFold(strings.TrimSpace(c.Engine), CompactionEngineOpenCode)
}

// EngineIsCoddy reports whether the coddy engine is selected (the default when unset).
func (c *CompactionConfig) EngineIsCoddy() bool { return !c.EngineIsOpenCode() }

// EffectiveThresholdPercent returns threshold_percent with the per-engine default applied (covers
// configs constructed without ApplyDefaults).
func (c *CompactionConfig) EffectiveThresholdPercent() int {
	if c.ThresholdPercent > 0 {
		return c.ThresholdPercent
	}
	if c.EngineIsOpenCode() {
		return CompactionDefaultThresholdOpenCode
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

// Normalize trims string fields and lowercases the engine in place.
func (c *CompactionConfig) Normalize() {
	c.Model = strings.TrimSpace(c.Model)
	c.Engine = strings.ToLower(strings.TrimSpace(c.Engine))
}

// ApplyDefaults sets zero values to safe defaults. An omitted enabled key defaults to true and an
// omitted engine defaults to coddy.
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
	// The opencode engine clamps the trigger into a conservative band; the coddy engine accepts
	// the full 1..100 range validated below.
	if c.EngineIsOpenCode() {
		if c.ThresholdPercent < compactionMinThresholdPercent {
			c.ThresholdPercent = compactionMinThresholdPercent
		}
		if c.ThresholdPercent > compactionMaxThresholdPercent {
			c.ThresholdPercent = compactionMaxThresholdPercent
		}
	}
	if c.MaxTokens <= 0 {
		c.MaxTokens = CompactionDefaultMaxTokens
	}
}

// Validate checks the engine value, compaction model reference, and threshold bounds.
func (c *CompactionConfig) Validate(cfg *Config) error {
	switch c.Engine {
	case "", CompactionEngineCoddy, CompactionEngineOpenCode:
	default:
		return fmt.Errorf("engine %q: must be %q or %q", c.Engine, CompactionEngineCoddy, CompactionEngineOpenCode)
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
	if c.EngineIsOpenCode() {
		if c.ThresholdPercent < compactionMinThresholdPercent || c.ThresholdPercent > compactionMaxThresholdPercent {
			return fmt.Errorf("threshold_percent: must be within %d..%d for the opencode engine", compactionMinThresholdPercent, compactionMaxThresholdPercent)
		}
	} else if c.ThresholdPercent < 1 || c.ThresholdPercent > 100 {
		return fmt.Errorf("threshold_percent: must be within 1..100")
	}
	if c.KeepRecentTurns != nil && *c.KeepRecentTurns < 0 {
		return fmt.Errorf("keep_recent_turns: must be >= 0")
	}
	return nil
}
