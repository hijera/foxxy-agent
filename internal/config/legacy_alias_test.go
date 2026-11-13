package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLegacySkillSourcesMoveFromAliasedSkills(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"direct alias", "defaults: &skills\n  auto_discovery: false\n  sources:\n    - example/alias\nskills: *skills\n"},
		{"merged alias", "defaults: &skills\n  auto_discovery: false\n  sources:\n    - example/alias\ncommon: &common\n  skills: *skills\n<<: *common\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			paths := Paths{Home: filepath.Join(dir, "home"), CWD: dir, ConfigPath: filepath.Join(dir, "config.yaml")}
			if err := os.WriteFile(paths.ConfigPath, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			for load := 0; load < 2; load++ {
				cfg, err := LoadWithPaths(paths)
				if err != nil {
					t.Fatal(err)
				}
				if cfg.Skills.AutoDiscoveryEnabled() {
					t.Fatal("migration changed an unrelated setting in the aliased mapping")
				}
				file, err := ReadMarketplacesFile(GlobalMarketplacesPath(paths.Home))
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(file.Sources, []string{"example/alias"}) {
					t.Fatalf("load %d: migrated sources = %v, want [example/alias]", load, file.Sources)
				}
			}
			backups, err := filepath.Glob(paths.ConfigPath + ".bak-*")
			if err != nil || len(backups) > 1 {
				t.Fatalf("migration backups = %v (error %v), want at most one", backups, err)
			}
			for _, path := range backups {
				backup, err := os.ReadFile(path)
				if err != nil || string(backup) != tc.body {
					t.Fatalf("migration backup did not preserve the original YAML (error %v)", err)
				}
			}
		})
	}
}

func TestLegacyDeclarationsIgnoreStringMergeKeys(t *testing.T) {
	for _, key := range []string{"'<<'", `"<<"`, "!!str <<"} {
		for _, nested := range []bool{false, true} {
			body := key + ":\n  mcp_servers:\n    - name: inactive\n      command: never-run\n  skills:\n    sources: [example/inactive]\n"
			if nested {
				body = "skills:\n  " + key + ":\n    sources: [example/inactive]\n"
			}
			name := "root/" + key
			if nested {
				name = "skills/" + key
			}
			t.Run(name, func(t *testing.T) {
				dir := t.TempDir()
				paths := Paths{Home: filepath.Join(dir, "home"), CWD: dir, ConfigPath: filepath.Join(dir, "config.yaml")}
				if err := os.WriteFile(paths.ConfigPath, []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
				if _, err := LoadWithPaths(paths); err != nil {
					t.Fatal(err)
				}
				for _, path := range []string{GlobalMCPJSONPath(paths.Home), GlobalMarketplacesPath(paths.Home)} {
					if _, err := os.Stat(path); !os.IsNotExist(err) {
						t.Fatalf("inactive declaration created %s (stat error %v)", path, err)
					}
				}
				after, err := os.ReadFile(paths.ConfigPath)
				if err != nil {
					t.Fatal(err)
				}
				if string(after) != body {
					t.Fatalf("migration rewrote a literal string key:\n%s", after)
				}
			})
		}
	}
}
