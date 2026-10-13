//go:build desktop && windows && amd64

package desktop

import (
	"encoding/json"
	"fmt"
	"os"
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
	n         Notification
	hide      string
	soundOnly bool
	stopSound bool
	reply     chan error
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
		player := &nativeSoundPlayer{}
		defer player.close()
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		var audioID string
		var audioWhenFocused bool
		live := make(map[string]nativeToast)
		remove := func(id string) {
			if item, ok := live[id]; ok {
				_ = notifier.Hide(&item.toast.IToastNotification)
				_ = item.toast.RemoveActivated(item.token)
				item.handler.Close()
				item.toast.Release()
				delete(live, id)
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
				if player.playing && !audioWhenFocused && nativeNotificationWindowActive(hwnd) {
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
						req.reply <- nil
					} else {
						req.reply <- player.play(req.n.SoundData)
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
					req.reply <- nil
					continue
				}
				remove(req.n.ID)
				if len(live) >= 128 {
					for id := range live {
						remove(id)
						break
					}
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
						showWindow := user32DLL.NewProc("ShowWindow")
						foreground := user32DLL.NewProc("SetForegroundWindow")
						_, _, _ = showWindow.Call(uintptr(w.Window()), 9) // SW_RESTORE
						_, _, _ = foreground.Call(uintptr(w.Window()))
						w.Eval("window.foxxycodeUi && window.foxxycodeUi.notificationAction.apply(null," + string(args) + ")")
					})
				})
				if err == nil {
					live[req.n.ID] = item
					if req.n.suppressed(nativeNotificationWindowActive(hwnd)) {
						remove(req.n.ID)
					} else if err = notifier.Show(&item.toast.IToastNotification); err != nil {
						remove(req.n.ID)
					} else {
						player.stop()
						if !req.n.Silent && req.n.SoundData != "" && nativeSoundAllowed() {
							if setting, settingErr := notifier.Setting(); settingErr == nil && setting == 0 {
								audioID, audioWhenFocused = req.n.ID, req.n.WhenFocused
								if audioErr := player.play(req.n.SoundData); audioErr != nil {
									_ = player.play("")
								}
							}
						}
					}
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
		err := send(notificationRequest{n: n})
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
		return send(notificationRequest{soundOnly: true, n: Notification{SoundData: data, WhenFocused: whenFocused}})
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

func registerNotificationIdentity(appID string) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\AppUserModelId\`+appID, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer func() { _ = key.Close() }()
	if err := key.SetStringValue("DisplayName", "FoxxyCode"); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return key.SetStringValue("IconUri", exe)
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
