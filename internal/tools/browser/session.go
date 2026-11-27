//go:build browser

// Package browser provides an interactive browser-automation tool set (navigate,
// click, fill, hover, scroll, screenshot, evaluate, close) backed by a local
// Chrome/Chromium instance driven over the DevTools Protocol via chromedp.
//
// It is gated behind the "browser" build tag and disabled by default; enable it
// with config browser.enable: true in a build compiled with -tags browser.
package browser

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
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
	"github.com/hijera/foxxycode-agent/internal/platform"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// Manager owns one Browser per agent session, keyed by session id. A single
// Manager instance is shared by all browser tools (see register.go).
type Manager struct {
	cfg *config.BrowserConfig

	mu       sync.Mutex
	browsers map[string]*Browser
	closed   bool
}

// NewManager returns a Manager bound to the given browser config.
func NewManager(cfg *config.BrowserConfig) *Manager {
	return &Manager{cfg: cfg, browsers: make(map[string]*Browser)}
}

func (m *Manager) sessionManager(env *tooling.Env) *Manager {
	if env == nil || env.SessionBrowser == nil {
		return m
	}
	// The owner is created with the same config, so a caller that forgets the
	// cfg argument to get cannot launch a browser off a nil config.
	owner, _ := env.SessionBrowser(func() io.Closer { return NewManager(m.cfg) }).(*Manager)
	return owner
}

func (m *Manager) open(env *tooling.Env) (*Browser, error) {
	return m.sessionManager(env).get(sessionKey(env), profileDirFor(sessionDir(env)), m.cfg)
}

// get reuses the live browser, or starts Chrome using the current tool config
// after an earlier connection was lost.
func (m *Manager) get(sessionID, profileDir string, cfg *config.BrowserConfig) (*Browser, error) {
	if m == nil {
		return nil, fmt.Errorf("browser session is closed")
	}
	key := strings.TrimSpace(sessionID)
	if key == "" {
		key = "_default"
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, fmt.Errorf("browser session is closed")
	}
	var stale *Browser
	if b, ok := m.browsers[key]; ok && b != nil {
		if b.connected() {
			b.mu.Lock()
			b.captureScreens = cfg.ScreenshotsEnabled()
			b.screenshotByRequest = cfg.ScreenshotPolicy().ModelDecides
			b.mu.Unlock()
			m.mu.Unlock()
			return b, nil
		}
		delete(m.browsers, key)
		stale = b
	}
	b, err := launchSessionBrowser(cfg, profileDir)
	if err != nil {
		m.mu.Unlock()
		if stale != nil {
			stale.close()
		}
		return nil, err
	}
	if stale != nil {
		b.adoptBrowserWideState(stale)
	}
	m.browsers[key] = b
	m.mu.Unlock()
	// Torn down outside the lock: close waits for Chrome's process tree to
	// exit, and holding the manager through it would stall every other
	// session behind this one.
	if stale != nil {
		stale.close()
	}
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
	// Outside the lock, for the same reason get tears its stale browser down
	// there: close waits for Chrome's process tree.
	if b != nil {
		b.close()
	}
}

// Close releases the live session's runtime and prevents further launches.
func (m *Manager) Close() error {
	m.mu.Lock()
	m.closed = true
	browsers := m.browsers
	m.browsers = make(map[string]*Browser)
	m.mu.Unlock()
	for _, b := range browsers {
		b.close()
	}
	return nil
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
	// chromePID is the Chrome process chromedp started, or 0 if it never did.
	// close needs it to find the children Chrome spawned, which outlive the
	// browser process itself by a short but unbounded moment.
	chromePID int
	// captureScreens mirrors browser.screenshots. When false no image is taken and
	// none is handed to the model; every action still reports URL and page log.
	captureScreens      bool
	screenshotByRequest bool
	recoveryProfile     string

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

func (b *Browser) connected() bool {
	if b.rootCtx.Err() != nil {
		return false
	}
	c := chromedp.FromContext(b.rootCtx)
	if c == nil || c.Browser == nil {
		// The browser is allocated by the first Run; a Browser never runs
		// there is no connection to keep alive.
		return false
	}
	select {
	case <-c.Browser.LostConnection:
		return false
	default:
		return true
	}
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
		} else {
			// Without the user-data directory Chrome falls back to its own
			// temporary profile: cookies and storage will not survive the
			// session. Recorded, because that loss otherwise looks like the
			// recovery-profile path and nobody looks for its cause here.
			cdpLogf("create browser profile directory %s: %v", profileDir, err)
		}
	}
	if execPath != "" {
		opts = append(opts, chromedp.ExecPath(execPath))
	}
	// Learn which process Chrome ends up being. chromedp keeps it to itself, and
	// this is the only hook it offers onto the command - which is also why
	// applyChromeCmdDefaults is here: chromedp's own setup of the command is the
	// else branch of this option, so passing one replaces it.
	var chromeCmd *exec.Cmd
	opts = append(opts, chromedp.ModifyCmdFunc(func(cmd *exec.Cmd) {
		applyChromeCmdDefaults(cmd)
		chromeCmd = cmd
	}))

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	// Route chromedp's own diagnostics into the configured log; see cdpLogf.
	ctx, ctxCancel := chromedp.NewContext(allocCtx,
		chromedp.WithErrorf(cdpLogf),
		chromedp.WithLogf(cdpLogf),
	)

	b := &Browser{
		allocCtx:            allocCtx,
		allocCancel:         allocCancel,
		timeout:             timeout,
		captureScreens:      cfg.ScreenshotsEnabled(),
		screenshotByRequest: cfg.ScreenshotPolicy().ModelDecides,
		tabs:                make(map[target.ID]*tab),
		rootCtx:             ctx,
		rootStop:            ctxCancel,
	}

	// Capture page events before the first Run so nothing emitted during startup
	// is lost. See attachListeners for what is collected and why.
	b.attachListeners(ctx)
	// Browser-level events carry no session id, so they reach the browser
	// listener instead of the per-tab ones: this is how a tab the page closed
	// on its own (window.close) is dropped before its dead context breaks a
	// later action. Registered before the first Run for the same reason.
	chromedp.ListenBrowser(ctx, func(ev interface{}) {
		if e, ok := ev.(*target.EventTargetDestroyed); ok {
			b.dropTab(e.TargetID)
		}
	})

	// Force the browser process to start now so failures surface immediately. This
	// first Run MUST use the long-lived ctx: chromedp binds the browser/tab lifetime
	// to the context of the first Run, so a short-lived (timeout) context here would
	// tear the browser down as soon as it is cancelled. It also creates the target
	// that ListenTarget above attaches to, and enables the Network and Runtime
	// domains: without them Chrome emits no response, failure or console events at
	// all, and the listener sees nothing however correct it looks.
	runErr := chromedp.Run(ctx, network.Enable(), runtime.Enable())
	// Safe to read only now, and read before the error is handled: chromedp
	// allocates the browser inside that first Run, on this goroutine, so the
	// option above has been called by the time it returns - and a Run that failed
	// after Chrome started is exactly the case where knowing its pid matters.
	if chromeCmd != nil && chromeCmd.Process != nil {
		b.chromePID = chromeCmd.Process.Pid
	}
	if runErr != nil {
		// Through close, so a browser that started but could not be set up leaves
		// no more behind than one that is closed normally.
		b.close()
		return nil, fmt.Errorf("launch browser: %w", runErr)
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

func (b *Browser) runContext(ctx context.Context, actions ...chromedp.Action) error {
	actionCtx, cancel := context.WithTimeout(b.currentCtx(), b.timeout)
	defer cancel()
	stop := context.AfterFunc(ctx, cancel)
	defer stop()
	return chromedp.Run(actionCtx, actions...)
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

// dropTab forgets a tab whose target no longer exists. The page can close its
// own tabs (window.close, a dismissed popup) without going through the tabs
// tool, which would leave a dead entry in b.tabs: currentCtx would keep
// handing it out and every later action would fail on it. Runs on the CDP
// message-loop goroutine, so it must not block on Chrome.
func (b *Browser) dropTab(id target.ID) {
	b.mu.Lock()
	t, ok := b.tabs[id]
	if ok {
		delete(b.tabs, id)
		if b.currentID == id {
			b.currentID = ""
			for _, rest := range b.tabs {
				b.currentID = rest.id
				break
			}
		}
	}
	b.mu.Unlock()
	if ok && t != nil && !t.root {
		// The chromedp cancel waits for the DetachFromTarget send to finish,
		// and that send needs the browser's listener mutex - the one this
		// event callback is running under. The map entry is already gone, so
		// the context cleanup moves aside and never runs on this goroutine.
		go t.cancel()
		b.addPageLog("[browser] the page closed its tab; later actions continue on another open tab")
	}
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

// adoptBrowserWideState carries the browser-wide setup a relaunched browser
// would otherwise lose: route mocks, offline emulation and an armed dialog
// answer. A crashed Chrome takes its mocks with it; without this the mocked
// endpoints start answering for real and the model reads the app as broken.
// Per-tab re-application for tabs opened later happens through initTab; the
// tabs that already exist are re-armed here.
func (b *Browser) adoptBrowserWideState(stale *Browser) {
	stale.mu.Lock()
	mocks := stale.mocks
	fetchOn := stale.fetchOn
	offline := stale.offline
	dialog := stale.dialogNext
	stale.mu.Unlock()

	b.mu.Lock()
	b.mocks = mocks
	b.fetchOn = fetchOn
	b.offline = offline
	b.dialogNext = dialog
	b.mu.Unlock()

	if fetchOn {
		if err := b.forEachTab(func(ctx context.Context) error { return fetch.Enable().Do(ctx) }); err != nil {
			b.mu.Lock()
			b.fetchOn = false
			b.mu.Unlock()
			b.addPageLog(fmt.Sprintf("[browser] relaunch could not re-arm request interception: %v", err))
		}
	}
	if offline {
		if err := b.forEachTab(func(ctx context.Context) error {
			return network.EmulateNetworkConditions(true, 0, -1, -1).Do(ctx)
		}); err != nil {
			b.mu.Lock()
			b.offline = false
			b.mu.Unlock()
			b.addPageLog(fmt.Sprintf("[browser] relaunch could not re-apply offline emulation: %v", err))
		}
	}

	carried := make([]string, 0, 3)
	if len(mocks) > 0 {
		carried = append(carried, fmt.Sprintf("%d route mock(s)", len(mocks)))
	}
	if offline {
		carried = append(carried, "offline emulation")
	}
	if dialog != nil {
		carried = append(carried, "the armed dialog answer")
	}
	if len(carried) == 0 {
		b.addPageLog("[browser] relaunched after the previous browser process exited")
		return
	}
	b.addPageLog("[browser] relaunched after the previous browser process exited; re-applied " + strings.Join(carried, ", "))
}

// browserExitGrace bounds how long close waits for Chrome's children to follow
// it out. They go on their own within a few hundred milliseconds even on a busy
// machine, so this is the point at which one of them is stuck rather than slow.
const browserExitGrace = 5 * time.Second

// close tears the browser down and does not return until Chrome and everything
// it spawned are actually gone.
//
// Cancelling the chromedp contexts kills the browser process and waits for that
// one process, which is not the same thing: Chrome's renderers, GPU process and
// crashpad handler are separate processes that notice the browser is gone and
// exit a moment later. On Windows that moment is visible, because the crashpad
// handler keeps CrashpadMetrics-active.pma mapped in the profile directory until
// it does - so a close that returns early leaves a profile that cannot be
// deleted and, if the session reopens straight away, a profile still held by the
// browser that was supposed to be closed.
func (b *Browser) close() {
	// Captured first, while Chrome is still running: after it exits, which of the
	// processes naming it as their parent are really its children can no longer
	// be told apart from a stranger that was handed its pid.
	pid := b.chromePID
	if c := chromedp.FromContext(b.rootCtx); c != nil && c.Browser != nil && !b.connected() {
		// Once an established connection is gone, the old PID can have
		// been reused. Let the allocator close its own process handle.
		pid = 0
	}
	tree := platform.CaptureProcessTree(pid)
	defer tree.Release()

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

	if !tree.WaitExit(browserExitGrace) {
		// Something below Chrome outlived the browser itself by five seconds. It
		// is not going to leave on its own, and it belongs to a session that is
		// over either way.
		tree.TerminateSurvivors()
	}
	if b.recoveryProfile != "" {
		if err := os.RemoveAll(b.recoveryProfile); err != nil {
			cdpLogf("remove temporary browser profile: %v", err)
		}
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
			matched := b.mocks[i]
			mock = &matched
			break
		}
	}
	b.mu.Unlock()

	rctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if mock == nil {
		// A continue that fails never settles the request: the page stalls and
		// a later action on it times out with no visible cause unless this is
		// recorded. The fulfil path below reports its failures the same way.
		if err := chromedp.Run(rctx, fetch.ContinueRequest(e.RequestID)); err != nil {
			b.addPageLog(fmt.Sprintf("[route] failed to continue %s: %v", e.Request.URL, err))
		}
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
