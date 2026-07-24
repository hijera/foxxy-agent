package session_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/session"
)

func TestEffectiveSessionCWD(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	wantBase, err := filepath.Abs(base)
	if err != nil {
		t.Fatal(err)
	}
	t.Run("client wins", func(t *testing.T) {
		t.Parallel()
		sub := filepath.Join(base, "proj")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		got, err := session.EffectiveSessionCWD(sub, "/should-not-use")
		if err != nil {
			t.Fatal(err)
		}
		want, err := filepath.Abs(sub)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})
	t.Run("fallback when client empty", func(t *testing.T) {
		t.Parallel()
		got, err := session.EffectiveSessionCWD("   ", base)
		if err != nil {
			t.Fatal(err)
		}
		if got != wantBase {
			t.Fatalf("got %q want %q", got, wantBase)
		}
	})
	t.Run("both empty errors", func(t *testing.T) {
		t.Parallel()
		_, err := session.EffectiveSessionCWD("", "")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestCWDInScope(t *testing.T) {
	t.Parallel()
	sep := string(filepath.Separator)
	root := filepath.Join(sep+"work", "proj")
	cases := []struct {
		name string
		cwd  string
		root string
		want bool
	}{
		{"same directory", root, root, true},
		{"trailing separator on root", root, root + sep, true},
		{"unclean root", root, filepath.Join(root, "sub", ".."), true},
		{"direct child", filepath.Join(root, "sub"), root, true},
		{"nested child", filepath.Join(root, "a", "b", "c"), root, true},
		{"sibling with shared prefix", filepath.Join(sep+"work", "proj-other"), root, false},
		{"parent", filepath.Join(sep + "work"), root, false},
		{"unrelated", filepath.Join(sep+"other", "proj"), root, false},
		{"empty cwd", "", root, false},
		{"empty root matches everything", filepath.Join(sep+"other", "proj"), "", true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := session.CWDInScope(tc.cwd, tc.root); got != tc.want {
				t.Fatalf("CWDInScope(%q, %q) = %v, want %v", tc.cwd, tc.root, got, tc.want)
			}
		})
	}

	t.Run("separator flavours are interchangeable", func(t *testing.T) {
		t.Parallel()
		if !session.CWDInScope(filepath.Join(root, "sub"), filepath.ToSlash(root)) {
			t.Fatal("a forward-slash root must match a native-separator cwd")
		}
	})

	t.Run("case sensitivity follows the platform", func(t *testing.T) {
		t.Parallel()
		got := session.CWDInScope(filepath.Join(sep+"Work", "Proj", "sub"), root)
		want := runtime.GOOS == "windows"
		if got != want {
			t.Fatalf("case-mismatched scope = %v, want %v", got, want)
		}
	})
}
