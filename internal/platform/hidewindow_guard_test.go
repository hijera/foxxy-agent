package platform

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// spawnSiteExceptions lists the functions that start a process and deliberately
// do not hide its window, with the reason. Everything else has to hide one: the
// desktop shell owns no console, so any console child it starts blinks a window
// and a taskbar button at the operator.
var spawnSiteExceptions = map[string]string{
	"cmd/foxxycode/providers.go:openBrowser": "hands the URL to the browser the operator is about to log in with; SW_HIDE can travel to a window that must be seen",
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

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
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
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
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
		return nil
	})
	if err != nil {
		t.Fatalf("walking the tree: %v", err)
	}

	for key := range unused {
		t.Errorf("spawnSiteExceptions lists %s, which no longer starts a process there", key)
	}
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
