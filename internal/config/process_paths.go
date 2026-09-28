package config

// Saving the process-scoped locations the way the config file wrote them.
//
// sessions.dir, memory.dir, logger.file and scheduler.dir belong to the process,
// not to a session, so the loader resolves them once: ${FOXXYCODE_HOME}, ${CWD}
// (the default working directory), environment references and a leading ~ are
// expanded, and an unset scheduler.dir becomes <home>/scheduler. Everything that
// reads them - the session store, the memory copilot, the logger, the scheduler
// daemon and its tools, the dry run - gets an absolute path and expands nothing.
//
// A save renders the config structs, so it used to write those absolute paths
// back: the first save from the Settings screen pinned the file to the home, the
// user and the machine it was saved on. The file stopped following
// FOXXYCODE_HOME (--home), and a copy kept in dotfiles or on a Docker volume
// pointed at the old home.
//
// So the loader also remembers how each of them was written (pathSpelling), and a
// save writes that spelling back for as long as the value is still the one it
// resolved to. A value changed since - on the Settings screen, by a tool, in
// code - is saved as it now is.

import (
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// pathSpelling is how one process-scoped location was written and what the
// loader made of it.
type pathSpelling struct {
	// written is the value as written, in the file or in the document the
	// Settings screen sent. "" is a location left to its default.
	written string
	// resolved is the absolute value the loader derived from written.
	resolved string
	// known is false for a Config that never went through the loader.
	known bool
}

// numProcessPaths is the length of processPaths and of Config.pathSpellings.
const numProcessPaths = 4

// processPaths lists the process-scoped locations, in the order of
// Config.pathSpellings.
var processPaths = [numProcessPaths]struct {
	// key is the field's place in config.yaml.
	key []string
	// field is the field in the loaded config.
	field func(*Config) *string
}{
	{[]string{"logger", "file"}, func(c *Config) *string { return &c.Logger.File }},
	{[]string{"sessions", "dir"}, func(c *Config) *string { return &c.Sessions.Dir }},
	{[]string{"memory", "dir"}, func(c *Config) *string { return &c.Memory.Dir }},
	{[]string{"scheduler", "dir"}, func(c *Config) *string { return &c.Scheduler.Dir }},
}

// processPathValues reads the process-scoped locations as they stand.
func (c *Config) processPathValues() [numProcessPaths]string {
	var out [numProcessPaths]string
	for i, p := range processPaths {
		out[i] = strings.TrimSpace(*p.field(c))
	}
	return out
}

// recordPathSpellings pairs the values applyDefaults started from with what it
// resolved them to.
func (c *Config) recordPathSpellings(written [numProcessPaths]string) {
	for i, p := range processPaths {
		c.pathSpellings[i] = pathSpelling{written: written[i], resolved: *p.field(c), known: true}
	}
}

// keepSourcePathSpellings replaces the recorded spellings with the text of the
// config file, raw before expandConfigBody, wherever that text expands to what
// the loader read. The body expansion has already substituted ${FOXXYCODE_HOME}
// and every environment reference and turned "$$" into "$" by the time
// applyDefaults records anything; ${CWD} and ~ reach it intact.
//
// A value is taken from the file only when expanding it alone gives exactly the
// value the loader parsed from the expanded file. Whatever reads differently that
// way (a merge key, an environment value that changes how its quoted scalar
// parses) keeps the spelling the loader saw, which is still the right value.
func keepSourcePathSpellings(cfg *Config, raw []byte) {
	var doc yaml.Node
	if err := yaml.Unmarshal(normalizeConfigSource(raw), &doc); err != nil {
		return
	}
	root := configDocumentRoot(&doc)
	for i, p := range processPaths {
		spelling := &cfg.pathSpellings[i]
		if !spelling.known {
			continue
		}
		source, ok := scalarAt(root, p.key)
		if !ok {
			continue
		}
		source = strings.TrimSpace(source)
		if source == spelling.written || strings.TrimSpace(expandConfigBody(source, cfg.Paths)) != spelling.written {
			continue
		}
		spelling.written = source
	}
}

// adoptPathSpellings carries current's spellings over to a config parsed from a
// Settings document, for every location the document sent back as current
// resolved it. GET /foxxycode/config reports the resolved paths, so without this
// the first save from the Settings screen would write them into the file. A
// location the document changed, or sent as a spelling of its own ("~/jobs",
// "${FOXXYCODE_HOME}/jobs", "" for the default), keeps what the document says.
func (c *Config) adoptPathSpellings(current *Config) {
	if current == nil {
		return
	}
	for i := range processPaths {
		next, prev := &c.pathSpellings[i], current.pathSpellings[i]
		if !next.known || !prev.known || !samePath(next.written, next.resolved) || !samePath(next.resolved, prev.resolved) {
			continue
		}
		next.written = prev.written
	}
}

// writeBackPathSpellings puts the recorded spelling in place of every
// process-scoped location that still holds the value it resolved to. out must be
// a copy the caller owns (escapeYAMLSecrets makes one): the live config keeps
// its absolute paths.
func writeBackPathSpellings(out *Config) *Config {
	for i, p := range processPaths {
		spelling := out.pathSpellings[i]
		if value := p.field(out); spelling.known && samePath(*value, spelling.resolved) {
			*value = spelling.written
		}
	}
	return out
}

// samePath reports whether two values name the same location, ignoring how
// the separators and dots are spelled. "" (unset) is only the same as "".
func samePath(a, b string) bool {
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	if a == "" || b == "" {
		return a == b
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

// scalarAt returns the string under keys in a parsed config document: "" for a
// key that is absent or null, as the loader would decode it. ok is false when a
// node on the way is not something the loader decodes into a string, and the
// value then says nothing about what the loader read.
func scalarAt(root *yaml.Node, keys []string) (string, bool) {
	node := root
	for _, key := range keys {
		node = dealias(node)
		if isNullNode(node) {
			return "", true
		}
		if node.Kind != yaml.MappingNode {
			return "", false
		}
		_, value, ok := mappingValue(node, key)
		if !ok {
			return "", true
		}
		node = value
	}
	node = dealias(node)
	if isNullNode(node) {
		return "", true
	}
	if node.Kind != yaml.ScalarNode {
		return "", false
	}
	return node.Value, true
}

func dealias(n *yaml.Node) *yaml.Node {
	for n != nil && n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	return n
}

func isNullNode(n *yaml.Node) bool {
	return n == nil || (n.Kind == yaml.ScalarNode && n.ShortTag() == "!!null")
}
