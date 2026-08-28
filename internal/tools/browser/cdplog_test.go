//go:build browser

package browser

import (
	"bytes"
	"log"
	"log/slog"
	"strings"
	"testing"
)

// TestCDPLogfKeepsChromedpOffStderr pins the reason this seam exists: chromedp
// defaults its error channel to log.Printf, and a pinned cdproto that does not
// know a newer CDP enum value ("unknown IPAddressSpace value: Loopback") turns
// every page load into dozens of stderr lines. That noise lands in the one-shot
// console output and in the desktop log file, so it has to go through slog.
func TestCDPLogfKeepsChromedpOffStderr(t *testing.T) {
	var stdLog bytes.Buffer
	prevWriter := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(&stdLog)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
	})

	var routed bytes.Buffer
	prevDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&routed, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prevDefault) })

	cdpLogf("could not unmarshal event: unknown IPAddressSpace value: %s", "Loopback")

	if stdLog.Len() != 0 {
		t.Errorf("chromedp output reached the standard logger (stderr): %q", stdLog.String())
	}
	if !strings.Contains(routed.String(), "Loopback") {
		t.Errorf("message did not reach slog: %q", routed.String())
	}
	if !strings.Contains(routed.String(), "level=DEBUG") {
		t.Errorf("CDP protocol noise must stay at debug level, got: %q", routed.String())
	}
}
