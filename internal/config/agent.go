package config

import "fmt"

// Defaults for the ReAct loop when YAML omits zero values.
const (
	AgentDefaultMaxTurns         = 30
	AgentDefaultMaxTokensPerTurn = 200000
	AgentDefaultLLMRetryMax      = 3
	AgentDefaultLLMRetryBaseMS   = 1000
)

// Agent is the YAML agent section (key agent) for ReAct loop settings.
type Agent struct {
	// Model is the models[].id used for LLM calls until the session overrides the model in the client.
	Model            string `yaml:"model"`
	MaxTurns         int    `yaml:"max_turns"`
	MaxTokensPerTurn int    `yaml:"max_tokens_per_turn"`
	// LLMRetryMax is the number of retries after a retryable LLM error such as HTTP 429 (default 3).
	LLMRetryMax      int `yaml:"llm_retry_max"`
	// LLMRetryBaseMS is the initial backoff between LLM retries in milliseconds (default 1000).
	LLMRetryBaseMS   int `yaml:"llm_retry_base_ms"`
	// LLMMinIntervalMS enforces a minimum gap between consecutive LLM calls in milliseconds (default 0).
	LLMMinIntervalMS int `yaml:"llm_min_interval_ms"`
}

// ApplyDefaults sets MaxTurns and MaxTokensPerTurn when they are zero.
func (c *Agent) ApplyDefaults() {
	if c.MaxTurns == 0 {
		c.MaxTurns = AgentDefaultMaxTurns
	}
	if c.MaxTokensPerTurn == 0 {
		c.MaxTokensPerTurn = AgentDefaultMaxTokensPerTurn
	}
	if c.LLMRetryMax == 0 {
		c.LLMRetryMax = AgentDefaultLLMRetryMax
	}
	if c.LLMRetryBaseMS == 0 {
		c.LLMRetryBaseMS = AgentDefaultLLMRetryBaseMS
	}
}

// Validate checks bounds after defaults.
func (c *Agent) Validate() error {
	if c.MaxTurns < 0 {
		return fmt.Errorf("agent.max_turns: must be >= 0")
	}
	if c.MaxTokensPerTurn < 0 {
		return fmt.Errorf("agent.max_tokens_per_turn: must be >= 0")
	}
	if c.LLMRetryMax < 0 {
		return fmt.Errorf("agent.llm_retry_max: must be >= 0")
	}
	if c.LLMRetryBaseMS < 0 {
		return fmt.Errorf("agent.llm_retry_base_ms: must be >= 0")
	}
	if c.LLMMinIntervalMS < 0 {
		return fmt.Errorf("agent.llm_min_interval_ms: must be >= 0")
	}
	return nil
}
