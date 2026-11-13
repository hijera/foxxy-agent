//go:build http && ui && miniapps && browser

package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	cdpbrowser "github.com/chromedp/cdproto/browser"
	"github.com/chromedp/chromedp"
	"github.com/hijera/foxxycode-agent/external/miniapps"
	"github.com/hijera/foxxycode-agent/internal/platform"
)

// Real-file integration coverage: no fake encoder, API interceptor or external
// model. The uploaded video travels through the generated production form and
// the MP3 is downloaded by Chrome, then checked independently with ffprobe.
func TestMiniAppsBrowserAudioExtractor(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe is not installed")
	}
	root := t.TempDir()
	video := filepath.Join(root, "video sample.mp4")
	audioTestCommand(t, ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "color=c=blue:s=160x90:r=10", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=44100", "-t", "1", "-threads", "1", "-c:v", "mpeg4", "-c:a", "aac", video)
	document, err := os.ReadFile("../../examples/miniapps/audio-extractor.json")
	if err != nil {
		t.Fatal(err)
	}
	ts := newMiniAppsBrowserServer(t)
	response, err := http.Post(ts.URL+"/foxxycode/miniapps", "application/json", bytes.NewReader(document))
	if err != nil {
		t.Fatal(err)
	}
	var app miniapps.MiniApp
	err = json.NewDecoder(response.Body).Decode(&app)
	_ = response.Body.Close()
	if err != nil || response.StatusCode != 201 {
		t.Fatalf("seed=%d err=%v", response.StatusCode, err)
	}
	browserCtx, cancel := newMiniAppsBrowserContext(t)
	defer cancel()
	downloads := filepath.Join(root, "downloads")
	if err := os.MkdirAll(downloads, 0o700); err != nil {
		t.Fatal(err)
	}
	runMiniAppsBrowser(t, browserCtx, cdpbrowser.SetDownloadBehavior(cdpbrowser.SetDownloadBehaviorBehaviorAllow).WithDownloadPath(downloads))
	runMiniAppsBrowser(t, browserCtx, chromedp.EmulateViewport(1280, 900), chromedp.Navigate(ts.URL+"/#/miniapps/"+app.ID))
	waitForMiniAppsSelector(t, browserCtx, `[data-testid="miniapps-test-run"]`)
	runMiniAppsBrowser(t, browserCtx, chromedp.Evaluate(`document.querySelector('.miniapps-editor-list').scrollIntoView({block:'center'})`, nil))
	audioTestScreenshot(t, browserCtx, "audio-editor-dark-1280.png", 1280)
	audioTestScreenshot(t, browserCtx, "audio-editor-dark-390.png", 390)
	runMiniAppsBrowser(t, browserCtx, chromedp.EmulateViewport(1280, 900), chromedp.Evaluate(`document.querySelector('.miniapps-page').scrollTop=0`, nil))
	runMiniAppsBrowser(t, browserCtx, chromedp.Click(`[data-testid="miniapps-test-run"]`, chromedp.ByQuery))
	waitForMiniAppsSelector(t, browserCtx, `input[type=file]`)
	audioTestScreenshot(t, browserCtx, "audio-form-dark-1280.png", 1280)
	runMiniAppsBrowser(t, browserCtx, chromedp.SetUploadFiles(`input[type=file]`, []string{video}, chromedp.ByQuery))
	waitForMiniAppsSelector(t, browserCtx, `.miniapps-file-selection`)
	runMiniAppsBrowser(t, browserCtx, chromedp.Click(`[data-testid="miniapps-run-start"]`, chromedp.ByQuery))
	waitForMiniAppsSelector(t, browserCtx, `.miniapps-confirmation .miniapps-primary`)
	audioTestScreenshot(t, browserCtx, "audio-trust-dark-1280.png", 1280)
	runMiniAppsBrowser(t, browserCtx, chromedp.Click(`.miniapps-confirmation .miniapps-primary`, chromedp.ByQuery))
	waitForMiniAppsSelector(t, browserCtx, `.miniapps-runner .miniapps-status-dot--succeeded`)
	waitForMiniAppsGone(t, browserCtx, `.miniapps-confirmation`)
	waitForMiniAppsSelector(t, browserCtx, `audio[src]`)
	runMiniAppsBrowser(t, browserCtx, chromedp.Poll(`document.querySelector('audio').readyState >= 1`, nil, chromedp.WithPollingTimeout(10*time.Second)))
	audioTestScreenshot(t, browserCtx, "audio-result-dark-1280.png", 1280)
	audioTestScreenshot(t, browserCtx, "audio-result-dark-390.png", 390)
	runMiniAppsBrowser(t, browserCtx, chromedp.Reload())
	waitForMiniAppsSelector(t, browserCtx, `audio[src]`)
	runMiniAppsBrowser(t, browserCtx, chromedp.Click(`.miniapps-output-file button`, chromedp.ByQuery))
	downloaded := filepath.Join(downloads, "audio.mp3")
	deadline := time.Now().Add(15 * time.Second)
	for {
		if _, err := os.Stat(downloaded); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Chrome did not download audio.mp3")
		}
		time.Sleep(50 * time.Millisecond)
	}
	probe := audioTestCommand(t, ffprobe, "-v", "error", "-show_entries", "stream=codec_name,codec_type", "-of", "json", downloaded)
	var inspected struct {
		Streams []struct {
			CodecName string `json:"codec_name"`
			CodecType string `json:"codec_type"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(probe, &inspected); err != nil {
		t.Fatal(err)
	}
	if len(inspected.Streams) != 1 || inspected.Streams[0].CodecName != "mp3" || inspected.Streams[0].CodecType != "audio" {
		t.Fatalf("invalid MP3: %s", probe)
	}
	// A verified draft can be released and the immutable version accepts a new
	// upload. A silent video fails visibly instead of reporting a false success.
	post := func(path string, body any) miniAppsResponse {
		state := miniAppsFeatureState{ts: ts}
		response, err := state.post(path, body)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	released := post("/foxxycode/miniapps/"+app.ID+"/release", map[string]any{"version": "1.0.0", "approved": true, "expected_revision": app.Revision})
	if released.StatusCode != 201 {
		t.Fatalf("release: %d %s", released.StatusCode, released.Body)
	}
	runMiniAppsBrowser(t, browserCtx, chromedp.Reload())
	waitForMiniAppsSelector(t, browserCtx, `[data-testid="miniapps-run-close"]`)
	runMiniAppsBrowser(t, browserCtx, chromedp.Click(`[data-testid="miniapps-run-close"]`, chromedp.ByQuery))
	waitForMiniAppsSelector(t, browserCtx, `[data-testid="miniapps-release-run"]`)
	runMiniAppsBrowser(t, browserCtx, chromedp.Click(`[data-testid="miniapps-release-run"]`, chromedp.ByQuery), chromedp.EmulateViewport(1280, 900))
	waitForMiniAppsSelector(t, browserCtx, `input[type=file]`)
	second := filepath.Join(root, "second clip.mp4")
	audioTestCommand(t, ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "color=c=green:s=160x90:r=10", "-f", "lavfi", "-i", "sine=frequency=880:sample_rate=44100", "-t", "1", "-threads", "1", "-c:v", "mpeg4", "-c:a", "aac", second)
	runMiniAppsBrowser(t, browserCtx, chromedp.SetUploadFiles(`input[type=file]`, []string{second}, chromedp.ByQuery))
	waitForMiniAppsSelector(t, browserCtx, `[aria-label="Remove second clip.mp4"]`)
	runMiniAppsBrowser(t, browserCtx, chromedp.Click(`[data-testid="miniapps-run-start"]`, chromedp.ByQuery))
	waitForMiniAppsSelector(t, browserCtx, `.miniapps-runner .miniapps-status-dot--succeeded`)
	runMiniAppsBrowser(t, browserCtx, chromedp.Poll(`document.querySelectorAll('.miniapps-history-row').length >= 2`, nil, chromedp.WithPollingTimeout(5*time.Second)))
	var count int
	runMiniAppsBrowser(t, browserCtx, chromedp.Evaluate(`document.querySelectorAll('.miniapps-history-row').length`, &count))
	if count < 2 {
		t.Fatalf("history has %d runs", count)
	}
	silent := filepath.Join(root, "silent.mp4")
	audioTestCommand(t, ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "color=c=black:s=160x90:r=10", "-t", "1", "-threads", "1", "-c:v", "mpeg4", silent)
	runMiniAppsBrowser(t, browserCtx, chromedp.Click(`[data-testid="miniapps-run-again"]`, chromedp.ByQuery))
	waitForMiniAppsSelector(t, browserCtx, `input[type=file]`)
	runMiniAppsBrowser(t, browserCtx, chromedp.SetUploadFiles(`input[type=file]`, []string{silent}, chromedp.ByQuery))
	waitForMiniAppsSelector(t, browserCtx, `[aria-label="Remove silent.mp4"]`)
	runMiniAppsBrowser(t, browserCtx, chromedp.Click(`[data-testid="miniapps-run-start"]`, chromedp.ByQuery))
	waitForMiniAppsSelector(t, browserCtx, `.miniapps-runner .miniapps-status-dot--failed`)
	waitForMiniAppsGone(t, browserCtx, `.miniapps-confirmation`)
	audioTestScreenshot(t, browserCtx, "audio-no-track-dark-1280.png", 1280)
	runMiniAppsBrowser(t, browserCtx, chromedp.Click(`.miniapps-history-row:nth-child(2)`, chromedp.ByQuery))
	waitForMiniAppsSelector(t, browserCtx, `audio[src]`)
	runMiniAppsBrowser(t, browserCtx, chromedp.Evaluate(`window.foxxycodeUi.setTheme('light')`, nil))
	audioTestScreenshot(t, browserCtx, "audio-result-light-1280.png", 1280)
}

func audioTestCommand(t *testing.T, binary string, args ...string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	platform.AdaptCommand(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", binary, err, output)
	}
	return output
}

func audioTestScreenshot(t *testing.T, ctx context.Context, name string, width int64) {
	t.Helper()
	dir := strings.TrimSpace(os.Getenv("FOXXYCODE_SHOT_DIR"))
	if dir == "" {
		return
	}
	var image []byte
	runMiniAppsBrowser(t, ctx, chromedp.EmulateViewport(width, 900), chromedp.CaptureScreenshot(&image))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), image, 0o600); err != nil {
		t.Fatal(err)
	}
	var overflow bool
	runMiniAppsBrowser(t, ctx, chromedp.Evaluate(`document.documentElement.scrollWidth > window.innerWidth`, &overflow))
	if overflow {
		t.Fatalf("page overflows at %d px", width)
	}
}
