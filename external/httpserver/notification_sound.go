//go:build http

package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/hijera/foxxycode-agent/internal/notificationsound"
)

func (s *Server) registerNotificationSoundRoutes() {
	s.mux.HandleFunc("POST /foxxycode/notification-sound", s.notificationSoundPost)
	s.mux.HandleFunc("GET /foxxycode/notification-sound", s.notificationSoundGet)
}

// Uploading does not select the sound. The settings form saves its path through config PUT.
func (s *Server) notificationSoundPost(w http.ResponseWriter, r *http.Request) {
	cfg := s.activeCfg()
	if cfg == nil {
		writeFoxxyCodeConfigErr(w, 500, "config unavailable")
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, notificationsound.MaxBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeFoxxyCodeConfigErr(w, 413, "sound exceeds 5 MiB")
		} else {
			writeFoxxyCodeConfigErr(w, 400, "read sound")
		}
		return
	}
	path, err := notificationsound.Store(cfg.Paths.Home, r.URL.Query().Get("filename"), data)
	if err != nil {
		if errors.Is(err, notificationsound.ErrFormat) {
			writeFoxxyCodeConfigErr(w, 415, "use an MP3 or WAV file")
		} else {
			writeFoxxyCodeConfigErr(w, 500, "save sound")
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"file": path})
}

func (s *Server) notificationSoundGet(w http.ResponseWriter, r *http.Request) {
	cfg := s.activeCfg()
	if cfg == nil {
		writeFoxxyCodeConfigErr(w, 500, "config unavailable")
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	// Uploads are validated by header sniffing only; keep browsers from
	// re-sniffing them into something executable, as sibling file routes do.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if cfg.UI.NotificationSoundFile == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	data, kind, err := notificationsound.Load(cfg.Paths.Home, cfg.UI.NotificationSoundFile)
	if err != nil {
		switch {
		case errors.Is(err, notificationsound.ErrLocation):
			writeFoxxyCodeConfigErr(w, 403, "sound path is outside the notification-sounds directory")
		case errors.Is(err, os.ErrNotExist):
			writeFoxxyCodeConfigErr(w, 404, "sound file not found")
		case errors.Is(err, notificationsound.ErrFormat):
			writeFoxxyCodeConfigErr(w, 415, "use an MP3 or WAV file")
		case errors.Is(err, notificationsound.ErrTooLarge):
			writeFoxxyCodeConfigErr(w, 413, "sound exceeds 5 MiB")
		default:
			writeFoxxyCodeConfigErr(w, 500, "read sound")
		}
		return
	}
	mime := "audio/mpeg"
	if kind == "wav" {
		mime = "audio/wav"
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	_, _ = w.Write(data)
}
