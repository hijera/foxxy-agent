package session

// What the model can already read is asked for in front of every tool call, to
// decide whether the rule that call is about to bring in is new. The answer is
// folded into the session as the history grows rather than rebuilt each time:
// the deliverable set only grows between compactions, so the same walk of a
// thousand messages behind a hundred calls is work the calls have in common.

import (
	"strings"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/mention"
)

// DeliveredRuleBodies returns the rule attachments the LLM-visible window of the
// history carries, keyed by the path each attachment names with the body it
// holds: the ones a user message mentioned, the ones a tool result brought in.
// The summary that opens the window counts for nothing - a summarizer retelling
// a rule has not delivered it - so what a compaction folded away comes with the
// next call that matches it again.
//
// The walk is kept between calls and extended over the messages appended since
// the last one. It starts over when the history is edited rather than appended
// (a rewind, a rewrite), when it would read past the end of what it has, and
// when a compaction summary appears among the new messages and moves the
// window's start out from under the part already folded in.
//
// The map is a copy: a caller records in it what it is about to attach as it
// goes, and that is its own bookkeeping rather than something the history says.
func (s *State) DeliveredRuleBodies() map[string]string {
	s.deliveredMu.Lock()
	defer s.deliveredMu.Unlock()
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.deliveredValid || s.deliveredEditRev != s.msgEditRev || s.deliveredCount > len(s.Messages) {
		s.deliveredValid = true
		s.deliveredBodies = nil
		s.deliveredCount = llmWindowStart(s.Messages)
		s.deliveredEditRev = s.msgEditRev
	}
	for i := s.deliveredCount; i < len(s.Messages); i++ {
		m := s.Messages[i]
		s.deliveredCount = i + 1
		if m.CompactionSummary {
			// The window opens here now: everything folded in before this
			// summary is out of the provider's view and must be deliverable
			// again, so the fold carries on from a clean slate.
			s.deliveredBodies = nil
			continue
		}
		var text string
		switch m.Role {
		case llm.RoleUser:
			text = m.Content
		case llm.RoleTool:
			text = m.Rules
		default:
			continue
		}
		if !strings.Contains(text, `kind="rule"`) {
			continue
		}
		if s.deliveredBodies == nil {
			s.deliveredBodies = make(map[string]string, 4)
		}
		for _, blk := range mention.Blocks(text) {
			if blk.Kind == mention.KindRule {
				s.deliveredBodies[blk.Path] = blk.Body(text)
			}
		}
	}
	out := make(map[string]string, len(s.deliveredBodies))
	for path, body := range s.deliveredBodies {
		out[path] = body
	}
	return out
}
