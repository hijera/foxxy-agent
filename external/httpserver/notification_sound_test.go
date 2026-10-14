//go:build http

package httpserver

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/notificationsound"
)

func TestNotificationSoundErrors(t *testing.T) {
	for _, tc := range []struct {
		name, method string
		body         []byte
		selected     string
		status       int
	}{
		{"invalid upload", http.MethodPost, []byte("not audio"), "", http.StatusUnsupportedMediaType},
		{"large upload", http.MethodPost, make([]byte, notificationsound.MaxBytes+1), "", http.StatusRequestEntityTooLarge},
		{"selected file outside the sounds directory", http.MethodGet, nil, "outside.wav", http.StatusForbidden},
		{"missing selected file", http.MethodGet, nil, "missing.wav", http.StatusNotFound},
		{"invalid selected file", http.MethodGet, []byte("not audio"), "invalid.wav", http.StatusUnsupportedMediaType},
		{"large selected file", http.MethodGet, make([]byte, notificationsound.MaxBytes+1), "large.wav", http.StatusRequestEntityTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			selected := tc.selected
			if tc.method == http.MethodGet {
				// Only uploads under the agent's notification-sounds directory
				// are served; the "outside" case keeps its path under home.
				if tc.status == http.StatusForbidden {
					selected = filepath.Join(home, tc.selected)
				} else {
					selected = filepath.Join(home, "notification-sounds", tc.selected)
				}
				if tc.body != nil {
					if err := os.MkdirAll(filepath.Dir(selected), 0o700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(selected, tc.body, 0o600); err != nil {
						t.Fatal(err)
					}
				}
			}
			cfg := &config.Config{Paths: config.Paths{Home: home}, UI: config.UIConfig{NotificationSoundFile: selected}}
			s := New(cfg, nil, slog.Default(), home)
			defer s.Drain()
			res := httptest.NewRecorder()
			s.Handler().ServeHTTP(res, httptest.NewRequest(tc.method, "/foxxycode/notification-sound?filename=alert.wav", bytes.NewReader(tc.body)))
			if res.Code != tc.status {
				t.Fatalf("status %d, want %d: %s", res.Code, tc.status, res.Body.String())
			}
		})
	}
}
