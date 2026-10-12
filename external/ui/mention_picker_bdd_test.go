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
	defer func() { _ = chromedp.Cancel(tab); stopTab() }()
	var state, theme string
	var width int64
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
		return os.WriteFile(filepath.Join(dir, fmt.Sprintf("mentions-%s-%s-%s-%d.png", stage, state, theme, width)), png, 0o600)
	}
	suite := godog.TestSuite{
		Name:    "composer-mention-layout",
		Options: &godog.Options{Format: "pretty", Paths: []string{"../../features/composer_mention_layout.feature"}, TestingT: t, Strict: true},
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			sc.Step(`^an (empty|docked) composer at (\d+) pixels in the (dark|light) theme$`, func(s string, w int, th string) error {
				state, width, theme = s, int64(w), th
				url := server.baseURL + "/mention-picker-check.html?state=" + state + "&theme=" + theme
				// The component mounts independently of unrelated page-load requests.
				// Re-evaluate in the current document while navigation replaces its context.
				return chromedp.Run(tab, chromedp.EmulateViewport(width, 900), chromedp.ActionFunc(func(ctx context.Context) error {
					if _, _, _, err := page.Navigate(url).Do(ctx); err != nil {
						return err
					}
					timer := time.NewTimer(10 * time.Second)
					defer timer.Stop()
					ticker := time.NewTicker(100 * time.Millisecond)
					defer ticker.Stop()
					for {
						var ready bool
						if err := chromedp.Evaluate(`(()=>{const ta=document.querySelector('#composer');return !!ta&&ta.value==='';})()`, &ready).Do(ctx); err == nil && ready {
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
				return check(`(()=>{const menu=document.querySelector('[data-testid="workspace-files-menu"]').getBoundingClientRect(),field=document.querySelector('.composer-field-wrap').getBoundingClientRect();return menu.bottom<=field.top-1&&menu.top>=0&&menu.left>=0&&menu.right<=innerWidth+1})()`)
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
