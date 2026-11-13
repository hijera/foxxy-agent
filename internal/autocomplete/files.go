package autocomplete

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/textenc"
)

const (
	relatedFileLines     = 40
	relatedFileBytes     = 1500
	relatedFileReadBytes = 16 * 1024
)

// RelatedFiles excerpts the other files the host reports open:
// their first lines hold the imports and signatures the model needs to complete a call into
// them. Only files inside the supplied workspace are read - an editor may have anything open,
// and a suggestion request carries no permission prompt.
func RelatedFiles(workspace, current string, limit int, openFiles []string) []llm.FIMFile {
	if limit <= 0 {
		return nil
	}
	root, err := filepath.Abs(strings.TrimSpace(workspace))
	if err != nil || strings.TrimSpace(workspace) == "" {
		return nil
	}
	currentAbs, _ := filepath.Abs(current)

	var out []llm.FIMFile
	for _, p := range openFiles {
		if len(out) >= limit {
			break
		}
		abs, err := filepath.Abs(p)
		if err != nil || strings.EqualFold(abs, currentAbs) {
			continue
		}
		rel, err := filepath.Rel(root, abs)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		excerpt, ok := readExcerpt(abs)
		if !ok {
			continue
		}
		out = append(out, llm.FIMFile{Path: filepath.ToSlash(rel), Content: excerpt})
	}
	return out
}

// readExcerpt returns the head of a text file, decoded the way every file tool decodes workspace
// bytes (internal/textenc), capped by lines and bytes. Binary and empty files are skipped.
func readExcerpt(path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, relatedFileReadBytes))
	if err != nil || len(data) == 0 {
		return "", false
	}
	text, _, err := textenc.Decode(data)
	if err != nil || strings.ContainsRune(text, 0) {
		return "", false
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if len(lines) > relatedFileLines {
		lines = lines[:relatedFileLines]
	}
	excerpt := headBytes(strings.Join(lines, "\n"), relatedFileBytes)
	if strings.TrimSpace(excerpt) == "" {
		return "", false
	}
	return excerpt, true
}

// DisplayPath shows the model a workspace-relative path when the file is inside the workspace,
// matching how related files are named, and the bare file name otherwise.
func DisplayPath(workspace, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if root := strings.TrimSpace(workspace); root != "" {
		if rootAbs, err := filepath.Abs(root); err == nil {
			if abs, err := filepath.Abs(path); err == nil {
				if rel, err := filepath.Rel(rootAbs, abs); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
					return filepath.ToSlash(rel)
				}
			}
		}
	}
	return filepath.Base(path)
}
