package fs

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/plans"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

func TestEditPreviewMatchesWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "new.txt")

	absPath, before, after, ok, err := EditPreview("write", `{"path":"sub/new.txt","content":"hello"}`, dir, "")
	if err != nil || !ok {
		t.Fatalf("EditPreview write: ok=%v err=%v", ok, err)
	}
	if absPath != path {
		t.Errorf("absPath = %q, want %q", absPath, path)
	}
	if len(before) != 0 {
		t.Errorf("before = %q, want empty for new file", before)
	}
	if string(after) != "hello" {
		t.Errorf("after = %q, want %q", after, "hello")
	}
}

func TestEditPreviewMatchesEdit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("alpha beta gamma"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, before, after, ok, err := EditPreview("edit", `{"path":"f.txt","oldString":"beta","newString":"BETA"}`, dir, "")
	if err != nil || !ok {
		t.Fatalf("EditPreview edit: ok=%v err=%v", ok, err)
	}
	if string(before) != "alpha beta gamma" {
		t.Errorf("before = %q", before)
	}
	if string(after) != "alpha BETA gamma" {
		t.Errorf("after = %q, want %q", after, "alpha BETA gamma")
	}

	// Preview must not touch disk.
	onDisk, _ := os.ReadFile(path)
	if string(onDisk) != "alpha beta gamma" {
		t.Errorf("EditPreview mutated the file: %q", onDisk)
	}
}

func TestEditPreviewUnknownTool(t *testing.T) {
	_, _, _, ok, err := EditPreview("run_command", `{"command":"ls"}`, t.TempDir(), "")
	if ok || err != nil {
		t.Errorf("unknown tool: ok=%v err=%v, want false/nil", ok, err)
	}
}

func TestExecuteEditFiresOnFileEditHook(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("one two"), 0o644); err != nil {
		t.Fatal(err)
	}

	var gotTool, gotPath string
	var gotBefore, gotAfter []byte
	env := &tooling.Env{
		CWD: dir,
		OnFileEdit: func(toolName, absPath string, before, after []byte) {
			gotTool, gotPath = toolName, absPath
			gotBefore, gotAfter = before, after
		},
	}

	if _, err := executeEdit(context.Background(), `{"path":"f.txt","oldString":"two","newString":"THREE"}`, env); err != nil {
		t.Fatalf("executeEdit: %v", err)
	}

	if gotTool != "edit" || gotPath != path {
		t.Errorf("hook tool/path = %q/%q, want edit/%q", gotTool, gotPath, path)
	}
	if string(gotBefore) != "one two" || string(gotAfter) != "one THREE" {
		t.Errorf("hook before/after = %q/%q", gotBefore, gotAfter)
	}
}

func TestEditPreviewMatchesPlanWrite(t *testing.T) {
	sd := t.TempDir()
	args := `{"slug":"my-plan","content":"Overview: ship it\n\n- [ ] one"}`
	absPath, before, after, ok, err := EditPreview("plan_write", args, "", sd)
	if err != nil || !ok {
		t.Fatalf("EditPreview: ok=%v err=%v", ok, err)
	}
	if filepath.Base(absPath) != "my-plan.plan.md" {
		t.Fatalf("path = %q", absPath)
	}
	if len(before) != 0 {
		t.Fatalf("before of a new plan = %q", before)
	}
	// The preview must be byte-for-byte what executePlanWrite writes.
	doc, err := plans.Parse("my-plan", "Overview: ship it\n\n- [ ] one")
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != doc.Content {
		t.Fatalf("after = %q, want the normalized plan content %q", after, doc.Content)
	}
	// Without a session directory the preview declines instead of guessing.
	if _, _, _, ok, _ := EditPreview("plan_write", args, "", ""); ok {
		t.Fatal("plan_write preview must decline without a session directory")
	}
}
