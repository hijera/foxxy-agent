package session_test

import (
	"errors"
	"log/slog"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/session"
)

// An operator's command-line decisions (--swarm-allow-insecure, the pairing
// token in the environment) are not in config.yaml, so a configuration that
// reaches the manager from a settings save, from config_commit or from a skill
// change must carry them too. ReplaceConfigChecked applies the adjuster the
// process installed before the configuration is published, and refuses it
// when the adjusted document is invalid, leaving the live one in place.
func TestReplaceConfigCheckedAppliesTheAdjuster(t *testing.T) {
	cfg := testConfig()
	m := session.NewManager(cfg, noopSender{}, noopRunner, slog.Default(), "/tmp", nil)
	m.SetConfigAdjuster(func(c *config.Config) error {
		c.Swarm.AllowInsecure = true
		return nil
	})

	next := testConfig()
	next.Swarm.AllowInsecure = false
	if err := m.ReplaceConfigChecked(next); err != nil {
		t.Fatalf("ReplaceConfigChecked: %v", err)
	}
	if !m.Cfg().Swarm.AllowInsecure {
		t.Fatal("the saved configuration lost the process override (swarm allow_insecure)")
	}
}

func TestReplaceConfigCheckedRefusesAnInvalidAdjustment(t *testing.T) {
	cfg := testConfig()
	m := session.NewManager(cfg, noopSender{}, noopRunner, slog.Default(), "/tmp", nil)
	m.SetConfigAdjuster(func(*config.Config) error { return errors.New("bad override") })

	next := testConfig()
	next.Logger.Level = "debug"
	if err := m.ReplaceConfigChecked(next); err == nil {
		t.Fatal("an adjuster error must be returned to the caller")
	}
	if m.Cfg() == next {
		t.Fatal("a configuration the adjuster refused was published anyway")
	}
}

// Without an adjuster the manager publishes the document as given, which is
// what a bare `foxxycode http` or a test expects.
func TestReplaceConfigCheckedWithoutAdjusterPublishesAsGiven(t *testing.T) {
	m := session.NewManager(testConfig(), noopSender{}, noopRunner, slog.Default(), "/tmp", nil)
	next := testConfig()
	next.Swarm.AllowInsecure = true
	if err := m.ReplaceConfigChecked(next); err != nil {
		t.Fatalf("ReplaceConfigChecked: %v", err)
	}
	if !m.Cfg().Swarm.AllowInsecure {
		t.Fatal("the document given was not published")
	}
}
