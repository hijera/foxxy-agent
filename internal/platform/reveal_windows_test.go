//go:build windows

package platform

import (
	"path/filepath"
	"testing"
)

func TestRevealFileMissingSourceReturnsShellError(t *testing.T) {
	if err := RevealFile(filepath.Join(t.TempDir(), "missing source.txt")); err == nil {
		t.Fatal("a missing Shell item must not report a successful reveal")
	}
}
