//go:build windows

package skills

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestReplaceSkillDirWaitsForWindowsReader(t *testing.T) {
	managed := t.TempDir()
	staged := stagingDir(managed, "demo")
	writeSkill(t, staged, "demo")
	path, err := windows.UTF16PtrFromString(staged)
	if err != nil {
		t.Fatal(err)
	}
	// Directory readers and scanners can share reads/writes without sharing
	// delete, temporarily denying the atomic directory rename on Windows.
	handle, err := windows.CreateFile(path, windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		t.Fatal(err)
	}
	released := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = windows.CloseHandle(handle)
		close(released)
	}()
	defer func() { <-released }()
	if err := replaceSkillDir(managed, "demo", staged); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(managed, "demo", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}
