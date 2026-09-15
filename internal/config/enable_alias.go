package config

// Upstream coddy spells every on/off switch `enable`; FoxxyCode kept `enabled`
// (see UPSTREAM_SYNC.md). The loader decodes non-strictly, so a config written
// for coddy, or copied from its documentation, would load with those switches
// silently at their defaults. Every place that reads config.yaml as a document
// renames the key first: the loader, the -t check, config_get / config_set, the
// comments a settings save carries over, and the locator diagnostics use.

import (
	"fmt"
	"reflect"

	"gopkg.in/yaml.v3"
)

const (
	coddyEnableKey = "enable"
	enabledKey     = "enabled"
)

// enableAlias is one coddy `enable` key found in a config document.
type enableAlias struct {
	// Path is the dotted path of the key as the file spells it ("httpserver.login.enable").
	Path string
	// Line and Column locate the key in the file.
	Line, Column int
	// Dropped is set when the same section also sets `enabled`: that key wins and
	// the alias was removed from the document.
	Dropped bool
}

// normalizeEnableAliases renames `enable` to `enabled` in every mapping whose
// config struct has an `enabled` field, and removes it where the mapping already
// has `enabled`. The walk follows the Config type, so a key that only happens to
// be called enable - a label, or a key of a section without a switch - keeps its
// name. n is a document node or its root mapping.
func normalizeEnableAliases(n *yaml.Node) []enableAlias {
	w := enableAliasWalker{seen: map[*yaml.Node]bool{}}
	w.walk("", n, reflect.TypeOf(Config{}))
	return w.found
}

type enableAliasWalker struct {
	// seen holds the mappings already walked: an anchor is shared by every alias
	// and merge key that names it.
	seen  map[*yaml.Node]bool
	found []enableAlias
}

func (w *enableAliasWalker) walk(path string, n *yaml.Node, typ reflect.Type) {
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
func (w *enableAliasWalker) mapping(path string, m *yaml.Node, typ reflect.Type) {
	_, hasEnabled := yamlStructField(typ, enabledKey)
	_, hasEnable := yamlStructField(typ, coddyEnableKey)
	aliased := hasEnabled && !hasEnable
	enabledSet := false
	if aliased {
		_, _, enabledSet = mappingValue(m, enabledKey)
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
		if aliased && name == coddyEnableKey {
			alias := enableAlias{Path: join(path, coddyEnableKey), Line: k.Line, Column: k.Column, Dropped: enabledSet}
			w.found = append(w.found, alias)
			if alias.Dropped {
				continue
			}
			k.Value = enabledKey
			name = enabledKey
		}
		kept = append(kept, k, v)
		if field, ok := yamlStructField(typ, name); ok {
			w.walk(join(path, name), v, field.Type)
		}
	}
	m.Content = kept
}

// enableAliasFindings reports the aliases the -t check found. They are warnings:
// the loader reads them, but an editor validating against the schema flags them.
func enableAliasFindings(aliases []enableAlias) []Finding {
	out := make([]Finding, 0, len(aliases))
	for _, a := range aliases {
		f := Finding{Severity: SeverityWarning, Line: a.Line, Column: a.Column, Path: a.Path}
		if a.Dropped {
			f.Message = `"enable" is coddy's spelling of "enabled", which this section also sets; "enabled" wins and this line has no effect`
			f.Fix = "delete this line"
		} else {
			f.Message = `"enable" is coddy's spelling; FoxxyCode reads it as "enabled"`
			f.Fix = `rename the key to "enabled" (a save from the settings screen or config_commit does it for you)`
		}
		out = append(out, f)
	}
	return out
}

// decodeConfigDocument decodes a parsed config file into cfg, reading coddy's
// `enable` keys as `enabled`. An empty document leaves cfg as it is.
func decodeConfigDocument(doc *yaml.Node, cfg *Config) error {
	if doc.Kind == 0 || (doc.Kind == yaml.DocumentNode && len(doc.Content) == 0) {
		return nil
	}
	normalizeEnableAliases(doc)
	return doc.Decode(cfg)
}
