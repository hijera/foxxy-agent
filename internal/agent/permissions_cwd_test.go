package agent

import (
	"path/filepath"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/tools"
)

// A session grant approves one command, not the directory it runs in. A
// run_command whose cwd leaves the workspace must ask again even when the
// command itself was granted, otherwise "npm test" approved in the project can
// run silently in any directory on disk.
func TestRunCommandOutsideTheWorkspaceAsksEvenWhenGranted(t *testing.T) {
	cwd := t.TempDir()
	outside := t.TempDir()
	env := &tools.Env{CWD: cwd, PermissionMode: config.PermModeAsk}
	grant := []string{"go test ./..."}

	inside := llm.ToolCall{Name: "run_command", InputJSON: `{"command":"go test ./...","cwd":"sub"}`}
	if permissionRequired(true, inside, env, grant, nil, nil) {
		t.Fatal("a granted command inside the workspace must not prompt again")
	}

	absolute := `{"command":"go test ./...","cwd":"` + filepath.ToSlash(outside) + `"}`
	out := llm.ToolCall{Name: "run_command", InputJSON: absolute}
	if !permissionRequired(true, out, env, grant, nil, nil) {
		t.Fatal("a granted command in a directory outside the workspace must prompt")
	}

	escape := llm.ToolCall{Name: "run_command", InputJSON: `{"command":"go test ./...","cwd":"../../.."}`}
	if !permissionRequired(true, escape, env, grant, nil, nil) {
		t.Fatal("a cwd that climbs out of the workspace must prompt")
	}
}
