//go:build cli

package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/external/cli/tui"
	"github.com/hijera/foxxycode-agent/internal/acp"
)

// TestDetectGitBranchGivesUpOnAHungGit pins the bound behind the footer's
// branch label: a git that never answers yields an empty label within the
// timeout instead of holding the console before its first frame.
func TestDetectGitBranchGivesUpOnAHungGit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the hung git stand-in is a shell script")
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\nexec sleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	prev := gitBranchTimeout
	gitBranchTimeout = 200 * time.Millisecond
	t.Cleanup(func() { gitBranchTimeout = prev })

	started := time.Now()
	if got := detectGitBranch(t.TempDir()); got != "" {
		t.Fatalf("branch = %q, want empty for a git that never answered", got)
	}
	if took := time.Since(started); took > 3*time.Second {
		t.Fatalf("detectGitBranch waited %v for a hung git", took)
	}
}

// TestDetectGitBranchReadsTheBranch keeps the label itself honest: a real
// repository on the checked-out branch names it.
func TestDetectGitBranchReadsTheBranch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in git is a shell script")
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\necho feature/x\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if got := detectGitBranch(t.TempDir()); got != "feature/x" {
		t.Fatalf("branch = %q, want feature/x", got)
	}
}

func TestFooterNamesRunningTasksAndTheCommandThatListsThem(t *testing.T) {
	f := newFooter(newTheme("dark"), "/work")
	if got := strings.Join(f.Render(120), "\n"); strings.Contains(got, "running") {
		t.Fatalf("an idle session names running tasks:\n%s", got)
	}
	f.SetRunningTasks(1)
	if got := f.Render(120)[0]; !strings.Contains(got, "1 task running (/tasks)") {
		t.Fatalf("footer = %q", got)
	}
	f.SetRunningTasks(3)
	if got := f.Render(120)[0]; !strings.Contains(got, "3 tasks running (/tasks)") {
		t.Fatalf("footer = %q", got)
	}
}

// On a narrow line with every note up - the MCP count, the running tasks, the
// permission mode - the notes stay whole together and the path gives way.
func TestFooterKeepsEveryNoteWhenTheLineIsNarrow(t *testing.T) {
	cwd := "/var/folders/36/tjdph2t965j8snz9_vkdnw0r0000gn/T/foxxycode-cli-bdd-1789734719342464000/work"
	f := newFooter(newTheme("dark"), cwd)
	f.title = "Session manager walkthrough"
	f.SetMCP(1, 3, true)
	f.SetRunningTasks(2)
	f.permission = "bypass"
	line := tui.StripTerminalSequences(f.Render(80)[0])
	for _, note := range []string{"MCP 1/3", "2 tasks running (/tasks)", "bypass"} {
		if !strings.Contains(line, note) {
			t.Fatalf("the note %q fell off the footer:\n%s", note, line)
		}
	}
	if got := tui.VisibleWidth(line); got > 80 {
		t.Fatalf("footer line is %d cells wide, want at most 80", got)
	}
	if !strings.Contains(line, "/var/folders") {
		t.Fatalf("the path vanished instead of giving way:\n%s", line)
	}
}

// On a line with room for the notes and not for any of the path, the path
// goes and the notes stay.
func TestFooterDropsThePathBeforeANote(t *testing.T) {
	f := newFooter(newTheme("dark"), "/home/operator/projects/foxxycode")
	f.SetMCP(1, 3, true)
	f.SetRunningTasks(2)
	f.permission = "bypass"
	line := tui.StripTerminalSequences(f.Render(48)[0])
	for _, note := range []string{"MCP 1/3", "2 tasks running (/tasks)", "bypass"} {
		if !strings.Contains(line, note) {
			t.Fatalf("the note %q fell off the footer:\n%s", note, line)
		}
	}
	if got := tui.VisibleWidth(line); got > 48 {
		t.Fatalf("footer line is %d cells wide, want at most 48", got)
	}
}

// A long working directory - a macOS temp folder, a deep monorepo path - must not push
// the running tasks off the line: the path gives way, the count and the command that
// lists the tasks stay.
func TestFooterKeepsTheRunningTasksWhenThePathIsLong(t *testing.T) {
	cwd := "/var/folders/36/tjdph2t965j8snz9_vkdnw0r0000gn/T/foxxycode-cli-bdd-1789734719342464000/work"
	f := newFooter(newTheme("dark"), cwd)
	f.title = "Session manager walkthrough"
	f.SetRunningTasks(1)
	line := f.Render(100)[0]
	if !strings.Contains(line, "1 task running (/tasks)") {
		t.Fatalf("the running tasks fell off the footer:\n%s", line)
	}
	if got := tui.VisibleWidth(line); got > 100 {
		t.Fatalf("footer line is %d cells wide, want at most 100", got)
	}
	if !strings.Contains(line, "/var/folders") {
		t.Fatalf("the path vanished instead of giving way:\n%s", line)
	}
}

// The token_usage updates carry the turn's cumulative counters, so the session
// totals count each turn once: a turn whose updates grew 70 to 140 spends 140,
// not 210, and the 90 of the next turn bring the total to 230.
func TestFooterTakesTheLatestCumulativeUsagePerTurn(t *testing.T) {
	a := newTestApp(t)
	send := func(u interface{}) { a.applyLoopMessage(updateMsg{update: u}) }

	first := "2026-11-26T10:00:00Z"
	send(acp.TurnProgressUpdate{SessionUpdate: acp.UpdateTypeTurnProgress, StartedAt: first})
	send(acp.TokenUsageUpdate{SessionUpdate: "token_usage", InputTokens: 70, OutputTokens: 30, TotalTokens: 100})
	send(acp.TokenUsageUpdate{SessionUpdate: "token_usage", InputTokens: 140, OutputTokens: 60, TotalTokens: 200})
	if got := a.foot.tokensIn; got != 140 {
		t.Fatalf("after a two-call turn the footer counts %d in, want the turn's cumulative 140, not the sum of the snapshots (210)", got)
	}

	second := "2026-11-26T10:01:00Z"
	send(acp.TurnProgressUpdate{SessionUpdate: acp.UpdateTypeTurnProgress, StartedAt: second})
	send(acp.TokenUsageUpdate{SessionUpdate: "token_usage", InputTokens: 50, OutputTokens: 20, TotalTokens: 70})
	send(acp.TokenUsageUpdate{SessionUpdate: "token_usage", InputTokens: 90, OutputTokens: 40, TotalTokens: 130})
	if in, out := a.foot.tokensIn, a.foot.tokensOut; in != 230 || out != 100 {
		t.Fatalf("footer totals = %d in, %d out; want the turn totals summed once (230 and 100)", in, out)
	}
}
