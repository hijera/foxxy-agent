package svn_test

import (
	"path/filepath"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/svn"
)

func TestPoliciesRemainAvailableWithoutAClient(t *testing.T) {
	cwd := t.TempDir()
	for _, name := range svn.ToolNames() {
		policy, ok := svn.ToolPolicy(name)
		if !ok {
			t.Fatalf("missing replay policy for %s", name)
		}
		if policy.Mutates && policy.Kind != "write" {
			t.Fatalf("%s: inconsistent mutation policy", name)
		}
	}
	for _, name := range []string{"svn_add", "svn_commit"} {
		if paths, handled := svn.WriteTargets(name, `{}`, cwd); !handled || len(paths) != 0 {
			t.Fatalf("%s invalidated existing file reads: %v, %v", name, paths, handled)
		}
	}
	paths, handled := svn.WriteTargets("svn_update", `{"paths":["src/main.go"]}`, cwd)
	if !handled || len(paths) != 1 || paths[0] != filepath.Join(cwd, "src", "main.go") {
		t.Fatalf("targeted update: %v, %v", paths, handled)
	}
	paths, _ = svn.WriteTargets("svn_switch", `{}`, cwd)
	if len(paths) != 1 || paths[0] != cwd {
		t.Fatalf("switch must invalidate the workspace: %v", paths)
	}
	if _, ok := svn.ToolPolicy("svn_unrecognized"); ok {
		t.Fatal("unknown tool acquired a registered policy")
	}
}

func TestModeContributionsDoNotGrantWrites(t *testing.T) {
	for _, mode := range []string{"plan", "ask", "docs", "debug"} {
		for _, name := range svn.ToolsForMode(mode) {
			p, ok := svn.ToolPolicy(name)
			if !ok || p.Mutates || mode != "plan" {
				t.Fatalf("unexpected contribution %s to %s", name, mode)
			}
		}
	}
	if len(svn.ToolsForMode("plan")) != 5 {
		t.Fatal("plan lost SVN inspection tools")
	}
}

// TestToolPolicyPartitionsTheRegisteredNames pins the classification to the
// name itself: ReadOnlyToolNames has to be exactly the non-mutating half of
// ToolNames, and every registered name has to resolve. A policy derived from
// positions in ToolNames rather than from the lists themselves shifts onto the
// wrong tools as soon as the two drift, and nothing else in the tree notices.
func TestToolPolicyPartitionsTheRegisteredNames(t *testing.T) {
	readOnly := map[string]bool{}
	for _, name := range svn.ReadOnlyToolNames() {
		readOnly[name] = true
	}
	mutating := 0
	for _, name := range svn.ToolNames() {
		policy, ok := svn.ToolPolicy(name)
		if !ok {
			t.Fatalf("%s is registered but has no policy", name)
		}
		if readOnly[name] != !policy.Mutates {
			t.Fatalf("%s: ReadOnlyToolNames says read-only=%v, policy says Mutates=%v",
				name, readOnly[name], policy.Mutates)
		}
		if policy.Mutates {
			mutating++
		}
	}
	if mutating == 0 || len(readOnly) == 0 {
		t.Fatalf("both classes must stay non-empty: readOnly=%d mutating=%d", len(readOnly), mutating)
	}
}
