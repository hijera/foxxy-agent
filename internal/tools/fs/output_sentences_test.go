package fs

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// The web UI renders these tools as cards by recognising the exact sentences
// their success paths return (external/ui/src/ui/chat/workspaceToolDisplay.ts).
// A rewording silently degrades those cards to raw text, so the wording is
// pinned here like any other user-visible copy.
func TestSuccessSentences(t *testing.T) {
	root := t.TempDir()
	env := &tooling.Env{CWD: root}
	ctx := context.Background()
	join := func(rel string) string { return filepath.Join(root, rel) }

	t.Run("mkdir", func(t *testing.T) {
		out, err := executeMkdir(ctx, `{"path":"build"}`, env)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if out != "created directory "+join("build") {
			t.Fatalf("mkdir = %q", out)
		}
	})

	t.Run("touch creates then updates", func(t *testing.T) {
		out, err := executeTouch(ctx, `{"path":"a.txt"}`, env)
		if err != nil {
			t.Fatalf("touch create: %v", err)
		}
		if out != "created file "+join("a.txt") {
			t.Fatalf("touch create = %q", out)
		}
		out, err = executeTouch(ctx, `{"path":"a.txt"}`, env)
		if err != nil {
			t.Fatalf("touch update: %v", err)
		}
		if out != "updated timestamps for "+join("a.txt") {
			t.Fatalf("touch update = %q", out)
		}
	})

	t.Run("rm file, tree and rmdir", func(t *testing.T) {
		out, err := executeRemove(ctx, `{"path":"a.txt"}`, env)
		if err != nil {
			t.Fatalf("rm file: %v", err)
		}
		if out != "removed "+join("a.txt") {
			t.Fatalf("rm file = %q", out)
		}
		if err := os.MkdirAll(join("cache/inner"), 0o755); err != nil {
			t.Fatalf("mkdir cache: %v", err)
		}
		out, err = executeRemove(ctx, `{"path":"cache","recursive":true}`, env)
		if err != nil {
			t.Fatalf("rm tree: %v", err)
		}
		if out != "removed tree "+join("cache") {
			t.Fatalf("rm tree = %q", out)
		}
		if err := os.Mkdir(join("empty"), 0o755); err != nil {
			t.Fatalf("mkdir empty: %v", err)
		}
		out, err = executeRmdir(ctx, `{"path":"empty"}`, env)
		if err != nil {
			t.Fatalf("rmdir: %v", err)
		}
		if out != "removed directory "+join("empty") {
			t.Fatalf("rmdir = %q", out)
		}
	})

	t.Run("mv", func(t *testing.T) {
		if err := os.WriteFile(join("draft.txt"), []byte("x"), 0o644); err != nil {
			t.Fatalf("write draft: %v", err)
		}
		out, err := executeMove(ctx, `{"src":"draft.txt","dst":"final.txt"}`, env)
		if err != nil {
			t.Fatalf("mv: %v", err)
		}
		if out != "moved "+join("draft.txt")+" -> "+join("final.txt") {
			t.Fatalf("mv = %q", out)
		}
	})

	t.Run("grep and glob sentinels", func(t *testing.T) {
		if err := os.WriteFile(join("note.txt"), []byte("hello\n"), 0o644); err != nil {
			t.Fatalf("write note: %v", err)
		}
		out, err := executeGrep(ctx, `{"pattern":"zzz"}`, env)
		if err != nil {
			t.Fatalf("grep: %v", err)
		}
		if out != "no matches found" {
			t.Fatalf("grep = %q", out)
		}
		out, err = executeGlob(ctx, `{"pattern":"*.zzz"}`, env)
		if err != nil {
			t.Fatalf("glob: %v", err)
		}
		if out != "(no files matched)" {
			t.Fatalf("glob = %q", out)
		}
	})
}
