//go:build windows

package session

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestListSnapshotsReportsLockedMetadataInsteadOfDroppingHistory(t *testing.T) {
	for _, child := range []bool{false, true} {
		name := "parent"
		if child {
			name = "child"
		}
		t.Run(name, func(t *testing.T) {
			store := &FileStore{Root: t.TempDir()}
			parentID, childID := "sess_locked_parent", "sess_locked_child"
			parentDir, err := store.EnsureLayout(parentID)
			if err != nil {
				t.Fatal(err)
			}
			childDir, err := store.EnsureChildLayout(parentID, childID)
			if err != nil {
				t.Fatal(err)
			}
			for id, dir := range map[string]string{parentID: parentDir, childID: childDir} {
				if err := os.WriteFile(filepath.Join(dir, sessionMetaFile), []byte(`{"version":1,"id":"`+id+`","cwd":"C:/plain-folder"}`), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			lockedDir := parentDir
			if child {
				lockedDir = childDir
			}
			path, err := syscall.UTF16PtrFromString(filepath.Join(lockedDir, sessionMetaFile))
			if err != nil {
				t.Fatal(err)
			}
			handle, err := syscall.CreateFile(path, syscall.GENERIC_READ, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if handle != syscall.InvalidHandle {
					if err := syscall.CloseHandle(handle); err != nil {
						t.Error(err)
					}
				}
			}()
			opts := ListOptions{IncludeSubagents: true}
			rows, err := store.ListSnapshotsWith(opts)
			if !isRetryableFileLockError(err) || rows != nil {
				t.Fatalf("locked metadata produced a successful partial history: rows=%+v err=%v", rows, err)
			}
			if err := syscall.CloseHandle(handle); err != nil {
				t.Fatal(err)
			}
			handle = syscall.InvalidHandle
			rows, err = store.ListSnapshotsWith(opts)
			if err != nil || len(rows) != 2 {
				t.Fatalf("history after releasing the lock: rows=%+v err=%v", rows, err)
			}
		})
	}
}
