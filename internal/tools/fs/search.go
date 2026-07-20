package fs

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

const maxSearchLineBytes = 10 * 1024 * 1024

// maxDecodedSearchFileBytes bounds the files read whole so they can be decoded
// before matching. Larger files stream as UTF-8, keeping search memory-bounded
// at the cost of legacy-encoding support for oversized inputs.
const maxDecodedSearchFileBytes = 8 * 1024 * 1024

var errStopSearch = errors.New("search result limit reached")

// nativeGrepSearch is the built-in content-search engine used when no system
// ripgrep is available and whenever the pattern is encoding-sensitive. It walks
// regular files under root, skips binaries, and emits path:line:content records
// up to maxResults.
func nativeGrepSearch(ctx context.Context, root, glob, storeRoot string, matcher *regexp.Regexp, maxResults int) (string, error) {
	var output strings.Builder
	matchCount := 0
	emit := func(filePath string, lineNumber int, line string) error {
		if output.Len() > 0 {
			output.WriteByte('\n')
		}
		fmt.Fprintf(&output, "%s:%d:%s", filePath, lineNumber, line)
		matchCount++
		if matchCount >= maxResults {
			return errStopSearch
		}
		return nil
	}
	err := walkSearchFiles(ctx, root, glob, storeRoot, func(filePath string) error {
		return searchFileLines(ctx, filePath, matcher, emit)
	})
	if errors.Is(err, errStopSearch) {
		err = nil
	}
	return output.String(), err
}

// searchFileLines matches one file, decoding it first so Windows-1251 sources
// answer the same patterns as UTF-8 ones (the encoding layer used by read/edit,
// see decodeText). Binary files and oversized inputs bypass the decode step.
func searchFileLines(ctx context.Context, filePath string, matcher *regexp.Regexp, emit func(string, int, string) error) (returnErr error) {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer func() {
		returnErr = errors.Join(returnErr, file.Close())
	}()

	reader := bufio.NewReaderSize(file, 8192)
	probe, _ := reader.Peek(8192)
	if bytes.IndexByte(probe, 0) >= 0 {
		return nil
	}

	info, statErr := file.Stat()
	if statErr == nil && info.Size() <= maxDecodedSearchFileBytes {
		data, err := io.ReadAll(reader)
		if err != nil {
			return err
		}
		content, _, err := decodeText(data)
		if err != nil {
			// Bytes that decode as neither UTF-8 nor Windows-1251 are not text;
			// skipping beats emitting mojibake into the model's context.
			return nil
		}
		return matchDecodedLines(ctx, filePath, content, matcher, emit)
	}

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), maxSearchLineBytes)
	lineNumber := 0
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		lineNumber++
		line := scanner.Text()
		if !matcher.MatchString(line) {
			continue
		}
		if err := emit(filePath, lineNumber, line); err != nil {
			return err
		}
	}
	return scanner.Err()
}

// matchDecodedLines splits decoded text the way bufio.ScanLines does: strip a
// trailing \r and do not report a phantom final line after a trailing newline.
func matchDecodedLines(ctx context.Context, filePath, content string, matcher *regexp.Regexp, emit func(string, int, string) error) error {
	lines := strings.Split(content, "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	for i, line := range lines {
		if err := ctx.Err(); err != nil {
			return err
		}
		line = strings.TrimSuffix(line, "\r")
		if !matcher.MatchString(line) {
			continue
		}
		if err := emit(filePath, i+1, line); err != nil {
			return err
		}
	}
	return nil
}

// nativeGlob is the built-in file-listing engine used when no system ripgrep
// is available.
func nativeGlob(ctx context.Context, root, pattern, storeRoot string) ([]string, error) {
	if err := validateSearchGlob(pattern); err != nil {
		return nil, err
	}
	var paths []string
	err := walkSearchFiles(ctx, root, pattern, storeRoot, func(filePath string) error {
		paths = append(paths, filePath)
		return nil
	})
	return paths, err
}

// walkSearchFiles visits regular files under root that match pattern, skipping
// dotfiles, symlinks, and the FoxxyCode session store. Unlike ripgrep it does not
// honor .gitignore, so fallback results may include vendored trees.
func walkSearchFiles(ctx context.Context, root, pattern, storeRoot string, visit func(string) error) error {
	root = filepath.Clean(root)
	info, err := os.Stat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		if isWithinDir(root, storeRoot) || !searchGlobMatches(pattern, filepath.Base(root)) {
			return nil
		}
		return visit(root)
	}

	return filepath.WalkDir(root, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			if filePath == root {
				return walkErr
			}
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if filePath == root {
			return nil
		}
		if isWithinDir(filePath, storeRoot) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(entry.Name(), ".") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		rel, err := filepath.Rel(root, filePath)
		if err != nil || !searchGlobMatches(pattern, filepath.ToSlash(rel)) {
			return nil
		}
		return visit(filePath)
	})
}

func validateSearchGlob(pattern string) error {
	if strings.TrimSpace(pattern) == "" {
		return nil
	}
	_, err := doublestar.Match(filepath.ToSlash(pattern), "candidate")
	return err
}

func searchGlobMatches(pattern, relativePath string) bool {
	pattern = filepath.ToSlash(strings.TrimSpace(pattern))
	relativePath = filepath.ToSlash(relativePath)
	if pattern == "" {
		return true
	}
	if !strings.Contains(pattern, "/") {
		matched, _ := doublestar.Match(pattern, filepath.Base(relativePath))
		return matched
	}
	matched, _ := doublestar.Match(pattern, relativePath)
	return matched
}

// limitSearchLines caps output to maxResults lines; ripgrep's --max-count is
// per file while the tool contract documents a total maximum.
func limitSearchLines(output string, maxResults int) string {
	output = strings.TrimRight(output, "\r\n")
	if output == "" {
		return ""
	}
	lines := strings.Split(output, "\n")
	if len(lines) > maxResults {
		lines = lines[:maxResults]
	}
	return strings.Join(lines, "\n")
}
