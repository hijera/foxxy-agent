package agent

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// projectionAgent is an agent whose model has a window large enough that the
// short histories below sit far under compaction.result_eviction.start_percent.
func projectionAgent(t *testing.T) *Agent {
	t.Helper()
	st := &session.State{ID: "sess_projection", CWD: t.TempDir(), Mode: session.ModeAgent, SessionDir: t.TempDir()}
	// The fixtures' tool results are about a kilobyte; the default
	// min_result_bytes would leave them alone whatever else is decided.
	minBytes := 10
	return NewAgent(&config.Config{
		Providers:  []config.ProviderConfig{{Name: "fake", Type: "openai", APIKey: "test"}},
		Models:     []config.ModelEntry{{Model: "fake/model", MaxContextTokens: 128000}},
		Agent:      config.Agent{Model: "fake/model"},
		Compaction: config.CompactionConfig{ResultEviction: config.ResultEviction{MinResultBytes: &minBytes}},
	}, st, resumePermissionSender{}, nil)
}

// Below start_percent the read/grep eviction is held back so the provider's
// prompt cache keeps the history. The loop guard's collapse is not a space
// saver and must not be held back with it: a quarantined loop replayed in full
// is how the model picks the loop straight back up.
func TestQuarantineCollapseIsNotGatedByEvictionStartPercent(t *testing.T) {
	ag := projectionAgent(t)
	args := `{"pattern":"**/*.go"}`
	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "list the go files"},
		asstCall("g1", "glob", args),
		toolResult("g1", bigBody("RUN-1")),
		asstCall("g2", "glob", args),
		toolResult("g2", bigBody("RUN-2")),
	}
	if ag.evictionDue(msgs) {
		t.Fatal("the fixture must sit below start_percent, or this test proves nothing")
	}
	if got := ag.prunedForLLM(msgs); !reflect.DeepEqual(got, msgs) {
		t.Fatal("below start_percent, with nothing quarantined, the history must go out untouched")
	}

	ag.quarantineKey(canonicalToolCallKey("glob", args))
	out := ag.prunedForLLM(msgs)
	if contentByID(out, "g1") != loopDuplicatePlaceholder {
		t.Fatalf("the earlier repeat of a quarantined call must collapse below start_percent too: %q", contentByID(out, "g1"))
	}
	if !strings.Contains(contentByID(out, "g2"), "RUN-2") {
		t.Fatalf("the newest copy is the one the model works from: %q", contentByID(out, "g2"))
	}
}

// The collapse costs the prompt cache once, on the step the loop is quarantined.
// From then on the prefix has to hold still again: the same history projects to
// the same bytes, and a step appended behind it leaves what came before alone.
func TestQuarantineProjectionIsStableAcrossSteps(t *testing.T) {
	ag := projectionAgent(t)
	args := `{"pattern":"**/*.go"}`
	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "list the go files"},
		asstCall("g1", "glob", args),
		toolResult("g1", bigBody("RUN-1")),
		asstCall("g2", "glob", args),
		toolResult("g2", bigBody("RUN-2")),
	}
	ag.quarantineKey(canonicalToolCallKey("glob", args))

	first := ag.prunedForLLM(msgs)
	if again := ag.prunedForLLM(msgs); !reflect.DeepEqual(first, again) {
		t.Fatal("the same history projected twice must give the same request")
	}

	next := append(append([]llm.Message(nil), msgs...),
		asstCall("r1", "read", `{"path":"main.go"}`),
		toolResult("r1", "package main"),
	)
	second := ag.prunedForLLM(next)
	if len(second) < len(first) || !reflect.DeepEqual(second[:len(first)], first) {
		t.Fatal("a step appended behind a quarantined loop rewrote the prefix the provider had cached")
	}
}
