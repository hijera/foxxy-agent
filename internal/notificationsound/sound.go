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
	// ErrLocation guards the read endpoint: the served file must live under
	// the agent's notification-sounds directory, never elsewhere on disk.
	ErrLocation = errors.New("notification sound must live under the agent notification-sounds directory")
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

type stagedSoundFile interface {
	io.Writer
	io.Closer
	Name() string
}

var createSoundTemp = func(root string) (stagedSoundFile, error) {
	return os.CreateTemp(root, ".sound-*")
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
	// Content-addressed writes stage through a temp file: a concurrent read of
	// the same digest never observes a truncated upload.
	tmp, err := createSoundTemp(root)
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr == nil && cerr == nil {
		werr = os.Rename(tmpName, path)
	} else if werr == nil {
		werr = cerr
	}
	if werr != nil {
		_ = os.Remove(tmpName)
		return "", werr
	}
	return path, nil
}

// Load resolves relative paths against the agent home, serves only files inside
// the notification-sounds directory and never reads unbounded files.
func Load(home, path string) ([]byte, string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(home, path)
	}
	rootPath := filepath.Join(home, "notification-sounds")
	rel, err := filepath.Rel(rootPath, path)
	if err != nil ||
		rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return nil, "", ErrLocation
	}
	// Check the resolved location for a useful ErrLocation response, including
	// links in parent directories. Root.Open below also confines the actual
	// read if a directory or link changes after this check.
	resolvedRoot, err := filepath.EvalSymlinks(rootPath)
	if err != nil {
		return nil, "", err
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, "", err
	}
	resolvedRel, err := filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil || resolvedRel == ".." || strings.HasPrefix(resolvedRel, ".."+string(filepath.Separator)) || filepath.IsAbs(resolvedRel) {
		return nil, "", ErrLocation
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = root.Close() }()
	// Keep rejecting a symlink as the selected file itself.
	info, err := root.Lstat(rel)
	if err != nil {
		return nil, "", err
	}
	if !info.Mode().IsRegular() {
		return nil, "", ErrFormat
	}
	if info.Size() > MaxBytes {
		return nil, "", ErrTooLarge
	}
	f, err := root.Open(rel)
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
