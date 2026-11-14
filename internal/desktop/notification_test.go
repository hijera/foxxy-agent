package desktop

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestNotificationXML(t *testing.T) {
	n := Notification{ID: "perm_1", Title: "FoxxyCode — chat <one>", Body: "Allow A & B?", Silent: true,
		Actions: []NotificationAction{{ID: "allow_once", Label: "Allow"}, {ID: "reject_once", Label: "Reject"}}}
	s, err := n.XML()
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Launch string `xml:"launch,attr"`
		Audio  struct {
			Silent string `xml:"silent,attr"`
		} `xml:"audio"`
		Actions []struct {
			Arguments string `xml:"arguments,attr"`
		} `xml:"actions>action"`
	}
	if err := xml.Unmarshal([]byte(s), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Launch != "perm_1" || parsed.Audio.Silent != "true" || len(parsed.Actions) != 2 {
		t.Fatalf("unexpected toast: %s", s)
	}
	if !strings.Contains(s, "&lt;one&gt;") || !strings.Contains(s, "A &amp; B") {
		t.Fatal("text was not escaped")
	}
}

func TestCustomSoundMutesWindowsDefault(t *testing.T) {
	text, err := (Notification{ID: "custom", SoundData: "audio bytes"}).XML()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, `<audio silent="true"/>`) || strings.Contains(text, "audio bytes") {
		t.Fatal("custom playback must mute the system tone without embedding audio in XML")
	}
}

func TestNotificationRejectsInvalidActions(t *testing.T) {
	for _, n := range []Notification{
		{Title: "missing ID"},
		{ID: "one", Actions: []NotificationAction{{ID: "a", Label: "A"}, {ID: "a", Label: "B"}}},
		{ID: "one", Actions: []NotificationAction{{ID: "a:b", Label: "A"}}},
	} {
		if _, err := n.XML(); err == nil {
			t.Fatalf("accepted %#v", n)
		}
	}
}
