package rules

import (
	"os"
	"path/filepath"
	"strings"
)

const projectDocMaxBytes = 256 * 1024

// ProjectDoc holds preamble content for AGENTS.md / DESIGN.md.
type ProjectDoc struct {
	Label   string
	Path    string
	Content string
}

// LoadProjectDocs reads AGENTS.md and DESIGN.md from cwd when present. The
// {{.Rules}} block is the one place these root files enter the prompt: the
// nested AGENTS.md chain starts below the root (AgentsForPaths), and the
// instruction files - whose default entry is AGENTS.md - skip them.
func LoadProjectDocs(cwd string) []ProjectDoc {
	var out []ProjectDoc
	for _, spec := range []struct {
		file  string
		label string
	}{
		{"AGENTS.md", "AGENTS.md"},
		{"DESIGN.md", "DESIGN.md"},
	} {
		p := filepath.Join(cwd, spec.file)
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		content := strings.TrimSpace(string(b))
		if content == "" {
			continue
		}
		if len(content) > projectDocMaxBytes {
			content = content[:projectDocMaxBytes] + "\n\n...(truncated)"
		}
		out = append(out, ProjectDoc{Label: spec.label, Path: p, Content: content})
	}
	return out
}
