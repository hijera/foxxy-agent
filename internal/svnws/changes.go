package svnws

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/workcopy"
)

type reviewStatus struct {
	Path string `xml:"path,attr"`
	WC   struct {
		Item           string `xml:"item,attr"`
		Props          string `xml:"props,attr"`
		TreeConflicted bool   `xml:"tree-conflicted,attr"`
	} `xml:"wc-status"`
}

func (s reviewStatus) changeStatus() string {
	if s.WC.TreeConflicted || s.WC.Props == "conflicted" {
		return ""
	}
	switch s.WC.Item {
	case "added", "unversioned":
		return "added"
	case "deleted", "missing":
		return "deleted"
	case "modified":
		return "modified"
	case "normal":
		if s.WC.Props == "modified" {
			return "modified"
		}
	}
	return ""
}

func reviewStatusEntries(ctx context.Context, o Options, dir string) ([]reviewStatus, error) {
	out, err := runBytes(ctx, o, dir, "status", "--xml", "--ignore-externals")
	if err != nil {
		return nil, err
	}
	type group struct {
		Entries []reviewStatus `xml:"entry"`
	}
	var status struct {
		Targets     []group `xml:"target"`
		Changelists []group `xml:"changelist"`
	}
	if err := xml.Unmarshal(out, &status); err != nil {
		return nil, fmt.Errorf("decode svn status: %w", err)
	}
	var entries []reviewStatus
	for _, g := range append(status.Targets, status.Changelists...) {
		for _, entry := range g.Entries {
			if entry.changeStatus() == "" || !filepath.IsLocal(entry.Path) || filepath.Clean(entry.Path) == "." {
				continue
			}
			entry.Path = filepath.Clean(entry.Path)
			entries = append(entries, entry)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

// svnTarget disambiguates literal @ signs in working copy names from peg revisions.
func svnTarget(path string) string { return path + "@" }

// reviewFile checks for a leaf file without following links. A missing or
// scheduled-deleted node needs SVN metadata to distinguish files and directories.
func reviewFile(ctx context.Context, o Options, root *os.Root, entry reviewStatus) (fs.FileInfo, bool, error) {
	fi, err := root.Lstat(entry.Path)
	if err == nil {
		return fi, isReviewFile(fi), nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, false, err
	}
	if entry.changeStatus() != "deleted" {
		return nil, false, nil
	}
	out, err := runBytes(ctx, o, root.Name(), "info", "--xml", "--", svnTarget(entry.Path))
	if err != nil {
		return nil, false, err
	}
	var info infoXML
	if err := xml.Unmarshal(out, &info); err != nil {
		return nil, false, fmt.Errorf("decode svn info: %w", err)
	}
	return nil, len(info.Entries) == 1 && info.Entries[0].Kind == "file", nil
}

func isReviewFile(fi fs.FileInfo) bool {
	return fi.Mode().IsRegular() || fi.Mode()&fs.ModeSymlink != 0
}

// skipped marks an unversioned file that exceeds the shared review size limit.
func readReviewChange(ctx context.Context, o Options, root *os.Root, entry reviewStatus, fi fs.FileInfo) (*workcopy.Change, bool, error) {
	change := &workcopy.Change{Path: entry.Path, Status: entry.changeStatus()}
	if change.Status != "added" {
		before, err := runBytes(ctx, o, root.Name(), "cat", "-r", "BASE", "--", svnTarget(entry.Path))
		if err != nil {
			return nil, false, err
		}
		change.Before = before
	}
	if change.Status == "deleted" {
		return change, false, nil
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		target, err := root.Readlink(entry.Path)
		change.After, change.Binary = []byte(target), true
		return change, false, err
	}
	if entry.WC.Item != "unversioned" {
		after, err := root.ReadFile(entry.Path)
		change.After = after
		return change, false, err
	}
	f, err := root.Open(entry.Path)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = f.Close() }()
	change.After, err = io.ReadAll(io.LimitReader(f, workcopy.MaxNewBytes+1))
	if len(change.After) > workcopy.MaxNewBytes {
		return nil, true, err
	}
	return change, false, err
}

// WorktreeChanges lists SVN's file changes against this checkout's local BASE.
func WorktreeChanges(ctx context.Context, o Options, dir string) ([]workcopy.Change, int, error) {
	entries, err := reviewStatusEntries(ctx, o, dir)
	if err != nil {
		return nil, 0, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = root.Close() }()
	var changes []workcopy.Change
	newFiles, skipped := 0, 0
	for _, entry := range entries {
		fi, file, err := reviewFile(ctx, o, root, entry)
		if err != nil {
			return nil, 0, err
		}
		if !file {
			continue
		}
		unversioned := entry.WC.Item == "unversioned"
		if unversioned && newFiles >= workcopy.MaxNewFiles {
			skipped++
			continue
		}
		change, tooLarge, err := readReviewChange(ctx, o, root, entry, fi)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, 0, err
		}
		if tooLarge {
			skipped++
			continue
		}
		changes = append(changes, *change)
		if unversioned {
			newFiles++
		}
	}
	return changes, skipped, nil
}

// WorktreeChangeFor reads only the requested file after SVN confirms membership.
func WorktreeChangeFor(ctx context.Context, o Options, dir, path string) (*workcopy.Change, error) {
	entries, err := reviewStatusEntries(ctx, o, dir)
	if err != nil {
		return nil, err
	}
	path = filepath.FromSlash(path)
	for _, entry := range entries {
		if entry.Path != path {
			continue
		}
		root, err := os.OpenRoot(dir)
		if err != nil {
			return nil, err
		}
		defer func() { _ = root.Close() }()
		fi, file, err := reviewFile(ctx, o, root, entry)
		if err != nil || !file {
			return nil, err
		}
		change, _, err := readReviewChange(ctx, o, root, entry, fi)
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return change, err
	}
	return nil, nil
}

// Discard restores the selected changed files, or all reviewable files if
// paths is empty. All requested paths are checked before any file is changed.
func Discard(ctx context.Context, o Options, dir string, paths []string) error {
	entries, err := reviewStatusEntries(ctx, o, dir)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	reported := make(map[string]bool, len(entries))
	for _, entry := range entries {
		reported[entry.Path] = true
	}
	wanted := make(map[string]bool, len(paths))
	for _, path := range paths {
		path = filepath.FromSlash(path)
		if !reported[path] {
			path = filepath.FromSlash(strings.ReplaceAll(path, `\`, "/"))
		}
		wanted[path] = true
	}
	var selected []reviewStatus
	for _, entry := range entries {
		if len(paths) != 0 && !wanted[entry.Path] {
			continue
		}
		_, file, err := reviewFile(ctx, o, root, entry)
		if err != nil {
			return err
		}
		if file {
			selected = append(selected, entry)
			delete(wanted, entry.Path)
		}
	}
	for path := range wanted {
		return fmt.Errorf("%w: %s", workcopy.ErrNotChanged, path)
	}
	for _, entry := range selected {
		if entry.WC.Item != "unversioned" {
			if _, err := run(ctx, o, dir, "revert", "--depth", "empty", "--", svnTarget(entry.Path)); err != nil {
				return err
			}
		}
		if entry.changeStatus() == "added" {
			if err := removeSVNReviewFile(root, entry.Path); err != nil {
				return err
			}
		}
	}
	return nil
}

func removeSVNReviewFile(root *os.Root, path string) error {
	fi, err := root.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !isReviewFile(fi) {
		return fmt.Errorf("%w: %s", workcopy.ErrNotChanged, path)
	}
	return root.Remove(path)
}
