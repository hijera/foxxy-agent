package config

import (
	"bytes"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// expandEnvEscaped expands ${VAR} and $VAR references from the process environment,
// like os.ExpandEnv, but treats "$$" as an escape for a literal "$". This lets secrets
// that contain a dollar sign (e.g. a proxy password like "$2y$10$...") survive the
// load-time expansion pass instead of having their "$WORD"/"$N" fragments resolved to
// empty environment variables. Values are written to disk with "$" doubled to "$$"
// (see escapeYAMLDollar) and read back here as a single literal "$".
func expandEnvEscaped(s string) string {
	// "$$" is the documented escape for a literal "$". It is resolved first,
	// pairing left to right exactly as os.Expand would, so that "$${CWD}"
	// still yields the literal "${CWD}" and escapeYAMLDollar stays an inverse.
	s = strings.ReplaceAll(s, "$$", escapedDollarSentinel)
	// ${CWD} is the session placeholder, not an environment reference: it is
	// hidden from os.Expand and restored afterwards, so it stays in the
	// document for whoever resolves it against a session working directory,
	// even when the environment has a CWD variable. Only the braced spelling
	// is the placeholder; a bare $CWD is an ordinary reference as before.
	s = strings.ReplaceAll(s, sessionCWDPlaceholder, sessionCWDSentinel)
	s = os.Expand(s, os.Getenv)
	s = strings.ReplaceAll(s, sessionCWDSentinel, sessionCWDPlaceholder)
	return strings.ReplaceAll(s, escapedDollarSentinel, "$")
}

// The sentinels stand in for "$$" and "${CWD}" while the environment is
// expanded; neither carries a "$", so os.Expand passes them through.
const (
	sessionCWDPlaceholder = "${CWD}"
	sessionCWDSentinel    = "\x00foxxycode-session-cwd\x00"
	escapedDollarSentinel = "\x00foxxycode-escaped-dollar\x00"
)

// expandConfigBody prepares raw config.yaml text for parsing: ${FOXXYCODE_HOME} is
// substituted (with forward slashes, see ExpandPathVars) and environment
// references are expanded, while ${CWD} survives verbatim. Per-session paths
// (skills.dirs, subagents.dirs, hooks.files, prompts.dir, mcp_servers) resolve
// it against the workspace of the session that uses them; the process-scoped
// directories expand it against Paths.CWD in applyDefaults. A leading ~ inside
// a value is left to those consumers as well, as it always was: the previous
// body-level pass only looked at the first byte of the whole document.
func expandConfigBody(s string, p Paths) string {
	// Every path that parses a config file passes through here, so this is where
	// what an editor left in the bytes - a byte order mark, Windows line endings -
	// stops travelling any further (see source.go).
	s = string(normalizeConfigSource([]byte(s)))
	// "$$" is resolved before ${FOXXYCODE_HOME} is substituted so that
	// "$${FOXXYCODE_HOME}" keeps a literal placeholder, like "$${CWD}" does.
	s = strings.ReplaceAll(s, "$$", escapedDollarSentinel)
	s = strings.ReplaceAll(s, "${FOXXYCODE_HOME}", yamlSafePath(p.Home))
	return expandEnvEscaped(s)
}

// homeMarker stands in for the agent home in the second expansion of a config file
// that keepHomePlaceholders reads. Letters, digits and underscores only, so every YAML
// scalar style carries it through unchanged.
const homeMarker = "FOXXYCODE_HOME_MARKER_5b0e9d2c"

// keepHomePlaceholders puts ${FOXXYCODE_HOME} back into the entries of the per-session
// path lists (skills.dirs, subagents.dirs, hooks.files) that expandConfigBody
// substituted with this machine's home.
//
// Those lists are resolved where they are read: every loader expands
// ${FOXXYCODE_HOME}, ${CWD} and a leading ~ itself, the defaults keep their
// placeholders, and a save writes back whatever the struct holds. An entry that
// reached the struct as an absolute path was therefore written to the file as one,
// and the file stopped following FOXXYCODE_HOME (--home) and could not be copied to
// another machine or user.
//
// The file is expanded a second time with homeMarker in place of the home, which
// tells an entry that wrote the placeholder from one that spelled the same
// directory out. Environment references and $$ escapes resolve the same way in both
// passes, and an entry is rewritten only when the marker accounts for every
// difference between them, so anything the second pass reads differently keeps
// what the loader parsed.
func keepHomePlaceholders(cfg *Config, raw []byte) {
	home := yamlSafePath(cfg.Paths.Home)
	if home == "" || !bytes.Contains(raw, []byte("${FOXXYCODE_HOME}")) {
		return
	}
	var marked struct {
		Skills struct {
			Dirs []string `yaml:"dirs"`
		} `yaml:"skills"`
		Subagents struct {
			Dirs []string `yaml:"dirs"`
		} `yaml:"subagents"`
		Hooks struct {
			Files []string `yaml:"files"`
		} `yaml:"hooks"`
	}
	if err := yaml.Unmarshal([]byte(expandConfigBody(string(raw), Paths{Home: homeMarker})), &marked); err != nil {
		return
	}
	restoreHomePlaceholder(cfg.Skills.Dirs, marked.Skills.Dirs, home)
	restoreHomePlaceholder(cfg.Subagents.Dirs, marked.Subagents.Dirs, home)
	restoreHomePlaceholder(cfg.Hooks.Files, marked.Hooks.Files, home)
}

// restoreHomePlaceholder rewrites the loaded entries whose marked twin carries the
// home marker and differs from them by nothing else. Lists of different lengths do
// not pair up: the file listed nothing and the loaded list is the defaults, which
// keep their placeholders anyway.
func restoreHomePlaceholder(loaded, marked []string, home string) {
	if len(loaded) != len(marked) {
		return
	}
	for i, entry := range marked {
		entry = strings.TrimSpace(entry)
		if !strings.Contains(entry, homeMarker) || strings.ReplaceAll(entry, homeMarker, home) != loaded[i] {
			continue
		}
		loaded[i] = strings.ReplaceAll(entry, homeMarker, "${FOXXYCODE_HOME}")
	}
}

// escapeYAMLDollar doubles every "$" so that expandEnvEscaped restores the exact literal
// on the next load. Applied to always-literal secret fields (proxy URLs) before they are
// serialized to disk, so a value never gets mangled by environment-variable expansion.
func escapeYAMLDollar(s string) string {
	return strings.ReplaceAll(s, "$", "$$")
}
