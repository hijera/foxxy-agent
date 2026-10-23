//go:build http && ui && browser

package ui

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/chromedp/chromedp"
	"github.com/cucumber/godog"
)

func TestLiveStatusLayoutFeature(t *testing.T) {
	server := startLayoutDevServer(t)
	t.Cleanup(func() { server.stop(t) })
	var browserLog bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), layoutBrowserTimeout)
	defer cancel()
	allocator, stopAllocator := chromedp.NewExecAllocator(ctx, headlessAllocatorOptions(t.TempDir(), &browserLog)...)
	defer stopAllocator()
	tab, stopTab := chromedp.NewContext(allocator)
	defer stopTab()

	suite := godog.TestSuite{
		Name: "live_status_layout",
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			sc.Step(`^the command clock and turn totals stay separated at phone and desktop widths in both locales and themes$`, func() error {
				var failures []string
				for _, lang := range []string{"ru", "en"} {
					for _, theme := range []string{"light", "dark"} {
						for _, width := range []int64{360, 390, 468, 599, 600, 834, 1280} {
							var geometry struct {
								Separated bool   `json:"separated"`
								Fits      bool   `json:"fits"`
								Text      string `json:"text"`
							}
							url := fmt.Sprintf("%s/phone-overflow-check.html?lang=%s&theme=%s", server.baseURL, lang, theme)
							if err := chromedp.Run(tab, chromedp.EmulateViewport(width, 900), chromedp.Navigate(url),
								chromedp.WaitVisible(`[data-testid="long-tool-status"]`, chromedp.ByQuery),
								chromedp.Poll(`getComputedStyle(document.querySelector('.messages-inner')).opacity === '1'`, nil),
								chromedp.Evaluate(liveStatusGeometry, &geometry)); err != nil {
								return fmt.Errorf("measure %dpx %s %s: %w\n%s", width, lang, theme, err, browserLog.String())
							}
							// Optional visual evidence uses the same real component and CSS as the assertions.
							if dir := os.Getenv("FOXXYCODE_BROWSER_SCREENSHOTS"); dir != "" && lang == "ru" {
								var png []byte
								if err := chromedp.Run(tab, chromedp.Screenshot(`[data-testid="long-tool-status"]`, &png, chromedp.NodeVisible)); err != nil {
									return err
								}
								name := fmt.Sprintf("status-timers-%s-%d.png", theme, width)
								if err := os.WriteFile(filepath.Join(dir, name), png, 0o600); err != nil {
									return err
								}
							}
							if !geometry.Separated || !geometry.Fits {
								failures = append(failures, fmt.Sprintf("%dpx %s %s: separated=%t fits=%t text=%q", width, lang, theme, geometry.Separated, geometry.Fits, geometry.Text))
							}
						}
					}
				}
				if len(failures) > 0 {
					return fmt.Errorf("status clocks overlap or overflow: %v", failures)
				}
				return nil
			})
		},
		Options: &godog.Options{Format: "pretty", Paths: []string{"../../features/live_status_layout.feature"}, TestingT: t, Strict: true},
	}
	if suite.Run() != 0 {
		t.Fatal("live status layout feature failed")
	}
}

const liveStatusGeometry = `(() => {
  const root = document.querySelector('[data-testid="long-tool-status"]');
  const bounds = selector => root.querySelector(selector).getBoundingClientRect();
  const phrase = bounds('.typing-dots-status-text');
  const step = bounds('[data-testid="typing-dots-elapsed"]');
  const turn = bounds('.typing-dots-turn');
  const phone = innerWidth < 600;
  return {
    separated: phone
      ? turn.top >= Math.max(phrase.bottom, step.bottom) - 1
      : turn.right <= phrase.left + 1 && phrase.right <= step.left + 1 && Math.abs(turn.top - step.top) < 3,
    fits: [...root.querySelectorAll('span')].every(el => {
      const r = el.getBoundingClientRect();
      return r.left >= -1 && r.right <= innerWidth + 1;
    }),
    text: root.innerText
  };
})()`
