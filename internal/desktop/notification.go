package desktop

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
)

// Notification is the host bridge payload. It contains presentation data only;
// actions return opaque IDs to the UI, which owns the pending permission or plan.
type Notification struct {
	ID          string               `json:"id"`
	Title       string               `json:"title"`
	Body        string               `json:"body"`
	Silent      bool                 `json:"silent"`
	WhenFocused bool                 `json:"whenFocused"`
	SoundData   string               `json:"soundData,omitempty"`
	Actions     []NotificationAction `json:"actions"`
}

type NotificationAction struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

func escapeNotificationText(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// XML builds a ToastGeneric payload for Windows 10 and 11. Text and arguments
// are escaped independently so model output can never become toast markup.
func (n Notification) XML() (string, error) {
	if n.ID == "" || len(n.ID) > 256 || strings.ContainsAny(n.ID, ":\x00") || len(n.Actions) > 5 {
		return "", fmt.Errorf("invalid notification ID or action count")
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<toast launch="%s"><visual><binding template="ToastGeneric"><text>%s</text><text>%s</text></binding></visual>`, escapeNotificationText(n.ID), escapeNotificationText(n.Title), escapeNotificationText(n.Body))
	if n.Silent || n.SoundData != "" {
		b.WriteString(`<audio silent="true"/>`)
	}
	b.WriteString(`<actions>`)
	seen := make(map[string]bool)
	for _, a := range n.Actions {
		if a.ID == "" || len(a.ID) > 256 || strings.ContainsAny(a.ID, ":\x00") || seen[a.ID] {
			return "", fmt.Errorf("invalid notification action ID")
		}
		seen[a.ID] = true
		fmt.Fprintf(&b, `<action content="%s" arguments="%s:%s" activationType="foreground"/>`, escapeNotificationText(a.Label), escapeNotificationText(n.ID), escapeNotificationText(a.ID))
	}
	b.WriteString(`</actions></toast>`)
	return b.String(), nil
}
