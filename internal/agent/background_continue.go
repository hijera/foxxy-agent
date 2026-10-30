package agent

import (
	"time"

	"github.com/hijera/foxxycode-agent/internal/llm"
)

// unattendedContinuation excludes the memory child whose report serves the
// interactive turn waiting for it. Scheduled and delegated runs share the policy.
func (a *Agent) unattendedContinuation() bool {
	return a.subagent != nil && a.subagent.Kind == ""
}

// fork(continue-budget): foreground and background budgets both stay outside max_turns.
func (a *Agent) continuationBudget() int {
	if a.unattendedContinuation() {
		return a.cfg.Agent.EffectiveLLMContinueBackgroundMax()
	}
	return a.cfg.Agent.EffectiveLLMContinueMax()
}

// continuationPause takes the number of continuations already attempted.
func (a *Agent) continuationPause(n int, stalled bool, err error) time.Duration {
	if !a.unattendedContinuation() {
		if stalled {
			return continueDelay(a.cfg.Agent.EffectiveLLMContinueStallDelays(), n)
		}
		return errorContinueDelay(&a.cfg.Agent, n, err)
	}
	delay := continueDelay(a.cfg.Agent.EffectiveLLMContinueBackgroundDelays(), n)
	limit := a.cfg.Agent.EffectiveLLMContinueRetryAfterMax()
	if asked, ok := llm.UpstreamRetryAfter(err); ok && limit > 0 && asked > delay {
		delay = min(asked, max(limit, delay))
	}
	return delay
}

type providerRecoveryObserver interface {
	ProviderRecovery(err error, delay time.Duration, attempt, budget int)
}

func (a *Agent) reportProviderRecovery(err error, delay time.Duration, attempt, budget int) {
	if observer, ok := a.server.(providerRecoveryObserver); ok {
		observer.ProviderRecovery(err, delay, attempt, budget)
	}
}
