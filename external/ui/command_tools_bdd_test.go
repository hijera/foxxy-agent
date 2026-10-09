//go:build http && ui && browser

package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/cucumber/godog"
)

func TestCommandToolLayoutFeature(t *testing.T) {
	server := startLayoutDevServer(t)
	t.Cleanup(func() { server.stop(t) })
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	alloc, stopAlloc := chromedp.NewExecAllocator(ctx, headlessAllocatorOptions(t.TempDir(), nil)...)
	defer stopAlloc()
	tab, stopTab := chromedp.NewContext(alloc)
	defer func() {
		// Close Chrome before its profile is removed; Crashpad can otherwise
		// keep the metrics file open during TempDir cleanup on Windows.
		if err := chromedp.Cancel(tab); err != nil && ctx.Err() == nil {
			t.Logf("close command layout browser: %v", err)
		}
		stopTab()
	}()
	check := func(expression string) error {
		var ok bool
		if err := chromedp.Run(tab, chromedp.Evaluate(expression, &ok)); err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("command layout assertion failed: %s", expression)
		}
		return nil
	}
	var width int64
	var theme string
	capture := func(state string) error {
		dir := os.Getenv("FOXXYCODE_COMMAND_SCREENSHOTS")
		if dir == "" {
			return nil
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		var png []byte
		if err := chromedp.Run(tab, chromedp.FullScreenshot(&png, 90)); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("command-tools-%s-%s-%d.png", state, theme, width)), png, 0o600); err != nil {
			return err
		}
		if state == "open" {
			if err := chromedp.Run(tab, chromedp.Evaluate(`window.scrollTo(0, 0)`, nil), chromedp.CaptureScreenshot(&png)); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(dir, fmt.Sprintf("command-tools-overview-%s-%d.png", theme, width)), png, 0o600)
		}
		return nil
	}
	const longCard = `[data-testid="tool-details-long"]`
	const outputCard = `[data-testid="tool-details-output"]`
	suite := godog.TestSuite{
		Name:    "command-tool-layout",
		Options: &godog.Options{Format: "pretty", Paths: []string{"../../features/command_tool_layout.feature"}, TestingT: t, Strict: true},
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			sc.Step(`^command tool cards at (\d+) pixels in the (dark|light) theme$`, func(w int, th string) error {
				width, theme = int64(w), th
				return chromedp.Run(tab, chromedp.EmulateViewport(width, 900), chromedp.Navigate(server.baseURL+"/command-tools-check.html?lang=ru&theme="+theme), chromedp.WaitReady(".foxxycode-tool-details"))
			})
			sc.Step(`^I expand the command tool cards$`, func() error {
				if err := capture("closed"); err != nil {
					return err
				}
				if err := chromedp.Run(tab, chromedp.Evaluate(`document.querySelectorAll('.foxxycode-tool-details').forEach(el => {el.open = true;})`, nil), chromedp.WaitVisible(".tool-result-pre")); err != nil {
					return err
				}
				return capture("open")
			})
			sc.Step(`^command output starts below its argument preview$`, func() error {
				return check(`Array.from(document.querySelectorAll('.tool-call-result-card')).every(result => {
					const preview = result.previousElementSibling;
					const line = result.querySelector('.tool-result-pre');
					return result.getBoundingClientRect().top >= preview.getBoundingClientRect().bottom - 1 && line.getBoundingClientRect().top >= result.getBoundingClientRect().top;
				})`)
			})
			sc.Step(`^short commands need no overflow toggle$`, func() error {
				return check(`!document.querySelector('[data-testid="tool-details-short"] [data-testid="tool-preview-more"]') && document.querySelector('[data-testid="tool-details-failed"] .tool-failed-marker') && document.querySelector('[data-testid="tool-details-short"] .tool-call-result-card').getBoundingClientRect().height < 80`)
			})
			sc.Step(`^long commands can be expanded, scrolled and collapsed$`, func() error {
				if err := chromedp.Run(tab, chromedp.Click(longCard+` [data-testid="tool-preview-more"]`), chromedp.WaitVisible(longCard+` .permission-preview-viewport--scroll`)); err != nil {
					return err
				}
				if err := check(`(() => { const el = document.querySelector('` + longCard + ` .permission-preview-viewport'); el.scrollTop = el.scrollHeight; return el.scrollTop > 0 && el.clientHeight <= 190 && el.textContent.includes('Command line 40'); })()`); err != nil {
					return err
				}
				if err := capture("command-expanded"); err != nil {
					return err
				}
				if err := chromedp.Run(tab, chromedp.Click(longCard+` [data-testid="tool-preview-less"]`), chromedp.WaitVisible(longCard+` .permission-preview-viewport--clip`)); err != nil {
					return err
				}
				return check(`document.querySelector('` + longCard + ` .permission-preview-viewport').scrollTop === 0`)
			})
			sc.Step(`^long output can be expanded, scrolled and collapsed$`, func() error {
				if err := chromedp.Run(tab, chromedp.Click(outputCard+` [data-testid="tool-result-more"]`), chromedp.WaitVisible(outputCard+` .tool-result-viewport--scroll`)); err != nil {
					return err
				}
				if err := check(`(() => { const el = document.querySelector('` + outputCard + ` .tool-call-result-content'); el.scrollTop = el.scrollHeight; return el.scrollTop > 0 && el.textContent.includes('Output line 80'); })()`); err != nil {
					return err
				}
				if err := capture("output-expanded"); err != nil {
					return err
				}
				if err := chromedp.Run(tab, chromedp.Click(outputCard+` [data-testid="tool-result-less"]`), chromedp.WaitVisible(outputCard+` .tool-result-viewport--clip`)); err != nil {
					return err
				}
				return check(`document.querySelector('` + outputCard + ` .tool-call-result-content').scrollTop === 0`)
			})
			sc.Step(`^command cards fit within the panel$`, func() error {
				return check(`document.documentElement.scrollWidth <= innerWidth && Array.from(document.querySelectorAll('.permission-preview-shell')).every(el => el.getBoundingClientRect().right <= innerWidth)`)
			})
			sc.Step(`^shell action labels remain readable beside long commands$`, func() error {
				return check(`['long', 'wrapped'].every(id => {
					const label = document.querySelector('[data-testid="tool-details-' + id + '"] .thinking-label');
					return label.clientWidth >= label.scrollWidth - 1;
				})`)
			})
		},
	}
	if suite.Run() != 0 {
		t.Fatal("command tool layout feature failed")
	}
}
