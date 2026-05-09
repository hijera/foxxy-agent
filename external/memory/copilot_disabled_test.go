package memory

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/EvilFreelancer/coddy-agent/internal/config"
)

func TestRunRecallWhenDisabledDoesNotHitStoreFiles(t *testing.T) {
	tmp := t.TempDir()
	cfg := &config.Config{
		Paths:  config.Paths{Home: tmp, CWD: tmp},
		Memory: config.MemoryConfig{Enabled: false},
	}
	cfg.Memory.ApplyDefaults()
	s, dur, paths, err := RunRecall(context.Background(), nil, cfg, filepath.Join(tmp, "w"), "hello world", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if dur != 0 {
		t.Fatalf("duration = %d want 0", dur)
	}
	if len(paths) != 0 {
		t.Fatalf("read paths = %v want empty", paths)
	}
	if s != "" {
		t.Fatalf("recall text = %q want empty", s)
	}
}

func TestRunPersistWhenDisabled(t *testing.T) {
	tmp := t.TempDir()
	cfg := &config.Config{
		Paths:  config.Paths{Home: tmp, CWD: tmp},
		Memory: config.MemoryConfig{Enabled: false},
	}
	cfg.Memory.ApplyDefaults()
	out, dur, err := RunPersist(context.Background(), nil, cfg, tmp, "", "u", "a", nil)
	if err != nil {
		t.Fatal(err)
	}
	if dur != 0 {
		t.Fatalf("duration = %d want 0", dur)
	}
	if out.Saved {
		t.Fatal("want not saved")
	}
}
