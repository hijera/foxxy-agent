package config

import "testing"

func TestConfigJSONRoundTripNotifications(t *testing.T) {
	off := false
	paths := Paths{Home: t.TempDir(), CWD: t.TempDir()}
	on := true
	j := ConfigJSON{UI: UIJSON{Notifications: &off, NotificationSound: &off, NotificationsWhenFocused: &on, NotificationSoundFile: "sounds/alert.wav"}}
	out := ConfigToJSONDTO(JSONDTOToConfig(&j, paths)).UI
	if out.NotificationSoundFile != "sounds/alert.wav" {
		t.Fatal("custom sound lost")
	}
	if out.Notifications == nil || *out.Notifications || out.NotificationSound == nil || *out.NotificationSound {
		t.Fatalf("disabled preferences lost: %#v", out)
	}
	if out.NotificationsWhenFocused == nil || !*out.NotificationsWhenFocused {
		t.Fatal("foreground notification preference lost")
	}
	j.UI.NotificationsWhenFocused = &off
	out = ConfigToJSONDTO(JSONDTOToConfig(&j, paths)).UI
	if out.NotificationsWhenFocused == nil || *out.NotificationsWhenFocused {
		t.Fatal("explicit background-only preference lost")
	}
	out = ConfigToJSONDTO(JSONDTOToConfig(&ConfigJSON{}, paths)).UI
	if out.Notifications != nil || out.NotificationSound != nil || out.NotificationsWhenFocused != nil {
		t.Fatal("unset preferences became explicit values")
	}
}
