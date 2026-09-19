package docsgen

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSlugMatchesGitHubAnchors(t *testing.T) {
	cases := map[string]string{
		"Quick start":                               "quick-start",
		"**`TAGS` vs `go build -tags`**":            "tags-vs-go-build--tags",
		"Dry run: probing what the file points at":  "dry-run-probing-what-the-file-points-at",
		"Paths (`FOXXYCODE_HOME`, `FOXXYCODE_CWD`)": "paths-foxxycode_home-foxxycode_cwd",
		"`session/new`":                             "sessionnew",
		"Настройка":                                 "настройка",
	}
	for in, want := range cases {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSpliceReplacesOnlyTheBlock(t *testing.T) {
	doc := "intro\n<!-- docsgen:nav:start -->\nold\n<!-- docsgen:nav:end -->\noutro\n"
	got, err := Splice(doc, MarkerNav, "new body")
	if err != nil {
		t.Fatal(err)
	}
	want := "intro\n<!-- docsgen:nav:start -->\nnew body\n<!-- docsgen:nav:end -->\noutro\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if _, err := Splice("no markers", MarkerNav, "x"); err == nil {
		t.Fatal("expected an error without markers")
	}
}

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCheckLinksFindsBrokenTargetsAndAnchors(t *testing.T) {
	root := t.TempDir()
	write(t, root, "docs/a.md", "# A\n\n## Some Heading\n\nSee [b](b.md), [anchor](b.md#other-heading), [bad](b.md#missing), [gone](nope.md), [img](assets/x.png), [ext](https://example.com), [self](#some-heading), [self-bad](#nowhere), `[code](inline.md)` and [ref][r].\n\n[r]: b.md\n[dead]: missing.md\n\n```\n[not a link](fenced.md)\n```\n\n~~~\n[not a link](tilde.md)\n~~~\n")
	write(t, root, "docs/b.md", "# B\n\nProse with inline ``` fences does not open a block.\n\n### Other heading\n")
	write(t, root, "docs/assets/x.png", "png")
	problems := CheckLinks(root, []string{"docs/a.md"})
	var msgs []string
	for _, p := range problems {
		msgs = append(msgs, p.Message)
	}
	joined := strings.Join(msgs, "\n")
	for _, want := range []string{"broken link nope.md", "#missing", "#nowhere", "broken link missing.md"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %v", want, problems)
		}
	}
	if len(problems) != 4 {
		t.Fatalf("unexpected problems: %v", problems)
	}
}

func TestCheckNavListsEveryPageOnce(t *testing.T) {
	root := t.TempDir()
	write(t, root, NavFile, "groups:\n  - id: g\n    title: G\n    pages:\n      - path: one.md\n        title: One\n        summary: s\n      - path: missing.md\n        title: Missing\n        summary: s\n")
	write(t, root, "docs/one.md", "# One\n")
	write(t, root, "docs/orphan.md", "# Orphan\n")
	write(t, root, "docs/noheading.md", "text\n")
	nav, err := LoadNav(root)
	if err != nil {
		t.Fatal(err)
	}
	problems := CheckNav(root, nav)
	want := []string{"page missing.md does not exist", "not listed in docs/nav.yaml"}
	for _, w := range want {
		found := false
		for _, p := range problems {
			if strings.Contains(p.Message, w) {
				found = true
			}
		}
		if !found {
			t.Errorf("missing problem %q in %v", w, problems)
		}
	}
}

// initGitRepo makes root a repository, so the walk has someone to ask what is
// ignored.
func initGitRepo(root string) error {
	cmd := exec.Command("git", "init")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git init: %w\n%s", err, out)
	}
	return nil
}

func gitInit(t *testing.T, root string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	if err := initGitRepo(root); err != nil {
		t.Skip(err)
	}
}

func TestDocsMarkdownSkipsWhatGitIgnores(t *testing.T) {
	// Neutralise the developer's own git configuration: the answer must come
	// from the .gitignore written below and from nothing else.
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	root := t.TempDir()
	gitInit(t, root)
	write(t, root, ".gitignore", "docs/superpowers/\ndocs/scratch.md\n")
	write(t, root, NavFile, "groups:\n  - id: g\n    title: G\n    pages:\n      - path: one.md\n        title: One\n        summary: s\n")
	write(t, root, "docs/one.md", "# One\n")
	write(t, root, "docs/orphan.md", "# Orphan\n")
	write(t, root, "docs/plans/design.md", "# Design\n")
	write(t, root, "docs/scratch.md", "# Scratch\n")
	write(t, root, "docs/superpowers/plans/scratch.md", "# Scratch\n\n[gone](nowhere.md)\n")

	files, err := DocsMarkdown(root, true)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"docs/one.md", "docs/orphan.md", "docs/plans/design.md"}
	if strings.Join(files, " ") != strings.Join(want, " ") {
		t.Fatalf("DocsMarkdown = %v, want %v", files, want)
	}

	nav, err := LoadNav(root)
	if err != nil {
		t.Fatal(err)
	}
	// The design records under docs/plans keep leaving the map through their
	// own rule, and only the page that really is missing from it is reported.
	problems := CheckNav(root, nav)
	if len(problems) != 1 || problems[0].File != "docs/orphan.md" {
		t.Fatalf("problems = %v, want only docs/orphan.md", problems)
	}
}

func TestDocsMarkdownWithoutGitListsEverything(t *testing.T) {
	root := t.TempDir()
	// Without a repository around it the walk cannot ask anyone what is
	// ignored, and it must carry on rather than fail.
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(root))
	write(t, root, ".gitignore", "docs/superpowers/\n")
	write(t, root, "docs/one.md", "# One\n")
	write(t, root, "docs/superpowers/plans/scratch.md", "# Scratch\n")

	files, err := DocsMarkdown(root, true)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"docs/one.md", "docs/superpowers/plans/scratch.md"}
	if strings.Join(files, " ") != strings.Join(want, " ") {
		t.Fatalf("DocsMarkdown = %v, want %v", files, want)
	}
}

func TestConfigReferenceRendersTypesDefaultsAndItems(t *testing.T) {
	schema := `{"type":"object","properties":{
	  "logger":{"type":"object","description":"Logging.","properties":{
	    "level":{"type":"string","enum":["debug","info"],"description":"Verbosity."},
	    "outputs":{"type":"array","items":{"type":"string"},"description":"Sinks."}}},
	  "providers":{"type":"array","description":"Backends.","items":{"type":"object","properties":{
	    "name":{"type":"string","description":"Name."},
	    "timeout":{"type":["integer","null"],"default":30,"description":"Seconds."}}}},
	  "agent":{"type":"object","properties":{"max_turns":{"type":"integer","description":"Cap."}}}}}`
	defaults := map[string]string{"logger.level": "info", "agent.max_turns": "35"}
	out, err := ConfigReference([]byte(schema), defaults)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"### `providers`",
		"| `providers` | list of objects |  | Backends. |",
		"| `providers[].timeout` | integer or null | 30 | Seconds. |",
		"| `logger.level` | string, one of `debug`, `info` | info | Verbosity. |",
		"| `logger.outputs` | list of strings |  | Sinks. |",
		"| `agent.max_turns` | integer | 35 | Cap. |",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Index(out, "### `providers`") > strings.Index(out, "### `agent`") {
		t.Errorf("sections out of order:\n%s", out)
	}
}

func TestFlattenDefaultsSkipsEmptyAndObjectLists(t *testing.T) {
	type inner struct {
		Level   string   `yaml:"level"`
		Outputs []string `yaml:"outputs"`
		Count   int      `yaml:"count"`
		On      bool     `yaml:"on"`
	}
	type cfg struct {
		Logger inner               `yaml:"logger"`
		Items  []map[string]string `yaml:"items"`
	}
	got, err := FlattenDefaults(cfg{Logger: inner{Level: "info", Outputs: []string{"stderr", "file"}, On: true}, Items: []map[string]string{{"a": "b"}}})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"logger.level": "info", "logger.outputs": "[stderr, file]", "logger.on": "true"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestPageAddressesPointAtTheForkOnGitHub(t *testing.T) {
	if got := PageURL("features/hooks.md"); got != "https://github.com/hijera/foxxy-agent/blob/main/docs/features/hooks.md" {
		t.Errorf("page url = %q", got)
	}
	if got := RawPageURL("../CONTRIBUTING.md"); got != GitHubRaw+"CONTRIBUTING.md" {
		t.Errorf("raw url = %q", got)
	}
}

// TestChecksReadACRLFCheckout pins the Windows case: a working tree under
// core.autocrlf holds CRLF, and neither the link check nor the staleness
// comparison may treat that as a difference from the LF the repository stores.
func TestChecksReadACRLFCheckout(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(strings.ReplaceAll(content, "\n", "\r\n")), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("docs/a.md", "# A\n\n## Section\n\n```text\n[not a link](missing.md)\n```\n\n[B](b.md#title)\n")
	write("docs/b.md", "# Title\n")
	if problems := CheckLinks(root, []string{"docs/a.md", "docs/b.md"}); len(problems) != 0 {
		t.Fatalf("CRLF checkout reported %v", problems)
	}
	res := &Result{Files: map[string]string{"docs/b.md": "# Title\n"}}
	if stale := res.Stale(root); len(stale) != 0 {
		t.Fatalf("CRLF copy of the generated file read as stale: %v", stale)
	}
}

// A committed symlink under docs/assets is a real link on Linux and, with
// core.symlinks=false, a small text file holding the target path on Windows.
// The inventory has to size both the same, by the link itself: following it on
// Linux listed the target's bytes, and CI read the Windows-made index as stale.
// foxxycode-favicon.svg used to be such a link before it became a file of its
// own; nothing under docs/assets is one today, and this keeps the behaviour.
func TestAssetInventorySizesASymlinkByTheLinkItself(t *testing.T) {
	root := t.TempDir()
	assets := filepath.Join(root, "docs", "assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	target := strings.Repeat("<svg/>", 100)
	if err := os.WriteFile(filepath.Join(assets, "mark.svg"), []byte(target), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assets, "INDEX.md"), []byte("# Assets index\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("mark.svg", filepath.Join(assets, "alias.svg")); err != nil {
		t.Skipf("symlink: %v", err)
	}
	inventory, err := AssetInventory(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range inventory {
		if a.Path == "alias.svg" {
			if a.Size != int64(len("mark.svg")) {
				t.Fatalf("alias.svg sized %d, want the %d bytes of the link itself", a.Size, len("mark.svg"))
			}
			return
		}
	}
	t.Fatalf("alias.svg missing from the inventory: %+v", inventory)
}

func TestCheckLinksAcceptsExplicitHTMLAnchors(t *testing.T) {
	root := t.TempDir()
	page := filepath.Join(root, "README.md")
	body := "# Readme\n\n- [Other ways](#другие-способы)\n\n<a id=\"другие-способы\"></a>\n### Установка\n"
	if err := os.WriteFile(page, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if problems := CheckLinks(root, []string{"README.md"}); len(problems) != 0 {
		t.Fatalf("an <a id> target was reported: %v", problems)
	}
}

func TestHelpScreensDoNotCarryTheMachine(t *testing.T) {
	out := "Usage of C:\\tmp\\docsgen-1\\foxxycode.exe:\r\n  -home string\r\n    \tstate directory (default \"C:\\Temp\")\r\n"
	got := normalizeHelp(out, "C:\\tmp\\docsgen-1\\foxxycode.exe", "C:\\Temp")
	want := "Usage of foxxycode:\n  -home string\n    \tstate directory (default \"~/.foxxycode\")"
	if got != want {
		t.Fatalf("normalizeHelp =\n%q\nwant\n%q", got, want)
	}
}

func TestRenderHubAndLLMSIndex(t *testing.T) {
	nav := &Nav{Groups: []Group{{ID: "g", Title: "Getting started", Summary: "Install.", Pages: []Page{{Path: "getting-started/install.md", Title: "Install", Summary: "How."}, {Path: "../CONTRIBUTING.md", Title: "Contributing", Summary: "Why."}}}}}
	hub := RenderHub(nav)
	if !strings.Contains(hub, "## Getting started\n\nInstall.\n\n- [Install](getting-started/install.md) - How.\n- [Contributing](../CONTRIBUTING.md) - Why.") {
		t.Fatalf("hub:\n%s", hub)
	}
	idx := RenderLLMSIndex(nav, "# FoxxyCode documentation\n\nOne binary.\n\n<!-- docsgen:nav:start -->\n", "https://raw.example/main/")
	if !strings.Contains(idx, "> One binary.") || !strings.Contains(idx, "(https://raw.example/main/docs/getting-started/install.md): How.") || !strings.Contains(idx, "(https://raw.example/main/CONTRIBUTING.md): Why.") {
		t.Fatalf("llms index:\n%s", idx)
	}
}
