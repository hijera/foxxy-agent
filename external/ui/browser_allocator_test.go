//go:build http && ui && browser

package ui

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// How the presentation tests in this package launch Chrome, in one place.
//
// Nothing in the test matrix carried http, ui and browser at once until the
// full-tag line joined `make test`, so these tests had only ever run on a
// developer's machine. Their first run on a CI runner found two things a
// desktop never shows, and both are environment, not page behaviour:
//
//   - The runner image restricts unprivileged user namespaces through AppArmor,
//     and Chrome refuses to start at all without a usable sandbox ("No usable
//     sandbox!", a FATAL before any page loads). `--no-sandbox` is the
//     workaround Chrome's own message names. What these tests open is a fixture
//     page from a local dev server, so the sandbox is guarding nothing here.
//
//   - chromedp waits browserWSURLTimeout for the DevTools websocket URL, and
//     its default of 20 s is not always enough on a two-core runner starting
//     several browsers at once. Past it the failure reads "websocket url
//     timeout reached", which looks like a hang rather than a slow start.
//
// The panel tests in external/httpserver build their own options for the same
// reasons; keep the two in step.
const browserWSURLTimeout = 60 * time.Second

// headlessAllocatorOptions returns the allocator options for a presentation
// test. userDataDir and browserLog are optional: pass a fresh directory to keep
// one test's profile out of another's, and a buffer to have Chrome's own output
// in the failure message. FOXXYCODE_UI_BROWSER overrides the executable.
func headlessAllocatorOptions(userDataDir string, browserLog io.Writer) []chromedp.ExecAllocatorOption {
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts,
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-gpu-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.WSURLReadTimeout(browserWSURLTimeout),
	)
	if strings.TrimSpace(userDataDir) != "" {
		opts = append(opts, chromedp.UserDataDir(userDataDir))
	}
	if browserLog != nil {
		opts = append(opts, chromedp.CombinedOutput(browserLog))
	}
	if executable := strings.TrimSpace(os.Getenv("FOXXYCODE_UI_BROWSER")); executable != "" {
		opts = append(opts, chromedp.ExecPath(executable))
	}
	return opts
}
