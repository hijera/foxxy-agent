package tools

import (
	"fmt"
	"path/filepath"
	"strings"

	toolfs "github.com/hijera/foxxycode-agent/internal/tools/fs"
)

// resolveDocsPath validates and resolves a documentation markdown path relative to cwd.
func resolveDocsPath(path, cwd string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("docs path must not be empty")
	}
	if strings.Contains(path, "..") {
		return "", fmt.Errorf("docs path must not contain ..")
	}

	abs := filepath.Clean(toolfs.ResolvePath(path, cwd))
	cwdClean := filepath.Clean(cwd)

	rel, err := filepath.Rel(cwdClean, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("docs path must stay within the working directory")
	}

	base := strings.ToLower(filepath.Base(abs))
	if !strings.HasSuffix(base, ".md") {
		return "", fmt.Errorf("docs path must end with .md")
	}

	relSlash := filepath.ToSlash(rel)
	if relSlash == "internal/prompts" || strings.HasPrefix(relSlash, "internal/prompts/") {
		return "", fmt.Errorf("docs path must not write to internal/prompts")
	}

	return abs, nil
}
