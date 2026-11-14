//go:build desktop && windows && amd64

package desktop

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
	"unsafe"

	syswinrt "github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/winrt"
	"github.com/deploymenttheory/go-bindings-winrt/bindings/runtime/winrt"
	"github.com/deploymenttheory/go-bindings-winrt/bindings/winrt/data/xml/dom"
	"github.com/deploymenttheory/go-bindings-winrt/bindings/winrt/ui/notifications"
	"golang.org/x/sys/windows/registry"
)

// This opt-in acceptance test checks the real Windows notification history.
// FOXXYCODE_TEST_NATIVE_ACTIONS additionally asks an operator to click Open.
// It never grants a permission or submits a real chat response.
func TestNativeNotificationDelivery(t *testing.T) {
	if os.Getenv("FOXXYCODE_TEST_NATIVE_NOTIFICATIONS") != "1" {
		t.Skip("interactive desktop test")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := syswinrt.RoInitialize(syswinrt.RO_INIT_MULTITHREADED); err != nil {
		t.Fatal(err)
	}
	defer syswinrt.RoUninitialize()
	appID := fmt.Sprintf("%s.Test.%d", notificationAppID, time.Now().UnixNano())
	if err := registerNotificationIdentity(appID); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = registry.DeleteKey(registry.CURRENT_USER, `Software\Classes\AppUserModelId\`+appID) }()
	manager, err := notifications.ToastNotificationManagerStatics()
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Release()
	notifier, err := manager.CreateToastNotifierWithId(appID)
	if err != nil {
		t.Fatal(err)
	}
	defer notifier.Release()
	activated := make(chan string, 1)
	n := Notification{ID: "native_test", Title: "FoxxyCode — notification test", Body: "Click Open to verify native action delivery.", Silent: true, Actions: []NotificationAction{{ID: "open", Label: "Open"}}}
	item, err := createNativeToast(n, func(args string) {
		select {
		case activated <- args:
		default:
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = notifier.Hide(&item.toast.IToastNotification)
		_ = item.toast.RemoveActivated(item.token)
		item.handler.Close()
		item.toast.Release()
	}()
	if err := notifier.Show(&item.toast.IToastNotification); err != nil {
		t.Fatal(err)
	}
	if setting, err := notifier.Setting(); err == nil {
		t.Logf("notification setting: %s", setting)
	}
	historyManager, err := notifications.ToastNotificationManagerStatics2()
	if err != nil {
		t.Fatal(err)
	}
	defer historyManager.Release()
	history, err := historyManager.History()
	if err != nil {
		t.Fatal(err)
	}
	defer history.Release()
	history2, err := winrt.QueryInterface[notifications.IToastNotificationHistory2](unsafe.Pointer(history), &notifications.IID_IToastNotificationHistory2)
	if err != nil {
		t.Fatal(err)
	}
	defer history2.Release()
	found := false
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline) && !found; {
		rows, err := history2.GetHistoryWithId(appID)
		if err != nil {
			t.Fatal(err)
		}
		count, err := rows.Size()
		if err != nil {
			rows.Release()
			t.Fatal(err)
		}
		for i := uint32(0); i < count; i++ {
			row, err := rows.GetAt(i)
			if err != nil {
				continue
			}
			content, err := row.Content()
			row.Release()
			if err != nil {
				continue
			}
			serializer, err := winrt.QueryInterface[dom.IXmlNodeSerializer](unsafe.Pointer(content), &dom.IID_IXmlNodeSerializer)
			content.Release()
			if err != nil {
				continue
			}
			text, err := serializer.GetXml()
			serializer.Release()
			if err == nil && strings.Contains(text, `launch="native_test"`) {
				found = true
			}
		}
		rows.Release()
		if !found {
			time.Sleep(100 * time.Millisecond)
		}
	}
	if !found {
		t.Fatal("toast was not delivered to Windows notification history")
	}
	t.Log("native toast found in Windows notification history")
	if os.Getenv("FOXXYCODE_TEST_NATIVE_ACTIONS") != "1" {
		return
	}
	t.Log("waiting for Open")
	select {
	case args := <-activated:
		if args != "native_test:open" {
			t.Fatalf("unexpected action: %s", args)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("no native action received")
	}
}
