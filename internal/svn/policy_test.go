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
