//go:build desktop && windows && amd64

package desktop

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unsafe"

	syswinrt "github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/winrt"
	"github.com/deploymenttheory/go-bindings-winrt/bindings/runtime/winrt"
	"github.com/deploymenttheory/go-bindings-winrt/bindings/winrt/data/xml/dom"
	"github.com/deploymenttheory/go-bindings-winrt/bindings/winrt/ui/notifications"
	"github.com/jchv/go-webview2"
	"golang.org/x/sys/windows/registry"
)

const notificationAppID = "FoxxyCode.Desktop"

type notificationRequest struct {
	n            Notification
	prepared     preparedSound
	preparedDone bool // SoundData was staged by the bind goroutine
	hide         string
	soundOnly    bool
	stopSound    bool
	reply        chan error
}

type nativeToast struct {
	toast   *notifications.ToastNotification
	handler *notifications.TypedEventHandlerOfToastNotificationAndObject
	token   syswinrt.EventRegistrationToken
}

// installNotifications owns WinRT objects on one MTA thread. Activation only
// returns opaque IDs to the live UI; no permission can be granted by toast XML.
func installNotifications(w webview2.WebView) (func(), error) {
	hwnd := uintptr(w.Window())
	requests := make(chan notificationRequest)
	done := make(chan struct{})
	finished := make(chan struct{})
	ready := make(chan error, 1)
	// The player is shared: bind callbacks stage clips on their own goroutines
	// while only the worker thread below starts and stops playback.
	player := &nativeSoundPlayer{}
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(finished)
		if err := syswinrt.RoInitialize(syswinrt.RO_INIT_MULTITHREADED); err != nil {
			ready <- err
			return
		}
		defer syswinrt.RoUninitialize()
		if err := registerNotificationApp(); err != nil {
			ready <- err
			return
		}
		manager, err := notifications.ToastNotificationManagerStatics()
		if err != nil {
			ready <- err
			return
		}
		defer manager.Release()
		notifier, err := manager.CreateToastNotifierWithId(notificationAppID)
		if err != nil {
			ready <- err
			return
		}
		defer notifier.Release()
		defer player.close()
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		var audioID string
		var audioWhenFocused bool
		live := make(map[string]nativeToast)
		var liveOrder []string
		remove := func(id string) {
			if item, ok := live[id]; ok {
				_ = notifier.Hide(&item.toast.IToastNotification)
				_ = item.toast.RemoveActivated(item.token)
				item.handler.Close()
				item.toast.Release()
				delete(live, id)
				for i, existing := range liveOrder {
					if existing == id {
						liveOrder = append(liveOrder[:i], liveOrder[i+1:]...)
						break
					}
				}
			}
		}
		defer func() {
			for id := range live {
				remove(id)
			}
		}()
		ready <- nil
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				// Natural completion clears the flag, so the ticker idles
				// instead of polling focus for the rest of the session.
				if player.playing && (player.finished() || (!audioWhenFocused && nativeNotificationWindowActive(hwnd))) {
					player.stop()
					audioID = ""
				}
			case req := <-requests:
				if req.stopSound {
					player.stop()
					audioID = ""
					req.reply <- nil
					continue
				}
				if req.soundOnly {
					audioWhenFocused = req.n.WhenFocused
					audioID = ""
					if req.n.suppressed(nativeNotificationWindowActive(hwnd)) {
						player.discard(req.prepared)
						req.reply <- nil
					} else {
						req.reply <- player.play(req.prepared)
					}
					continue
				}
				if req.hide != "" {
					if audioID == req.hide {
						player.stop()
						audioID = ""
					}
					remove(req.hide)
					req.reply <- nil
					continue
				}
				// Focus can change after the SPA checks it. Suppression is handled
				// successfully so it cannot trigger an in-window fallback or sound.
				if req.n.suppressed(nativeNotificationWindowActive(hwnd)) {
					player.discard(req.prepared)
					req.reply <- nil
					continue
				}
				remove(req.n.ID)
				// Evict the oldest toast, matching the SPA's pending FIFO, so an
				// actionable notification is never the one unregistered.
				if len(live) >= 128 && len(liveOrder) > 0 {
					remove(liveOrder[0])
				}
				item, err := createNativeToast(req.n, func(arguments string) {
					select {
					case <-done:
						return
					default:
					}
					id, action, _ := strings.Cut(arguments, ":")
					args, _ := json.Marshal([]string{id, action})
					w.Dispatch(func() {
						bringWindowToFront(uintptr(w.Window()))
						w.Eval("window.foxxycodeUi && window.foxxycodeUi.notificationAction.apply(null," + string(args) + ")")
					})
				})
				if err == nil {
					live[req.n.ID] = item
					liveOrder = append(liveOrder, req.n.ID)
					if req.n.suppressed(nativeNotificationWindowActive(hwnd)) {
						remove(req.n.ID)
					} else if err = notifier.Show(&item.toast.IToastNotification); err != nil {
						remove(req.n.ID)
					} else {
						player.stop()
						if req.preparedDone && !req.n.Silent && nativeSoundAllowed() {
							if setting, settingErr := notifier.Setting(); settingErr == nil && setting == 0 {
								audioID, audioWhenFocused = req.n.ID, req.n.WhenFocused
								if audioErr := player.play(req.prepared); audioErr != nil {
									_ = player.play(preparedSound{system: true})
								}
							} else {
								player.discard(req.prepared)
							}
						} else {
							player.discard(req.prepared)
						}
					}
				} else {
					player.discard(req.prepared)
				}
				req.reply <- err
			}
		}
	}()
	if err := <-ready; err != nil {
		return nil, err
	}
	closeService := func() { close(done); <-finished }
	send := func(req notificationRequest) error {
		req.reply = make(chan error, 1)
		select {
		case requests <- req:
			return <-req.reply
		case <-done:
			return fmt.Errorf("notifications closed")
		}
	}
	if err := w.Bind("foxxycodeHostNotify", func(n Notification) (bool, error) {
		req := notificationRequest{n: n}
		if n.SoundData != "" {
			// Decode off the worker thread; a corrupt clip degrades to the
			// system ding instead of losing the toast.
			prep, prepErr := player.prepare(n.SoundData)
			if prepErr != nil {
				prep = preparedSound{system: true}
			}
			req.prepared, req.preparedDone = prep, true
		}
		err := send(req)
		return err == nil, err
	}); err != nil {
		closeService()
		return nil, err
	}
	if err := w.Bind("foxxycodeHostDismiss", func(id string) error {
		return send(notificationRequest{hide: id})
	}); err != nil {
		closeService()
		return nil, err
	}
	if err := w.Bind("foxxycodeHostPlaySound", func(data string, whenFocused bool) error {
		// Preview sound errors reach the settings form, so they are not degraded.
		prep, err := player.prepare(data)
		if err != nil {
			return err
		}
		return send(notificationRequest{soundOnly: true, prepared: prep, preparedDone: true, n: Notification{WhenFocused: whenFocused}})
	}); err != nil {
		closeService()
		return nil, err
	}
	if err := w.Bind("foxxycodeHostStopSound", func() error {
		return send(notificationRequest{stopSound: true})
	}); err != nil {
		closeService()
		return nil, err
	}
	return closeService, nil
}

func registerNotificationApp() error {
	return registerNotificationIdentity(notificationAppID)
}

//go:embed notification-icon.png
var notificationIconPNG []byte

// notificationIconFile materialises the toast logo once per user cache. The
// registry IconUri must name an image; the executable path is ignored by
// Windows and left the toast with a generic glyph.
func notificationIconFile() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cache, "FoxxyCode")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "notification-icon.png")
	info, statErr := os.Stat(path)
	if statErr != nil || info.Size() != int64(len(notificationIconPNG)) {
		if err := os.WriteFile(path, notificationIconPNG, 0o600); err != nil {
			return "", err
		}
	}
	return path, nil
}

func registerNotificationIdentity(appID string) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\AppUserModelId\`+appID, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer func() { _ = key.Close() }()
	if err := key.SetStringValue("DisplayName", "FoxxyCode"); err != nil {
		return err
	}
	icon, err := notificationIconFile()
	if err != nil {
		// The identity is still usable without a logo; Windows substitutes a
		// generic glyph rather than failing the toast.
		return nil
	}
	return key.SetStringValue("IconUri", icon)
}

func createNativeToast(n Notification, activated func(string)) (nativeToast, error) {
	text, err := n.XML()
	if err != nil {
		return nativeToast{}, err
	}
	doc, err := dom.NewXmlDocument()
	if err != nil {
		return nativeToast{}, err
	}
	defer doc.Release()
	loader, err := doc.AsXmlDocumentIO()
	if err != nil {
		return nativeToast{}, err
	}
	defer loader.Release()
	if err := loader.LoadXml(text); err != nil {
		return nativeToast{}, err
	}
	toast, err := notifications.CreateToastNotification(&doc.IXmlDocument)
	if err != nil {
		return nativeToast{}, err
	}
	handler, err := notifications.NewTypedEventHandlerOfToastNotificationAndObject(func(_ *notifications.IToastNotification, args *syswinrt.IInspectable) {
		event, err := winrt.QueryInterface[notifications.IToastActivatedEventArgs](unsafe.Pointer(args), &notifications.IID_IToastActivatedEventArgs)
		if err != nil {
			return
		}
		defer event.Release()
		if arguments, err := event.Arguments(); err == nil {
			activated(arguments)
		}
	})
	if err != nil {
		toast.Release()
		return nativeToast{}, err
	}
	token, err := toast.AddActivated(handler)
	if err != nil {
		handler.Close()
		toast.Release()
		return nativeToast{}, err
	}
	return nativeToast{toast: toast, handler: handler, token: token}, nil
}
