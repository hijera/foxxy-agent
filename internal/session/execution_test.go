package session

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestExecutionObservationsSurviveReloadAndResetOnNewResult(t *testing.T) {
	dir := t.TempDir()
	cp := &ExecutionCheckpoint{}
	a := ObservationHash("read", `{"path":"a","offset":1}`, "unchanged")
	b := ObservationHash("read", `{"offset":1,"path":"a"}`, "unchanged")
	if a != b {
		t.Fatal("JSON ordering changed fingerprint")
	}
	for _, v := range []string{a, "second", a, "second"} {
		cp.Observe(v)
	}
	if err := cp.Save(dir); err != nil {
		t.Fatal(err)
	}
	cp, err := ReadExecutionCheckpoint(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cp.Repeats != 2 {
		t.Fatalf("lost repeat count: %+v", cp)
	}
	cp.Observe(ObservationHash("read", `{"path":"a","offset":1}`, "changed"))
	if cp.Repeats != 0 {
		t.Fatal("new result did not reset stall count")
	}
}

func TestResponseClaimIsExclusiveAcrossStores(t *testing.T) {
	root := t.TempDir()
	var wg sync.WaitGroup
	var claims atomic.Int32
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fs := &FileStore{Root: root}
			_, claimed, _ := fs.ClaimResponseRequest("key", "fingerprint", "")
			if claimed {
				claims.Add(1)
			}
		}()
	}
	wg.Wait()
	if claims.Load() != 1 {
		t.Fatalf("claimed %d times", claims.Load())
	}
}
