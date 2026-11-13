//go:build http && ui && browser

package ui

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/chromedp/chromedp"
	"github.com/cucumber/godog"
)

func TestHeroLayoutFeature(t *testing.T) {
	server := startLayoutDevServer(t)
	t.Cleanup(func() { server.stop(t) })
	ctx, cancel := context.WithTimeout(context.Background(), layoutBrowserTimeout)
	defer cancel()
	var browserLog bytes.Buffer
	alloc, stop := chromedp.NewExecAllocator(ctx, headlessAllocatorOptions(t.TempDir(), &browserLog)...)
	defer stop()
	tab, closeTab := chromedp.NewContext(alloc)
	defer closeTab()
	var edges struct {
		Hero, Title, Composer, Footer struct{ Top, Bottom float64 }
		Overflow                      float64
	}
	suite := godog.TestSuite{
		Name: "web_ui_hero_layout",
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			sc.Step(`^an empty editor panel measuring (\d+) by (\d+) pixels$`, func(width, height int64) error {
				return chromedp.Run(tab, chromedp.EmulateViewport(width, height), chromedp.Navigate(server.baseURL+"/hero-layout-check.html"), chromedp.WaitReady(".hero-footer", chromedp.ByQuery), chromedp.Evaluate(`(() => {
                  const rect = s => { const r = document.querySelector(s).getBoundingClientRect(); return {top:r.top,bottom:r.bottom,left:r.left,right:r.right}; };
                  return {hero:rect('.hero'),title:rect('.hero-title'),composer:rect('.composer-card'),footer:rect('.hero-footer'),overflow:document.documentElement.scrollWidth-innerWidth};
                })()`, &edges))
			})
			sc.Step(`^the title and composer are vertically centered$`, func() error {
				delta := math.Abs((edges.Title.Top+edges.Composer.Bottom)/2 - (edges.Hero.Top+edges.Hero.Bottom)/2)
				if delta > 32 {
					return fmt.Errorf("title/composer center is %.1fpx away from the panel center", delta)
				}
				return nil
			})
			sc.Step(`^the project footer is below the composer without horizontal overflow$`, func() error {
				if gap := edges.Footer.Top - edges.Composer.Bottom; gap < 12 {
					return fmt.Errorf("footer gap = %.1fpx, want at least 12px", gap)
				}
				if edges.Overflow > 1 {
					return fmt.Errorf("horizontal overflow = %.1fpx", edges.Overflow)
				}
				return nil
			})
		},
		Options: &godog.Options{Format: "pretty", Paths: []string{"../../features/web_ui_hero_layout.feature"}, TestingT: t},
	}
	if suite.Run() != 0 {
		t.Fatal("start-screen layout failed")
	}
}
