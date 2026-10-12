//go:build http && ui && browser

package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
	"github.com/cucumber/godog"
)

func TestComposerMentionLayoutFeature(t *testing.T) {
	server := startLayoutDevServer(t)
	t.Cleanup(func() { server.stop(t) })
	ctx, cancel := context.WithTimeout(context.Background(), layoutBrowserTimeout)
	defer cancel()
	alloc, stopAlloc := chromedp.NewExecAllocator(ctx, headlessAllocatorOptions(t.TempDir(), nil)...)
	defer stopAlloc()
	tab, stopTab := chromedp.NewContext(alloc)
	defer func() {
		// Close Chrome before its profile is removed; Crashpad can otherwise
		// keep the metrics file open during TempDir cleanup on Windows.
		if err := chromedp.Cancel(tab); err != nil && ctx.Err() == nil {
			t.Logf("close mention picker browser: %v", err)
		}
		stopTab()
	}()
	var state, theme string
	var width, height int64
	check := func(expression string) error {
		var ok bool
		if err := chromedp.Run(tab, chromedp.Evaluate(expression, &ok)); err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("mention picker assertion failed: %s", expression)
		}
		return nil
	}
	capture := func(stage string) error {
		dir := os.Getenv("FOXXYCODE_MENTION_SCREENSHOTS")
		if dir == "" {
			return nil
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		var png []byte
		if err := chromedp.Run(tab, chromedp.CaptureScreenshot(&png)); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, fmt.Sprintf("mentions-%s-%s-%s-%dx%d.png", stage, state, theme, width, height)), png, 0o600)
	}
	suite := godog.TestSuite{
		Name:    "composer-mention-layout",
		Options: &godog.Options{Format: "pretty", Paths: []string{"../../features/composer_mention_layout.feature"}, TestingT: t, Strict: true},
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			sc.Step(`^an (empty|docked) composer at (\d+) by (\d+) pixels in the (dark|light) theme$`, func(s string, w, h int, th string) error {
				state, width, height, theme = s, int64(w), int64(h), th
				url := server.baseURL + "/mention-picker-check.html?state=" + state + "&theme=" + theme
				// The component mounts independently of unrelated page-load requests.
				// Re-evaluate in the current document while navigation replaces its context.
				return chromedp.Run(tab, chromedp.EmulateViewport(width, height), chromedp.ActionFunc(func(ctx context.Context) error {
					if _, _, _, err := page.Navigate(url).Do(ctx); err != nil {
						return err
					}
					timer := time.NewTimer(10 * time.Second)
					defer timer.Stop()
					ticker := time.NewTicker(100 * time.Millisecond)
					defer ticker.Stop()
					for {
						var ready bool
						if err := chromedp.Evaluate(`(()=>{const ta=document.querySelector('#composer');return !!ta&&ta.value===''&&document.fonts.status==='loaded';})()`, &ready).Do(ctx); err == nil && ready {
							return nil
						}
						select {
						case <-ctx.Done():
							return ctx.Err()
						case <-timer.C:
							return fmt.Errorf("composer did not mount at %s", url)
						case <-ticker.C:
						}
					}
				}))
			})
			sc.Step(`^I open the mention picker$`, func() error {
				if err := chromedp.Run(tab, chromedp.Focus("#composer"), chromedp.ActionFunc(func(ctx context.Context) error { return input.InsertText("@").Do(ctx) }), chromedp.WaitVisible(`[data-testid="mention-row-directory-docs_"]`)); err != nil {
					return err
				}
				return capture("open")
			})
			sc.Step(`^the mention picker stays above the composer and inside the viewport$`, func() error {
				return chromedp.Run(tab, chromedp.Poll(`(()=>{const el=document.querySelector('[data-testid="workspace-files-menu"]'),menu=el.getBoundingClientRect(),scroll=el.querySelector('.slash-menu-scroll'),field=document.querySelector('.composer-field-wrap').getBoundingClientRect(),rail=document.querySelector('.rail-column').getBoundingClientRect(),top=innerWidth<1200?rail.bottom:0;return menu.bottom<=field.top-1&&menu.top>=top+7&&menu.left>=0&&menu.right<=innerWidth+1&&scroll.scrollWidth<=scroll.clientWidth+1})()`, nil, chromedp.WithPollingTimeout(2*time.Second)))
			})
			sc.Step(`^I reduce the window height to (\d+) pixels$`, func(h int) error {
				height = int64(h)
				return chromedp.Run(tab, chromedp.EmulateViewport(width, height), chromedp.Poll(`innerHeight===`+fmt.Sprint(h), nil))
			})
			sc.Step(`^I can scroll the list to its last candidate without scrolling the page$`, func() error {
				return check(`(()=>{const menu=document.querySelector('[data-testid="workspace-files-menu"]'),scroll=menu.querySelector('.slash-menu-scroll'),y=window.scrollY,lastRow=menu.querySelector('[data-testid="mention-row-directory-docs_"]');scroll.scrollTop=lastRow.offsetTop;const last=lastRow.getBoundingClientRect(),box=scroll.getBoundingClientRect();return scroll.scrollHeight>scroll.clientHeight&&scroll.scrollTop>0&&last.top>=box.top&&last.bottom<=box.bottom&&window.scrollY===y})()`)
			})
			sc.Step(`^I can click a folder and keep its mention in the draft$`, func() error {
				if err := chromedp.Run(tab, chromedp.ScrollIntoView(`[data-testid="mention-row-directory-docs_"]`), chromedp.Click(`[data-testid="mention-row-directory-docs_"]`), chromedp.WaitVisible(`[data-testid="mention-row-file-docs_README_md"]`)); err != nil {
					return err
				}
				if err := check(`document.querySelector('#composer').value==='@docs/'`); err != nil {
					return err
				}
				if err := capture("folder"); err != nil {
					return err
				}
				if err := chromedp.Run(tab, chromedp.KeyEvent(kb.Escape)); err != nil {
					return err
				}
				return check(`document.querySelector('#composer').value==='@docs/'`)
			})
			sc.Step(`^I can click a file inside that folder$`, func() error {
				// Focusing the existing folder token reopens its children.
				if err := chromedp.Run(tab, chromedp.Focus("#composer"), chromedp.ActionFunc(func(ctx context.Context) error { return input.InsertText("R").Do(ctx) }), chromedp.WaitVisible(`[data-testid="mention-row-file-docs_README_md"]`), chromedp.Click(`[data-testid="mention-row-file-docs_README_md"]`)); err != nil {
					return err
				}
				return check(`document.querySelector('#composer').value==='@docs/README.md '`)
			})
		},
	}
	if suite.Run() != 0 {
		t.Fatal("composer mention picker layout failed")
	}
}
