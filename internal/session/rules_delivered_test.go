package session

import (
	"testing"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/mention"
)

// The look this answers runs in front of every tool call (agent/rules_activation.go):
// it is folded with the history as the history grows, and a compaction that
// takes a rule out of the provider's window makes that rule deliverable again -
// a summarizer retelling it has not delivered it.
func TestDeliveredRuleBodiesAreFoldedWithTheHistory(t *testing.T) {
	st := &State{ID: "sess_delivered_rules", Mode: ModeAgent}
	ruleBody := mention.Attachment{Kind: mention.KindRule, Path: "AGENTS.md", Body: "ROOT_RULES"}.XML()

	if got := st.DeliveredRuleBodies(); len(got) != 0 {
		t.Fatalf("an empty history delivered %v", got)
	}
	st.AddMessage(llm.Message{Role: llm.RoleTool, Rules: ruleBody})
	if got := st.DeliveredRuleBodies(); got["AGENTS.md"] != "ROOT_RULES" {
		t.Fatalf("the attached rule is not in the delivered set: %v", got)
	}
	// More history without rules leaves what was read standing.
	st.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: "done"})
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: "and now?"})
	if got := st.DeliveredRuleBodies(); got["AGENTS.md"] != "ROOT_RULES" {
		t.Fatalf("the fold lost what it had already read: %v", got)
	}

	// A summary at the end is where the window opens now.
	st.InsertCompactionSummary(len(st.GetMessages()), NewCompactionSummaryMessage("the earlier work, with "+ruleBody, "fake/model"))
	if got := st.DeliveredRuleBodies(); len(got) != 0 {
		t.Fatalf("a rule the compaction folded away is still counted as delivered: %v", got)
	}
	st.AddMessage(llm.Message{Role: llm.RoleTool, Rules: ruleBody})
	if got := st.DeliveredRuleBodies(); got["AGENTS.md"] != "ROOT_RULES" {
		t.Fatalf("the rule did not come back after the compaction: %v", got)
	}
}

// A summary inserted among what was already read opens the window later than
// the start of the history: what sits before it is out of the provider's view
// and the fold must not keep counting it as delivered.
func TestDeliveredRuleBodiesStartOverWhenACompactionMovesTheWindow(t *testing.T) {
	st := &State{ID: "sess_delivered_window", Mode: ModeAgent}
	ruleBody := mention.Attachment{Kind: mention.KindRule, Path: "AGENTS.md", Body: "ROOT_RULES"}.XML()
	st.AddMessage(llm.Message{Role: llm.RoleTool, Rules: ruleBody})
	st.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: "kept verbatim"})
	if got := st.DeliveredRuleBodies(); got["AGENTS.md"] != "ROOT_RULES" {
		t.Fatalf("the attached rule is not in the delivered set: %v", got)
	}

	st.InsertCompactionSummary(1, NewCompactionSummaryMessage("what came before", "fake/model"))
	if got := st.DeliveredRuleBodies(); len(got) != 0 {
		t.Fatalf("the rule folded away by the summary is still counted as delivered: %v", got)
	}
}

// A mention and a tool result can name the same rule; the last body written
// against its path is the one the delivered set holds.
func TestDeliveredRuleBodiesKeepTheLatestBodyOfAPath(t *testing.T) {
	st := &State{ID: "sess_delivered_bodies", Mode: ModeAgent}
	first := mention.Attachment{Kind: mention.KindRule, Path: "AGENTS.md", Body: "V1"}.XML()
	second := mention.Attachment{Kind: mention.KindRule, Path: "AGENTS.md", Body: "V2"}.XML()
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: first})
	st.AddMessage(llm.Message{Role: llm.RoleTool, Rules: second})
	got := st.DeliveredRuleBodies()
	if len(got) != 1 || got["AGENTS.md"] != "V2" {
		t.Fatalf("delivered = %v, want the one path with its latest body", got)
	}
}
