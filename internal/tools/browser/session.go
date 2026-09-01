//go:build browser

// Package browser provides an interactive browser-automation tool set (navigate,
// click, fill, hover, scroll, screenshot, evaluate, close) backed by a local
// Chrome/Chromium instance driven over the DevTools Protocol via chromedp.
//
// It is gated behind the "browser" build tag and disabled by default; enable it
// with config browser.enabled: true in a build compiled with -tags browser.
package browser

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"

	"github.com/hijera/foxxycode-agent/internal/config"
)

// Manager owns one Browser per agent session, keyed by session id. A single
// Manager instance is shared by all browser tools (see register.go).
type Manager struct {
	cfg *config.BrowserConfig

	mu       sync.Mutex
	browsers map[string]*Browser
}

// NewManager returns a Manager bound to the given browser config.
func NewManager(cfg *config.BrowserConfig) *Manager {
	return &Manager{cfg: cfg, browsers: make(map[string]*Browser)}
}

// get returns the Browser for the session, lazily launching Chrome on first use.
func (m *Manager) get(sessionID, profileDir string) (*Browser, error) {
	key := strings.TrimSpace(sessionID)
	if key == "" {
		key = "_default"
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if b, ok := m.browsers[key]; ok && b != nil {
		return b, nil
	}
	b, err := launch(m.cfg, profileDir)
	if err != nil {
		return nil, err
	}
	m.browsers[key] = b
	return b, nil
}

// closeSession tears down and forgets the Browser for the session, if any.
func (m *Manager) closeSession(sessionID string) {
	key := strings.TrimSpace(sessionID)
	if key == "" {
		key = "_default"
	}
	m.mu.Lock()
	b := m.browsers[key]
	delete(m.browsers, key)
	m.mu.Unlock()
	if b != nil {
		b.close()
	}
}

// Browser wraps a live chromedp context for a single session.
type Browser struct {
	allocCtx    context.Context
	allocCancel context.CancelFunc
	ctx         context.Context
	ctxCancel   context.CancelFunc
	timeout     time.Duration
	// captureScreens mirrors browser.screenshots. When false no image is taken and
	// none is handed to the model; every action still reports URL and page log.
	captureScreens bool

	mu sync.Mutex
	// pageLog buffers everything the page told us since it was last read: console
	// calls, uncaught exceptions and failed or error-status network responses.
	// One buffer, because they answer the same question - what went wrong on the
	// page - and a caller reading it wants all three.
	pageLog []string
}

// launch starts a headless (or headful) Chrome and returns a ready Browser.
func launch(cfg *config.BrowserConfig, profileDir string) (*Browser, error) {
	headless := true
	timeout := time.Duration(config.BrowserDefaultTimeoutSeconds) * time.Second
	execPath := ""
	if cfg != nil {
		headless = cfg.HeadlessEnabled()
		timeout = time.Duration(cfg.ResolvedTimeoutSeconds()) * time.Second
		execPath = strings.TrimSpace(cfg.ExecutablePath)
	}

	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, chromedp.Flag("headless", headless))
	if profileDir != "" {
		if err := os.MkdirAll(profileDir, 0o755); err == nil {
			opts = append(opts, chromedp.UserDataDir(profileDir))
		}
	}
	if execPath != "" {
		opts = append(opts, chromedp.ExecPath(execPath))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	// Route chromedp's own diagnostics into the configured log; see cdpLogf.
	ctx, ctxCancel := chromedp.NewContext(allocCtx,
		chromedp.WithErrorf(cdpLogf),
		chromedp.WithLogf(cdpLogf),
	)

	b := &Browser{
		allocCtx:       allocCtx,
		allocCancel:    allocCancel,
		ctx:            ctx,
		ctxCancel:      ctxCancel,
		timeout:        timeout,
		captureScreens: cfg.ScreenshotsEnabled(),
	}

	// Force the browser process to start now so failures surface immediately. This
	// first Run MUST use the long-lived ctx: chromedp binds the browser/tab lifetime
	// to the context of the first Run, so a short-lived (timeout) context here would
	// tear the browser down as soon as it is cancelled. It also creates the target
	// that ListenTarget below attaches to, and enables the Network and Runtime
	// domains: without them Chrome emits no response, failure or console events at
	// all, and the listener sees nothing however correct it looks.
	if err := chromedp.Run(ctx, network.Enable(), runtime.Enable()); err != nil {
		ctxCancel()
		allocCancel()
		return nil, fmt.Errorf("launch browser: %w", err)
	}

	// Capture what the page reports about itself so the tools can surface it without
	// needing a screenshot. Network failures matter as much as console lines: a
	// request that 500s shows up in a screenshot only if the app happens to render
	// an error, so without this a broken backend looks like a blank page.
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch e := ev.(type) {
		case *runtime.EventConsoleAPICalled:
			b.addPageLog(formatConsole(e))
		case *runtime.EventExceptionThrown:
			if e.ExceptionDetails != nil {
				b.addPageLog("[exception] " + formatException(e.ExceptionDetails))
			}
		case *network.EventLoadingFailed:
			// ERR_ABORTED trails a response that was already reported (a rejected
			// fetch, a navigation replaced mid-flight), so reporting it too would
			// double every failure the caller can already see.
			if !e.Canceled && e.ErrorText != "net::ERR_ABORTED" {
				b.addPageLog(fmt.Sprintf("[network] failed %s: %s", e.Type, e.ErrorText))
			}
		case *network.EventResponseReceived:
			if e.Response != nil && e.Response.Status >= 400 {
				b.addPageLog(fmt.Sprintf("[network] %d %s", int(e.Response.Status), e.Response.URL))
			}
		}
	})
	return b, nil
}

// run executes chromedp actions under the per-action timeout.
func (b *Browser) run(actions ...chromedp.Action) error {
	ctx, cancel := context.WithTimeout(b.ctx, b.timeout)
	defer cancel()
	return chromedp.Run(ctx, actions...)
}

// close tears the browser down.
func (b *Browser) close() {
	if b.ctxCancel != nil {
		b.ctxCancel()
	}
	if b.allocCancel != nil {
		b.allocCancel()
	}
}

func (b *Browser) addPageLog(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pageLog = append(b.pageLog, line)
	// Bound memory: keep the most recent entries.
	const maxPageLog = 200
	if len(b.pageLog) > maxPageLog {
		b.pageLog = b.pageLog[len(b.pageLog)-maxPageLog:]
	}
}

// drainPageLog returns and clears what the page reported since the last read.
// Whoever reads first gets the lines - an action result or the page-log tool.
func (b *Browser) drainPageLog() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.pageLog) == 0 {
		return nil
	}
	out := b.pageLog
	b.pageLog = nil
	return out
}

// formatException renders a thrown exception the way a developer would read it.
// ExceptionDetails.Text alone is almost always the bare word "Uncaught", which
// tells the model nothing; the message and stack live on the exception object.
func formatException(d *runtime.ExceptionDetails) string {
	if d == nil {
		return "unknown"
	}
	if d.Exception != nil {
		if desc := strings.TrimSpace(d.Exception.Description); desc != "" {
			// The description carries the stack too; the first line is the message.
			if line, _, ok := strings.Cut(desc, "\n"); ok {
				return line
			}
			return desc
		}
	}
	if text := strings.TrimSpace(d.Text); text != "" {
		return text
	}
	return "unknown"
}

func formatConsole(e *runtime.EventConsoleAPICalled) string {
	parts := make([]string, 0, len(e.Args))
	for _, a := range e.Args {
		if a == nil {
			continue
		}
		if len(a.Value) > 0 {
			parts = append(parts, strings.Trim(string(a.Value), `"`))
			continue
		}
		if a.Description != "" {
			parts = append(parts, a.Description)
		}
	}
	return fmt.Sprintf("[%s] %s", e.Type, strings.Join(parts, " "))
}

// profileDirFor returns the persistent user-data directory for a session.
func profileDirFor(sessionDir string) string {
	sessionDir = strings.TrimSpace(sessionDir)
	if sessionDir == "" {
		return ""
	}
	return filepath.Join(sessionDir, "browser-profile")
}
