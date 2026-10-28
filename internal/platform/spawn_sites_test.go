package platform

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// adaptsBeforeStart reports whether the file calls AdaptCommand, and does so
// before the first run of a command. The check reads the syntax tree, so a
// comment or a string that merely names the function does not count. A run is a
// Start, Run, Output or CombinedOutput on a variable whose name mentions cmd,
// the way every spawn site in the tree names its exec.Cmd.
func adaptsBeforeStart(path string, src []byte) (bool, error) {
	f, err := parser.ParseFile(token.NewFileSet(), path, src, 0)
	if err != nil {
		return false, err
	}
	adapt, start := token.NoPos, token.NoPos
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fn := call.Fun.(type) {
		case *ast.SelectorExpr:
			if fn.Sel.Name == "AdaptCommand" {
				adapt = earliest(adapt, call.Pos())
			}
			if runsCommand(fn) {
				start = earliest(start, call.Pos())
			}
		case *ast.Ident:
			if fn.Name == "AdaptCommand" {
				adapt = earliest(adapt, call.Pos())
			}
		}
		return true
	})
	return adapt != token.NoPos && (start == token.NoPos || adapt < start), nil
}

// The guard must not be fooled by text: a comment naming AdaptCommand, or a call
// made after the command already ran, is not an adapted spawn site.
func TestAdaptsBeforeStartReadsTheSyntaxTree(t *testing.T) {
	tests := []struct {
		name, src string
		want      bool
	}{
		{"called before start", `package p
func f(cmd *exec.Cmd) error {
	AdaptCommand(cmd)
	return cmd.Start()
}`, true},
		{"qualified call before run", `package p
func f(cmd *exec.Cmd) error {
	platform.AdaptCommand(cmd)
	return cmd.Run()
}`, true},
		{"only in a comment", `package p
func f(cmd *exec.Cmd) error {
	// platform.AdaptCommand(cmd) would go here
	return cmd.Start()
}`, false},
		{"only in a string", `package p
func f(cmd *exec.Cmd) error {
	_ = "platform.AdaptCommand(cmd)"
	return cmd.Start()
}`, false},
		{"after the command ran", `package p
func f(cmd *exec.Cmd) error {
	_ = cmd.Start()
	platform.AdaptCommand(cmd)
	return nil
}`, false},
		{"never called", `package p
func f(cmd *exec.Cmd) error { return cmd.Run() }`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := adaptsBeforeStart("x.go", []byte(tc.src))
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("adaptsBeforeStart = %v, want %v", got, tc.want)
			}
		})
	}
}

func earliest(cur, pos token.Pos) token.Pos {
	if cur == token.NoPos || pos < cur {
		return pos
	}
	return cur
}

// runsCommand matches cmd.Start(), cmd.Run(), cmd.Output() and
// cmd.CombinedOutput() on a receiver whose name mentions cmd.
func runsCommand(fn *ast.SelectorExpr) bool {
	switch fn.Sel.Name {
	case "Start", "Run", "Output", "CombinedOutput":
	default:
		return false
	}
	recv, ok := fn.X.(*ast.Ident)
	return ok && strings.Contains(strings.ToLower(recv.Name), "cmd")
}

// spawnSiteExemptions are the files that build an exec.Cmd without calling
// AdaptCommand, each with the reason it never starts a program on Android.
var spawnSiteExemptions = map[string]string{
	"internal/svnws/svntest/":     "test fixture helpers, never linked into a shipped binary",
	"internal/serve/systemd.go":   "systemctl, behind NewUserService, which refuses every GOOS but linux",
	"internal/update/packages.go": "dpkg-query, rpm and brew, asked on linux and darwin only",
	"internal/docsgen/":           "the documentation generator behind make docs, never shipped",
}

// TestEverySpawnSiteAdaptsTheCommand holds the rule AdaptCommand relies on. On
// Android a program FoxxyCode starts only runs once AdaptCommand has fitted the
// command to the device, and nothing but a device notices a spawn site that
// forgot it: every other host treats the call as a no-op. So every file outside
// the tests that builds an exec.Cmd calls it, or is exempted above with its
// reason.
func TestEverySpawnSiteAdaptsTheCommand(t *testing.T) {
	root := filepath.Join("..", "..")
	used := map[string]bool{}
	for _, top := range []string{"cmd", "internal", "external"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "node_modules" || d.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			name := d.Name()
			if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, "_windows.go") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			src := string(data)
			if first, _, _ := strings.Cut(src, "\n"); strings.TrimSpace(first) == "//go:build windows" {
				return nil
			}
			if !strings.Contains(src, "exec.Command(") && !strings.Contains(src, "exec.CommandContext(") {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			for exempt := range spawnSiteExemptions {
				if rel == exempt || (strings.HasSuffix(exempt, "/") && strings.HasPrefix(rel, exempt)) {
					used[exempt] = true
					return nil
				}
			}
			adapted, err := adaptsBeforeStart(path, data)
			if err != nil {
				t.Errorf("%s: %v", rel, err)
			} else if !adapted {
				t.Errorf("%s builds an exec.Cmd without platform.AdaptCommand before it starts: call it first, or exempt the file with the reason it never runs on Android", rel)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for exempt, reason := range spawnSiteExemptions {
		if !used[exempt] {
			t.Errorf("exemption %s (%s) names no file that starts a program: drop it", exempt, reason)
		}
	}
}
