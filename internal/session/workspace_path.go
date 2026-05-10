package session

import (
	"errors"
	"path/filepath"
	"strings"
)

// ErrPathTraversal means a relative path escapes the allowed workspace root.
var ErrPathTraversal = errors.New("path escapes allowed root")

// NormalizeWorkspaceRelativePath returns a clean relative path for workspace use.
// It rejects absolute paths, ".." segments, and empty components.
func NormalizeWorkspaceRelativePath(p string) (string, error) {
	raw := filepath.ToSlash(strings.TrimSpace(p))
	if raw == "" {
		return "", nil
	}
	if strings.Contains(raw, "..") {
		return "", ErrPathTraversal
	}
	cl := filepath.Clean(raw)
	cl = filepath.ToSlash(cl)
	if cl == "." || cl == "/" {
		return "", nil
	}
	if filepath.IsAbs(cl) {
		return "", ErrPathTraversal
	}
	for _, seg := range strings.Split(cl, "/") {
		if seg == ".." || seg == "" {
			return "", ErrPathTraversal
		}
	}
	return filepath.FromSlash(strings.Trim(cl, `\`)), nil
}

// AbsPathUnderWorkspaceRoot joins root with a normalized relative path and checks containment.
func AbsPathUnderWorkspaceRoot(root string, rel string) (string, error) {
	rootClean, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", err
	}
	joined := filepath.Join(rootClean, rel)
	target, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}
	relCmp, err := filepath.Rel(rootClean, target)
	if err != nil {
		return "", ErrPathTraversal
	}
	for _, seg := range strings.Split(relCmp, string(filepath.Separator)) {
		if seg == ".." {
			return "", ErrPathTraversal
		}
	}
	return target, nil
}
