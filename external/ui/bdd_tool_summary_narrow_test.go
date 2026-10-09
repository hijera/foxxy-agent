//go:build http && ui && browser

package ui

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/cucumber/godog"
)

func TestToolSummaryNarrowFeature(t *testing.T) {
	server := startLayoutDevServer(t)
	t.Cleanup(func() { server.stop(t) })
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	alloc, stopAlloc := chromedp.NewExecAllocator(ctx, headlessAllocatorOptions(t.TempDir(), nil)...)
	defer stopAlloc()
	tab, stopTab := chromedp.NewContext(alloc)
	defer stopTab()
	check := func(expression string) error {
		var ok bool
		if err := chromedp.Run(tab, chromedp.Evaluate(expression, &ok)); err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("tool summary layout assertion failed: %s", expression)
		}
		return nil
	}
	suite := godog.TestSuite{
		Name: "tool-summary-narrow",
		Options: &godog.Options{
			Format: "pretty", Paths: []string{"../../features/tool_summary_narrow.feature"}, TestingT: t, Strict: true,
		},
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			sc.Step(`^tool summaries in "([^"]+)" at (\d+) pixels$`, func(locale string, width int) error {
				return chromedp.Run(tab,
					chromedp.EmulateViewport(int64(width), 800),
					chromedp.Navigate(server.baseURL+"/tool-summary-check.html?lang="+locale),
					chromedp.WaitVisible(".thinking-dur", chromedp.ByQuery),
				)
			})
			sc.Step(`^the tool summaries do not widen the page$`, func() error {
				return check(`document.documentElement.scrollWidth <= document.documentElement.clientWidth`)
			})
			sc.Step(`^every tool duration stays visible inside its summary$`, func() error {
				return check(`Array.from(document.querySelectorAll('.thinking-dur')).every(duration => {
					const box = duration.getBoundingClientRect();
					const row = duration.closest('summary').getBoundingClientRect();
					return box.width > 0 && box.left >= row.left && box.right <= row.right + 1;
				})`)
			})
		},
	}
	if suite.Run() != 0 {
		t.Fatal("tool summary narrow layout feature failed")
	}
}
