package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestConcurrentConfigBackupsRemainAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	const workers = 16
	start := make(chan struct{})
	errors := make(chan error, workers)
	var wg sync.WaitGroup
	for i := range workers {
		wg.Go(func() {
			<-start
			for range 8 {
				if err := WriteBackup(path, []byte(fmt.Sprintf("# writer %d\n", i))); err != nil {
					errors <- err
					return
				}
			}
		})
	}
	close(start)
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Errorf("concurrent backup: %v", err)
	}
	data, err := os.ReadFile(BackupPath(path))
	if err != nil {
		t.Fatal(err)
	}
	var writer int
	if n, err := fmt.Sscanf(string(data), "# writer %d\n", &writer); n != 1 || err != nil || writer < 0 || writer >= workers || strings.Count(string(data), "\n") != 1 {
		t.Fatalf("backup is not one complete writer's document: %q", data)
	}
}
