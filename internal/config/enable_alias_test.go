package config

import (
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// coddyEnableLine matches a coddy-spelled switch left in a rendered file.
var coddyEnableLine = regexp.MustCompile(`(?m)^\s*enable:`)

// A config written for coddy spells every switch `enable`. Each of them must reach the same
// field the FoxxyCode spelling does, instead of being ignored in favour of the default.
func TestLoadReadsCoddyEnableKeys(t *testing.T) {
	paths := testPathConfig(t, `httpserver:
  enable: false
  login:
    enable: false
gateways:
  telegram:
    enable: true
scheduler:
  enable: true
subagents:
  enable: false
hooks:
  enable: false
tools:
  background:
    enable: false
ui:
  enable: false
compaction:
  enable: false
  result_eviction:
    enable: false
`)
	cfg, err := LoadWithPaths(paths)
	if err != nil {
		t.Fatalf("load a coddy config: %v", err)
	}
	for name, off := range map[string]*bool{
		"httpserver.enable":                 cfg.HTTPServer.Enabled,
		"httpserver.login.enable":           cfg.HTTPServer.Login.Enabled,
		"subagents.enable":                  cfg.Subagents.Enabled,
		"hooks.enable":                      cfg.Hooks.Enabled,
		"tools.background.enable":           cfg.Tools.Background.Enabled,
		"ui.enable":                         cfg.UI.Enabled,
		"compaction.enable":                 cfg.Compaction.Enabled,
		"compaction.result_eviction.enable": cfg.Compaction.ResultEviction.Enabled,
	} {
		if off == nil || *off {
			t.Errorf("%s: false was not read (got %v)", name, off)
		}
	}
	if !cfg.Gateways.Telegram.Enabled {
		t.Error("gateways.telegram.enable: true was not read")
	}
	if !cfg.Scheduler.Enabled {
		t.Error("scheduler.enable: true was not read")
	}
}

// enabledSwitch is one field tagged yaml:"enabled" reachable from Config: the keys that
// lead to its section ("[]" for a list entry) and the field index path.
type enabledSwitch struct {
	keys    []string
	index   [][]int
	pointer bool
}

func enabledSwitches(typ reflect.Type, keys []string, index [][]int, stack map[reflect.Type]bool) []enabledSwitch {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	switch typ.Kind() {
	case reflect.Slice, reflect.Array:
		return enabledSwitches(typ.Elem(), append(append([]string{}, keys...), "[]"), index, stack)
	case reflect.Map:
		return enabledSwitches(typ.Elem(), append(append([]string{}, keys...), "key"), index, stack)
	case reflect.Struct:
	default:
		return nil
	}
	if stack[typ] {
		return nil
	}
	stack[typ] = true
	defer delete(stack, typ)
	var out []enabledSwitch
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		name := strings.Split(f.Tag.Get("yaml"), ",")[0]
		if name == "" || name == "-" || !f.IsExported() {
			continue
		}
		idx := append(append([][]int{}, index...), []int{i})
		if name == enabledKey {
			out = append(out, enabledSwitch{keys: keys, index: idx, pointer: f.Type.Kind() == reflect.Pointer})
			continue
		}
		out = append(out, enabledSwitches(f.Type, append(append([]string{}, keys...), name), idx, stack)...)
	}
	return out
}

// switchDocument builds the smallest document that sets one switch through coddy's key.
func switchDocument(keys []string, value string) *yaml.Node {
	leaf := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: coddyEnableKey},
		{Kind: yaml.ScalarNode, Tag: "!!bool", Value: value},
	}}
	node := leaf
	for i := len(keys) - 1; i >= 0; i-- {
		switch keys[i] {
		case "[]":
			node = &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{node}}
		default:
			node = &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
				{Kind: yaml.ScalarNode, Value: keys[i]}, node,
			}}
		}
	}
	return &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{node}}
}

// Every switch the fork has, today's and any added later, is reached by the alias walk:
// the test enumerates the yaml:"enabled" fields of Config instead of listing them.
func TestCoddyEnableReachesEverySwitch(t *testing.T) {
	switches := enabledSwitches(reflect.TypeOf(Config{}), nil, nil, map[reflect.Type]bool{})
	paths := map[string]bool{}
	for _, sw := range switches {
		path := strings.Join(append(append([]string{}, sw.keys...), enabledKey), ".")
		paths[path] = true
		t.Run(path, func(t *testing.T) {
			// A *bool switch defaults to nil, a bool one to false: set the value that is
			// not the zero one, so reading it proves the alias was applied.
			want := "true"
			if sw.pointer {
				want = "false"
			}
			var cfg Config
			if err := decodeConfigDocument(switchDocument(sw.keys, want), &cfg); err != nil {
				t.Fatalf("decode: %v", err)
			}
			got := fieldAt(reflect.ValueOf(&cfg).Elem(), sw.index)
			if !got.IsValid() {
				t.Fatal("the document did not produce the switch's section")
			}
			if got.Kind() == reflect.Pointer {
				if got.IsNil() {
					t.Fatal("the coddy key did not reach the switch")
				}
				got = got.Elem()
			}
			if strconv.FormatBool(got.Bool()) != want {
				t.Fatalf("switch = %v, want %s", got.Bool(), want)
			}
		})
	}
	// The sections upstream coddy spells with enable must be among them.
	for _, p := range []string{
		"httpserver.enabled", "httpserver.login.enabled", "httpserver.cors.enabled", "memory.enabled",
		"gateways.telegram.enabled", "scheduler.enabled", "swarm.enabled", "subagents.enabled",
		"hooks.enabled", "tools.background.enabled", "ui.enabled", "compaction.enabled",
		"compaction.result_eviction.enabled",
	} {
		if !paths[p] {
			t.Errorf("%s is not a switch of Config any more; found %v", p, paths)
		}
	}
}

// fieldAt follows a field index path through the decoded config, stepping through
// pointers, into the first entry of a list and into the entry switchDocument named "key".
// A section the document did not produce yields the zero Value.
func fieldAt(v reflect.Value, index [][]int) reflect.Value {
	for _, step := range index {
	descend:
		for {
			switch v.Kind() {
			case reflect.Pointer:
				if v.IsNil() {
					return reflect.Value{}
				}
				v = v.Elem()
			case reflect.Slice:
				if v.Len() == 0 {
					return reflect.Value{}
				}
				v = v.Index(0)
			case reflect.Map:
				if v = v.MapIndex(reflect.ValueOf("key")); !v.IsValid() {
					return v
				}
			default:
				break descend
			}
		}
		v = v.FieldByIndex(step)
	}
	return v
}

// Both spellings in one section: the FoxxyCode key is the one that counts, wherever it
// stands, and the file still loads (two `enabled` keys would be a duplicate).
func TestEnabledWinsOverCoddyEnable(t *testing.T) {
	for name, body := range map[string]string{
		"enabled first": "scheduler:\n  enabled: true\n  enable: false\n",
		"enable first":  "scheduler:\n  enable: false\n  enabled: true\n",
	} {
		t.Run(name, func(t *testing.T) {
			cfg, err := parseValidateYAMLBytes(body, Paths{Home: t.TempDir()})
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if !cfg.Scheduler.Enabled {
				t.Error("scheduler.enabled: true lost to the coddy spelling")
			}
		})
	}
}

// Only a section whose struct has the switch is rewritten: a label, a list entry or a key
// the loader does not know keeps the name the operator gave it.
func TestCoddyEnableOutsideASwitchIsLeftAlone(t *testing.T) {
	cfg, err := parseValidateYAMLBytes(`swarm:
  join:
    - url: https://relay.example
      labels:
        enable: "yes"
`, Paths{Home: t.TempDir()})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.Swarm.Join) != 1 || cfg.Swarm.Join[0].Labels["enable"] != "yes" {
		t.Fatalf("a label named enable was renamed: %+v", cfg.Swarm.Join)
	}
}

// Anchors and merge keys carry the switch into another section; the alias follows it there.
func TestCoddyEnableThroughAnchorsAndMergeKeys(t *testing.T) {
	cfg, err := parseValidateYAMLBytes(`subagents: &off
  enable: false
hooks: *off
compaction:
  <<: *off
  keep_recent_turns: 3
`, Paths{Home: t.TempDir()})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	for name, off := range map[string]*bool{
		"subagents (anchor)": cfg.Subagents.Enabled,
		"hooks (alias)":      cfg.Hooks.Enabled,
		"compaction (merge)": cfg.Compaction.Enabled,
	} {
		if off == nil || *off {
			t.Errorf("%s: false was not read (got %v)", name, off)
		}
	}
}

// config_get answers for the FoxxyCode spelling, and config_set rewrites the coddy key in
// place instead of adding a second switch beside it.
func TestConfigPathToolsRewriteCoddyEnable(t *testing.T) {
	paths := testPathConfig(t, "httpserver:\n  enable: true\n  port: 8080\n")
	got, err := ReadConfigPath(paths, "httpserver.enabled")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Exists || got.Value != true {
		t.Fatalf("config_get httpserver.enabled on a coddy file = %+v", got)
	}

	result, err := CommitUCICommands(paths, mustParseUCI(t, "set httpserver.enabled=false"))
	if err != nil {
		t.Fatalf("config_set: %v", err)
	}
	if result.Config.HTTPServer.Enabled == nil || *result.Config.HTTPServer.Enabled {
		t.Fatalf("httpserver.enabled after config_set = %v", result.Config.HTTPServer.Enabled)
	}
	raw, err := os.ReadFile(paths.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	written := string(raw)
	if coddyEnableLine.MatchString(written) {
		t.Fatalf("config_set left the coddy key behind:\n%s", written)
	}
	if n := len(regexp.MustCompile(`(?m)^\s*enabled: false$`).FindAllStringIndex(written, -1)); n != 1 {
		t.Fatalf("want exactly one enabled: false, got %d:\n%s", n, written)
	}
}

// A save from the settings screen renders the FoxxyCode key; the note the operator wrote
// above the coddy one must come along.
func TestSettingsSaveCarriesTheCommentOfACoddyEnable(t *testing.T) {
	existing := "httpserver:\n  # this node only relays a swarm\n  enable: false\n"
	cfg := loadForRewrite(t, existing)
	saved := rewrite(t, cfg, existing)
	if coddyEnableLine.MatchString(saved) {
		t.Errorf("the save kept the coddy key:\n%s", saved)
	}
	if !regexp.MustCompile(`# this node only relays a swarm\n\s*enabled: false`).MatchString(saved) {
		t.Errorf("the save lost the value or the comment of the switch:\n%s", saved)
	}
}

// Dry-run and serve diagnostics locate switches by their FoxxyCode path.
func TestLocatorFindsACoddyEnable(t *testing.T) {
	loc := NewLocator([]byte("httpserver:\n  port: 8080\n  enable: false\n"))
	line, col, ok := loc.Locate("httpserver.enabled")
	if !ok || line != 3 || col != 11 {
		t.Fatalf("httpserver.enabled: got %d:%d ok=%v, want the value at 3:11", line, col, ok)
	}
}
