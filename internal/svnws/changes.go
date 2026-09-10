package svnws

import (
	"bytes"
	"context"
	"encoding/xml"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	// maxUnversionedFiles bounds how many brand-new files the all-files scope
	// reads, mirroring the git side. A working copy that just produced hundreds
	// of them is not something anyone reviews file by file.
	maxUnversionedFiles = 500

	// maxUnversionedBytes skips a single new file too large to diff usefully.
	maxUnversionedBytes = 2 << 20
)

// WorkChange is one file the working copy holds differently from its pristine
// base.
//
// It deliberately mirrors gitws.WorkChange rather than sharing it: the two
// packages are siblings that shell out to different clients, and neither imports
// the other. The HTTP layer adapts both onto one wire shape.
type WorkChange struct {
	Path   string // working-copy-relative, OS separators
	Status string // "added" | "modified" | "deleted"
	Before []byte // nil when the file is new since BASE
	After  []byte // nil when the file is gone from the working copy
}

// statusXML mirrors the parts of `svn status --xml` this package consumes.
type statusXML struct {
	Targets []struct {
		Entries []struct {
			Path     string `xml:"path,attr"`
			WCStatus struct {
				Item string `xml:"item,attr"`
			} `xml:"wc-status"`
		} `xml:"entry"`
	} `xml:"target"`
}

// WorkingCopyChanges reports what the working copy holds that its base revision
// does not. It is the Subversion counterpart of gitws.UncommittedChanges and,
// with includeUnversioned, of gitws.WorktreeChanges.
//
// skipped counts unversioned files not included: every one of them when they
// were not asked for, and otherwise only those past the caps. A folder that is
// not a working copy, or a missing svn client, yields an empty result rather
// than an error, so the viewer can offer the scope everywhere.
func WorkingCopyChanges(ctx context.Context, dir string, o Options, includeUnversioned bool) (changes []WorkChange, skipped int, err error) {
	if !Available(o) || !Describe(ctx, dir, o).IsSVNRepo {
		return nil, 0, nil
	}
	out, err := run(ctx, o, dir, "status", "--xml")
	if err != nil {
		// A working copy svn refuses to status is reported as unchanged rather
		// than failing the whole review.
		return nil, 0, nil
	}
	var doc statusXML
	if err := xml.Unmarshal([]byte(out), &doc); err != nil {
		return nil, 0, nil
	}

	unversioned := 0
	for _, target := range doc.Targets {
		for _, entry := range target.Entries {
			rel := relativeTo(dir, entry.Path)
			if rel == "" {
				continue
			}
			switch entry.WCStatus.Item {
			case "modified", "replaced", "conflicted":
				change, ok := versionedChange(ctx, dir, o, rel, "modified")
				if ok {
					changes = append(changes, change)
				}
			case "added":
				if after, ok := readWorkingFile(dir, rel, 0); ok {
					changes = append(changes, WorkChange{Path: rel, Status: "added", After: after})
				}
			case "deleted", "missing":
				change, ok := versionedChange(ctx, dir, o, rel, "deleted")
				if ok {
					changes = append(changes, change)
				}
			case "unversioned":
				// svn reports an unversioned directory as one entry, so the
				// files inside it have to be walked out or a whole new folder
				// would show up as a single unreadable row.
				for _, file := range unversionedFiles(dir, rel) {
					if !includeUnversioned {
						unversioned++
						continue
					}
					if unversioned >= maxUnversionedFiles {
						skipped++
						continue
					}
					after, ok := readWorkingFile(dir, file, maxUnversionedBytes)
					if !ok {
						skipped++
						continue
					}
					unversioned++
					changes = append(changes, WorkChange{Path: file, Status: "added", After: after})
				}
			}
		}
	}
	if !includeUnversioned {
		skipped = unversioned
	}

	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return changes, skipped, nil
}

// WorkingCopyChangeFor resolves one file, reading only it.
//
// The path comes from a request, so it is matched against what svn reports as
// changed before anything is read: an unlisted file stays unreadable through
// this route.
func WorkingCopyChangeFor(ctx context.Context, dir string, o Options, path string, includeUnversioned bool) (*WorkChange, error) {
	want := strings.TrimSpace(path)
	if want == "" {
		return nil, nil
	}
	// Statusing the whole copy is one invocation and reads no file content; the
	// expensive part is the pristine text, and only the wanted file gets that.
	changes, _, err := workingCopyEntries(ctx, dir, o, includeUnversioned)
	if err != nil {
		return nil, err
	}
	target := filepath.FromSlash(want)
	for _, rel := range changes {
		if filepath.FromSlash(rel.path) != target {
			continue
		}
		switch rel.status {
		case "added":
			var limit int64
			if rel.unversioned {
				limit = maxUnversionedBytes
			}
			after, ok := readWorkingFile(dir, rel.path, limit)
			if !ok {
				return nil, nil
			}
			return &WorkChange{Path: rel.path, Status: "added", After: after}, nil
		default:
			change, ok := versionedChange(ctx, dir, o, rel.path, rel.status)
			if !ok {
				return nil, nil
			}
			return &change, nil
		}
	}
	return nil, nil
}

// entry is one status line before any content is read.
type entry struct {
	path        string
	status      string
	unversioned bool
}

// workingCopyEntries lists what changed without reading pristine text.
func workingCopyEntries(ctx context.Context, dir string, o Options, includeUnversioned bool) ([]entry, int, error) {
	if !Available(o) || !Describe(ctx, dir, o).IsSVNRepo {
		return nil, 0, nil
	}
	out, err := run(ctx, o, dir, "status", "--xml")
	if err != nil {
		return nil, 0, nil
	}
	var doc statusXML
	if err := xml.Unmarshal([]byte(out), &doc); err != nil {
		return nil, 0, nil
	}
	var entries []entry
	skipped := 0
	for _, target := range doc.Targets {
		for _, e := range target.Entries {
			rel := relativeTo(dir, e.Path)
			if rel == "" {
				continue
			}
			switch e.WCStatus.Item {
			case "modified", "replaced", "conflicted":
				entries = append(entries, entry{path: rel, status: "modified"})
			case "added":
				entries = append(entries, entry{path: rel, status: "added"})
			case "deleted", "missing":
				entries = append(entries, entry{path: rel, status: "deleted"})
			case "unversioned":
				for _, file := range unversionedFiles(dir, rel) {
					if !includeUnversioned {
						skipped++
						continue
					}
					entries = append(entries, entry{path: file, status: "added", unversioned: true})
				}
			}
		}
	}
	return entries, skipped, nil
}

// unversionedFiles expands one unversioned status entry into the files it
// stands for: itself when it is a file, everything beneath it when it is a
// directory. The walk stops once it has produced more than the cap could use, so
// a stray build directory cannot turn one status line into a filesystem crawl.
func unversionedFiles(dir, rel string) []string {
	abs := filepath.Join(dir, filepath.FromSlash(rel))
	info, err := os.Stat(abs)
	if err != nil {
		return nil
	}
	if !info.IsDir() {
		if !info.Mode().IsRegular() {
			return nil
		}
		return []string{rel}
	}

	var out []string
	_ = filepath.WalkDir(abs, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if len(out) > maxUnversionedFiles {
			return filepath.SkipAll
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		child, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return nil
		}
		out = append(out, child)
		return nil
	})
	return out
}

// versionedChange loads both sides of a file svn already tracks.
func versionedChange(ctx context.Context, dir string, o Options, rel, status string) (WorkChange, bool) {
	change := WorkChange{Path: rel, Status: status}
	base, err := runRaw(ctx, o, dir, "cat", "--revision", "BASE", rel)
	if err != nil {
		// No pristine text to compare against; treat it as new rather than
		// failing the scope.
		change.Status = "added"
	} else {
		change.Before = normalizeEOL(base)
	}
	if status != "deleted" {
		if after, ok := readWorkingFile(dir, rel, 0); ok {
			change.After = after
		} else {
			change.Status = "deleted"
		}
	}
	return change, true
}

// readWorkingFile loads a file from the working copy. A limit of 0 means no cap.
func readWorkingFile(dir, rel string, limit int64) ([]byte, bool) {
	abs := filepath.Join(dir, filepath.FromSlash(rel))
	info, err := os.Stat(abs)
	if err != nil || !info.Mode().IsRegular() {
		return nil, false
	}
	if limit > 0 && info.Size() > limit {
		return nil, false
	}
	content, err := os.ReadFile(abs)
	if err != nil {
		return nil, false
	}
	return normalizeEOL(content), true
}

// relativeTo turns a status path into one relative to dir, dropping anything
// that escapes the working copy.
func relativeTo(dir, path string) string {
	p := strings.TrimSpace(path)
	if p == "" {
		return ""
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(dir, filepath.FromSlash(p))
	}
	rel, err := filepath.Rel(dir, p)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return ""
	}
	return rel
}

// normalizeEOL strips CR from CRLF pairs.
//
// svn stores pristine text with the repository's line endings while the working
// file carries the platform's, so comparing raw bytes would mark every line of a
// CRLF checkout as changed the moment one line is edited. Levelling both sides
// keeps the diff about content. The git side does the same.
func normalizeEOL(b []byte) []byte {
	if !bytes.Contains(b, crlfBytes) {
		return b
	}
	return bytes.ReplaceAll(b, crlfBytes, lfBytes)
}

var (
	crlfBytes = []byte("\r\n")
	lfBytes   = []byte("\n")
)
