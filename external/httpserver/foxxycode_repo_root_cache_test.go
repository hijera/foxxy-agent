//go:build http

package httpserver

import (
	"testing"
	"time"
)

// sessionRepoRoot memoizes the repo root per cwd so History polling does not
// spawn git for every row; a serve that runs for weeks must not keep one
// entry per cwd it ever saw.
func TestRepoRootCacheEvictsExpiredEntries(t *testing.T) {
	repoRootCache.Store("stale-cwd", repoRootCacheEntry{root: "/stale", expires: time.Now().Add(-time.Second)})
	repoRootCache.Store("live-cwd", repoRootCacheEntry{root: "/live", expires: time.Now().Add(time.Minute)})
	miss := t.TempDir()
	t.Cleanup(func() {
		repoRootCache.Delete("stale-cwd")
		repoRootCache.Delete("live-cwd")
		repoRootCache.Delete(miss)
	})

	sessionRepoRoot(miss) // a miss refreshes the cache and sweeps expired entries

	if _, ok := repoRootCache.Load("stale-cwd"); ok {
		t.Fatal("an expired entry outlived the sweep")
	}
	if _, ok := repoRootCache.Load("live-cwd"); !ok {
		t.Fatal("the sweep dropped a live entry")
	}
}
