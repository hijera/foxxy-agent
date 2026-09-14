package session

import (
	"os"
	"path/filepath"
	"strings"
)

// LoadInstructions reads the configured instruction files from cwd and concatenates their contents.
// Files that don't exist are silently skipped (matching other-agent AGENTS.md convention).
//
// A file already in the prompt is skipped too: alreadyLoaded names the files another prompt block
// carries (the root AGENTS.md and DESIGN.md enter the rules block through rules.LoadProjectDocs),
// and a file named twice in files loads once. Files are compared on disk, not by name, so
// "./AGENTS.md", a differently cased name on a case-insensitive volume, and a symlink such as
// CLAUDE.md -> AGENTS.md all count as the same file.
func LoadInstructions(cwd string, files []string, alreadyLoaded ...string) string {
	var seen []os.FileInfo
	for _, p := range alreadyLoaded {
		if fi, err := os.Stat(p); err == nil {
			seen = append(seen, fi)
		}
	}
	var parts []string
	for _, name := range files {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		path := filepath.Join(cwd, name)
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
