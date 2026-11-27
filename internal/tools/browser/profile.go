//go:build browser

package browser

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hijera/foxxycode-agent/internal/config"
)

func launchSessionBrowser(cfg *config.BrowserConfig, profileDir string) (*Browser, error) {
	var originalErr error
	if !profileInUse(profileDir) {
		var b *Browser
		b, originalErr = launch(cfg, profileDir)
		if originalErr == nil || profileDir == "" || !profileStartupRejected(originalErr) {
			return b, originalErr
		}
	}
	// A Chrome left behind by the previous backend can still own the
	// persistent profile. Keep it intact and use a disposable profile.
	dir, err := os.MkdirTemp("", "foxxycode-browser-recovery-")
	if err != nil {
		return nil, fmt.Errorf("create browser recovery profile: %w", err)
	}
	b, err := launch(cfg, dir)
	if err != nil {
		_ = os.RemoveAll(dir)
		if originalErr != nil {
			return nil, fmt.Errorf("original browser profile: %v; temporary profile: %w", originalErr, err)
		}
		return nil, fmt.Errorf("start browser with recovery profile: %w", err)
	}
	b.recoveryProfile = dir
	cdpLogf("could not use the session profile; Chrome started with a temporary profile")
	return b, nil
}

// Chrome can exit without a diagnostic when a previous process owns the
// profile. The holder may have stopped answering CDP, so a closed listener
// alone does not rule out that collision.
func profileStartupRejected(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "chrome failed to start:") || strings.Contains(msg, "websocket url timeout reached")
}

// A stale DevToolsActivePort file can survive a crash. Check its loopback
// listener before skipping the profile. The probe sends no DevTools commands.
func profileInUse(profileDir string) bool {
	if profileDir == "" {
		return false
	}
	data, err := os.ReadFile(filepath.Join(profileDir, "DevToolsActivePort"))
	if err != nil {
		return false
	}
	portText, endpoint, ok := strings.Cut(string(data), "\n")
	port, err := strconv.Atoi(strings.TrimSpace(portText))
	if !ok || err != nil || port < 1 || port > 65535 || !strings.HasPrefix(strings.TrimSpace(endpoint), "/devtools/browser/") {
		return false
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 250*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
