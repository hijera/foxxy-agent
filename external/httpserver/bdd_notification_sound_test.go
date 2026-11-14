//go:build http

package httpserver

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/cucumber/godog"
	"github.com/hijera/foxxycode-agent/internal/acp"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/session"
)

func notificationTestWAV() []byte {
	data := make([]byte, 46)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], 38)
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 1)
	binary.LittleEndian.PutUint32(data[24:], 8000)
	binary.LittleEndian.PutUint32(data[28:], 16000)
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], 2)
	return data
}

func TestNotificationSoundFeature(t *testing.T) {
	var srv *Server
	var selected string
	request := func(method, path string, body []byte) *httptest.ResponseRecorder {
		res := httptest.NewRecorder()
		srv.Handler().ServeHTTP(res, httptest.NewRequest(method, path, bytes.NewReader(body)))
		return res
	}
	save := func(path string) error {
		res := request(http.MethodGet, "/foxxycode/config", nil)
		var doc map[string]any
		if err := json.Unmarshal(res.Body.Bytes(), &doc); err != nil {
			return err
		}
		ui, _ := doc["ui"].(map[string]any)
		if ui == nil {
			ui = make(map[string]any)
			doc["ui"] = ui
		}
		ui["notification_sound_file"] = path
		body, err := json.Marshal(doc)
		if err != nil {
			return err
		}
		res = request(http.MethodPut, "/foxxycode/config", body)
		if res.Code != http.StatusOK {
			return fmt.Errorf("save status %d: %s", res.Code, res.Body.String())
		}
		return nil
	}
	suite := godog.TestSuite{ScenarioInitializer: func(sc *godog.ScenarioContext) {
		sc.Step(`^FoxxyCode has no custom notification sound$`, func() error {
			home := t.TempDir()
			path := filepath.Join(home, "config.yaml")
			if err := os.WriteFile(path, []byte("ui:\n  locale: en\n"), 0o600); err != nil {
				return err
			}
			cfg, err := config.LoadWithPaths(config.Paths{Home: home, CWD: home, ConfigPath: path})
			if err != nil {
				return err
			}
			runner := func(context.Context, *session.State, []acp.ContentBlock, acp.UpdateSender) (string, error) {
				return "", nil
			}
			mgr := session.NewManager(cfg, noopSender{}, runner, slog.Default(), home, nil)
			srv = New(cfg, mgr, slog.Default(), home)
			t.Cleanup(srv.Drain)
			return nil
		})
		sc.Step(`^I upload a WAV notification sound and select it in settings$`, func() error {
			res := request(http.MethodPost, "/foxxycode/notification-sound?filename=alert.wav", notificationTestWAV())
			if res.Code != http.StatusOK {
				return fmt.Errorf("upload status %d", res.Code)
			}
			var answer struct {
				File string `json:"file"`
			}
			if err := json.Unmarshal(res.Body.Bytes(), &answer); err != nil {
				return err
			}
			selected = answer.File
			return save(selected)
		})
		sc.Step(`^the selected audio is available for notification playback$`, func() error {
			res := request(http.MethodGet, "/foxxycode/notification-sound", nil)
			if res.Code != http.StatusOK || !bytes.Equal(res.Body.Bytes(), notificationTestWAV()) {
				return fmt.Errorf("playback status %d", res.Code)
			}
			return nil
		})
		sc.Step(`^I restore the standard notification sound$`, func() error { return save("") })
		sc.Step(`^no custom audio is selected$`, func() error {
			if res := request(http.MethodGet, "/foxxycode/notification-sound", nil); res.Code != http.StatusNoContent {
				return fmt.Errorf("reset status %d", res.Code)
			}
			return nil
		})
	}, Options: &godog.Options{Format: "progress", Paths: []string{"../../features/notification_sound.feature"}, TestingT: t}}
	if suite.Run() != 0 {
		t.Fatal("notification sound feature failed")
	}
}
