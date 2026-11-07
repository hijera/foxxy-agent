//go:build gateway || gateway.telegram

package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cucumber/godog"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/platform"
	"github.com/hijera/foxxycode-agent/internal/tgfake"
)

func TestMain(m *testing.M) {
	if dir := os.Getenv(WorkerEnv); dir != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := RunWorker(ctx, dir); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	if dir := os.Getenv("FOXXYCODE_RELAY_TEST_LAUNCHER"); dir != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		d, err := ensure(ctx, dir)
		if err != nil {
			os.Exit(2)
		}
		data, _ := json.Marshal(d)
		if err := os.WriteFile(filepath.Join(dir, "launcher-ready.json"), data, 0o600); err != nil {
			os.Exit(3)
		}
		<-ctx.Done()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type relayWorld struct {
	fake            *tgfake.Server
	upstream, local *httptest.Server
	server          *Server
	cancel          context.CancelFunc
}

func (w *relayWorld) connected() error {
	w.fake = tgfake.New(tgfake.Options{Token: "123:test", MaxPollWait: 20 * time.Millisecond})
	w.upstream = httptest.NewServer(w.fake.Handler())
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.server = NewServer(ctx, "secret")
	w.local = httptest.NewServer(w.server)
	for id, editor := range map[string]string{"idea": "intellij", "code": "vscode"} {
		cfg := config.TelegramGatewayConfig{Token: "123:test", DefaultAccess: config.AccessAll}
		if err := w.server.Register(Registration{ID: id, Project: id, Label: id, Editor: editor, Config: cfg, APIBase: w.upstream.URL}); err != nil {
			return err
		}
	}
	return nil
}

func (w *relayWorld) selectCode() error {
	w.server.route("123:test", tgbotapi.Update{UpdateID: 1, Message: &tgbotapi.Message{MessageID: 1, From: &tgbotapi.User{ID: 7}, Chat: &tgbotapi.Chat{ID: 7, Type: "private"}, Text: "/project code", Entities: []tgbotapi.MessageEntity{{Type: "bot_command", Offset: 0, Length: 8}}}})
	return nil
}

func (w *relayWorld) message() error {
	w.server.route("123:test", tgbotapi.Update{UpdateID: 2, Message: &tgbotapi.Message{MessageID: 2, From: &tgbotapi.User{ID: 7}, Chat: &tgbotapi.Chat{ID: 7, Type: "private"}, Text: "hello"}})
	return nil
}

func (w *relayWorld) onlyCode() error {
	w.server.mu.Lock()
	defer w.server.mu.Unlock()
	if len(w.server.clients["code"].pending) != 1 || len(w.server.clients["idea"].pending) != 0 {
		return fmt.Errorf("message was not routed exclusively to VS Code")
	}
	return nil
}

func (w *relayWorld) replies() error {
	for _, id := range []string{"idea", "code"} {
		req, _ := http.NewRequest(http.MethodPost, w.local.URL+"/api/"+id+"/sendMessage", strings.NewReader("chat_id=7&text=reply"))
		req.Header.Set("Authorization", "Bearer secret")
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		var result tgbotapi.APIResponse
		err = json.NewDecoder(res.Body).Decode(&result)
		_ = res.Body.Close()
		if err != nil || !result.Ok {
			return fmt.Errorf("%s reply failed: %v", id, err)
		}
	}
	if len(w.fake.Calls("sendMessage")) != 3 {
		return fmt.Errorf("upstream replies missing")
	}
	return nil
}

func (w *relayWorld) close() {
	if w.cancel != nil {
		w.cancel()
	}
	if w.local != nil {
		w.local.Close()
	}
	if w.fake != nil {
		w.fake.Close()
	}
	if w.upstream != nil {
		w.upstream.Close()
	}
}

func newWorld(t *testing.T) *relayWorld {
	t.Helper()
	w := &relayWorld{}
	if err := w.connected(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.close)
	return w
}

func (w *relayWorld) request(ctx context.Context, id, method string, form url.Values) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.local.URL+"/api/"+id+"/"+method, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return http.DefaultClient.Do(req)
}

func TestConcurrentLaunchersReuseOneWorker(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	results := make(chan discovery, 6)
	errs := make(chan error, 6)
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); d, err := ensure(ctx, dir); results <- d; errs <- err }()
	}
	wg.Wait()
	close(results)
	close(errs)
	var first discovery
	for d := range results {
		if d.PID == 0 {
			continue
		}
		if first.PID == 0 {
			first = d
			t.Cleanup(func() {
				p, err := os.FindProcess(d.PID)
				if err == nil {
					_ = p.Kill()
					_, _ = p.Wait()
				}
			})
		} else if first != d {
			t.Errorf("launchers started distinct relay workers")
		}
	}
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	if first.PID == 0 || first.PID == os.Getpid() {
		t.Fatal("no separate relay process")
	}
}

func TestRelaySurvivesLauncherProcessTreeTermination(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows recursive process termination")
	}
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	launcher := exec.CommandContext(ctx, os.Args[0], "-test.run=^$")
	launcher.Env = append(os.Environ(), "FOXXYCODE_RELAY_TEST_LAUNCHER="+dir)
	platform.HideConsoleWindow(launcher)
	platform.AdaptCommand(launcher)
	if err := launcher.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = launcher.Process.Kill(); _ = launcher.Wait() })
	var before discovery
	for {
		data, err := os.ReadFile(filepath.Join(dir, "launcher-ready.json"))
		if err == nil && json.Unmarshal(data, &before) == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("launcher did not become ready")
		case <-time.After(20 * time.Millisecond):
		}
	}
	t.Cleanup(func() {
		if p, err := os.FindProcess(before.PID); err == nil {
			_ = p.Kill()
			_, _ = p.Wait()
		}
	})
	// IntelliJ force-stops the complete backend tree, not just its root PID.
	kill := exec.CommandContext(ctx, "taskkill", "/PID", fmt.Sprint(launcher.Process.Pid), "/T", "/F")
	platform.HideConsoleWindow(kill)
	platform.AdaptCommand(kill)
	if out, err := kill.CombinedOutput(); err != nil {
		t.Fatalf("terminate launcher tree: %v: %s", err, out)
	}
	if after, ok := readLive(ctx, dir); !ok || after != before {
		t.Fatal("terminating an IDE backend tree killed the shared relay")
	}
}

func TestRelayRejectsUnauthenticatedCalls(t *testing.T) {
	w := newWorld(t)
	res, err := http.Post(w.local.URL+"/register", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status=%d", res.StatusCode)
	}
}

func TestClientReconnectsAfterRelayWorkerDies(t *testing.T) {
	w := newWorld(t)
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	transport, err := connect(ctx, dir, Registration{Project: "project", Editor: "vscode", Label: "project", APIBase: w.upstream.URL, Config: config.TelegramGatewayConfig{Token: "123:test", DefaultAccess: config.AccessAll}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if d, ok := readLive(context.Background(), dir); ok && d.PID != os.Getpid() {
			p, err := os.FindProcess(d.PID)
			if err == nil {
				_ = p.Kill()
				_, _ = p.Wait()
			}
		}
	})
	t.Cleanup(transport.Close)
	api, err := tgbotapi.NewBotAPIWithClient("123:test", tgbotapi.APIEndpoint, transport)
	if err != nil {
		t.Fatal(err)
	}
	before, ok := readLive(ctx, dir)
	if !ok || before.PID == os.Getpid() {
		t.Fatal("missing independent relay worker")
	}
	p, err := os.FindProcess(before.PID)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Kill(); err != nil {
		t.Fatal(err)
	}
	_, _ = p.Wait()
	// The failed request triggers reconnection; mutations are deliberately not
	// retried automatically, and the library's next request uses the new relay.
	_, _ = api.GetMe()
	if _, err := api.GetMe(); err != nil {
		t.Fatal(err)
	}
	after, ok := readLive(ctx, dir)
	if !ok || after.PID == before.PID {
		t.Fatal("client did not replace the crashed relay")
	}
}

func TestRelayDoesNotExposeProjectsToUnauthorizedUsers(t *testing.T) {
	w := newWorld(t)
	for _, c := range w.server.clients {
		c.Config.DefaultAccess = config.AccessAdmins
		c.Config.Admins = []int64{99}
	}
	_ = w.selectCode()
	_ = w.message()
	if len(w.fake.Calls("sendMessage")) != 0 {
		t.Fatal("unauthorized user received project information")
	}
	for _, c := range w.server.clients {
		if len(c.pending) != 0 {
			t.Fatal("unauthorized update was delivered")
		}
	}
}

func TestRelayKeepsUpdatesUntilClientAcknowledgesOffset(t *testing.T) {
	w := newWorld(t)
	_ = w.selectCode()
	_ = w.message()
	for i := 0; i < 2; i++ {
		res, err := w.request(context.Background(), "code", "getUpdates", url.Values{"offset": {"0"}})
		if err != nil {
			t.Fatal(err)
		}
		var result struct {
			Result []tgbotapi.Update `json:"result"`
		}
		err = json.NewDecoder(res.Body).Decode(&result)
		_ = res.Body.Close()
		if err != nil || len(result.Result) != 1 || result.Result[0].UpdateID != 2 {
			t.Fatalf("unacknowledged update missing: %+v, %v", result, err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	res, _ := w.request(ctx, "code", "getUpdates", url.Values{"offset": {"3"}})
	if res != nil {
		_ = res.Body.Close()
	}
	w.server.mu.Lock()
	defer w.server.mu.Unlock()
	if len(w.server.clients["code"].pending) != 0 {
		t.Fatal("offset did not acknowledge the delivered update")
	}
}

func TestRelayPollsUpstreamOnceForBothEditors(t *testing.T) {
	w := newWorld(t)
	tracker := &pollTracker{base: http.DefaultTransport}
	w.server.bots["123:test"].http.Transport = tracker
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for _, id := range []string{"idea", "code"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, _ := w.request(ctx, id, "getUpdates", url.Values{})
			if res != nil {
				_ = res.Body.Close()
			}
		}()
	}
	if !w.fake.WaitCall("getUpdates", 2, time.Second) {
		t.Fatal("relay did not poll upstream")
	}
	w.server.mu.Lock()
	if len(w.server.bots) != 1 || !w.server.bots["123:test"].started {
		t.Error("editors do not share their poller")
	}
	w.server.mu.Unlock()
	w.fake.InjectMessage(tgfake.IncomingMessage{UserID: 7, Text: "/project code"})
	w.fake.InjectMessage(tgfake.IncomingMessage{UserID: 7, Text: "hello over polling"})
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		w.server.mu.Lock()
		n := len(w.server.clients["code"].pending)
		w.server.mu.Unlock()
		if n == 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if err := w.onlyCode(); err != nil {
		t.Error(err)
	}
	cancel()
	wg.Wait()
	if tracker.max.Load() != 1 {
		t.Fatalf("concurrent upstream polls=%d", tracker.max.Load())
	}
}

type pollTracker struct {
	base        http.RoundTripper
	active, max atomic.Int32
}

func (p *pollTracker) RoundTrip(r *http.Request) (*http.Response, error) {
	if strings.HasSuffix(r.URL.Path, "/getUpdates") {
		n := p.active.Add(1)
		defer p.active.Add(-1)
		for old := p.max.Load(); n > old; old = p.max.Load() {
			if p.max.CompareAndSwap(old, n) {
				break
			}
		}
	}
	return p.base.RoundTrip(r)
}

func TestRelayPersistsChoiceWithoutPersistingBotToken(t *testing.T) {
	w := newWorld(t)
	file := filepath.Join(t.TempDir(), "selection.json")
	w.server.loadSelections(file)
	_ = w.selectCode()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "123:test") {
		t.Fatal("selection file contains bot token")
	}
	restarted := NewServer(context.Background(), "secret")
	restarted.loadSelections(file)
	if restarted.selected[selectionKey("123:test", 7, 7, 0)] != "code" {
		t.Fatal("project choice did not survive a relay restart")
	}
}

func TestCallbackReturnsToOriginalProjectAfterSwitch(t *testing.T) {
	w := newWorld(t)
	_ = w.selectCode()
	markup := `{"inline_keyboard":[[{"text":"Choose","callback_data":"model:test"}]]}`
	res, err := w.request(context.Background(), "idea", "sendMessage", url.Values{"chat_id": {"7"}, "text": {"Choose model"}, "reply_markup": {markup}})
	if err != nil {
		t.Fatal(err)
	}
	var reply struct {
		Result tgbotapi.Message `json:"result"`
	}
	err = json.NewDecoder(res.Body).Decode(&reply)
	_ = res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	_, cbid, err := w.fake.InjectCallback(tgfake.IncomingCallback{ChatID: 7, UserID: 7, MessageID: reply.Result.MessageID, Data: "model:test"})
	if err != nil {
		t.Fatal(err)
	}
	w.server.route("123:test", tgbotapi.Update{UpdateID: 3, CallbackQuery: &tgbotapi.CallbackQuery{ID: cbid, From: &tgbotapi.User{ID: 7}, Message: &reply.Result, Data: "model:test"}})
	if len(w.server.clients["idea"].pending) != 1 || len(w.server.clients["code"].pending) != 0 {
		t.Fatal("callback followed selected project instead of original keyboard")
	}
}

func TestProjectMenuButtonsSelectConnectedProjects(t *testing.T) {
	w := newWorld(t)
	_, messageID := w.fake.InjectMessage(tgfake.IncomingMessage{UserID: 7, Text: "/project"})
	w.server.route("123:test", tgbotapi.Update{UpdateID: 1, Message: &tgbotapi.Message{MessageID: messageID, From: &tgbotapi.User{ID: 7}, Chat: &tgbotapi.Chat{ID: 7, Type: "private"}, Text: "/project", Entities: []tgbotapi.MessageEntity{{Type: "bot_command", Length: 8}}}})
	calls := w.fake.Calls("sendMessage")
	if len(calls) != 1 {
		t.Fatal("project menu missing")
	}
	var response struct {
		Result tgbotapi.Message `json:"result"`
	}
	if err := json.Unmarshal(calls[0].Response, &response); err != nil {
		t.Fatal(err)
	}
	_, cbid, err := w.fake.InjectCallback(tgfake.IncomingCallback{UserID: 7, ChatID: 7, Data: "relay:project:code"})
	if err != nil {
		t.Fatal(err)
	}
	w.server.route("123:test", tgbotapi.Update{UpdateID: 2, CallbackQuery: &tgbotapi.CallbackQuery{ID: cbid, From: &tgbotapi.User{ID: 7}, Message: &response.Result, Data: "relay:project:code"}})
	_ = w.message()
	if err := w.onlyCode(); err != nil {
		t.Fatal(err)
	}
	if len(w.fake.Calls("answerCallbackQuery")) != 1 {
		t.Fatal("project picker tap was not acknowledged")
	}
}

func TestGroupChatterDoesNotTriggerProjectNotices(t *testing.T) {
	w := newWorld(t)
	w.server.route("123:test", tgbotapi.Update{UpdateID: 1, Message: &tgbotapi.Message{MessageID: 1, From: &tgbotapi.User{ID: 7}, Chat: &tgbotapi.Chat{ID: -7, Type: "supergroup"}, Text: "hello everyone"}})
	if len(w.fake.Calls("sendMessage")) != 0 {
		t.Fatal("relay replied to unaddressed group chatter")
	}
}

func TestExpiredProjectIsNotSilentlyReplaced(t *testing.T) {
	w := newWorld(t)
	_ = w.selectCode()
	w.server.clients["code"].seen = time.Now().Add(-2 * leaseTime)
	w.server.Sweep(time.Now())
	_ = w.message()
	if len(w.server.clients["idea"].pending) != 0 {
		t.Fatal("message moved to another workspace without selection")
	}
	if w.server.Sweep(time.Now().Add(2*leaseTime)) != true {
		t.Fatal("idle relay stays running")
	}
}

func TestBotOriginAndProxyCannotChangeUnderAnotherEditor(t *testing.T) {
	w := newWorld(t)
	r := w.server.clients["code"].Registration
	r.ID = "other"
	r.Config.Proxy = "none"
	if err := w.server.Register(r); err == nil {
		t.Fatal("conflicting route accepted for a shared token")
	}
}

func TestEditorRelayFeature(t *testing.T) {
	suite := godog.TestSuite{Name: "telegram-editor-relay", ScenarioInitializer: func(sc *godog.ScenarioContext) {
		w := &relayWorld{}
		sc.Step(`^an IntelliJ project and a VS Code project connected to one Telegram relay$`, w.connected)
		sc.Step(`^a Telegram user selects the VS Code project$`, w.selectCode)
		sc.Step(`^the user sends a message to the bot$`, w.message)
		sc.Step(`^only the VS Code project receives the message$`, w.onlyCode)
		sc.Step(`^both projects send replies through the relay$`, w.replies)
		sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
			w.close()
			return ctx, nil
		})
	}, Options: &godog.Options{Format: "pretty", Paths: []string{"../../../../features/telegram_editor_relay.feature"}, TestingT: t}}
	if suite.Run() != 0 {
		t.Fatal("relay feature failed")
	}
}
