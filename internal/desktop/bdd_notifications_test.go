package desktop

import (
	"encoding/xml"
	"fmt"
	"testing"

	"github.com/cucumber/godog"
)

func TestDesktopNotificationsFeature(t *testing.T) {
	var n Notification
	var windowState notificationWindowState
	var parsed struct {
		Text  []string `xml:"visual>binding>text"`
		Audio struct {
			Silent string `xml:"silent,attr"`
		} `xml:"audio"`
		Actions []struct {
			Content string `xml:"content,attr"`
		} `xml:"actions>action"`
	}
	suite := godog.TestSuite{ScenarioInitializer: func(sc *godog.ScenarioContext) {
		sc.Step(`^FoxxyCode is the foreground window$`, func() {
			n = Notification{}
			windowState = notificationWindowState{foreground: 10, rootOwner: 10}
		})
		sc.Step(`^notifications in the active window are enabled$`, func() {
			n.WhenFocused = true
		})
		sc.Step(`^another application becomes the foreground window$`, func() {
			windowState = notificationWindowState{foreground: 20, rootOwner: 20}
		})
		sc.Step(`^a new desktop notification is (suppressed|allowed)$`, func(want string) error {
			if suppressed := n.suppressed(windowState.active(10)); suppressed != (want == "suppressed") {
				return fmt.Errorf("notification suppression = %v, want %s", suppressed, want)
			}
			return nil
		})
		sc.Step(`^a silent permission notification for chat "([^"]+)"$`, func(chat string) error {
			n = Notification{ID: "permission", Title: "FoxxyCode — " + chat, Body: "Permission required to continue", Silent: true,
				Actions: []NotificationAction{{ID: "allow", Label: "Allow"}, {ID: "reject", Label: "Reject"}}}
			return nil
		})
		sc.Step(`^the native notification payload is built$`, func() error {
			text, err := n.XML()
			if err != nil {
				return err
			}
			return xml.Unmarshal([]byte(text), &parsed)
		})
		sc.Step(`^the native notification names chat "([^"]+)"$`, func(chat string) error {
			if len(parsed.Text) != 2 || parsed.Text[0] != "FoxxyCode — "+chat {
				return fmt.Errorf("wrong toast title: %v", parsed.Text)
			}
			return nil
		})
		sc.Step(`^the native notification offers "([^"]+)" and "([^"]+)"$`, func(a, b string) error {
			if len(parsed.Actions) != 2 || parsed.Actions[0].Content != a || parsed.Actions[1].Content != b {
				return fmt.Errorf("wrong actions: %v", parsed.Actions)
			}
			return nil
		})
		sc.Step(`^the native notification is silent$`, func() error {
			if parsed.Audio.Silent != "true" {
				return fmt.Errorf("audio is not silent")
			}
			return nil
		})
	}, Options: &godog.Options{Format: "progress", Paths: []string{"../../features/desktop_notifications.feature"}, TestingT: t}}
	if suite.Run() != 0 {
		t.Fatal("desktop notifications feature failed")
	}
}
