package skills

// Edge cases of added marketplaces. The happy path is
// features/plugin_marketplace_commands.feature; what is here is what the
// commands must refuse or keep apart: a marketplace without a usable name or
// without a marketplace.json, two marketplaces under one name, a plugin or a
// marketplace that is not there, a plugin a marketplace dropped, the updates
// that must not install what was never installed, and a registry file that
// does not parse.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/gitws"
)

// testCatalogue serves a marketplace over https: /marketplace.json names its
// plugins, each an archive at /p/<plugin>.zip.
type testCatalogue struct {
	srv *httptest.Server

	mu       sync.Mutex
	name     string
	archives map[string][]byte
}

func newCatalogue(t *testing.T, name string, plugins ...string) *testCatalogue {
	t.Helper()
	c := &testCatalogue{name: name, archives: map[string][]byte{}}
	for _, p := range plugins {
		c.set(p, pluginArchive(t, p, 1))
	}
	c.srv = httptest.NewTLSServer(http.HandlerFunc(c.serve))
	t.Cleanup(c.srv.Close)
	return c
}

func (c *testCatalogue) serve(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if r.URL.Path == "/marketplace.json" {
		names := make([]string, 0, len(c.archives))
		for n := range c.archives {
			names = append(names, n)
		}
		sort.Strings(names)
		plugins := make([]any, 0, len(names))
		for _, n := range names {
			plugins = append(plugins, map[string]any{
				"name":   n,
				"source": map[string]string{"source": "archive", "url": "https://" + r.Host + "/p/" + n + ".zip"},
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"name": c.name, "plugins": plugins})
		return
	}
	if data, ok := c.archives[strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/p/"), ".zip")]; ok {
		_, _ = w.Write(data)
		return
	}
	http.NotFound(w, r)
}

func (c *testCatalogue) url() string { return c.srv.URL + "/marketplace.json" }

func (c *testCatalogue) set(plugin string, archive []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.archives[plugin] = archive
}

func (c *testCatalogue) drop(plugin string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.archives, plugin)
}

// pluginArchive is a plugin archive holding the one skill named after it.
func pluginArchive(t *testing.T, name string, revision int) []byte {
	return mustZip(t,
		zipEntry{name: ".claude-plugin/plugin.json", body: `{"name":"` + name + `"}`},
		zipEntry{name: "skills/" + name + "/SKILL.md", body: fmt.Sprintf("---\nname: %s\ndescription: d\n---\n\nrevision %d\n", name, revision)},
	)
}

// marketHome is a home with a config file and no sources, off the network.
func marketHome(t *testing.T, sources ...string) *config.Config {
	t.Helper()
	offlineSystemSources(t)
	home := t.TempDir()
	cfgPath := filepath.Join(home, "config.yaml")
	body := "skills:\n  sources: []\n"
	if len(sources) > 0 {
		body = "skills:\n  sources:\n"
		for _, s := range sources {
			body += fmt.Sprintf("    - %q\n", s)
		}
	}
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return &config.Config{
		Paths:  config.Paths{Home: home, ConfigPath: cfgPath},
		Skills: config.Skills{Dirs: []string{filepath.Join(home, "skills")}, Sources: sources},
	}
}

func run(t *testing.T, cfg *config.Config, words ...string) (string, error) {
	t.Helper()
	return RunPluginCommand(context.Background(), cfg, ".", words)
}

func mustRun(t *testing.T, cfg *config.Config, words ...string) string {
	t.Helper()
	out, err := run(t, cfg, words...)
	if err != nil {
		t.Fatalf("plugin %s: %v", strings.Join(words, " "), err)
	}
	return out
}

func installedSkills(t *testing.T, cfg *config.Config) []string {
	t.Helper()
	var out []string
	for name := range readRemoteLock(cfg.Skills.ManagedDir(cfg.Paths.Home)) {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func TestParsePluginRef(t *testing.T) {
	ok := map[string][2]string{
		"yandex-wordstat@neuraldeep": {"yandex-wordstat", "neuraldeep"},
		"rpa-feat@rpa-skills":        {"rpa-feat", "rpa-skills"},
		" a.b_c@M1 ":                 {"a.b_c", "M1"},
	}
	for in, want := range ok {
		p, m, got := ParsePluginRef(in)
		if !got || p != want[0] || m != want[1] {
			t.Errorf("ParsePluginRef(%q) = %q, %q, %v", in, p, m, got)
		}
	}
	for _, in := range []string{
		"owner/repo",
		"owner/repo@v1.2",
		"git@github.com:owner/repo.git",
		"https://neuraldeep.ru/skapi/marketplace.json",
		"plugin@",
		"@market",
		"a@b@c",
		"-flag@market",
		"plugin@-market",
		"",
	} {
		if _, _, got := ParsePluginRef(in); got {
			t.Errorf("ParsePluginRef(%q) took it for <plugin>@<marketplace>", in)
		}
	}
}

func TestAddMarketplaceRefusesWhatItCannotName(t *testing.T) {
	unnamed := newCatalogue(t, "", "a")
	spaced := newCatalogue(t, "two words", "a")
	reachRemote(t, unnamed.srv, spaced.srv)
	cfg := marketHome(t)
	for _, c := range []*testCatalogue{unnamed, spaced} {
		_, err := run(t, cfg, "marketplace", "add", c.url())
		if err == nil || !strings.Contains(err.Error(), "`plugin install "+c.url()+"`") {
			t.Errorf("add %q-named marketplace: %v, want a refusal naming the whole install", c.name, err)
		}
	}
	if ms, _ := AddedMarketplaces(cfg); len(ms) != 0 {
		t.Errorf("added %+v", ms)
	}
}

func TestAddMarketplaceWithoutAManifestPointsAtInstall(t *testing.T) {
	if !gitws.GitAvailable() {
		t.Skip("git binary not available")
	}
	cfg := marketHome(t)
	repo := t.TempDir()
	writeSkill(t, filepath.Join(repo, "skills", "loose"), "loose")
	gitCommitAllRepo(t, repo, true, "loose skill")
	src := "file://" + filepath.ToSlash(repo)
	_, err := run(t, cfg, "marketplace", "add", src)
	if err == nil || !strings.Contains(err.Error(), "publishes no marketplace.json") || !strings.Contains(err.Error(), "`plugin install "+src+"`") {
		t.Fatalf("add a repo without marketplace.json: %v", err)
	}
}

func TestAddMarketplaceKeepsOneMarketplacePerName(t *testing.T) {
	first := newCatalogue(t, "shop", "a")
	second := newCatalogue(t, "shop", "b")
	reachRemote(t, first.srv, second.srv)
	cfg := marketHome(t)
	mustRun(t, cfg, "marketplace", "add", first.url())
	_, err := run(t, cfg, "marketplace", "add", second.url())
	if err == nil || !strings.Contains(err.Error(), "`plugin marketplace remove shop`") {
		t.Fatalf("a second marketplace named shop: %v", err)
	}
	ms, _ := AddedMarketplaces(cfg)
	if len(ms) != 1 || ms[0].Source != first.url() {
		t.Fatalf("added = %+v, want the first one kept", ms)
	}
	// Once the first is removed, the second takes the name.
	if out := mustRun(t, cfg, "marketplace", "remove", "shop"); !strings.Contains(out, `Removed marketplace "shop"`) {
		t.Fatalf("remove: %q", out)
	}
	mustRun(t, cfg, "marketplace", "add", second.url())
	if ms, _ := AddedMarketplaces(cfg); len(ms) != 1 || ms[0].Source != second.url() {
		t.Fatalf("added = %+v, want the second one", ms)
	}
}

func TestInstallFromMarketplaceSaysWhatToDo(t *testing.T) {
	c := newCatalogue(t, "neuraldeep", "a", "b")
	reachRemote(t, c.srv)
	cfg := marketHome(t)

	_, err := run(t, cfg, "install", "a@neuraldeep")
	if err == nil || !strings.Contains(err.Error(), `marketplace "neuraldeep" is not added`) || !strings.Contains(err.Error(), "`plugin marketplace add") {
		t.Fatalf("install from a marketplace not added: %v", err)
	}

	mustRun(t, cfg, "marketplace", "add", c.url())
	_, err = run(t, cfg, "install", "a@other")
	if err == nil || !strings.Contains(err.Error(), "(added: neuraldeep)") {
		t.Fatalf("install from another marketplace: %v, want the added ones named", err)
	}
	_, err = run(t, cfg, "install", "missing@neuraldeep")
	if err == nil || !strings.Contains(err.Error(), `plugin "missing" is not in marketplace "neuraldeep"`) ||
		!strings.Contains(err.Error(), "2 plugin(s)") || !strings.Contains(err.Error(), "`plugin marketplace list neuraldeep`") {
		t.Fatalf("install a plugin the marketplace does not list: %v", err)
	}
	if got := installedSkills(t, cfg); len(got) != 0 {
		t.Fatalf("installed %v", got)
	}

	c.srv.Close()
	_, err = run(t, cfg, "install", "a@neuraldeep")
	if err == nil || !strings.Contains(err.Error(), `read marketplace "neuraldeep"`) {
		t.Fatalf("install while the marketplace is down: %v", err)
	}
}

func TestInstallFromMarketplaceFailingIsAnError(t *testing.T) {
	c := newCatalogue(t, "neuraldeep", "a")
	c.set("a", []byte("not a zip"))
	reachRemote(t, c.srv)
	cfg := marketHome(t)
	mustRun(t, cfg, "marketplace", "add", c.url())
	if _, err := run(t, cfg, "install", "a@neuraldeep"); err == nil || !strings.Contains(err.Error(), "install a@neuraldeep") {
		t.Fatalf("an archive that does not unpack: %v, want an error", err)
	}
}

func TestUpdatesTouchOnlyWhatWasInstalled(t *testing.T) {
	c := newCatalogue(t, "neuraldeep", "a", "b")
	reachRemote(t, c.srv)

	t.Run("sync of everything", func(t *testing.T) {
		cfg := marketHome(t)
		mustRun(t, cfg, "marketplace", "add", c.url())
		mustRun(t, cfg, "install", "a@neuraldeep")
		c.set("a", pluginArchive(t, "a", 2))
		out := mustRun(t, cfg, "marketplace", "sync")
		if !strings.Contains(out, "0 added, 1 updated, 0 failed.") {
			t.Fatalf("sync answered %q", out)
		}
		if got := installedSkills(t, cfg); strings.Join(got, ",") != "a" {
			t.Fatalf("installed %v, want only a", got)
		}
	})
	t.Run("the update button of one skill", func(t *testing.T) {
		cfg := marketHome(t)
		mustRun(t, cfg, "marketplace", "add", c.url())
		mustRun(t, cfg, "install", "a@neuraldeep")
		res, err := UpdateSkill(context.Background(), cfg, "a")
		if err != nil || len(res.Failed) != 0 || strings.Join(res.Updated, ",") != "a" {
			t.Fatalf("UpdateSkill = %+v, %v", res, err)
		}
		if got := installedSkills(t, cfg); strings.Join(got, ",") != "a" {
			t.Fatalf("installed %v, want only a", got)
		}
	})
	t.Run("a plugin the marketplace dropped", func(t *testing.T) {
		cfg := marketHome(t)
		mustRun(t, cfg, "marketplace", "add", c.url())
		mustRun(t, cfg, "install", "b@neuraldeep")
		c.drop("b")
		t.Cleanup(func() { c.set("b", pluginArchive(t, "b", 1)) })
		out := mustRun(t, cfg, "marketplace", "update", "neuraldeep")
		if !strings.Contains(out, "1 failed") || !strings.Contains(out, `plugin "b" is no longer in the marketplace`) {
			t.Fatalf("update answered %q", out)
		}
		if got := installedSkills(t, cfg); strings.Join(got, ",") != "b" {
			t.Fatalf("installed %v, want b kept", got)
		}
	})
	t.Run("an address nobody added", func(t *testing.T) {
		cfg := marketHome(t)
		if _, err := run(t, cfg, "marketplace", "update", "nothing-here"); err == nil || !strings.Contains(err.Error(), `marketplace "nothing-here" is not added`) {
			t.Fatalf("update of an unknown name: %v", err)
		}
	})
}

func TestAWholeSourceSyncsWholeByItsMarketplaceName(t *testing.T) {
	c := newCatalogue(t, "neuraldeep", "a", "b")
	reachRemote(t, c.srv)
	cfg := marketHome(t, c.url())
	mustRun(t, cfg, "marketplace", "add", c.url())
	out := mustRun(t, cfg, "marketplace", "update", "neuraldeep")
	if !strings.Contains(out, "2 added") {
		t.Fatalf("update by name of a marketplace that is a source too answered %q, want every plugin installed", out)
	}
	if got := installedSkills(t, cfg); strings.Join(got, ",") != "a,b" {
		t.Fatalf("installed %v", got)
	}
}

func TestASourceInConfigStaysWhole(t *testing.T) {
	c := newCatalogue(t, "neuraldeep", "a", "b")
	reachRemote(t, c.srv)
	cfg := marketHome(t, c.url())
	out := mustRun(t, cfg, "marketplace", "add", c.url())
	if !strings.Contains(out, "every plugin it lists is installed") {
		t.Fatalf("adding a configured source answered %q", out)
	}
	out = mustRun(t, cfg, "marketplace", "update", c.url())
	if !strings.Contains(out, "2 added") {
		t.Fatalf("updating a configured source answered %q, want every plugin installed", out)
	}
	if got := installedSkills(t, cfg); strings.Join(got, ",") != "a,b" {
		t.Fatalf("installed %v", got)
	}
}

func TestMarketplaceListAndShow(t *testing.T) {
	c := newCatalogue(t, "neuraldeep", "a", "b")
	reachRemote(t, c.srv)
	cfg := marketHome(t)
	if out := mustRun(t, cfg, "marketplace", "list"); !strings.Contains(out, "No marketplaces configured") {
		t.Fatalf("empty list: %q", out)
	}
	mustRun(t, cfg, "marketplace", "add", c.url())
	mustRun(t, cfg, "install", "b@neuraldeep")
	out := mustRun(t, cfg, "marketplace", "list")
	if !strings.Contains(out, "1 added marketplace(s)") || !strings.Contains(out, "- neuraldeep  [valid marketplace — neuraldeep, 2 plugin(s); "+c.url()+"]") {
		t.Fatalf("list: %q", out)
	}
	out = mustRun(t, cfg, "marketplace", "list", "neuraldeep")
	if !strings.Contains(out, "  - a\n") || !strings.Contains(out, "  - b  [installed]") || !strings.Contains(out, "`plugin install <plugin>@neuraldeep`") {
		t.Fatalf("show: %q", out)
	}
	if _, err := run(t, cfg, "marketplace", "list", "other"); err == nil {
		t.Fatal("showing a marketplace nobody added succeeded")
	}
	// Removing by address works as by name, and leaves installed skills be.
	if out := mustRun(t, cfg, "marketplace", "remove", c.url()); !strings.Contains(out, `Removed marketplace "neuraldeep"`) {
		t.Fatalf("remove by address: %q", out)
	}
	if got := installedSkills(t, cfg); strings.Join(got, ",") != "b" {
		t.Fatalf("installed %v after remove, want b kept", got)
	}
	if out := mustRun(t, cfg, "marketplace", "remove", "neuraldeep"); !strings.Contains(out, "was not configured") {
		t.Fatalf("second remove: %q", out)
	}
}

func TestAvailablePluginsListsAddedMarketplaces(t *testing.T) {
	c := newCatalogue(t, "neuraldeep", "a", "b")
	reachRemote(t, c.srv)
	cfg := marketHome(t)
	mustRun(t, cfg, "marketplace", "add", c.url())
	mustRun(t, cfg, "install", "a@neuraldeep")
	avail, err := AvailablePlugins(context.Background(), cfg, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, p := range avail {
		if p.Source != c.url() {
			t.Errorf("plugin %q from %q, want the added marketplace", p.Name, p.Source)
		}
		got[p.Name] = p.Installed
	}
	if len(got) != 2 || !got["a"] || got["b"] {
		t.Fatalf("available = %+v, want a installed and b not", avail)
	}
}

func TestACorruptMarketplaceListIsNotOverwritten(t *testing.T) {
	c := newCatalogue(t, "neuraldeep", "a")
	reachRemote(t, c.srv)
	cfg := marketHome(t)
	managed := cfg.Skills.ManagedDir(cfg.Paths.Home)
	if err := os.MkdirAll(managed, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(managed, marketplacesFile)
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, cfg, "marketplace", "add", c.url()); err == nil {
		t.Fatal("add over a corrupt list succeeded")
	}
	if data, _ := os.ReadFile(path); string(data) != "{not json" {
		t.Fatalf("the corrupt list was overwritten: %q", data)
	}
	res, err := Sync(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Failed) != 1 || res.Failed[0].Source != marketplacesFile {
		t.Fatalf("sync = %+v, want the list reported", res)
	}
}

func TestSameSource(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"owner/repo", "https://github.com/owner/repo", true},
		{"owner/repo", "https://github.com/owner/repo.git", true},
		{"https://example.com/marketplace.json", "https://example.com/marketplace.json/", true},
		{"HTTPS://Example.com/marketplace.json", "https://example.com/marketplace.json", true},
		{"owner/repo@v1", "owner/repo", false},
		{"https://example.com/a.json", "https://example.com/b.json", false},
		{"neuraldeep", "https://neuraldeep.ru/skapi/marketplace.json", false},
	}
	for _, tc := range cases {
		if got := sameSource(tc.a, tc.b); got != tc.want {
			t.Errorf("sameSource(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestAnAddedMarketplaceAnswersToAnySpellingOfItsSource(t *testing.T) {
	c := newCatalogue(t, "neuraldeep", "a", "b")
	reachRemote(t, c.srv)
	cfg := marketHome(t)
	mustRun(t, cfg, "marketplace", "add", c.url())
	mustRun(t, cfg, "install", "a@neuraldeep")
	out := mustRun(t, cfg, "marketplace", "update", c.url()+"/")
	if !strings.Contains(out, `Updated marketplace "neuraldeep"`) || !strings.Contains(out, "0 added, 1 updated") {
		t.Fatalf("update by another spelling answered %q, want the added marketplace updated", out)
	}
	if got := installedSkills(t, cfg); strings.Join(got, ",") != "a" {
		t.Fatalf("installed %v, want only a", got)
	}
}

func TestRemovingAMarketplaceByNameTakesItsSourceOut(t *testing.T) {
	c := newCatalogue(t, "neuraldeep", "a")
	reachRemote(t, c.srv)
	cfg := marketHome(t, c.url())
	mustRun(t, cfg, "marketplace", "add", c.url())
	out := mustRun(t, cfg, "marketplace", "remove", "neuraldeep")
	if !strings.Contains(out, "and its source") {
		t.Fatalf("remove answered %q", out)
	}
	if got := ListSources(cfg); len(got) != 0 {
		t.Fatalf("sources = %v, want the marketplace's source gone too", got)
	}
}
