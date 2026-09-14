package shell

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/bgtask"
	"github.com/hijera/foxxycode-agent/internal/platform"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

func longRunningTestEnv(t *testing.T) (*tooling.Env, *bgtask.Pool) {
	t.Helper()
	// The temp directory is claimed first so that its cleanup is registered
	// before the one below: cleanups run last-registered-first, and Windows
	// refuses to remove a directory a background task still has open as its
	// working directory.
	cwd := t.TempDir()
	pool := bgtask.NewWithRunner(bgtask.Config{}, bgtask.NewCommandRunner())
	t.Cleanup(func() { pool.StopSession("bdd-longrunning") })
	return &tooling.Env{
		SessionID:         "bdd-longrunning",
		CWD:               cwd,
		BackgroundEnabled: true,
		Background:        pool,
	}, pool
}

// startBackground runs the run_command tool with background: true and returns the
// tool result the model would read.
func startBackground(t *testing.T, env *tooling.Env, command string) string {
	t.Helper()
	tool := RunCommandToolForShell(platform.CurrentShell())
	args, err := json.Marshal(runCommandArgs{Command: command, Background: true, ExpectedSeconds: 30})
	if err != nil {
		t.Fatal(err)
	}
	out, err := tool.Execute(context.Background(), string(args), env)
	if err != nil {
		t.Fatalf("run_command(%q): %v", command, err)
	}
	return out
}

// waitForTaskOutput blocks until the session's single task has printed text.
func waitForTaskOutput(t *testing.T, pool *bgtask.Pool, sessionID, text string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if tasks := pool.List(sessionID); len(tasks) == 1 {
			// Listed before the output is read: a task that had already finished
			// by then has written everything it ever will.
			task := tasks[0]
			out, _, err := pool.Output(sessionID, task.ID, 0)
			if err != nil {
				t.Fatalf("read the output of task %s: %v", task.ID, err)
			}
			if strings.Contains(out, text) {
				return
			}
			if task.Status.Finished() {
				t.Fatalf("task %s ended %s without printing %q: %q", task.ID, task.Status, text, out)
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no task of session %s printed %q in time", sessionID, text)
}

// TestLongRunningEnvStopsTheTaskBeforeRemovingItsDirectory pins the order of the
// helper's cleanups. A task runs with the test's temp directory as its working
// directory, and Windows refuses to remove a directory a live process is using
// that way, so the pool must be stopped before t.TempDir's RemoveAll runs.
// Cleanups run last-registered-first: with the pool's cleanup registered first,
// the tests below failed on Windows whenever yarn or go outlived the test body.
func TestLongRunningEnvStopsTheTaskBeforeRemovingItsDirectory(t *testing.T) {
	kind := platform.CurrentShell().Kind
	printing, err := printCommand(kind, "task-started")
	if err != nil {
		t.Skipf("no printing form for this shell: %v", err)
	}
	sleeping, err := sleepCommand(kind, 60)
	if err != nil {
		t.Skipf("no sleep form for this shell: %v", err)
	}

	var env *tooling.Env
	var pool *bgtask.Pool
	t.Run("task outlives the test body", func(t *testing.T) {
		env, pool = longRunningTestEnv(t)
		startBackground(t, env, printing+"; "+sleeping)
		// Only a shell that has started is standing in the directory. Ending the
		// test before that would let RemoveAll win the race by luck.
		waitForTaskOutput(t, pool, env.SessionID, "task-started")
	})
	if env == nil {
		return
	}

	if _, err := os.Stat(env.CWD); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the task's working directory %s outlived the test (stat: %v)", env.CWD, err)
	}
	for _, task := range pool.List(env.SessionID) {
		if !task.Status.Finished() {
			t.Errorf("task %s is still %s after the test that started it ended", task.ID, task.Status)
		}
	}
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
