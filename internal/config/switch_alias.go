package config

// Every on/off switch in config.yaml is spelled `enable`, the way upstream coddy
// spells it, so a configuration travels between the two agents unchanged. FoxxyCode
// spelled it `enabled` until 0.3.x, and the files on disk still do, so that key is
// read as an alias wherever config.yaml is read as a document: the loader, the -t
// check, config_get / config_set, the comments a settings save carries over, and
// the locator diagnostics use. The same alias is accepted in the JSON config API
// (jsondto.go); the canonical spelling is what every write renders.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// switchKey is how a switch is spelled now, in YAML and in JSON.
	switchKey = "enable"
	// legacySwitchKey is the spelling FoxxyCode used before 0.3.x, still read.
	legacySwitchKey = "enabled"
)

// switchAlias is one legacy `enabled` key found in a config document.
type switchAlias struct {
	// Path is the dotted path of the key as the file spells it ("httpserver.login.enabled").
	Path string
	// Line and Column locate the key in the file.
	Line, Column int
	// Dropped is set when the same section also sets `enable`: that key wins and
	// the alias was removed from the document.
	Dropped bool
}

// normalizeSwitchAliases renames `enabled` to `enable` in every mapping whose
// config struct has an `enable` field, and removes it where the mapping already
// has `enable`. The walk follows the Config type, so a key that only happens to
// be called enabled - a label, or a key of a section without a switch - keeps its
// name. n is a document node or its root mapping.
func normalizeSwitchAliases(n *yaml.Node) []switchAlias {
	w := switchAliasWalker{seen: map[*yaml.Node]bool{}}
	w.walk("", n, reflect.TypeOf(Config{}))
	return w.found
}

type switchAliasWalker struct {
	// seen holds the mappings already walked: an anchor is shared by every alias
	// and merge key that names it.
	seen  map[*yaml.Node]bool
	found []switchAlias
}

func (w *switchAliasWalker) walk(path string, n *yaml.Node, typ reflect.Type) {
	if n == nil {
		return
	}
	switch n.Kind {
	case yaml.DocumentNode:
		for _, c := range n.Content {
			w.walk(path, c, typ)
		}
		return
	case yaml.AliasNode:
		w.walk(path, n.Alias, typ)
		return
	}
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	switch typ.Kind() {
	case reflect.Struct:
		if n.Kind == yaml.MappingNode && !w.seen[n] {
			w.seen[n] = true
			w.mapping(path, n, typ)
		}
	case reflect.Slice, reflect.Array:
		if n.Kind == yaml.SequenceNode {
			for i, item := range n.Content {
				w.walk(fmt.Sprintf("%s[%d]", path, i), item, typ.Elem())
			}
		}
	case reflect.Map:
		if n.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(n.Content); i += 2 {
				w.walk(join(path, n.Content[i].Value), n.Content[i+1], typ.Elem())
			}
		}
	}
}

// mapping renames the alias in one section and walks its known keys.
func (w *switchAliasWalker) mapping(path string, m *yaml.Node, typ reflect.Type) {
	aliased := structHasSwitch(typ)
	switchSet := false
	if aliased {
		_, _, switchSet = mappingValue(m, switchKey)
	}
	kept := m.Content[:0]
	for i := 0; i+1 < len(m.Content); i += 2 {
		k, v := m.Content[i], m.Content[i+1]
		if k.Kind != yaml.ScalarNode {
			kept = append(kept, k, v)
			continue
		}
		if k.ShortTag() == "!!merge" {
			// The merged mappings are this section's keys too.
			if v.Kind == yaml.SequenceNode {
				for _, item := range v.Content {
					w.walk(path, item, typ)
				}
			} else {
				w.walk(path, v, typ)
			}
			kept = append(kept, k, v)
			continue
		}
		name := k.Value
		if aliased && name == legacySwitchKey {
			alias := switchAlias{Path: join(path, legacySwitchKey), Line: k.Line, Column: k.Column, Dropped: switchSet}
			w.found = append(w.found, alias)
			if alias.Dropped {
				continue
			}
			k.Value = switchKey
			name = switchKey
		}
		kept = append(kept, k, v)
		if field, ok := yamlStructField(typ, name); ok {
			w.walk(join(path, name), v, field.Type)
		}
	}
	m.Content = kept
}

// structHasSwitch reports whether typ is a section with an `enable` switch and no
// key of its own called `enabled`, so the legacy spelling can be read as the switch.
func structHasSwitch(typ reflect.Type) bool {
	_, hasSwitch := yamlStructField(typ, switchKey)
	_, hasLegacy := yamlStructField(typ, legacySwitchKey)
	return hasSwitch && !hasLegacy
}

// switchAliasFindings reports the aliases the -t check found. They are warnings:
// the loader reads them, but an editor validating against the schema flags them.
func switchAliasFindings(aliases []switchAlias) []Finding {
	out := make([]Finding, 0, len(aliases))
	for _, a := range aliases {
		f := Finding{Severity: SeverityWarning, Line: a.Line, Column: a.Column, Path: a.Path}
		if a.Dropped {
			f.Message = `"enabled" is the old spelling of "enable", which this section also sets; "enable" wins and this line has no effect`
			f.Fix = "delete this line"
		} else {
			f.Message = `"enabled" is the old spelling of this switch; it is read as "enable"`
			f.Fix = `rename the key to "enable" (a save from the settings screen or config_commit does it for you)`
		}
		out = append(out, f)
	}
	return out
}

// decodeConfigDocument decodes a parsed config file into cfg, reading the legacy
// `enabled` keys as `enable`. An empty document leaves cfg as it is.
func decodeConfigDocument(doc *yaml.Node, cfg *Config) error {
	if doc.Kind == 0 || (doc.Kind == yaml.DocumentNode && len(doc.Content) == 0) {
		return nil
	}
	normalizeSwitchAliases(doc)
	return doc.Decode(cfg)
}

// normalizeJSONSwitchAliases renames the legacy `enabled` key to `enable` in a JSON
// config body, the way normalizeSwitchAliases does it in a YAML document: guided by
// the ConfigJSON DTO, so only a section that actually has the switch is rewritten.
// A body that is not a JSON object is returned untouched - the caller reports the
// real parse error.
func normalizeJSONSwitchAliases(data []byte) []byte {
	dec := json.NewDecoder(bytes.NewReader(data))
	// Numbers keep the text the client sent, so re-encoding cannot turn a port
	// into 8080.000001 or lose precision on a large value.
	dec.UseNumber()
	var body interface{}
	if err := dec.Decode(&body); err != nil {
		return data
	}
	if _, ok := body.(map[string]interface{}); !ok {
		return data
	}
	walkJSONSwitchAliases(body, reflect.TypeOf(ConfigJSON{}))
	out, err := json.Marshal(body)
	if err != nil {
		return data
	}
	return out
}

func walkJSONSwitchAliases(v interface{}, typ reflect.Type) {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	switch typ.Kind() {
	case reflect.Struct:
		obj, ok := v.(map[string]interface{})
		if !ok {
			return
		}
		if structHasJSONSwitch(typ) {
			if legacy, ok := obj[legacySwitchKey]; ok {
				if _, canonical := obj[switchKey]; !canonical {
					obj[switchKey] = legacy
				}
				delete(obj, legacySwitchKey)
			}
		}
		for key, child := range obj {
			if field, ok := jsonStructField(typ, key); ok {
				walkJSONSwitchAliases(child, field.Type)
			}
		}
	case reflect.Slice, reflect.Array:
		items, ok := v.([]interface{})
		if !ok {
			return
		}
		for _, item := range items {
			walkJSONSwitchAliases(item, typ.Elem())
		}
	case reflect.Map:
		obj, ok := v.(map[string]interface{})
		if !ok {
			return
		}
		for _, child := range obj {
			walkJSONSwitchAliases(child, typ.Elem())
		}
	}
}

// structHasJSONSwitch is structHasSwitch for the JSON DTO tags.
func structHasJSONSwitch(typ reflect.Type) bool {
	_, hasSwitch := jsonStructField(typ, switchKey)
	_, hasLegacy := jsonStructField(typ, legacySwitchKey)
	return hasSwitch && !hasLegacy
}

func jsonStructField(typ reflect.Type, name string) (reflect.StructField, bool) {
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		tag := strings.Split(field.Tag.Get("json"), ",")[0]
		if tag == "" {
			tag = field.Name
		}
		if tag == name {
			return field, true
		}
	}
	return reflect.StructField{}, false
}
