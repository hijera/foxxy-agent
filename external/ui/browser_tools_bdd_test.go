//go:build http && ui && browser

package ui

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/cucumber/godog"
)

func TestBrowserToolPresentationFeature(t *testing.T) {
	shotPath := filepath.Join(t.TempDir(), "browser-fixture.png")
	assets := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/page" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body style="margin:0;background:#f6f5fa;font:20px system-ui;color:#272331"><main style="margin:55px auto;max-width:460px"><h1>Example page</h1><p>A browser action opens this page.</p><button style="background:#7c3aed;color:white;border:0;padding:12px 24px;border-radius:8px">Continue</button></main></body></html>`))
			return
		}
		http.ServeFile(w, r, shotPath)
	}))
	defer assets.Close()
	t.Setenv("FOXXYCODE_UI_BACKEND", assets.URL)
	server := startLayoutDevServer(t)
	t.Cleanup(func() { server.stop(t) })
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	if executable := os.Getenv("FOXXYCODE_UI_BROWSER"); executable != "" {
		opts = append(opts, chromedp.ExecPath(executable))
	}
	alloc, stopAlloc := chromedp.NewExecAllocator(ctx, opts...)
	defer stopAlloc()
	tab, stopTab := chromedp.NewContext(alloc)
	defer stopTab()
	var pageShot []byte
	if err := chromedp.Run(tab, chromedp.EmulateViewport(640, 360), chromedp.Navigate(assets.URL+"/page"), chromedp.WaitReady("button"), chromedp.CaptureScreenshot(&pageShot)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shotPath, pageShot, 0o600); err != nil {
		t.Fatal(err)
	}
	check := func(expression string) error {
		var ok bool
		if err := chromedp.Run(tab, chromedp.Evaluate(expression, &ok)); err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("browser presentation assertion failed: %s", expression)
		}
		return nil
	}
	suite := godog.TestSuite{
		Name:    "browser-tool-presentation",
		Options: &godog.Options{Format: "pretty", Paths: []string{"../../features/browser_tool_presentation.feature"}, TestingT: t, Strict: true},
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			sc.Step(`^a transcript containing browser tool calls$`, func() error {
				return chromedp.Run(tab, chromedp.EmulateViewport(1280, 900), chromedp.Navigate(server.baseURL+"/browser-tools-check.html"), chromedp.WaitReady(".foxxycode-tool-details"))
			})
			sc.Step(`^I expand the browser calls$`, func() error {
				return chromedp.Run(tab, chromedp.Evaluate(`document.querySelectorAll('.foxxycode-tool-details').forEach(el => { el.open = true; })`, nil))
			})
			sc.Step(`^navigation has a readable action heading$`, func() error {
				return check(`document.querySelector('.thinking-label').textContent === 'Open page'`)
			})
			sc.Step(`^the screenshot result can be enlarged$`, func() error {
				if err := chromedp.Run(tab, chromedp.Poll(`document.querySelector('.browser-shot-img')?.naturalWidth > 0`, nil, chromedp.WithPollingTimeout(5*time.Second)), chromedp.Click(".browser-shot")); err != nil {
					return err
				}
				if err := check(`document.querySelector('.browser-shot').getAttribute('aria-expanded') === 'true'`); err != nil {
					return err
				}
				return chromedp.Run(tab, chromedp.Click(".browser-shot"))
			})
			sc.Step(`^JavaScript is formatted and highlighted$`, func() error {
				if err := chromedp.Run(tab, chromedp.Poll(`document.querySelector('.language-javascript')?.textContent.includes('const value0 = 0;')`, nil, chromedp.WithPollingTimeout(10*time.Second))); err != nil {
					return err
				}
				return check(`document.querySelector('.language-javascript').textContent.includes('const value0 = 0;')`)
			})
			sc.Step(`^long code can be expanded and collapsed$`, func() error {
				if err := chromedp.Run(tab, chromedp.Click(".browser-code .tool-overflow-toggle")); err != nil {
					return err
				}
				if err := check(`document.querySelector('.browser-code .tool-overflow-toggle').getAttribute('aria-expanded') === 'true' && getComputedStyle(document.querySelector('.browser-code .browser-content-viewport')).overflowY === 'auto'`); err != nil {
					return err
				}
				return chromedp.Run(tab, chromedp.Click(".browser-code .tool-overflow-toggle"))
			})
			sc.Step(`^scroll offsets appear inside a screen diagram$`, func() error {
				return check(`document.querySelector('.browser-scroll-screen').textContent.includes('X: -120 px') && !!document.querySelector('.browser-scroll-arrow') && getComputedStyle(document.querySelector('.browser-scroll-screen')).borderTopStyle === 'solid'`)
			})
			sc.Step(`^text results and page log errors remain visible$`, func() error {
				return check(`document.body.textContent.includes('button "Submit"') && !!document.querySelector('.browser-log-line--error')`)
			})
		},
	}
	status := suite.Run()
	if dir := os.Getenv("FOXXYCODE_BROWSER_SCREENSHOTS"); dir != "" {
		for _, theme := range []string{"dark", "light"} {
			for _, width := range []int64{390, 1280} {
				var png []byte
				if err := chromedp.Run(tab, chromedp.EmulateViewport(width, 900), chromedp.Navigate(server.baseURL+"/browser-tools-check.html?lang=ru&theme="+theme), chromedp.WaitReady(".foxxycode-tool-details"), chromedp.Evaluate(`document.querySelectorAll('.foxxycode-tool-details').forEach(el => { el.open = true; }); document.querySelectorAll('img').forEach(el => {el.loading = 'eager';})`, nil), chromedp.Poll(`document.querySelector('.language-javascript')?.textContent.includes('const value0 = 0;') && [...document.images].every(img => img.naturalWidth > 0)`, nil, chromedp.WithPollingTimeout(10*time.Second)), chromedp.FullScreenshot(&png, 90)); err != nil {
					t.Fatal(err)
				}
				if err := check(`document.documentElement.scrollWidth <= window.innerWidth`); err != nil {
					var overflow string
					_ = chromedp.Run(tab, chromedp.Evaluate(`JSON.stringify([...document.querySelectorAll('*')].filter(el => el.getBoundingClientRect().right > window.innerWidth).slice(0,12).map(el => ({tag:el.tagName, cls:el.className, right:el.getBoundingClientRect().right, width:el.getBoundingClientRect().width})))`, &overflow))
					t.Log(overflow)
					t.Fatal(err)
				}
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("browser-tools-%s-%d.png", theme, width)), png, 0o600); err != nil {
					t.Fatal(err)
				}
				for _, card := range []string{"browser-1", "browser-6"} {
					selector := `[data-testid="tool-details-` + card + `"]`
					if err := chromedp.Run(tab, chromedp.ScrollIntoView(selector), chromedp.Screenshot(selector, &png, chromedp.NodeVisible)); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%s-%s-%d.png", card, theme, width)), png, 0o600); err != nil {
						t.Fatal(err)
					}
					if card == "browser-1" {
						if err := chromedp.Run(tab, chromedp.Click(".browser-code .tool-overflow-toggle"), chromedp.Screenshot(selector, &png, chromedp.NodeVisible)); err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%s-expanded-%s-%d.png", card, theme, width)), png, 0o600); err != nil {
							t.Fatal(err)
						}
						if err := chromedp.Run(tab, chromedp.Click(".browser-code .tool-overflow-toggle")); err != nil {
							t.Fatal(err)
						}
					}
				}
				if err := chromedp.Run(tab, chromedp.Evaluate(`document.querySelectorAll('.foxxycode-tool-details').forEach(el => {el.open=false;})`, nil), chromedp.FullScreenshot(&png, 90)); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("compact-%s-%d.png", theme, width)), png, 0o600); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	if status != 0 {
		t.Fatal("browser tool presentation feature failed")
	}
}
