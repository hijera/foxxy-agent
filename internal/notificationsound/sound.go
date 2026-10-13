// Package notificationsound stores bounded audio uploads and reads the selected sound.
package notificationsound

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

const MaxBytes = 5 << 20

var (
	ErrTooLarge = errors.New("notification sound exceeds 5 MiB")
	ErrFormat   = errors.New("notification sound must be MP3 or WAV")
)

func Format(data []byte) (string, error) {
	if len(data) > MaxBytes {
		return "", ErrTooLarge
	}
	// MP3 files may start directly with a Layer III frame instead of an ID3 tag.
	if len(data) >= 4 && data[0] == 0xff && data[1]&0xe0 == 0xe0 &&
		data[1]&0x18 != 0x08 && data[1]&0x06 == 0x02 && data[2]&0xf0 != 0xf0 && data[2]&0x0c != 0x0c {
		return "mp3", nil
	}
	switch http.DetectContentType(data) {
	case "audio/mpeg":
		return "mp3", nil
	case "audio/wave", "audio/wav":
		return "wav", nil
	default:
		return "", ErrFormat
	}
}

// Store uses a content digest and a safe name; uploaded names cannot choose a path.
func Store(home, name string, data []byte) (string, error) {
	kind, err := Format(data)
	if err != nil {
		return "", err
	}
	base := filepath.Base(strings.ReplaceAll(name, `\`, "/"))
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	stem = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, stem)
	runes := []rune(strings.Trim(stem, " ."))
	if len(runes) > 40 {
		runes = runes[:40]
	}
	stem = string(runes)
	if stem == "" {
		stem = "sound"
	}
	root := filepath.Join(home, "notification-sounds")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(root, fmt.Sprintf("%x-%s.%s", sha256.Sum256(data), stem, kind))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// Load resolves relative paths against the agent home and never reads unbounded files.
func Load(home, path string) ([]byte, string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(home, path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, "", err
	}
	if !info.Mode().IsRegular() {
		return nil, "", ErrFormat
	}
	if info.Size() > MaxBytes {
		return nil, "", ErrTooLarge
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, MaxBytes+1))
	if err != nil {
		return nil, "", err
	}
	kind, err := Format(data)
	return data, kind, err
}
