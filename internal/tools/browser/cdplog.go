//go:build browser

package browser

import (
	"fmt"
	"log/slog"
)

// cdpLogf receives chromedp's own diagnostics. Left at its default, chromedp
// writes them with log.Printf — straight to stderr, ahead of everything the
// process itself logs. The volume is not hypothetical: cdproto is pinned to a
// snapshot that predates CDP enum values current Chrome sends, so a single page
// load prints dozens of "could not unmarshal event: unknown IPAddressSpace value:
// Loopback" lines. They are harmless (the event is still processed), which is
// exactly why they belong at debug level in the configured log rather than in the
// user's console output or the desktop log file.
func cdpLogf(format string, args ...interface{}) {
	slog.Default().Debug("chromedp: " + fmt.Sprintf(format, args...))
}
