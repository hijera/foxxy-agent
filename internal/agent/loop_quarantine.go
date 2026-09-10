package agent

// Loop quarantine: what the guard does with a tool loop once nudging has failed
// and agent.loop_stuck_action is "quarantine". The looping calls are taken away
// for the rest of the turn while the turn itself runs on to a real answer -
// throwing the whole turn away is expensive, because by the time the guard trips
// the model has usually gathered most of what it needs.
//
// read and grep are the exception. Their loop exists precisely because the
// content keeps disappearing from the projection, so refusing the call would
// leave the model without the file it is asking for and doom the turn anyway.
// Each of them instead gets one last execution whose result is pinned against
// eviction: the oscillation ends because the model finally holds stable content,
// not because it was forbidden.

import (
	"encoding/json"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/llm"
)

// loopPinPrefix separates the two pin namespaces so a file path and a grep
// pattern that happen to read alike cannot pin each other.
const (
	loopPinReadPrefix = "read\x00"
	loopPinGrepPrefix = "grep\x00"
)

// loopContentTool reports whether a tool's whole purpose is to fetch content into
// the context, which is what makes blocking it self-defeating.
func loopContentTool(name string) bool {
	return name == "read" || name == "grep"
}

// quarantineLoop takes the detected loop away for the rest of the turn and reports
// whether the call in hand should still run once. Every member of the cycle is
// removed, not just the call that tripped the check, or the model would simply
// keep turning the rest of the same wheel - except content-fetching members, which
// each get their own final pinned pass when they come round.
func (a *Agent) quarantineLoop(tc llm.ToolCall, unit []string) (runOnce bool) {
	for _, k := range unit {
		if strings.HasPrefix(k, loopPinReadPrefix) || strings.HasPrefix(k, loopPinGrepPrefix) {
			continue
		}
		a.quarantineKey(k)
	}
	a.quarantineKey(canonicalToolCallKey(tc.Name, tc.InputJSON))
	if !loopContentTool(tc.Name) {
		return false
	}
	a.pinLoopResult(tc)
	return true
}

// resetLoopQuarantine clears what the guard took away, at the start of a turn.
// The pins deliberately survive: the file the model was circling stays available.
func (a *Agent) resetLoopQuarantine() {
	a.pinMu.Lock()
	defer a.pinMu.Unlock()
	a.loopQuarantine = nil
}

func (a *Agent) quarantineKey(key string) {
	a.pinMu.Lock()
	defer a.pinMu.Unlock()
	if a.loopQuarantine == nil {
		a.loopQuarantine = make(map[string]struct{}, 8)
	}
	a.loopQuarantine[key] = struct{}{}
}

func (a *Agent) isQuarantined(key string) bool {
	a.pinMu.Lock()
	defer a.pinMu.Unlock()
	_, ok := a.loopQuarantine[key]
	return ok
}

// loopQuarantineSnapshot copies the set for one projection pass.
func (a *Agent) loopQuarantineSnapshot() map[string]struct{} {
	a.pinMu.Lock()
	defer a.pinMu.Unlock()
	if len(a.loopQuarantine) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(a.loopQuarantine))
	for k := range a.loopQuarantine {
		out[k] = struct{}{}
	}
	return out
}

// pinLoopResult marks the result this call is about to produce as one eviction
// must never collapse again, so the content the model kept circling back for
// stays in the projection for the rest of the session.
func (a *Agent) pinLoopResult(tc llm.ToolCall) {
	key := loopPinKey(tc, a.state.GetCWD())
	if key == "" {
		return
	}
	a.pinMu.Lock()
	defer a.pinMu.Unlock()
	if a.loopPins == nil {
		a.loopPins = make(map[string]struct{}, 4)
	}
	a.loopPins[key] = struct{}{}
}

// loopPins returns a snapshot of the pinned keys for one eviction pass.
func (a *Agent) loopPinSnapshot() map[string]struct{} {
	a.pinMu.Lock()
	defer a.pinMu.Unlock()
	if len(a.loopPins) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(a.loopPins))
	for k := range a.loopPins {
		out[k] = struct{}{}
	}
	return out
}

// loopPinKey names the read or grep a call targets, in the same terms
// pruneToolResults uses to identify a surviving result: an absolute path for a
// read, the pattern for a grep.
func loopPinKey(tc llm.ToolCall, cwd string) string {
	var a struct {
		Path    string `json:"path"`
		Pattern string `json:"pattern"`
	}
	_ = json.Unmarshal([]byte(tc.InputJSON), &a)
	switch tc.Name {
	case "read":
		if strings.TrimSpace(a.Path) == "" {
			return ""
		}
		return loopPinReadPrefix + absPath(a.Path, cwd)
	case "grep":
		if a.Pattern == "" {
			return ""
		}
		return loopPinGrepPrefix + a.Pattern
	}
	return ""
}

// loopDuplicatePlaceholder replaces a repeat the guard has already answered once.
const loopDuplicatePlaceholder = "[duplicate: this exact call was already answered above; that result still stands]"

// collapseLoopDuplicates blanks the earlier copies of a result whose call the
// guard has quarantined, keeping the newest substantial one. One pass was enough:
// the repeats add nothing, they cost context, and leaving them in place hands the
// model the very pattern it was looping on - the same reason trimRepeatedTail cuts
// a repeated passage out of a stored assistant message rather than only hiding it.
//
// Scoped deliberately to quarantined keys. Collapsing identical calls in general
// would be wrong: run_command("go test") before and after an edit returns
// different output, and the second result is not a duplicate of the first.
//
// Messages are never removed, only rewritten: every tool_call still needs its
// tool_result or the next provider request is rejected.
// loopSyntheticResult reports whether a tool result was written by the agent
// itself rather than produced by an execution. Those must never be taken for the
// surviving copy: they are placeholders standing in for content, so keeping one
// and collapsing the real result would throw the content away. Size alone does
// not separate them - the quarantine answer is a couple of hundred bytes.
func loopSyntheticResult(content string) bool {
	trimmed := strings.TrimSpace(content)
	switch trimmed {
	case toolLoopNudge, toolLoopSkippedResult, toolCycleNudge, toolCycleSkippedResult,
		toolQuarantinedResult, loopDuplicatePlaceholder:
		return true
	}
	return strings.HasPrefix(trimmed, "[evicted:")
}

func collapseLoopDuplicates(history []llm.Message, quarantined map[string]struct{}, minBytes int) []llm.Message {
	if len(quarantined) == 0 || len(history) == 0 {
		return history
	}
	pendingCalls := make(map[string]llm.ToolCall)
	// Substantial results per quarantined key, in message order. Short synthetic
	// answers (the guard's own refusals, permission denials) are left alone: they
	// cost nothing and rewriting them would only lose information.
	byKey := make(map[string][]int)
	for i := range history {
		m := history[i]
		for _, tc := range m.ToolCalls {
			if id := strings.TrimSpace(tc.ID); id != "" {
				pendingCalls[id] = tc
			}
		}
		if m.Role != llm.RoleTool || len(m.Content) <= minBytes || loopSyntheticResult(m.Content) {
			continue
		}
		call, ok := pendingCalls[m.ToolCallID]
		if !ok {
			continue
		}
		delete(pendingCalls, m.ToolCallID)
		key := canonicalToolCallKey(call.Name, call.InputJSON)
		if _, ok := quarantined[key]; ok {
			byKey[key] = append(byKey[key], i)
		}
	}

	out := history
	cloned := false
	for _, idxs := range byKey {
		// Keep the newest: for read and grep that is the pinned final pass, the one
		// copy the model is meant to work from.
		for _, idx := range idxs[:len(idxs)-1] {
			if !cloned {
				out = append([]llm.Message(nil), history...)
				cloned = true
			}
			out[idx].Content = loopDuplicatePlaceholder
		}
	}
	return out
}
