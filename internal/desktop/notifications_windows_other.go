//go:build desktop && windows && !amd64

package desktop

import (
	"fmt"
	"github.com/jchv/go-webview2"
)

func installNotifications(webview2.WebView) (func(), error) {
	return nil, fmt.Errorf("native notifications require the Windows amd64 desktop build")
}
