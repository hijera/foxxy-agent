package shell

import (
	"context"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/bgtask"
	"github.com/hijera/foxxycode-agent/internal/platform"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

func longRunningTestEnv(t *testing.T) (*tooling.Env, *bgtask.Pool) {
	t.Helper()
	pool := bgtask.NewWithRunner(bgtask.Config{}, bgtask.NewCommandRunner())
	t.Cleanup(func() { pool.StopSession("bdd-longrunning") })
	return &tooling.Env{
		SessionID:         "bdd-longrunning",
		CWD:               t.TempDir(),
		BackgroundEnabled: true,
		Background:        pool,
	}, pool
}

// startBackground runs the run_command tool with background: true and returns the
// tool result the model would read.
func startBackground(t *testing.T, env *tooling.Env, command string) string {
	t.Helper()
	tool := RunCommandToolForShell(platform.CurrentShell())
	args := `{"command":` + quoteJSON(command) + `,"background":true,"expected_seconds":30}`
	out, err := tool.Execute(context.Background(), args, env)
	if err != nil {
		t.Fatalf("run_command(%q): %v", command, err)
	}
	return out
}

func quoteJSON(s string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\`), `"`, `\"`) + `"`
}

// TestDevServerGetsNoHardTimeout is the regression for the reported failure: a
// dev server started in the background was killed 90 seconds in (an honest
// expected_seconds of 30, tripled), so by the time the browser tool was allowed
// to navigate, the port was dead and every attempt returned ERR_CONNECTION_REFUSED.
func TestDevServerGetsNoHardTimeout(t *testing.T) {
	env, pool := longRunningTestEnv(t)
	out := startBackground(t, env, "yarn serve --port 8082")

	tasks := pool.List(env.SessionID)
	if len(tasks) != 1 {
		t.Fatalf("expected one task, got %d", len(tasks))
	}
	if got := tasks[0].TimeoutSeconds; got > 0 {
		t.Errorf("dev server got a %ds hard timeout; it must have none", got)
	}
	// The launch message must not read as an instant kill, and must say how the
	// task is meant to end.
	if strings.Contains(out, "timeout 0s") {
		t.Errorf("launch message reports a zero timeout, which reads as an instant kill: %q", out)
	}
	if !strings.Contains(out, ToolBackgroundStop) {
		t.Errorf("launch message does not say how to end the task (%s): %q", ToolBackgroundStop, out)
	}
}

// TestOrdinaryCommandKeepsTheDefaultTimeout keeps the exemption narrow: work that
// does end on its own must still be bounded.
func TestOrdinaryCommandKeepsTheDefaultTimeout(t *testing.T) {
	env, pool := longRunningTestEnv(t)
	startBackground(t, env, "go build ./...")

	tasks := pool.List(env.SessionID)
	if len(tasks) != 1 {
		t.Fatalf("expected one task, got %d", len(tasks))
	}
	if tasks[0].TimeoutSeconds <= 0 {
		t.Errorf("an ordinary command lost its hard timeout (%d)", tasks[0].TimeoutSeconds)
	}
	// And the estimate must not have shortened it — that is the other half of the bug.
	if got := tasks[0].TimeoutSeconds; got == 90 {
		t.Errorf("timeout %d looks derived from expected_seconds*3, not the configured default", got)
	}
}
