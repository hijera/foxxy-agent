//go:build memory

package memory

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/config"
)

func TestRunBeforeTurnWhenDisabled(t *testing.T) {
	tmp := t.TempDir()
	cfg := &config.Config{}
	cfg.Memory.Enabled = false
	cfg.Memory.ApplyDefaults()

	out, dur, err := RunBeforeTurn(context.Background(), nil, cfg, filepath.Join(tmp, "w"), "hello world", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if dur != 0 {
		t.Fatalf("duration = %d want 0", dur)
	}
	if out.ContextText != "" {
		t.Fatalf("context = %q want empty", out.ContextText)
	}
}
