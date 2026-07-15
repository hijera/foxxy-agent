package tools

import (
	"fmt"
	"os"
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
	if hasParentSegment(path) {
		return "", fmt.Errorf("docs path must not contain a parent-directory segment")
	}

	abs := filepath.Clean(toolfs.ResolvePath(path, cwd))
	cwdClean := filepath.Clean(cwd)

	rel, err := filepath.Rel(cwdClean, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("docs path must stay within the working directory")
	}

	realCWD, err := filepath.EvalSymlinks(cwdClean)
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	realAbs, err := evalPathWithMissingTail(abs)
	if err != nil {
		return "", fmt.Errorf("resolve docs path: %w", err)
	}
	rel, err = filepath.Rel(realCWD, realAbs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("docs path must stay within the working directory after resolving links")
	}
	abs = realAbs

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

func hasParentSegment(path string) bool {
	parts := strings.FieldsFunc(path, func(r rune) bool {
		return r == '/' || r == '\\'
	})
	for _, part := range parts {
		if part == ".." {
			return true
		}
	}
	return false
}

// evalPathWithMissingTail resolves links in the longest existing ancestor and
// then restores any not-yet-created path components below it.
func evalPathWithMissingTail(path string) (string, error) {
	current := filepath.Clean(path)
	var missing []string
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		if _, lstatErr := os.Lstat(current); lstatErr == nil {
			return "", fmt.Errorf("path contains a link with an unresolved target: %s", current)
		} else if !os.IsNotExist(lstatErr) {
			return "", lstatErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}
