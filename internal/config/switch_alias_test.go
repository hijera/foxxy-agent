package config

import (
	"encoding/json"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// legacySwitchLine matches a switch left in a rendered file under its old name.
var legacySwitchLine = regexp.MustCompile(`(?m)^\s*enabled:`)

// Every config on disk was written when the switch was called `enabled`. Each of those keys
// must still reach the field `enable` reaches, instead of being ignored for the default.
func TestLoadReadsLegacyEnabledKeys(t *testing.T) {
	paths := testPathConfig(t, `httpserver:
  enabled: false
  login:
    enabled: false
gateways:
  telegram:
    enabled: true
scheduler:
  enabled: true
subagents:
  enabled: false
hooks:
  enabled: false
tools:
  background:
    enabled: false
ui:
  enabled: false
compaction:
  enabled: false
  result_eviction:
    enabled: false
`)
	cfg, err := LoadWithPaths(paths)
	if err != nil {
		t.Fatalf("load a config with the old spelling: %v", err)
	}
	for name, off := range map[string]*bool{
		"httpserver.enabled":                 cfg.HTTPServer.Enabled,
		"httpserver.login.enabled":           cfg.HTTPServer.Login.Enabled,
		"subagents.enabled":                  cfg.Subagents.Enabled,
		"hooks.enabled":                      cfg.Hooks.Enabled,
		"tools.background.enabled":           cfg.Tools.Background.Enabled,
		"ui.enabled":                         cfg.UI.Enabled,
		"compaction.enabled":                 cfg.Compaction.Enabled,
		"compaction.result_eviction.enabled": cfg.Compaction.ResultEviction.Enabled,
	} {
		if off == nil || *off {
			t.Errorf("%s: false was not read (got %v)", name, off)
		}
	}
	if !cfg.Gateways.Telegram.Enabled {
		t.Error("gateways.telegram.enabled: true was not read")
	}
	if !cfg.Scheduler.Enabled {
		t.Error("scheduler.enabled: true was not read")
	}
}

// enabledSwitch is one field tagged yaml:"enable" reachable from Config: the keys that
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
		if name == switchKey {
			out = append(out, enabledSwitch{keys: keys, index: idx, pointer: f.Type.Kind() == reflect.Pointer})
			continue
		}
		out = append(out, enabledSwitches(f.Type, append(append([]string{}, keys...), name), idx, stack)...)
	}
	return out
}

// switchDocument builds the smallest document that sets one switch through the old key.
func switchDocument(keys []string, value string) *yaml.Node {
	leaf := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: legacySwitchKey},
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
// the test enumerates the yaml:"enable" fields of Config instead of listing them.
func TestLegacyEnabledReachesEverySwitch(t *testing.T) {
	switches := enabledSwitches(reflect.TypeOf(Config{}), nil, nil, map[reflect.Type]bool{})
	paths := map[string]bool{}
	for _, sw := range switches {
		path := strings.Join(append(append([]string{}, sw.keys...), switchKey), ".")
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
					t.Fatal("the old key did not reach the switch")
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
		"httpserver.enable", "httpserver.login.enable", "httpserver.cors.enable", "memory.enable",
		"gateways.telegram.enable", "scheduler.enable", "swarm.enable", "subagents.enable",
		"hooks.enable", "tools.background.enable", "ui.enable", "compaction.enable",
		"compaction.result_eviction.enable",
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

// Both spellings in one section: the current key is the one that counts, wherever it
// stands, and the file still loads (two `enable` keys would be a duplicate).
func TestSwitchWinsOverLegacyEnabled(t *testing.T) {
	for name, body := range map[string]string{
		"enable first":  "scheduler:\n  enable: true\n  enabled: false\n",
		"enabled first": "scheduler:\n  enabled: false\n  enable: true\n",
	} {
		t.Run(name, func(t *testing.T) {
			cfg, err := parseValidateYAMLBytes(body, Paths{Home: t.TempDir()})
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if !cfg.Scheduler.Enabled {
				t.Error("scheduler.enable: true lost to the old spelling")
			}
		})
	}
}

// Only a section whose struct has the switch is rewritten: a label, a list entry or a key
// the loader does not know keeps the name the operator gave it.
func TestLegacyEnabledOutsideASwitchIsLeftAlone(t *testing.T) {
	cfg, err := parseValidateYAMLBytes(`swarm:
  join:
    - url: https://relay.example
      labels:
        enabled: "yes"
`, Paths{Home: t.TempDir()})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.Swarm.Join) != 1 || cfg.Swarm.Join[0].Labels["enabled"] != "yes" {
		t.Fatalf("a label named enabled was renamed: %+v", cfg.Swarm.Join)
	}
}

// Anchors and merge keys carry the switch into another section; the alias follows it there.
func TestLegacyEnabledThroughAnchorsAndMergeKeys(t *testing.T) {
	cfg, err := parseValidateYAMLBytes(`subagents: &off
  enabled: false
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

// config_get answers for the current spelling, and config_set rewrites the old key in
// place instead of adding a second switch beside it.
func TestConfigPathToolsRewriteLegacyEnabled(t *testing.T) {
	paths := testPathConfig(t, "httpserver:\n  enabled: true\n  port: 8080\n")
	got, err := ReadConfigPath(paths, "httpserver.enable")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Exists || got.Value != true {
		t.Fatalf("config_get httpserver.enable on a file with the old key = %+v", got)
	}

	result, err := CommitUCICommands(paths, mustParseUCI(t, "set httpserver.enable=false"))
	if err != nil {
		t.Fatalf("config_set: %v", err)
	}
	if result.Config.HTTPServer.Enabled == nil || *result.Config.HTTPServer.Enabled {
		t.Fatalf("httpserver.enable after config_set = %v", result.Config.HTTPServer.Enabled)
	}
	raw, err := os.ReadFile(paths.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	written := string(raw)
	if legacySwitchLine.MatchString(written) {
		t.Fatalf("config_set left the old key behind:\n%s", written)
	}
	if n := len(regexp.MustCompile(`(?m)^\s*enable: false$`).FindAllStringIndex(written, -1)); n != 1 {
		t.Fatalf("want exactly one enable: false, got %d:\n%s", n, written)
	}
}

// A save from the settings screen renders the current key; the note the operator wrote
// above the old one must come along.
func TestSettingsSaveCarriesTheCommentOfALegacyEnabled(t *testing.T) {
	existing := "httpserver:\n  # this node only relays a swarm\n  enabled: false\n"
	cfg := loadForRewrite(t, existing)
	saved := rewrite(t, cfg, existing)
	if legacySwitchLine.MatchString(saved) {
		t.Errorf("the save kept the old key:\n%s", saved)
	}
	if !regexp.MustCompile(`# this node only relays a swarm\n\s*enable: false`).MatchString(saved) {
		t.Errorf("the save lost the value or the comment of the switch:\n%s", saved)
	}
}

// Dry-run and serve diagnostics locate switches by their current path.
func TestLocatorFindsALegacyEnabled(t *testing.T) {
	loc := NewLocator([]byte("httpserver:\n  port: 8080\n  enabled: false\n"))
	line, col, ok := loc.Locate("httpserver.enable")
	if !ok || line != 3 || col != 12 {
		t.Fatalf("httpserver.enable: got %d:%d ok=%v, want the value at 3:12", line, col, ok)
	}
}

// The JSON config API takes the same alias: a settings screen or a script written against
// the old spelling keeps working, and what comes back is the current key.
func TestConfigJSONReadsLegacyEnabled(t *testing.T) {
	body := `{"memory":{"enabled":true},"ui":{"enabled":false},
	  "compaction":{"enabled":false,"result_eviction":{"enabled":false}},
	  "gateways":{"telegram":{"enabled":true,"token":"t"}},
	  "httpserver":{"port":8080,"login":{"enabled":true,"user":"operator"}}}`
	cfg, err := ParseAndValidateConfigJSON([]byte(body), Paths{Home: t.TempDir()})
	if err != nil {
		t.Fatalf("parse config JSON: %v", err)
	}
	if !cfg.Memory.Enabled || !cfg.Gateways.Telegram.Enabled {
		t.Errorf("a true switch was lost: memory=%v telegram=%v", cfg.Memory.Enabled, cfg.Gateways.Telegram.Enabled)
	}
	for name, off := range map[string]*bool{
		"ui":                         cfg.UI.Enabled,
		"compaction":                 cfg.Compaction.Enabled,
		"compaction.result_eviction": cfg.Compaction.ResultEviction.Enabled,
	} {
		if off == nil || *off {
			t.Errorf("%s: false was not read (got %v)", name, off)
		}
	}
	if cfg.HTTPServer.Login.Enabled == nil || !*cfg.HTTPServer.Login.Enabled {
		t.Errorf("httpserver.login: true was not read (got %v)", cfg.HTTPServer.Login.Enabled)
	}
	if cfg.HTTPServer.Port != 8080 {
		t.Errorf("port %d: rewriting the body must not change other values", cfg.HTTPServer.Port)
	}

	// The current key wins over the old one, and the response carries only the current key.
	cfg, err = ParseAndValidateConfigJSON([]byte(`{"ui":{"enabled":false,"enable":true}}`), Paths{Home: t.TempDir()})
	if err != nil {
		t.Fatalf("parse config JSON: %v", err)
	}
	if cfg.UI.Enabled == nil || !*cfg.UI.Enabled {
		t.Errorf("ui.enable lost to ui.enabled: %v", cfg.UI.Enabled)
	}
	out, err := json.Marshal(ConfigToJSONDTO(cfg))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), `"enabled"`) {
		t.Errorf("the config API still answers with the old key:\n%s", out)
	}
}

// A key called enabled outside a switch - an MCP environment value, a swarm label - is
// not a switch and must survive the JSON rewrite.
func TestConfigJSONLeavesUnrelatedEnabledAlone(t *testing.T) {
	body := `{"mcp_servers":[{"name":"ctx","command":"npx","env":[{"name":"enabled","value":"yes"}]}]}`
	cfg, err := ParseAndValidateConfigJSON([]byte(body), Paths{Home: t.TempDir()})
	if err != nil {
		t.Fatalf("parse config JSON: %v", err)
	}
	if len(cfg.MCPServers) != 1 || len(cfg.MCPServers[0].Env) != 1 || cfg.MCPServers[0].Env[0].Name != "enabled" {
		t.Fatalf("an environment variable named enabled was rewritten: %+v", cfg.MCPServers)
	}
}
