package plans_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/plans"
)

func TestValidateSlug(t *testing.T) {
	cases := []struct {
		slug string
		ok   bool
	}{
		{"auth-refactor", true},
		{"a", true},
		{"", false},
		{"Auth", false},
		{"-bad", false},
		{"bad-", false},
	}
	for _, tc := range cases {
		err := plans.ValidateSlug(tc.slug)
		if tc.ok && err != nil {
			t.Errorf("slug %q: want ok, got %v", tc.slug, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("slug %q: want error", tc.slug)
		}
	}
}

func TestParseTodosAsPlainStrings(t *testing.T) {
	raw := `---
name: QA plan
overview: Test scenarios
todos:
  - Опросить требования
  - Сформировать сценарии
  - Добавить отчёт
---
## Body
`
	doc, err := plans.Parse("qa-plan", raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Todos) != 3 {
		t.Fatalf("todos len: %d %+v", len(doc.Todos), doc.Todos)
	}
	if doc.Todos[0].Content != "Опросить требования" {
		t.Fatalf("todo[0]: %q", doc.Todos[0].Content)
	}
}

func TestParseTodosWithTitleField(t *testing.T) {
	raw := `---
name: Demo
todos:
  - title: Set up UI component
    status: pending
---
## Steps
`
	doc, err := plans.Parse("demo", raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Todos) != 1 || doc.Todos[0].Content != "Set up UI component" {
		t.Fatalf("todos: %+v", doc.Todos)
	}
	if doc.Todos[0].Status != "pending" {
		t.Fatalf("status: %q", doc.Todos[0].Status)
	}
}

func TestParsePlanFileWithIndentedTodos(t *testing.T) {
	raw := "---\nname: Meta title\noverview: Short overview\ntodos:\n  - content: Step A\n---\n# New body\n\nDone.\n"
	doc, err := plans.Parse("x", raw)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Body != "# New body\n\nDone." {
		t.Fatalf("body: %q", doc.Body)
	}
}

func TestWriteBodyPreservesFrontmatter(t *testing.T) {
	dir := t.TempDir()
	slug := "keep-meta"
	initial := `---
name: Meta title
overview: Short overview
todos:
  - content: Step A
---
# Old body
`
	if _, err := plans.Write(dir, slug, initial); err != nil {
		t.Fatal(err)
	}
	updated, err := plans.WriteBody(dir, slug, "# New body\n\nDone.")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Body != "# New body\n\nDone." {
		t.Fatalf("body: %q content:\n%s", updated.Body, updated.Content)
	}
	if updated.Name != "Meta title" || updated.Overview != "Short overview" {
		t.Fatalf("meta changed: %+v", updated)
	}
	if len(updated.Todos) != 1 || updated.Todos[0].Content != "Step A" {
		t.Fatalf("todos: %+v", updated.Todos)
	}
	if !strings.Contains(updated.Content, "name: Meta title") {
		t.Fatal("frontmatter missing from stored file")
	}
}

func TestWriteBodyWithFallbackBootstrapsFromContent(t *testing.T) {
	dir := t.TempDir()
	slug := "bootstrap-plan"
	bootstrap := `---
name: Bootstrap title
overview: From transcript
---
# Draft body
`
	updated, err := plans.WriteBodyWithFallback(dir, slug, "# Saved from editor\n", bootstrap)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Bootstrap title" || updated.Overview != "From transcript" {
		t.Fatalf("meta: %+v", updated)
	}
	if updated.Body != "# Saved from editor" {
		t.Fatalf("body: %q", updated.Body)
	}
	if _, err := plans.Read(dir, slug); err != nil {
		t.Fatal(err)
	}
}

func TestWriteRejectsInvalidFrontmatter(t *testing.T) {
	dir := t.TempDir()
	slug := "bad-plan"
	invalid := `---
name: Bad
todos: not-a-list
---
body
`
	_, err := plans.Write(dir, slug, invalid)
	if err == nil {
		t.Fatal("expected error")
	}
	if _, statErr := os.Stat(filepath.Join(dir, plans.DirName, slug+".plan.md")); statErr == nil {
		t.Fatal("invalid plan file should not be written")
	}
}

func TestParseFormatRoundTrip(t *testing.T) {
	raw := `---
name: Auth refactor
overview: JWT migration
todos:
  - content: Add middleware
    status: pending
---
## Steps

1. Do thing
`
	doc, err := plans.Parse("auth-refactor", raw)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Name != "Auth refactor" {
		t.Fatalf("name: %q", doc.Name)
	}
	if doc.Overview != "JWT migration" {
		t.Fatalf("overview: %q", doc.Overview)
	}
	if len(doc.Todos) != 1 || doc.Todos[0].Content != "Add middleware" {
		t.Fatalf("todos: %+v", doc.Todos)
	}
	if !strings.Contains(doc.Body, "## Steps") {
		t.Fatalf("body: %q", doc.Body)
	}
	entries := plans.EntriesFromTodos(doc.Todos)
	if len(entries) != 1 || entries[0].Status != "pending" {
		t.Fatalf("entries: %+v", entries)
	}
}

func TestCRUD(t *testing.T) {
	dir := t.TempDir()
	slug := "my-plan"
	created, err := plans.Create(dir, slug, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(created.Content, "my-plan") && created.Name == "" {
		t.Fatalf("unexpected default: %+v", created)
	}
	_, err = plans.Create(dir, slug, "")
	if err != plans.ErrExists {
		t.Fatalf("create again: %v", err)
	}
	updated, err := plans.Write(dir, slug, plans.DefaultContent(slug, "Renamed"))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Renamed" {
		t.Fatalf("name: %q", updated.Name)
	}
	items, err := plans.List(dir)
	if err != nil || len(items) != 1 || items[0].Slug != slug {
		t.Fatalf("list: %v %v", items, err)
	}
	if err := plans.Delete(dir, slug); err != nil {
		t.Fatal(err)
	}
	_, err = plans.Read(dir, slug)
	if err != plans.ErrNotFound {
		t.Fatalf("read after delete: %v", err)
	}
}

func TestReadByMention(t *testing.T) {
	dir := t.TempDir()
	slug := "foo"
	if _, err := plans.Create(dir, slug, plans.DefaultContent(slug, "Foo")); err != nil {
		t.Fatal(err)
	}
	doc, err := plans.ReadByMention(dir, "plans/foo.plan.md")
	if err != nil || doc.Slug != slug {
		t.Fatalf("mention: %v %+v", err, doc)
	}
}

func TestEnsureDirCreatesPlansPath(t *testing.T) {
	dir := t.TempDir()
	if err := plans.EnsureDir(dir); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(filepath.Join(dir, plans.DirName))
	if err != nil || !st.IsDir() {
		t.Fatalf("stat plans: %v", err)
	}
}

// Qwen-family models routinely emit the frontmatter keys without the --- fences.
// Before normalization that degraded the card to name "Plan" with the raw YAML as body.
func TestParseRepairsFrontmatterWithoutFences(t *testing.T) {
	raw := `name: Improve Qwen family system prompt
overview: Expand internal/prompts/agent.qwen.md with model-family guidance.
todos:
  - content: "Analyze current agent.qwen.md"
    status: completed
  - content: "Draft improved agent.qwen.md"
    status: pending

## Summary

Rewrite the prompt.
`
	doc, err := plans.Parse("improve-qwen-prompt", raw)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Name != "Improve Qwen family system prompt" {
		t.Errorf("name = %q, want the frontmatter name", doc.Name)
	}
	if !strings.HasPrefix(doc.Overview, "Expand internal/prompts/agent.qwen.md") {
		t.Errorf("overview = %q, want the frontmatter overview", doc.Overview)
	}
	if len(doc.Todos) != 2 {
		t.Fatalf("todos = %d, want 2", len(doc.Todos))
	}
	if doc.Todos[0].Status != "completed" {
		t.Errorf("todos[0].status = %q, want completed", doc.Todos[0].Status)
	}
	if strings.Contains(doc.Body, "overview:") {
		t.Errorf("body still holds raw frontmatter: %q", doc.Body)
	}
	if !strings.Contains(doc.Body, "## Summary") {
		t.Errorf("body lost the markdown section: %q", doc.Body)
	}
	if !strings.HasPrefix(strings.TrimSpace(doc.Content), "---") {
		t.Errorf("content must be repaired with fences, got %q", doc.Content)
	}
}

func TestParseLeavesFencedAndPlainMarkdownUntouched(t *testing.T) {
	fenced := "---\nname: Fenced\n---\n# Body\n"
	doc, err := plans.Parse("fenced", fenced)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Content != fenced {
		t.Errorf("fenced content changed: %q", doc.Content)
	}
	if doc.Name != "Fenced" || doc.Body != "# Body" {
		t.Errorf("fenced parse drifted: name=%q body=%q", doc.Name, doc.Body)
	}

	plain := "# Just markdown\n\nNo frontmatter here.\n"
	doc2, err := plans.Parse("plain", plain)
	if err != nil {
		t.Fatal(err)
	}
	if doc2.Content != plain {
		t.Errorf("plain content changed: %q", doc2.Content)
	}
	if doc2.Name != "Plan" {
		t.Errorf("plain name = %q, want the Plan default", doc2.Name)
	}
	if doc2.Body != strings.TrimRight(plain, "\n") {
		t.Errorf("plain body drifted: %q", doc2.Body)
	}
}

func TestWriteRepairsFenceLessFrontmatterOnDisk(t *testing.T) {
	dir := t.TempDir()
	raw := "name: Disk plan\noverview: Written without fences\ntodos:\n  - Step one\n\n## Body\n"
	doc, err := plans.Write(dir, "disk-plan", raw)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Name != "Disk plan" {
		t.Errorf("name = %q", doc.Name)
	}
	b, err := os.ReadFile(filepath.Join(dir, "plans", "disk-plan.plan.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(b), "---\n") {
		t.Errorf("file on disk is not repaired: %q", string(b))
	}
	reread, err := plans.Read(dir, "disk-plan")
	if err != nil {
		t.Fatal(err)
	}
	if reread.Name != "Disk plan" || len(reread.Todos) != 1 {
		t.Errorf("reread drifted: name=%q todos=%d", reread.Name, len(reread.Todos))
	}
}
