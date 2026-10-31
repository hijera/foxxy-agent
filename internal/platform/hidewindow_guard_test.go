package platform

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// spawnSiteExceptions lists the functions that start a process and deliberately
// do not hide its window, with the reason. Everything else has to hide one: the
// desktop shell owns no console, so any console child it starts blinks a window
// and a taskbar button at the operator.
var spawnSiteExceptions = map[string]string{
	"internal/serve/daemon.go:StartDetached":              "DetachFromTerminal already starts the dispatcher with DETACHED_PROCESS and HideWindow, so it has no console to show a window for",
	"cmd/foxxycode/providers.go:openBrowser":              "hands the URL to the browser the operator is about to log in with; SW_HIDE can travel to a window that must be seen",
	"internal/update/windows_windows.go:restartFoxxyCode": "restarts the freshly installed FoxxyCode for the operator; hiding it would make the update end in a disappearance",
	"internal/docsgen/cliref.go:BuildFoxxyCode":           "the documentation generator (make docs, CI) builds the binary from a terminal; it never runs inside the desktop shell",
	"internal/docsgen/cliref.go:CLIReference":             "the documentation generator runs the freshly built binary's --help from a terminal or CI, never inside the desktop shell",
}

// The fix is only worth as much as its coverage: one forgotten spawn site is one
// window still flashing. Rather than trust that every future caller remembers,
// this walks the tree and asks the compiler's own parse of it.
func TestEverySpawnSiteHidesItsConsoleWindow(t *testing.T) {
	root := filepath.Join("..", "..")
	unused := map[string]bool{}
	for key := range spawnSiteExceptions {
		unused[key] = true
	}

	for _, path := range guardGoFiles(t, root) {
		fset := token.NewFileSet()
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			t.Fatalf("%s: %v", path, parseErr)
		}
		rel := filepath.ToSlash(mustRel(t, root, path))

		spawnsInFuncs := 0
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			spawns := countCalls(fn.Body, "exec", "Command", "CommandContext")
			spawnsInFuncs += spawns
			if spawns == 0 {
				continue
			}
			key := rel + ":" + fn.Name.Name
			if _, exempt := spawnSiteExceptions[key]; exempt {
				delete(unused, key)
				continue
			}
			if countCalls(fn.Body, "", "HideConsoleWindow") == 0 {
				t.Errorf("%s starts a process without platform.HideConsoleWindow; on the desktop shell that child pops a console window (add the call, or document an exception in spawnSiteExceptions)", key)
			}
		}
		if total := countCalls(file, "exec", "Command", "CommandContext"); total != spawnsInFuncs {
			t.Errorf("%s starts a process outside any function, where no window can be hidden", rel)
		}
	}

	for key := range unused {
		t.Errorf("spawnSiteExceptions lists %s, which no longer starts a process there", key)
	}
}

// guardGoFiles lists the .go sources the guard covers: everything git tracks
// plus everything not ignored - the same set a commit could carry. A worktree
// the worktree tool or an editor leaves inside the checkout is a gitignored
// copy of this very tree, and walking it fails the guard with the stale spawn
// sites of the snapshot it holds (internal/docsgen filters its walks the same
// way). Without git - a tarball, a zip download - the whole tree is covered.
func guardGoFiles(t *testing.T, root string) []string {
	t.Helper()
	out, err := exec.Command("git", "-C", root, "ls-files", "--cached", "--others", "--exclude-standard").Output()
	if err == nil {
		var files []string
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasSuffix(line, ".go") && !strings.HasSuffix(line, "_test.go") {
				files = append(files, filepath.Join(root, line))
			}
		}
		return files
	}
	var files []string
	werr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "build", "dist", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(d.Name(), ".go") && !strings.HasSuffix(d.Name(), "_test.go") {
			files = append(files, path)
		}
		return nil
	})
	if werr != nil {
		t.Fatalf("walking the tree: %v", werr)
	}
	return files
}

// countCalls counts calls to pkg.name(...) under node. An empty pkg matches the
// selector on any receiver, which is what lets one query cover both the
// platform.HideConsoleWindow callers and the call inside the package itself.
func countCalls(node ast.Node, pkg string, names ...string) int {
	wanted := map[string]bool{}
	for _, name := range names {
		wanted[name] = true
	}
	count := 0
	ast.Inspect(node, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fn := call.Fun.(type) {
		case *ast.SelectorExpr:
			if !wanted[fn.Sel.Name] {
				return true
			}
			if pkg == "" {
				count++
				return true
			}
			if ident, ok := fn.X.(*ast.Ident); ok && ident.Name == pkg {
				count++
			}
		case *ast.Ident:
			if pkg == "" && wanted[fn.Name] {
				count++
			}
		}
		return true
	})
	return count
}

func mustRel(t *testing.T, base, path string) string {
	t.Helper()
	rel, err := filepath.Rel(base, path)
	if err != nil {
		t.Fatalf("relative path of %s: %v", path, err)
	}
	return rel
}
