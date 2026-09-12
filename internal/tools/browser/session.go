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
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/cdproto/target"
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

// tab is one attached page target. The first tab doubles as the browser handle:
// its context must never be cancelled while the browser lives, because every
// later tab context is created from it.
type tab struct {
	id     target.ID
	ctx    context.Context
	cancel context.CancelFunc
	root   bool
}

// dialogResponse is an armed one-shot answer for the next JavaScript dialog.
type dialogResponse struct {
	accept     bool
	promptText string
}

// routeMock serves a canned response for every request whose URL contains substr.
type routeMock struct {
	substr      string
	status      int
	contentType string
	body        string
}

// netEntry is one observed network request. Entries are updated in place as the
// request/response/finished events arrive.
type netEntry struct {
	id     network.RequestID
	method string
	url    string
	rtype  string
	status int64
	mime   string
	size   float64
}

// netLogMax bounds the request journal; the newest entries win.
const netLogMax = 300

// Browser wraps a live chromedp browser for a single session. It owns one or
// more tabs; tool actions run against the current tab.
type Browser struct {
	allocCtx    context.Context
	allocCancel context.CancelFunc
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

	tabs      map[target.ID]*tab
	currentID target.ID
	rootCtx   context.Context
	rootStop  context.CancelFunc

	dialogNext *dialogResponse
	netLog     []*netEntry
	mocks      []routeMock
	fetchOn    bool
	offline    bool
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
	// Repeat chromedp's default disable-features (a later flag of the same name
	// replaces, not extends) and add BackForwardCache: a bfcache-restored page
	// fires no load event, which leaves NavigateBack hanging until its timeout.
	opts = append(opts, chromedp.Flag("disable-features", "site-per-process,Translate,BlinkGenPropertyTrees,BackForwardCache"))
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
		timeout:        timeout,
		captureScreens: cfg.ScreenshotsEnabled(),
		tabs:           make(map[target.ID]*tab),
		rootCtx:        ctx,
		rootStop:       ctxCancel,
	}

	// Capture page events before the first Run so nothing emitted during startup
	// is lost. See attachListeners for what is collected and why.
	b.attachListeners(ctx)

	// Force the browser process to start now so failures surface immediately. This
	// first Run MUST use the long-lived ctx: chromedp binds the browser/tab lifetime
	// to the context of the first Run, so a short-lived (timeout) context here would
	// tear the browser down as soon as it is cancelled. It also creates the target
	// that ListenTarget above attaches to, and enables the Network and Runtime
	// domains: without them Chrome emits no response, failure or console events at
	// all, and the listener sees nothing however correct it looks.
	if err := chromedp.Run(ctx, network.Enable(), runtime.Enable()); err != nil {
		ctxCancel()
		allocCancel()
		return nil, fmt.Errorf("launch browser: %w", err)
	}

	id := chromedp.FromContext(ctx).Target.TargetID
	b.mu.Lock()
	b.tabs[id] = &tab{id: id, ctx: ctx, cancel: ctxCancel, root: true}
	b.currentID = id
	b.mu.Unlock()
	return b, nil
}

// attachListeners captures what the page reports about itself so the tools can
// surface it without needing a screenshot, journals every network request, and
// resolves JavaScript dialogs and paused (mocked) requests. Network failures
// matter as much as console lines: a request that 500s shows up in a screenshot
// only if the app happens to render an error, so without this a broken backend
// looks like a blank page. Called once per tab context.
func (b *Browser) attachListeners(ctx context.Context) {
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch e := ev.(type) {
		case *runtime.EventConsoleAPICalled:
			b.addPageLog(formatConsole(e))
		case *runtime.EventExceptionThrown:
			if e.ExceptionDetails != nil {
				b.addPageLog("[exception] " + formatException(e.ExceptionDetails))
			}
		case *network.EventRequestWillBeSent:
			b.recordRequest(e)
		case *network.EventResponseReceived:
			b.recordResponse(e)
		case *network.EventLoadingFinished:
			b.recordFinished(e)
		case *network.EventLoadingFailed:
			// ERR_ABORTED trails a response that was already reported (a rejected
			// fetch, a navigation replaced mid-flight), so reporting it too would
			// double every failure the caller can already see.
			if !e.Canceled && e.ErrorText != "net::ERR_ABORTED" {
				b.addPageLog(fmt.Sprintf("[network] failed %s: %s", e.Type, e.ErrorText))
			}
		case *page.EventJavascriptDialogOpening:
			// The event handler must not block (it runs on the message loop), and
			// answering the dialog is itself a CDP command; resolve it aside.
			go b.resolveDialog(ctx, e)
		case *fetch.EventRequestPaused:
			go b.resolvePaused(ctx, e)
		}
	})
}

// run executes chromedp actions on the current tab under the per-action timeout.
func (b *Browser) run(actions ...chromedp.Action) error {
	ctx, cancel := context.WithTimeout(b.currentCtx(), b.timeout)
	defer cancel()
	return chromedp.Run(ctx, actions...)
}

// currentCtx returns the current tab's context, falling back to any live tab
// (the current one may have been closed from inside the page).
func (b *Browser) currentCtx() context.Context {
	b.mu.Lock()
	defer b.mu.Unlock()
	if t, ok := b.tabs[b.currentID]; ok {
		return t.ctx
	}
	for _, t := range b.tabs {
		b.currentID = t.id
		return t.ctx
	}
	return b.rootCtx
}

// forEachTab runs f against every attached tab's context (snapshot; f runs
// outside the lock). Used to apply browser-wide switches: fetch interception,
// offline emulation.
func (b *Browser) forEachTab(f func(ctx context.Context) error) error {
	b.mu.Lock()
	ctxs := make([]context.Context, 0, len(b.tabs))
	for _, t := range b.tabs {
		ctxs = append(ctxs, t.ctx)
	}
	b.mu.Unlock()
	var firstErr error
	for _, ctx := range ctxs {
		tctx, cancel := context.WithTimeout(ctx, b.timeout)
		err := chromedp.Run(tctx, chromedp.ActionFunc(func(c context.Context) error { return f(c) }))
		cancel()
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// close tears the browser down.
func (b *Browser) close() {
	b.mu.Lock()
	tabs := b.tabs
	b.tabs = make(map[target.ID]*tab)
	b.mu.Unlock()
	for _, t := range tabs {
		if !t.root {
			t.cancel()
		}
	}
	if b.rootStop != nil {
		b.rootStop()
	}
	if b.allocCancel != nil {
		b.allocCancel()
	}
}

// listPages returns the open page targets in a stable order, so the index a
// caller saw in one listing still names the same tab in the next call.
func (b *Browser) listPages() ([]*target.Info, error) {
	infos, err := chromedp.Targets(b.rootCtx)
	if err != nil {
		return nil, err
	}
	pages := make([]*target.Info, 0, len(infos))
	for _, info := range infos {
		if info.Type == "page" {
			pages = append(pages, info)
		}
	}
	sort.Slice(pages, func(i, j int) bool { return pages[i].TargetID < pages[j].TargetID })
	return pages, nil
}

// ensureTab attaches to an existing page target (e.g. one the page itself opened
// with target=_blank) and returns its context, creating the attachment on first
// use.
func (b *Browser) ensureTab(id target.ID) (context.Context, error) {
	b.mu.Lock()
	if t, ok := b.tabs[id]; ok {
		b.mu.Unlock()
		return t.ctx, nil
	}
	b.mu.Unlock()

	ctx, cancel := chromedp.NewContext(b.rootCtx, chromedp.WithTargetID(id))
	b.attachListeners(ctx)
	if err := b.initTab(ctx); err != nil {
		cancel()
		return nil, err
	}
	b.mu.Lock()
	b.tabs[id] = &tab{id: id, ctx: ctx, cancel: cancel}
	b.mu.Unlock()
	return ctx, nil
}

// newTab opens a fresh tab and returns its target id. The caller decides whether
// to make it current.
func (b *Browser) newTab() (target.ID, error) {
	ctx, cancel := chromedp.NewContext(b.rootCtx)
	b.attachListeners(ctx)
	if err := b.initTab(ctx); err != nil {
		cancel()
		return "", err
	}
	id := chromedp.FromContext(ctx).Target.TargetID
	b.mu.Lock()
	b.tabs[id] = &tab{id: id, ctx: ctx, cancel: cancel}
	b.mu.Unlock()
	return id, nil
}

// initTab enables the event domains a tab needs and re-applies the browser-wide
// switches (request interception, offline) so a new tab behaves like the rest.
func (b *Browser) initTab(ctx context.Context) error {
	// Like the launch Run, this first Run must use the long-lived context.
	if err := chromedp.Run(ctx, network.Enable(), runtime.Enable()); err != nil {
		return fmt.Errorf("open tab: %w", err)
	}
	b.mu.Lock()
	fetchOn, offline := b.fetchOn, b.offline
	b.mu.Unlock()
	if fetchOn {
		if err := chromedp.Run(ctx, fetch.Enable()); err != nil {
			return fmt.Errorf("enable interception on tab: %w", err)
		}
	}
	if offline {
		if err := chromedp.Run(ctx, network.EmulateNetworkConditions(true, 0, -1, -1)); err != nil {
			return fmt.Errorf("apply offline to tab: %w", err)
		}
	}
	return nil
}

// closeTab closes a page target. The root tab's context is never cancelled (it
// is the parent of every other tab context); its target is closed directly.
func (b *Browser) closeTab(id target.ID) error {
	b.mu.Lock()
	t := b.tabs[id]
	delete(b.tabs, id)
	if b.currentID == id {
		b.currentID = ""
		for _, rest := range b.tabs {
			b.currentID = rest.id
			break
		}
	}
	b.mu.Unlock()

	// Close the target synchronously first: cancelling the context alone lets a
	// tab linger long enough to show up in the very next listing. The command
	// must go to the browser executor — a target session refuses CloseTarget.
	c := chromedp.FromContext(b.rootCtx)
	if c == nil || c.Browser == nil {
		return fmt.Errorf("browser is not running")
	}
	ctx, cancel := context.WithTimeout(b.rootCtx, b.timeout)
	defer cancel()
	err := target.CloseTarget(id).Do(cdp.WithExecutor(ctx, c.Browser))
	if t != nil && !t.root {
		t.cancel()
	}
	return err
}

// selectTab makes the given target the current tab, attaching to it if needed.
func (b *Browser) selectTab(id target.ID) error {
	if _, err := b.ensureTab(id); err != nil {
		return err
	}
	b.mu.Lock()
	b.currentID = id
	b.mu.Unlock()
	return nil
}

// armDialog stores the one-shot answer for the next JavaScript dialog.
func (b *Browser) armDialog(accept bool, promptText string) {
	b.mu.Lock()
	b.dialogNext = &dialogResponse{accept: accept, promptText: promptText}
	b.mu.Unlock()
}

// resolveDialog answers a JavaScript dialog so the action that triggered it does
// not hang until its timeout. An armed response (foxxycode_browser_handle_dialog)
// wins; otherwise alerts/confirms/prompts are dismissed and beforeunload is
// accepted, because silently refusing to leave a page would make navigation
// fail with no visible cause. Either way the page log records what happened.
func (b *Browser) resolveDialog(ctx context.Context, e *page.EventJavascriptDialogOpening) {
	b.mu.Lock()
	resp := b.dialogNext
	b.dialogNext = nil
	b.mu.Unlock()

	accept := e.Type == page.DialogTypeBeforeunload
	promptText := ""
	how := "auto-handled; use foxxycode_browser_handle_dialog before the triggering action to choose"
	if resp != nil {
		accept, promptText, how = resp.accept, resp.promptText, "handled as armed"
	}
	p := page.HandleJavaScriptDialog(accept)
	if promptText != "" {
		p = p.WithPromptText(promptText)
	}
	rctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := chromedp.Run(rctx, p); err != nil {
		b.addPageLog(fmt.Sprintf("[dialog] failed to handle %s %q: %v", e.Type, e.Message, err))
		return
	}
	verdict := "dismissed"
	if accept {
		verdict = "accepted"
	}
	b.addPageLog(fmt.Sprintf("[dialog] %s %s %q (%s)", verdict, e.Type, e.Message, how))
}

// resolvePaused answers a request paused by fetch interception: fulfilled from a
// matching mock, or continued untouched.
func (b *Browser) resolvePaused(ctx context.Context, e *fetch.EventRequestPaused) {
	b.mu.Lock()
	var mock *routeMock
	for i := range b.mocks {
		if strings.Contains(e.Request.URL, b.mocks[i].substr) {
			mock = &b.mocks[i]
			break
		}
	}
	b.mu.Unlock()

	rctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if mock == nil {
		_ = chromedp.Run(rctx, fetch.ContinueRequest(e.RequestID))
		return
	}
	p := fetch.FulfillRequest(e.RequestID, int64(mock.status)).
		WithBody(base64.StdEncoding.EncodeToString([]byte(mock.body)))
	if mock.contentType != "" {
		p = p.WithResponseHeaders([]*fetch.HeaderEntry{{Name: "Content-Type", Value: mock.contentType}})
	}
	if err := chromedp.Run(rctx, p); err != nil {
		b.addPageLog(fmt.Sprintf("[route] failed to mock %s: %v", e.Request.URL, err))
		return
	}
	b.addPageLog(fmt.Sprintf("[route] mocked %s %s -> %d", e.Request.Method, e.Request.URL, mock.status))
}

// recordRequest journals an outgoing request.
func (b *Browser) recordRequest(e *network.EventRequestWillBeSent) {
	if e.Request == nil || strings.HasPrefix(e.Request.URL, "data:") {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.netLog = append(b.netLog, &netEntry{
		id:     e.RequestID,
		method: e.Request.Method,
		url:    e.Request.URL,
		rtype:  string(e.Type),
	})
	if len(b.netLog) > netLogMax {
		b.netLog = b.netLog[len(b.netLog)-netLogMax:]
	}
}

// recordResponse fills in the response half of a journal entry and keeps the
// page log's error reporting: an error status is something that went wrong.
func (b *Browser) recordResponse(e *network.EventResponseReceived) {
	if e.Response == nil {
		return
	}
	if e.Response.Status >= 400 {
		b.addPageLog(fmt.Sprintf("[network] %d %s", int(e.Response.Status), e.Response.URL))
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := len(b.netLog) - 1; i >= 0; i-- {
		if b.netLog[i].id == e.RequestID {
			b.netLog[i].status = e.Response.Status
			b.netLog[i].mime = e.Response.MimeType
			return
		}
	}
}

func (b *Browser) recordFinished(e *network.EventLoadingFinished) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := len(b.netLog) - 1; i >= 0; i-- {
		if b.netLog[i].id == e.RequestID {
			b.netLog[i].size = e.EncodedDataLength
			return
		}
	}
}

// requestLog returns a snapshot of the journal, newest last.
func (b *Browser) requestLog() []netEntry {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]netEntry, 0, len(b.netLog))
	for _, e := range b.netLog {
		out = append(out, *e)
	}
	return out
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
