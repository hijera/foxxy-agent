package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestAgentMaxTurnsYAMLMergePresence(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		want       int
	}{
		{"absent", "model: test", AgentDefaultMaxTurns},
		{"direct zero", "max_turns: 0", 0},
		{"merged zero", "<<: {max_turns: 0}", 0},
		{"merged positive", "<<: {max_turns: 7}", 7},
		{"direct wins", "<<: {max_turns: 0}\nmax_turns: 9", 9},
		{"zero overrides merge", "<<: {max_turns: 7}\nmax_turns: 0", 0},
		{"merge sequence", "<<: [{max_turns: 0}, {max_turns: 7}]", 0},
		{"nested anchor", "base: &base {max_turns: 0}\n<<: *base", 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var a Agent
			if err := yaml.Unmarshal([]byte(tt.body), &a); err != nil {
				t.Fatal(err)
			}
			a.ApplyDefaults()
			if a.MaxTurns != tt.want {
				t.Fatalf("max_turns=%d, want %d", a.MaxTurns, tt.want)
			}
		})
	}
}

func TestLegacySkillSourceExpansion(t *testing.T) {
	t.Setenv("FOXXY_TEST_SKILL_SOURCE", "example/skills")
	t.Setenv("FOXXY_TEST_SKILL_EMPTY", "")
	home := t.TempDir()
	paths := Paths{Home: home, CWD: home, ConfigPath: filepath.Join(home, "config.yaml")}
	raw := []byte("skills:\n  sources:\n    - ${FOXXY_TEST_SKILL_SOURCE}\n    - $FOXXY_TEST_SKILL_SOURCE\n    - ${FOXXY_TEST_SKILL_EMPTY}\n    - 'owner/$$literal'\n    - 'file://${FOXXYCODE_HOME}/skills.git'\n")
	if err := os.WriteFile(paths.ConfigPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteMarketplacesFile(GlobalMarketplacesPath(home), MarketplacesFile{Sources: []string{"example/skills"}}); err != nil {
		t.Fatal(err)
	}
	migrateLegacyKeys(paths, raw)
	file, err := ReadMarketplacesFile(GlobalMarketplacesPath(home))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"example/skills", "owner/$literal", "file://" + yamlSafePath(home) + "/skills.git"}
	if !reflect.DeepEqual(file.Sources, want) {
		t.Fatalf("sources=%q, want %q", file.Sources, want)
	}
}
