package agent

import "testing"

// The fork has four modes where upstream has two, so which background tools each
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

// Ask mode grants run_command in its extended set, so a backgrounded command is
// reachable there and has to stay observable.
func TestAskExtendedObservesBackgroundTasksButCannotReap(t *testing.T) {
	set := ToolSetForMode("ask", false)
	for _, name := range backgroundObserveTools() {
		if !set.Allows(name) {
			t.Errorf("ask mode should allow %s", name)
		}
	}
	if set.Allows("background_reap") {
		t.Errorf("ask mode must not allow background_reap")
	}
}

// Ask basic-only drops run_command, so nothing can start a task and the pool
// tools have nothing to observe.
func TestAskBasicOnlyHasNoBackgroundTools(t *testing.T) {
	set := ToolSetForMode("ask", false, true)
	for _, name := range append(backgroundObserveTools(), "background_reap") {
		if set.Allows(name) {
			t.Errorf("ask basic-only must not allow %s", name)
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
// which is what actually refuses a call the model makes anyway.
func TestBackgroundReapRefusedByEnforcedModes(t *testing.T) {
	cases := []struct {
		mode      string
		noSelfRun bool
		basicOnly []bool
	}{
		{mode: "ask"},
		{mode: "plan", noSelfRun: true},
	}
	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			if !toolCallRefusedByMode(tc.mode, "background_reap", tc.noSelfRun, tc.basicOnly...) {
				t.Errorf("%s mode should refuse background_reap", tc.mode)
			}
			for _, name := range backgroundObserveTools() {
				if toolCallRefusedByMode(tc.mode, name, tc.noSelfRun, tc.basicOnly...) {
					t.Errorf("%s mode should not refuse %s", tc.mode, name)
				}
			}
		})
	}
}

// Agent mode is unrestricted, so every background tool including reap is
// reachable there.
func TestAgentModeAllowsEveryBackgroundTool(t *testing.T) {
	for _, name := range append(backgroundObserveTools(), "background_reap") {
		if toolCallRefusedByMode("agent", name, false) {
			t.Errorf("agent mode should not refuse %s", name)
		}
	}
}
