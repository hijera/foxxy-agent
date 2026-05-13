package agent

import (
	"testing"

	"github.com/EvilFreelancer/coddy-agent/internal/tools"
)

func TestPlanToolSetFiltersToReadWebAndShell(t *testing.T) {
	r := tools.NewRegistry()
	set := ToolSetForMode("plan")
	filtered := FilterToolDefinitions(r.AllToolDefinitions(), set)
	got := make(map[string]bool)
	for _, d := range filtered {
		got[d.Name] = true
	}
	for _, want := range []string{"read_file", "list_dir", "search_files", "search_web", "extract_page_content", "run_command"} {
		if !got[want] {
			t.Errorf("plan toolset should include %q", want)
		}
	}
	for _, forbid := range []string{"write_text_file", "coddy_todo_plan_read"} {
		if got[forbid] {
			t.Errorf("plan toolset should not include %q", forbid)
		}
	}
}

func TestToolSetForAgentIsUnrestricted(t *testing.T) {
	set := ToolSetForMode("agent")
	if !set.Unrestricted() {
		t.Fatal("agent mode should use unrestricted tool set")
	}
}
