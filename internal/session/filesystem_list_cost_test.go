package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Listing sessions is on the critical path of the very first paint in the editor
// panel, so it must stay a scan of session.json headers - never a read of every
// stored transcript. The budget is loose on purpose; the failure it guards against
// is orders of magnitude, not milliseconds.
func TestListSnapshotsDoesNotReadTranscripts(t *testing.T) {
	root := t.TempDir()
	store := &FileStore{Root: root}
	filler := make([]byte, 0, 24_000)
	for len(filler) < 24_000 {
		filler = append(filler, "lorem ipsum dolor sit amet, consectetur adipiscing elit. "...)
	}
	const sessions, messagesPerSession = 12, 200
	for i := 0; i < sessions; i++ {
		id := fmt.Sprintf("sess_bulk_%02d", i)
		dir := filepath.Join(root, id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		meta, err := json.Marshal(SessionMeta{
			Version:   sessionFileLayout,
			ID:        id,
			CWD:       root,
			Title:     fmt.Sprintf("bulk chat %02d", i),
			UpdatedAt: "2026-07-01T10:00:00Z",
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, sessionMetaFile), meta, 0o644); err != nil {
			t.Fatal(err)
		}
		msgs := make([]map[string]string, 0, messagesPerSession)
		for j := 0; j < messagesPerSession; j++ {
			role := "assistant"
			if j%2 == 0 {
				role = "user"
			}
			msgs = append(msgs, map[string]string{"role": role, "content": string(filler)})
		}
		payload, err := json.Marshal(map[string]interface{}{"version": 1, "messages": msgs})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, messagesFile), payload, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Warm the page cache so the number measures parsing, not first-touch disk reads.
	if _, err := store.ListSnapshots("", false); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	rows, err := store.ListSnapshots("", false)
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if len(rows) != sessions {
		t.Fatalf("listed %d sessions, want %d", len(rows), sessions)
	}
	if rows[0].Title == "" {
		t.Fatal("list rows lost their titles")
	}
	t.Logf("listed %d sessions holding ~%d MB of transcripts in %s",
		sessions, sessions*messagesPerSession*len(filler)/(1<<20), elapsed)
	if elapsed > 50*time.Millisecond {
		t.Fatalf("listing took %s; it is reading transcripts instead of session headers", elapsed)
	}
}
