//go:build desktop && windows

package desktop

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/ncruces/zenity"
)

// folderPicker opens the native Windows folder dialog (COM IFileOpenDialog
// with FOS_PICKFOLDERS via zenity). zenity locks an OS thread and
// initializes an STA apartment per call, so Pick is safe to invoke from
// HTTP handler goroutines without touching the WebView2 message loop.
type folderPicker struct {
	// hwnd is the WebView2 window handle, set once the window exists;
	// the dialog is owned by (modal to) that window.
	hwnd atomic.Uintptr
	// busy guards against stacked dialogs from double-clicks; the HTTP
	// layer has its own 409 guard, this is belt and braces.
	busy sync.Mutex
}

func (p *folderPicker) Pick(ctx context.Context) (string, bool, error) {
	if !p.busy.TryLock() {
		return "", false, errors.New("folder dialog already open")
	}
	defer p.busy.Unlock()
	opts := []zenity.Option{
		zenity.Directory(),
		zenity.Title("Open project folder"),
		zenity.Context(ctx),
	}
	if h := p.hwnd.Load(); h != 0 {
		opts = append(opts, zenity.Attach(h))
	}
	path, err := zenity.SelectFile(opts...)
	if errors.Is(err, zenity.ErrCanceled) || errors.Is(err, context.Canceled) {
		return "", true, nil
	}
	if err != nil {
		return "", false, err
	}
	return path, false, nil
}
