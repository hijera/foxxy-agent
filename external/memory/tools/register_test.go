//go:build memory

package memtools

import (
	"testing"

	memstorage "github.com/hijera/foxxycode-agent/external/memory/storage"
	"github.com/hijera/foxxycode-agent/internal/config"
)

func testMemoryStore(t *testing.T) (*memstorage.Store, *config.MemoryConfig) {
	t.Helper()
	st := memstorage.NewWithRoots(t.TempDir(), t.TempDir())
	m := &config.MemoryConfig{}
	m.ApplyDefaults()
	return st, m
}

func TestRecallToolDefinitionsReadOnly(t *testing.T) {
	st, mem := testMemoryStore(t)
	defs := RecallToolDefinitions(st, mem)
	want := map[string]bool{NameSearch: true, NameRead: true, NameList: true}
	for _, d := range defs {
		if !want[d.Name] {
			t.Fatalf("unexpected recall tool %q", d.Name)
		}
		delete(want, d.Name)
	}
	if len(defs) != 3 || len(want) != 0 {
		t.Fatalf("expected exactly three recall tools, got %d missing %v", len(defs), want)
	}
}

func TestToolDefinitionsShape(t *testing.T) {
	st, mem := testMemoryStore(t)
	defs := PersistToolDefinitions(st, mem)
	if len(defs) != 6 {
		t.Fatalf("want 6 tools, got %d", len(defs))
	}
	seen := make(map[string]bool)
	for _, d := range defs {
		if d.Name == "" || d.InputSchema == nil {
			t.Fatalf("bad def %#v", d)
		}
		seen[d.Name] = true
	}
	for _, n := range []string{NameSearch, NameList, NameRead, NameMkdir, NameSave, NameDelete} {
		if !seen[n] {
			t.Fatalf("missing %q", n)
		}
	}
}
