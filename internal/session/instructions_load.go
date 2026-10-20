package session

import (
	"os"
	"path/filepath"
	"strings"
)

// ResolveInstructionFile resolves one instructions.files entry to an absolute
// path. ${FOXXYCODE_HOME} and ${CWD} expand, a leading ~ expands to the user's home
// directory, an absolute entry is taken as it stands, and a relative one is
// anchored at the session workspace. An entry that cannot be resolved - a
// ${FOXXYCODE_HOME} reference without a home, a relative entry without a cwd -
// returns "" rather than a path assembled out of a literal placeholder.
func ResolveInstructionFile(entry, cwd, home string) string {
	path := strings.TrimSpace(entry)
	if path == "" {
		return ""
	}
	if strings.Contains(path, "${FOXXYCODE_HOME}") {
		if strings.TrimSpace(home) == "" {
			return ""
		}
		path = strings.ReplaceAll(path, "${FOXXYCODE_HOME}", home)
	}
	if strings.Contains(path, "${CWD}") {
		if strings.TrimSpace(cwd) == "" {
			return ""
		}
		path = strings.ReplaceAll(path, "${CWD}", cwd)
	}
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		path = filepath.Join(userHome, strings.TrimLeft(strings.TrimPrefix(path, "~"), `/\`))
	}
	if !filepath.IsAbs(path) {
		if strings.TrimSpace(cwd) == "" {
			return ""
		}
		path = filepath.Join(cwd, path)
	}
	return filepath.Clean(path)
}

// LoadInstructions reads the configured instruction files and concatenates their
// contents. Files that don't exist are silently skipped (matching other-agent
// AGENTS.md convention), as are empty ones, a file named twice, and any file in
// skip - the preamble documents the rules block already carries (the agent
// home's and the workspace's AGENTS.md and DESIGN.md, through
// rules.LoadProjectDocs), which would otherwise reach the model a second time.
//
// Files are compared on disk, not by name, so "./AGENTS.md", a differently cased
// name on a case-insensitive volume, a hard link and a symlink such as
// CLAUDE.md -> AGENTS.md all count as the same file.
func LoadInstructions(cwd, home string, files, skip []string) string {
	var seen []os.FileInfo
	for _, p := range skip {
		if fi, err := os.Stat(p); err == nil {
			seen = append(seen, fi)
		}
	}
	var parts []string
	for _, entry := range files {
		path := ResolveInstructionFile(entry, cwd, home)
		if path == "" {
			continue
		}
		fi, err := os.Stat(path)
		if err != nil || sameFileAsAny(fi, seen) {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		seen = append(seen, fi)
		body := strings.TrimSpace(string(data))
		if body == "" {
			continue
		}
		parts = append(parts, body)
	}
	return strings.Join(parts, "\n\n")
}

func sameFileAsAny(fi os.FileInfo, seen []os.FileInfo) bool {
	for _, s := range seen {
		if os.SameFile(fi, s) {
			return true
		}
	}
	return false
}
