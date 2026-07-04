package skills_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/skills"
)

func TestReadDisabledMissingFile(t *testing.T) {
	tmp := t.TempDir()
	got := skills.ReadDisabled(tmp)
	if len(got) != 0 {
		t.Fatalf("expected empty map, got %v", got)
	}
}

func TestDisableAndEnable(t *testing.T) {
	tmp := t.TempDir()

	if err := skills.DisableSkill(tmp, "alpha"); err != nil {
		t.Fatalf("DisableSkill: %v", err)
	}
	if err := skills.DisableSkill(tmp, "beta"); err != nil {
		t.Fatalf("DisableSkill: %v", err)
	}

	got := skills.ReadDisabled(tmp)
	if !skills.IsDisabled(got, "alpha") {
		t.Error("expected alpha disabled")
	}
	if !skills.IsDisabled(got, "beta") {
		t.Error("expected beta disabled")
	}
	if skills.IsDisabled(got, "gamma") {
		t.Error("gamma should not be disabled")
	}

	if err := skills.EnableSkill(tmp, "alpha"); err != nil {
		t.Fatalf("EnableSkill: %v", err)
	}

	got2 := skills.ReadDisabled(tmp)
	if skills.IsDisabled(got2, "alpha") {
		t.Error("alpha should be enabled now")
	}
	if !skills.IsDisabled(got2, "beta") {
		t.Error("beta should still be disabled")
	}
}

func TestDisableIdempotent(t *testing.T) {
	tmp := t.TempDir()
	for range 3 {
		if err := skills.DisableSkill(tmp, "alpha"); err != nil {
			t.Fatalf("DisableSkill: %v", err)
		}
	}
	got := skills.ReadDisabled(tmp)
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 entry, got %d", len(got))
	}
}

func TestEnableIdempotent(t *testing.T) {
	tmp := t.TempDir()
	for range 3 {
		if err := skills.EnableSkill(tmp, "missing"); err != nil {
			t.Fatalf("EnableSkill on unknown name: %v", err)
		}
	}
}

func TestReadDisabledIgnoresComments(t *testing.T) {
	tmp := t.TempDir()
	content := "# this is a comment\nalpha\n# another comment\nbeta\n"
	if err := os.WriteFile(filepath.Join(tmp, ".disabled"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got := skills.ReadDisabled(tmp)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d: %v", len(got), got)
	}
	if !skills.IsDisabled(got, "alpha") || !skills.IsDisabled(got, "beta") {
		t.Error("expected alpha and beta to be disabled")
	}
}

func TestIsDisabledNilMap(t *testing.T) {
	if skills.IsDisabled(nil, "anything") {
		t.Error("nil map should report not-disabled")
	}
}
