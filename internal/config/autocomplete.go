package config

import (
	"fmt"
	"strings"
)

// Autocomplete trigger modes (autocomplete.trigger).
const (
	// AutocompleteTriggerAuto asks for a suggestion while the user types, after a debounce pause.
	AutocompleteTriggerAuto = "auto"
	// AutocompleteTriggerManual only asks when the user presses the editor shortcut.
	AutocompleteTriggerManual = "manual"
)

// Autocomplete prompt modes (autocomplete.mode): how the hole in the code reaches the model.
const (
	// AutocompleteModeAuto uses native fill-in-the-middle when the model family has known FIM
	// control tokens and the provider serves raw completions, and a chat prompt otherwise. A raw
	// call that fails switches that model to chat for the rest of the process.
	AutocompleteModeAuto = "auto"
	// AutocompleteModeChat always sends a chat prompt with the caret marked in the user message.
	AutocompleteModeChat = "chat"
	// AutocompleteModeFIM always sends native FIM tokens through a raw completion and reports an
	// error when the model or provider cannot do that.
	AutocompleteModeFIM = "fim"
)

// Default values for AutocompleteConfig.
const (
	// AutocompleteDefaultMaxTokens caps one inline suggestion. Suggestions are a line or a short
	// block, so this is deliberately small: it bounds both latency and cost per keystroke.
	AutocompleteDefaultMaxTokens = 128
	// AutocompleteDefaultTimeoutMS bounds one suggestion request end to end.
	AutocompleteDefaultTimeoutMS = 4000
	// AutocompleteDefaultDebounceMS is the typing pause before an automatic request goes out.
	AutocompleteDefaultDebounceMS = 350
	// AutocompleteDefaultMaxPrefixBytes caps the text before the caret sent as context.
	AutocompleteDefaultMaxPrefixBytes = 8000
	// AutocompleteDefaultMaxSuffixBytes caps the text after the caret sent as context.
	AutocompleteDefaultMaxSuffixBytes = 2000
	// AutocompleteDefaultRelatedFiles is how many other open editor tabs are excerpted into the
	// prompt so the model sees symbols from neighbouring files.
	AutocompleteDefaultRelatedFiles = 3
)

// AutocompleteConfig controls LLM-backed inline code completion: the greyed suggestion an editor
// plugin renders ahead of the caret, accepted with Tab. Editors fetch it over
// POST /foxxycode/completion, one single-shot LLM call with no tools and no agent loop.
//
// Unlike CompactionConfig and TitleConfig, this section is off unless enabled explicitly: a
// suggestion is requested as the user types, so leaving it on by default would spend tokens on
// every keystroke of every install.
type AutocompleteConfig struct {
	// Enabled turns inline completion on. A pointer so an unset value is distinguishable, but
	// unlike the other sections the default is false. Use AutocompleteEnabled to read it.
	Enabled *bool `yaml:"enabled"`

	// Model selects a cfg.models entry for the completion pass. Empty uses agent.model. A small,
	// fast model matters more here than a clever one: the suggestion is worthless once the user
	// has typed past it.
	Model string `yaml:"model"`

	// Mode is how the hole reaches the model: "auto" (default), "chat", or "fim".
	// See the AutocompleteMode* constants.
	Mode string `yaml:"mode"`

	// Temperature for the suggestion pass. Unlike models[].temperature, 0 here is the value, not
	// "unset": suggestions are sampled greedily by default because determinism is what makes a
	// suggestion re-appear after the user types a character of it.
	Temperature float64 `yaml:"temperature"`

	// MaxTokens caps one suggestion. Default AutocompleteDefaultMaxTokens.
	MaxTokens int `yaml:"max_tokens"`

	// TimeoutMS bounds one suggestion request. Default AutocompleteDefaultTimeoutMS.
	TimeoutMS int `yaml:"timeout_ms"`

	// DebounceMS is how long typing must pause before an automatic request goes out. Ignored when
	// Trigger is manual. Default AutocompleteDefaultDebounceMS.
	DebounceMS int `yaml:"debounce_ms"`

	// Trigger is "auto" (ask while typing) or "manual" (only on the editor shortcut).
	// Default AutocompleteTriggerAuto.
	Trigger string `yaml:"trigger"`

	// MultiLine allows suggestions spanning several lines. When false only the first line of a
	// suggestion is kept. Unset defaults to true. Even when allowed, a block is only produced
	// where the caret invites one (end of a line that opened a block, or an empty line).
	MultiLine *bool `yaml:"multi_line"`

	// MaxPrefixBytes caps the text before the caret sent as context.
	// Default AutocompleteDefaultMaxPrefixBytes.
	MaxPrefixBytes int `yaml:"max_prefix_bytes"`

	// MaxSuffixBytes caps the text after the caret sent as context.
	// Default AutocompleteDefaultMaxSuffixBytes.
	MaxSuffixBytes int `yaml:"max_suffix_bytes"`

	// RelatedFiles is how many other open editor tabs (as reported over
	// POST /foxxycode/ide/editor-state) are excerpted into the prompt. A pointer so an explicit 0
	// disables it while unset means AutocompleteDefaultRelatedFiles.
	RelatedFiles *int `yaml:"related_files"`
}

// AutocompleteEnabled reports whether inline completion is active. Unset (nil) defaults to false.
func (c *AutocompleteConfig) AutocompleteEnabled() bool {
	return c.Enabled != nil && *c.Enabled
}

// MultiLineEnabled reports whether multi-line suggestions are allowed. Unset (nil) defaults to true.
func (c *AutocompleteConfig) MultiLineEnabled() bool {
	return c.MultiLine == nil || *c.MultiLine
}

// RelatedFileCount is the effective number of neighbouring files to excerpt.
func (c *AutocompleteConfig) RelatedFileCount() int {
	if c.RelatedFiles == nil {
		return AutocompleteDefaultRelatedFiles
	}
	if *c.RelatedFiles < 0 {
		return 0
	}
	return *c.RelatedFiles
}

// Normalize trims and lowercases string fields in place.
func (c *AutocompleteConfig) Normalize() {
	c.Model = strings.TrimSpace(c.Model)
	c.Trigger = strings.ToLower(strings.TrimSpace(c.Trigger))
	c.Mode = strings.ToLower(strings.TrimSpace(c.Mode))
}

// ApplyDefaults sets zero values to safe defaults. An omitted enabled key stays false.
func (c *AutocompleteConfig) ApplyDefaults() {
	if c.Enabled == nil {
		enabled := false
		c.Enabled = &enabled
	}
	if c.MultiLine == nil {
		multi := true
		c.MultiLine = &multi
	}
	if c.RelatedFiles == nil {
		n := AutocompleteDefaultRelatedFiles
		c.RelatedFiles = &n
	}
	if c.Trigger == "" {
		c.Trigger = AutocompleteTriggerAuto
	}
	if c.Mode == "" {
		c.Mode = AutocompleteModeAuto
	}
	if c.Temperature < 0 {
		c.Temperature = 0
	}
	if c.MaxTokens <= 0 {
		c.MaxTokens = AutocompleteDefaultMaxTokens
	}
	if c.TimeoutMS <= 0 {
		c.TimeoutMS = AutocompleteDefaultTimeoutMS
	}
	if c.DebounceMS <= 0 {
		c.DebounceMS = AutocompleteDefaultDebounceMS
	}
	if c.MaxPrefixBytes <= 0 {
		c.MaxPrefixBytes = AutocompleteDefaultMaxPrefixBytes
	}
	if c.MaxSuffixBytes <= 0 {
		c.MaxSuffixBytes = AutocompleteDefaultMaxSuffixBytes
	}
}

// Validate checks the trigger and prompt modes and that the completion model reference resolves
// when set. A disabled section is not validated: an unusable leftover model must not block startup.
func (c *AutocompleteConfig) Validate(cfg *Config) error {
	if !c.AutocompleteEnabled() {
		return nil
	}
	switch c.Trigger {
	case "", AutocompleteTriggerAuto, AutocompleteTriggerManual:
	default:
		return fmt.Errorf("trigger %q: must be %q or %q", c.Trigger, AutocompleteTriggerAuto, AutocompleteTriggerManual)
	}
	switch c.Mode {
	case "", AutocompleteModeAuto, AutocompleteModeChat, AutocompleteModeFIM:
	default:
		return fmt.Errorf("mode %q: must be %q, %q or %q", c.Mode, AutocompleteModeAuto, AutocompleteModeChat, AutocompleteModeFIM)
	}
	if c.Model != "" && cfg.FindModelEntry(c.Model) == nil {
		return fmt.Errorf("model %q: not found in models list", c.Model)
	}
	return nil
}
