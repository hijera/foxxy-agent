package config

import (
	"fmt"
	"time"
)

// Defaults for the ReAct loop when YAML omits zero values.
const (
	AgentDefaultMaxTurns         = 30
	AgentDefaultMaxTokensPerTurn = 200000
	AgentDefaultLLMRetryMax      = 3
	AgentDefaultLLMRetryBaseMS   = 1000
	// AgentDefaultLLMFirstTokenTimeoutMS is how long a streamed LLM call may
	// stay silent before the turn cancels it (the API hang guard in the ReAct
	// loop). It has to clear a real prefill, not just a healthy handshake: a
	// reasoning model given a large tool result routinely needs more than half a
	// minute before its first token, and 30s cut those turns as if the provider
	// had hung.
	AgentDefaultLLMFirstTokenTimeoutMS = 90000
	// AgentDefaultLLMStallTimeoutMS bounds how long a stream that has ALREADY
	// produced output may stay silent before the turn cuts it. The first-token
	// guard above stops for good at the first token and is never re-armed, so
	// without this a provider that abandons a half-written answer holds the turn
	// open until its context dies.
	//
	// Five minutes, matching the stream_idle_timeout_ms default the Codex CLI uses
	// for the same job. Deliberately far above the measured healthy inter-frame gap
	// (1.5-3.9s on a saturated hub) because the two mistakes cost very differently:
	// waiting too long merely delays a turn that was already stuck, while cutting
	// too early throws away a half-written answer and spends a continuation, and a
	// reasoning model can legitimately go quiet for minutes mid-answer. The stall
	// this bounds holds the connection open indefinitely, so a patient guard still
	// ends it.
	AgentDefaultLLMStallTimeoutMS = 300000
	// AgentDefaultLLMStallRetryMaxWaitMS is the wall-clock budget for waiting out a
	// silent provider within one LLM call. An explicit 0 means unbounded.
	AgentDefaultLLMStallRetryMaxWaitMS = 3600000
	// AgentDefaultLoopToolRepeatLimit is how many consecutive identical tool calls
	// (same name, same canonical arguments) the loop guard tolerates.
	AgentDefaultLoopToolRepeatLimit = 3
	// AgentDefaultLoopStreamRepeatCycles is how many identical back-to-back output
	// cycles inside one streamed response trip the loop guard.
	AgentDefaultLoopStreamRepeatCycles = 5
	// AgentDefaultLoopToolCycleRepeats is how many back-to-back repetitions of the
	// same sequence of tool calls trip the loop guard.
	AgentDefaultLoopToolCycleRepeats = 3
	// AgentDefaultLoopNudgeMax is how many times a turn may be nudged back on track
	// before the loop guard stops it.
	AgentDefaultLoopNudgeMax = 2
)

// agentDefaultLLMStallRetryDelaysMS is the pause before each retry of a silent LLM
// call: one minute, then three, then five - and five for every attempt after that,
// because the last entry repeats. Long enough to outlast a saturated gateway
// without hammering it.
var agentDefaultLLMStallRetryDelaysMS = []int{60000, 180000, 300000}

// Loop-guard terminal actions (agent.loop_stuck_action): what the guard does once
// a tool loop has survived every nudge.
const (
	// AgentLoopStuckActionQuarantine takes the looping calls away for the rest of
	// the turn - they stop being executed and the model is told why - and lets the
	// turn run on to a real answer. Throwing the turn away is expensive: the model
	// has usually gathered most of what it needs by then.
	AgentLoopStuckActionQuarantine = "quarantine"
	// AgentLoopStuckActionStop ends the turn with a notice, the behaviour ported
	// from upstream.
	AgentLoopStuckActionStop = "stop"
	// AgentDefaultLoopStuckAction is the action applied when the key is unset.
	AgentDefaultLoopStuckAction = AgentLoopStuckActionQuarantine
)

// Agent is the YAML agent section (key agent) for ReAct loop settings.
type Agent struct {
	// Model is the models[].id used for LLM calls until the session overrides the model in the client.
	Model            string `yaml:"model"`
	MaxTurns         int    `yaml:"max_turns"`
	MaxTokensPerTurn int    `yaml:"max_tokens_per_turn"`
	// LLMRetryMax is the number of retries after a retryable LLM error such as HTTP 429.
	// A nil pointer means the default (3); an explicit 0 disables retries.
	LLMRetryMax *int `yaml:"llm_retry_max"`
	// LLMRetryBaseMS is the initial backoff between LLM retries in milliseconds (default 1000).
	LLMRetryBaseMS int `yaml:"llm_retry_base_ms"`
	// LLMMinIntervalMS enforces a minimum gap between consecutive LLM calls in milliseconds (default 0).
	LLMMinIntervalMS int `yaml:"llm_min_interval_ms"`
	// LLMFirstTokenTimeoutMS is how long a streamed LLM call may stay silent before
	// the turn cancels it. A nil pointer means the default (90000); an explicit 0
	// disables the guard, leaving the turn context as the only bound.
	LLMFirstTokenTimeoutMS *int `yaml:"llm_first_token_timeout_ms"`
	// LLMStallTimeoutMS is how long a streamed LLM call that has already produced
	// output may stay silent before the turn cuts it. A nil pointer means the
	// default (90000); an explicit 0 disables the guard.
	LLMStallTimeoutMS *int `yaml:"llm_stall_timeout_ms"`
	// LLMStallRetry toggles waiting out a provider that produced no output at all
	// and re-issuing the same call. A nil pointer means the default (true).
	LLMStallRetry *bool `yaml:"llm_stall_retry"`
	// LLMStallRetryDelaysMS is the pause before each retry; the last entry repeats
	// for every later attempt. Empty means the default ladder (1m, 3m, 5m, 5m...).
	LLMStallRetryDelaysMS []int `yaml:"llm_stall_retry_delays_ms"`
	// LLMStallRetryMaxWaitMS caps the total time spent waiting between retries of
	// one call. A nil pointer means the default (3600000, one hour); an explicit 0
	// retries until the model answers or the user stops the turn.
	LLMStallRetryMaxWaitMS *int `yaml:"llm_stall_retry_max_wait_ms"`
	// LoopGuard toggles runaway-loop protection: aborting a streamed response that
	// degenerates into repeating itself, and blocking identical tool calls issued
	// over and over. A nil pointer means the default (true).
	LoopGuard *bool `yaml:"loop_guard"`
	// LoopToolRepeatLimit is how many consecutive identical tool calls trip the guard.
	// A nil pointer means the default (3); an explicit 0 disables the tool-repeat check.
	LoopToolRepeatLimit *int `yaml:"loop_tool_repeat_limit"`
	// LoopStreamRepeatCycles is how many identical back-to-back output cycles inside one
	// streamed response trip the guard. A nil pointer means the default (5); an explicit
	// 0 disables the stream check.
	LoopStreamRepeatCycles *int `yaml:"loop_stream_repeat_cycles"`
	// LoopToolCycleRepeats is how many back-to-back repetitions of the same sequence
	// of tool calls trip the guard, which is what catches a model rotating through
	// several calls instead of repeating one. A nil pointer means the default (3);
	// an explicit 0 or 1 disables the cycle check.
	LoopToolCycleRepeats *int `yaml:"loop_tool_cycle_repeats"`
	// LoopStuckAction selects what happens once a tool loop has survived every
	// nudge: "quarantine" (default) blocks the looping calls for the rest of the
	// turn and lets it continue to an answer, "stop" ends the turn with a notice.
	// Empty defaults to quarantine. Streamed-output loops always stop the turn -
	// there is nothing to quarantine when the output itself is degenerate.
	LoopStuckAction string `yaml:"loop_stuck_action"`
	// LoopNudgeMax is how many times one turn may be nudged back on track before the
	// guard stops it with a notice. A nil pointer means the default (2); an explicit 0
	// stops the turn on the first detected loop.
	LoopNudgeMax *int `yaml:"loop_nudge_max"`
}

// EffectiveLLMRetryMax returns llm_retry_max with the default applied.
// An explicit 0 means retries are disabled.
func (c *Agent) EffectiveLLMRetryMax() int {
	if c.LLMRetryMax == nil {
		return AgentDefaultLLMRetryMax
	}
	return *c.LLMRetryMax
}

// EffectiveLLMFirstTokenTimeout returns llm_first_token_timeout_ms as a
// duration with the default applied. An explicit 0 disables the guard.
func (c *Agent) EffectiveLLMFirstTokenTimeout() time.Duration {
	if c.LLMFirstTokenTimeoutMS == nil {
		return AgentDefaultLLMFirstTokenTimeoutMS * time.Millisecond
	}
	return time.Duration(*c.LLMFirstTokenTimeoutMS) * time.Millisecond
}

// EffectiveLLMStallTimeout returns llm_stall_timeout_ms as a duration with the
// default applied. An explicit 0 disables the guard.
func (c *Agent) EffectiveLLMStallTimeout() time.Duration {
	if c.LLMStallTimeoutMS == nil {
		return AgentDefaultLLMStallTimeoutMS * time.Millisecond
	}
	return time.Duration(*c.LLMStallTimeoutMS) * time.Millisecond
}

// LLMStallRetryEnabled reports whether a silent LLM call is waited out and retried.
// Defaults to true when unset.
func (c *Agent) LLMStallRetryEnabled() bool {
	return c.LLMStallRetry == nil || *c.LLMStallRetry
}

// EffectiveLLMStallRetryDelays returns llm_stall_retry_delays_ms as durations with
// the default ladder applied. The caller repeats the last entry for every attempt
// past the end of the slice.
func (c *Agent) EffectiveLLMStallRetryDelays() []time.Duration {
	src := c.LLMStallRetryDelaysMS
	if len(src) == 0 {
		src = agentDefaultLLMStallRetryDelaysMS
	}
	out := make([]time.Duration, 0, len(src))
	for _, ms := range src {
		out = append(out, time.Duration(ms)*time.Millisecond)
	}
	return out
}

// EffectiveLLMStallRetryMaxWait returns llm_stall_retry_max_wait_ms as a duration
// with the default applied. An explicit 0 means unbounded.
func (c *Agent) EffectiveLLMStallRetryMaxWait() time.Duration {
	if c.LLMStallRetryMaxWaitMS == nil {
		return AgentDefaultLLMStallRetryMaxWaitMS * time.Millisecond
	}
	return time.Duration(*c.LLMStallRetryMaxWaitMS) * time.Millisecond
}

// LoopGuardEnabled reports whether runaway-loop protection is active. Defaults to true when unset.
func (c *Agent) LoopGuardEnabled() bool {
	return c.LoopGuard == nil || *c.LoopGuard
}

// EffectiveLoopToolRepeatLimit returns loop_tool_repeat_limit with the default applied.
func (c *Agent) EffectiveLoopToolRepeatLimit() int {
	if c.LoopToolRepeatLimit == nil {
		return AgentDefaultLoopToolRepeatLimit
	}
	return *c.LoopToolRepeatLimit
}

// EffectiveLoopStreamRepeatCycles returns loop_stream_repeat_cycles with the default applied.
func (c *Agent) EffectiveLoopStreamRepeatCycles() int {
	if c.LoopStreamRepeatCycles == nil {
		return AgentDefaultLoopStreamRepeatCycles
	}
	return *c.LoopStreamRepeatCycles
}

// EffectiveLoopToolCycleRepeats returns loop_tool_cycle_repeats with the default applied.
func (c *Agent) EffectiveLoopToolCycleRepeats() int {
	if c.LoopToolCycleRepeats == nil {
		return AgentDefaultLoopToolCycleRepeats
	}
	return *c.LoopToolCycleRepeats
}

// EffectiveLoopStuckAction returns loop_stuck_action with the default applied.
func (c *Agent) EffectiveLoopStuckAction() string {
	if c.LoopStuckAction == "" {
		return AgentDefaultLoopStuckAction
	}
	return c.LoopStuckAction
}

// EffectiveLoopNudgeMax returns loop_nudge_max with the default applied.
func (c *Agent) EffectiveLoopNudgeMax() int {
	if c.LoopNudgeMax == nil {
		return AgentDefaultLoopNudgeMax
	}
	return *c.LoopNudgeMax
}

// ApplyDefaults sets MaxTurns and MaxTokensPerTurn when they are zero.
func (c *Agent) ApplyDefaults() {
	if c.MaxTurns == 0 {
		c.MaxTurns = AgentDefaultMaxTurns
	}
	if c.MaxTokensPerTurn == 0 {
		c.MaxTokensPerTurn = AgentDefaultMaxTokensPerTurn
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
	if c.LLMRetryMax != nil && *c.LLMRetryMax < 0 {
		return fmt.Errorf("agent.llm_retry_max: must be >= 0")
	}
	if c.LLMRetryBaseMS < 0 {
		return fmt.Errorf("agent.llm_retry_base_ms: must be >= 0")
	}
	if c.LLMMinIntervalMS < 0 {
		return fmt.Errorf("agent.llm_min_interval_ms: must be >= 0")
	}
	if c.LLMFirstTokenTimeoutMS != nil && *c.LLMFirstTokenTimeoutMS < 0 {
		return fmt.Errorf("agent.llm_first_token_timeout_ms: must be >= 0")
	}
	if c.LLMStallTimeoutMS != nil && *c.LLMStallTimeoutMS < 0 {
		return fmt.Errorf("agent.llm_stall_timeout_ms: must be >= 0")
	}
	if c.LLMStallRetryMaxWaitMS != nil && *c.LLMStallRetryMaxWaitMS < 0 {
		return fmt.Errorf("agent.llm_stall_retry_max_wait_ms: must be >= 0")
	}
	for i, ms := range c.LLMStallRetryDelaysMS {
		if ms < 0 {
			return fmt.Errorf("agent.llm_stall_retry_delays_ms[%d]: must be >= 0", i)
		}
	}
	// Unbounded retries whose final pause is zero would re-issue the request in a
	// tight loop against a provider that is already struggling. Every other
	// combination is survivable, so this is the one pairing worth rejecting.
	if c.LLMStallRetryMaxWaitMS != nil && *c.LLMStallRetryMaxWaitMS == 0 {
		if delays := c.EffectiveLLMStallRetryDelays(); delays[len(delays)-1] <= 0 {
			return fmt.Errorf("agent.llm_stall_retry_max_wait_ms: 0 (unbounded) requires a non-zero final delay in agent.llm_stall_retry_delays_ms")
		}
	}
	if c.LoopToolRepeatLimit != nil && *c.LoopToolRepeatLimit < 0 {
		return fmt.Errorf("agent.loop_tool_repeat_limit: must be >= 0")
	}
	if c.LoopStreamRepeatCycles != nil && *c.LoopStreamRepeatCycles < 0 {
		return fmt.Errorf("agent.loop_stream_repeat_cycles: must be >= 0")
	}
	if c.LoopToolCycleRepeats != nil && *c.LoopToolCycleRepeats < 0 {
		return fmt.Errorf("agent.loop_tool_cycle_repeats: must be >= 0")
	}
	if c.LoopNudgeMax != nil && *c.LoopNudgeMax < 0 {
		return fmt.Errorf("agent.loop_nudge_max: must be >= 0")
	}
	switch c.LoopStuckAction {
	case "", AgentLoopStuckActionQuarantine, AgentLoopStuckActionStop:
	default:
		return fmt.Errorf("agent.loop_stuck_action: %q must be %q or %q",
			c.LoopStuckAction, AgentLoopStuckActionQuarantine, AgentLoopStuckActionStop)
	}
	return nil
}
