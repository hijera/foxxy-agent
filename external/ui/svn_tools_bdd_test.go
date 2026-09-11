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

func TestSVNToolPresentationFeature(t *testing.T) {
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
	check := func(expression string) error {
		var ok bool
		if err := chromedp.Run(tab, chromedp.Evaluate(expression, &ok)); err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("SVN presentation assertion failed: %s", expression)
		}
		return nil
	}
	suite := godog.TestSuite{
		Name:    "svn-tool-presentation",
		Options: &godog.Options{Format: "pretty", Paths: []string{"../../features/svn_tool_presentation.feature"}, TestingT: t, Strict: true},
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			sc.Step(`^a transcript containing SVN tool calls$`, func() error {
				return chromedp.Run(tab, chromedp.EmulateViewport(1280, 900), chromedp.Navigate(server.baseURL+"/svn-tools-check.html"), chromedp.WaitReady(".foxxycode-tool-details"))
			})
			sc.Step(`^I expand the SVN calls$`, func() error {
				return chromedp.Run(tab, chromedp.Evaluate(`document.querySelectorAll('.foxxycode-tool-details').forEach(el => {el.open = true;})`, nil))
			})
			sc.Step(`^SVN operations have readable headings and paths$`, func() error {
				return check(`document.querySelector('.thinking-label').textContent === 'SVN · Working copy info' && document.body.textContent.includes('branches/encoding')`)
			})
			sc.Step(`^SVN status distinguishes property and tree conflicts$`, func() error {
				return check(`document.querySelectorAll('.svn-status-row--conflict').length === 2`)
			})
			sc.Step(`^SVN diffs retain their content with highlighted changes$`, func() error {
				return check(`document.querySelector('.svn-diff-line--add').textContent.includes('decoder.Decode(data)') && document.querySelector('.svn-diff').textContent.includes('Index: internal/textenc/decode.go') && document.querySelector('.svn-diff').getBoundingClientRect().height > 100`)
			})
			sc.Step(`^SVN errors and commit approvals are clearly presented$`, func() error {
				return check(`document.querySelector('.svn-action--failed').textContent.includes('E155015') && document.querySelector('.svn-permission-preview').textContent.includes('fix: preserve Windows-1251 encoding')`)
			})
		},
	}
	status := suite.Run()
	if dir := os.Getenv("FOXXYCODE_SVN_SCREENSHOTS"); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, theme := range []string{"dark", "light"} {
			for _, width := range []int64{390, 1280} {
				var png []byte
				if err := chromedp.Run(tab, chromedp.EmulateViewport(width, 900), chromedp.Navigate(server.baseURL+"/svn-tools-check.html?lang=ru&theme="+theme), chromedp.WaitReady(".foxxycode-tool-details"), chromedp.Evaluate(`document.querySelectorAll('.foxxycode-tool-details').forEach(el => {el.open=true;})`, nil), chromedp.FullScreenshot(&png, 90)); err != nil {
					t.Fatal(err)
				}
				if err := check(`document.documentElement.scrollWidth <= window.innerWidth`); err != nil {
					var overflow string
					_ = chromedp.Run(tab, chromedp.Evaluate(`JSON.stringify([...document.querySelectorAll('*')].filter(el => el.getBoundingClientRect().right > innerWidth).slice(0,15).map(el => ({tag:el.tagName, cls:el.className, width:el.getBoundingClientRect().width})))`, &overflow))
					t.Log(overflow)
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("svn-tools-%s-%d.png", theme, width)), png, 0o600); err != nil {
					t.Fatal(err)
				}
				for _, card := range []string{"svn-1", "svn-2", "svn-3", "svn-4"} {
					selector := `[data-testid="tool-details-` + card + `"]`
					if err := chromedp.Run(tab, chromedp.ScrollIntoView(selector), chromedp.Screenshot(selector, &png, chromedp.NodeVisible)); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%s-%s-%d.png", card, theme, width)), png, 0o600); err != nil {
						t.Fatal(err)
					}
					if card == "svn-1" || card == "svn-2" {
						button := selector + " .browser-content .tool-overflow-toggle"
						if err := chromedp.Run(tab, chromedp.Click(button), chromedp.Screenshot(selector, &png, chromedp.NodeVisible)); err != nil {
							t.Fatal(err)
						}
						if err := check(`document.querySelector('` + button + `').getAttribute('aria-expanded') === 'true'`); err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%s-expanded-%s-%d.png", card, theme, width)), png, 0o600); err != nil {
							t.Fatal(err)
						}
						if err := chromedp.Run(tab, chromedp.Click(button)); err != nil {
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
		t.Fatal("SVN tool presentation feature failed")
	}
}
