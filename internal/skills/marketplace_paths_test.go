package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/gitws"
)

func scopedMarketplace(t *testing.T) *config.Config {
	t.Helper()
	if !gitws.GitAvailable() {
		t.Skip("git binary not available")
	}
	offlineSystemSources(t)
	repo := t.TempDir()
	writeSkill(t, filepath.Join(repo, "skills", "one"), "one")
	writeSkill(t, filepath.Join(repo, "skills", "two"), "two")
	manifestDir := filepath.Join(repo, ".claude-plugin")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"name":"scoped","plugins":[{"name":"one","source":"./","strict":false,"skills":["./skills/one"]},{"name":"two","source":"./","strict":false,"skills":["./skills/two"]}]}`
	if err := os.WriteFile(filepath.Join(manifestDir, "marketplace.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommitAllRepo(t, repo, true, "catalogue")
	cfg := marketHome(t)
	mustRun(t, cfg, "marketplace", "add", "file://"+filepath.ToSlash(repo))
	return cfg
}

func TestMarketplaceEntrySkillPaths(t *testing.T) {
	cfg := scopedMarketplace(t)
	mustRun(t, cfg, "install", "one@scoped")
	if got := installedSkills(t, cfg); !reflect.DeepEqual(got, []string{"one"}) {
		t.Fatalf("selected one plugin, installed %v", got)
	}
	mustRun(t, cfg, "disable", "one")
	mustRun(t, cfg, "marketplace", "update", "scoped")
	if got := installedSkills(t, cfg); !reflect.DeepEqual(got, []string{"one"}) {
		t.Fatalf("update installed other plugins: %v", got)
	}
	if !IsDisabled(ReadDisabled(cfg.Skills.ManagedDir(cfg.Paths.Home)), "one") {
		t.Fatal("update enabled a disabled skill")
	}
}

func TestMarketplaceSkillPathsFeature(t *testing.T) {
	var cfg *config.Config
	suite := godog.TestSuite{Name: "marketplace-skill-paths", Options: &godog.Options{
		Format: "progress", Paths: []string{"../../features/plugin_marketplace_skill_paths.feature"}, TestingT: t,
	}, ScenarioInitializer: func(sc *godog.ScenarioContext) {
		sc.Step(`^an added marketplace whose plugins share a source root and declare separate skills$`, func() {
			cfg = scopedMarketplace(t)
		})
		sc.Step(`^I run the scoped plugin command "([^"]*)"$`, func(command string) error {
			_, err := RunPluginCommand(context.Background(), cfg, ".", strings.Fields(command))
			return err
		})
		sc.Step(`^only the selected skill "([^"]*)" is installed$`, func(name string) error {
			if got := installedSkills(t, cfg); !reflect.DeepEqual(got, []string{name}) {
				return fmt.Errorf("installed %v, want only %s", got, name)
			}
			return nil
		})
	}}
	if suite.Run() != 0 {
		t.Fatal("marketplace skill paths failed")
	}
}

func TestMarketplaceSkillPathsRejectInvalidSelections(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, filepath.Join(root, "skills", "one"), "one")
	for _, raw := range []string{`[]`, `false`, `{}`, `["../outside"]`, `["/absolute"]`, `["C:\\outside"]`, `["missing"]`} {
		t.Run(raw, func(t *testing.T) {
			managed := t.TempDir()
			p := MarketplacePlugin{Name: "selected", Skills: json.RawMessage(raw)}
			lock := map[string]RemoteEntry{}
			res := &SyncResult{}
			if err := installPluginDir(root, p, RemoteEntry{}, managed, lock, res); err == nil {
				t.Fatal("invalid selection fell back to installing the whole source")
			}
			assertNothingInstalled(t, managed, lock, res)
		})
	}
}

func TestMarketplaceSkillPathsRejectEscapingSymlink(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	writeSkill(t, outside, "outside")
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	p := MarketplacePlugin{Name: "selected", Skills: json.RawMessage(`["link"]`)}
	managed := t.TempDir()
	lock, res := map[string]RemoteEntry{}, &SyncResult{}
	if err := installPluginDir(root, p, RemoteEntry{}, managed, lock, res); err == nil {
		t.Fatal("installed a skill outside the source root")
	}
	assertNothingInstalled(t, managed, lock, res)
}

func TestArchiveMarketplaceEntrySkillPaths(t *testing.T) {
	archive := mustZip(t,
		zipEntry{name: "skills/one/SKILL.md", body: "---\nname: one\ndescription: one\n---\n"},
		zipEntry{name: "skills/two/SKILL.md", body: "---\nname: two\ndescription: two\n---\n"},
	)
	srv := httptest.NewTLSServer(serveBytes(archive, nil))
	t.Cleanup(srv.Close)
	reachRemote(t, srv)
	p := archivePlugin(srv.URL+"/one.zip", "")
	p.Skills = json.RawMessage(`"./one"`)
	// The archive's single wrapper (skills/) becomes the plugin root.
	managed, lock, _, err := installArchive(t, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(lock) != 1 || lock["one"].Plugin != p.Name {
		t.Fatalf("installed %v, want only one", lock)
	}
	if _, err := os.Stat(filepath.Join(managed, "one", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}
