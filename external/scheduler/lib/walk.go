//go:build scheduler

package scheduler

import (
	"io/fs"
	"path/filepath"
	"strings"
)

// ListJobMarkdownFiles collects *.md files under scheduler roots (excluding dotfiles).
func ListJobMarkdownFiles(roots []string) ([]string, error) {
	var out []string
	seen := map[string]struct{}{}
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(strings.ToLower(path), ".md") {
				return nil
			}
			base := filepath.Base(path)
			if strings.HasPrefix(base, ".") {
				return nil
			}
			if _, ok := seen[path]; ok {
				return nil
			}
			seen[path] = struct{}{}
			out = append(out, path)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
