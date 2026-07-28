//go:build desktop && windows

package desktop

import (
	"errors"
	"strings"

	"github.com/jchv/go-webview2"
)

var ErrStandaloneUnavailable = errors.New("standalone desktop window is unavailable")

// RunStandalone opens a single-purpose WebView2 window for a generated mini app.
func RunStandalone(title, url string) error {
	window := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug: false, AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title: strings.TrimSpace(title), Width: 920, Height: 760, Center: true, IconId: 1,
		},
	})
	if window == nil {
		return ErrStandaloneUnavailable
	}
	defer window.Destroy()
	window.Navigate(url)
	window.Run()
	return nil
}
