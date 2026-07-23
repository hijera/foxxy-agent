package agent

import "testing"

func TestModeToolCallRefusedOnlyUnderTheGuard(t *testing.T) {
	cases := []struct {
		name      string
		mode      string
		tool      string
		noSelfRun bool
		refused   bool
	}{
		{name: "plan_exit refused under the guard", mode: "plan", tool: "plan_exit", noSelfRun: true, refused: true},
		{name: "plan_exit allowed by default", mode: "plan", tool: "plan_exit", noSelfRun: false, refused: false},
		{name: "write refused under the guard", mode: "plan", tool: "write", noSelfRun: true, refused: true},
		{name: "apply_patch refused under the guard", mode: "plan", tool: "apply_patch", noSelfRun: true, refused: true},
		{name: "write still runs without the guard", mode: "plan", tool: "write", noSelfRun: false, refused: false},
		{name: "allowlisted tool always runs", mode: "plan", tool: "plan_write", noSelfRun: true, refused: false},
		{name: "MCP tools stay reachable in plan mode", mode: "plan", tool: "srv__do", noSelfRun: true, refused: false},
		{name: "agent mode is never restricted", mode: "agent", tool: "write", noSelfRun: true, refused: false},
		{name: "docs mode is untouched by the plan guard", mode: "docs", tool: "write", noSelfRun: true, refused: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := toolCallRefusedByMode(tc.mode, tc.tool, tc.noSelfRun); got != tc.refused {
				t.Fatalf("toolCallRefusedByMode(%q, %q, %v) = %v, want %v",
					tc.mode, tc.tool, tc.noSelfRun, got, tc.refused)
			}
		})
	}
}

func TestModeToolRefusalMentionsTheToolAndMode(t *testing.T) {
	msg := modeToolRefusalMessage("plan", "write")
	if msg == "" {
		t.Fatal("refusal message must not be empty")
	}
	for _, want := range []string{"write", "plan"} {
		if !contains(msg, want) {
			t.Errorf("refusal %q should mention %q", msg, want)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
