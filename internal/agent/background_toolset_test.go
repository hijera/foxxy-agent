package agent

import "testing"

// The fork has five modes where upstream has three, so which background tools each
// mode gets is a fork decision rather than a ported one. The observing tools go
// wherever a command can be started; background_reap never does, because it
// kills process groups the session did not start.

func backgroundObserveTools() []string {
	return []string{"background_list", "background_output", "background_wait", "background_stop"}
}

func TestPlanModeObservesBackgroundTasksButCannotReap(t *testing.T) {
	set := ToolSetForMode("plan", false)
	for _, name := range backgroundObserveTools() {
		if !set.Allows(name) {
			t.Errorf("plan mode should allow %s", name)
		}
	}
	if set.Allows("background_reap") {
		t.Errorf("plan mode must not allow background_reap")
	}
}

// Ask mode has no run_command at all (upstream's flat read-only set), so
// nothing can start a task and the pool tools have nothing to observe.
func TestAskModeHasNoBackgroundTools(t *testing.T) {
	set := ToolSetForMode("ask", false)
	for _, name := range append(backgroundObserveTools(), "background_reap") {
		if set.Allows(name) {
			t.Errorf("ask mode must not allow %s", name)
		}
	}
}

// Docs mode has no run_command at all, so background tools would be dead weight.
func TestDocsModeHasNoBackgroundTools(t *testing.T) {
	set := ToolSetForMode("docs", false)
	for _, name := range append(backgroundObserveTools(), "background_reap") {
		if set.Allows(name) {
			t.Errorf("docs mode must not allow %s", name)
		}
	}
}

// The allowlists above only bite when the mode is enforced at execution time,
// which is what actually refuses a call the model makes anyway. Plan keeps the
// observing tools under the self-run guard; ask refuses the whole family.
func TestBackgroundReapRefusedByEnforcedModes(t *testing.T) {
	if _, refused := toolCallRefusedByMode("plan", "background_reap", true); !refused {
		t.Error("plan mode should refuse background_reap")
	}
	for _, name := range backgroundObserveTools() {
		if _, refused := toolCallRefusedByMode("plan", name, true); refused {
			t.Errorf("plan mode should not refuse %s", name)
		}
	}
	for _, name := range append(backgroundObserveTools(), "background_reap") {
		if _, refused := toolCallRefusedByMode("ask", name, false); !refused {
			t.Errorf("ask mode should refuse %s", name)
		}
	}
}

// Agent mode is unrestricted, so every background tool including reap is
// reachable there.
func TestAgentModeAllowsEveryBackgroundTool(t *testing.T) {
	for _, name := range append(backgroundObserveTools(), "background_reap") {
		if _, refused := toolCallRefusedByMode("agent", name, false); refused {
			t.Errorf("agent mode should not refuse %s", name)
		}
	}
}
