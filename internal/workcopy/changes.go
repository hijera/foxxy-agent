// Package workcopy contains the file snapshots shared by working copy backends.
package workcopy

import "errors"

// Change contains the original and current bytes of one workspace-relative file.
type Change struct {
	Path   string
	Status string // "added" | "modified" | "deleted"
	Before []byte
	After  []byte
	Binary bool
}

// ErrNotChanged reports a path outside the reviewable change set.
var ErrNotChanged = errors.New("path is not among the working copy changes")

const (
	MaxNewFiles = 500
	MaxNewBytes = 2 << 20
)
