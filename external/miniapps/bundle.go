//go:build miniapps

package miniapps

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	bundleFormat      = "foxxycode.miniapp.bundle/1"
	embeddedMagicText = "\nFOXXYAPPv1\n"
)

type BundleManifest struct {
	Format string            `json:"format"`
	Mode   string            `json:"mode,omitempty"`
	Files  map[string]string `json:"files,omitempty"`
}

type Portable struct {
	App      MiniApp
	Manifest BundleManifest
	Files    map[string][]byte
}

type embeddedPayload struct {
	Format string            `json:"format"`
	Mode   string            `json:"mode"`
	App    MiniApp           `json:"app"`
	Files  map[string][]byte `json:"files,omitempty"`
}

func LoadPortable(path string) (Portable, error) {
	file, err := os.Open(path)
	if err != nil {
		return Portable{}, err
	}
	defer func() { _ = file.Close() }()
	header := make([]byte, 4)
	n, _ := io.ReadFull(file, header)
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return Portable{}, err
	}
	if n == 4 && bytes.Equal(header, []byte{'P', 'K', 3, 4}) {
		return loadZipPortable(file)
	}
	var app MiniApp
	decoder := json.NewDecoder(io.LimitReader(file, 32<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&app); err != nil {
		return Portable{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return Portable{App: app, Manifest: BundleManifest{Format: bundleFormat}, Files: map[string][]byte{}}, nil
}

func loadZipPortable(reader io.ReaderAt) (Portable, error) {
	sizeReader, ok := reader.(*os.File)
	if !ok {
		return Portable{}, errors.New("bundle reader must be a file")
	}
	stat, err := sizeReader.Stat()
	if err != nil {
		return Portable{}, err
	}
	if stat.Size() > 512<<20 {
		return Portable{}, errors.New("bundle exceeds the 512 MiB limit")
	}
	archive, err := zip.NewReader(reader, stat.Size())
	if err != nil {
		return Portable{}, err
	}
	portable := Portable{Files: map[string][]byte{}}
	for _, entry := range archive.File {
		clean := filepath.ToSlash(filepath.Clean(entry.Name))
		if strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") || clean == ".." {
			return Portable{}, fmt.Errorf("unsafe bundle path %q", entry.Name)
		}
		if entry.UncompressedSize64 > 128<<20 {
			return Portable{}, fmt.Errorf("bundle file %q is too large", entry.Name)
		}
		stream, err := entry.Open()
		if err != nil {
			return Portable{}, err
		}
		raw, readErr := io.ReadAll(io.LimitReader(stream, 128<<20))
		_ = stream.Close()
		if readErr != nil {
			return Portable{}, readErr
		}
		switch clean {
		case "miniapp.json":
			if err := json.Unmarshal(raw, &portable.App); err != nil {
				return Portable{}, fmt.Errorf("miniapp.json: %w", err)
			}
		case "manifest.json":
			if err := json.Unmarshal(raw, &portable.Manifest); err != nil {
				return Portable{}, fmt.Errorf("manifest.json: %w", err)
			}
		default:
			portable.Files[clean] = raw
		}
	}
	if portable.Manifest.Format != bundleFormat || portable.App.Kind != KindMiniApp {
		return Portable{}, fmt.Errorf("%w: incomplete or unsupported bundle", ErrInvalid)
	}
	for name, expected := range portable.Manifest.Files {
		raw, ok := portable.Files[name]
		if !ok {
			return Portable{}, fmt.Errorf("%w: manifest file %q is missing", ErrInvalid, name)
		}
		sum := sha256.Sum256(raw)
		if !strings.EqualFold(hex.EncodeToString(sum[:]), expected) {
			return Portable{}, fmt.Errorf("%w: bundle file %q failed its integrity check", ErrInvalid, name)
		}
	}
	if len(portable.Manifest.Files) != len(portable.Files) {
		return Portable{}, fmt.Errorf("%w: bundle contains undeclared files", ErrInvalid)
	}
	return portable, nil
}

func WriteBundle(path string, portable Portable) error {
	if report := Validate(portable.App); !report.Valid {
		return fmt.Errorf("%w: %s", ErrInvalid, report.Issues[0].Message)
	}
	if portable.Manifest.Format == "" {
		portable.Manifest.Format = bundleFormat
	}
	portable.Manifest.Files = make(map[string]string, len(portable.Files))
	for name, raw := range portable.Files {
		sum := sha256.Sum256(raw)
		portable.Manifest.Files[filepath.ToSlash(filepath.Clean(name))] = hex.EncodeToString(sum[:])
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	archive := zip.NewWriter(file)
	write := func(name string, value any) error {
		stream, err := archive.Create(name)
		if err != nil {
			return err
		}
		raw, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return err
		}
		_, err = stream.Write(append(raw, '\n'))
		return err
	}
	if err := write("manifest.json", portable.Manifest); err != nil {
		_ = archive.Close()
		_ = file.Close()
		return err
	}
	if err := write("miniapp.json", portable.App); err != nil {
		_ = archive.Close()
		_ = file.Close()
		return err
	}
	for name, raw := range portable.Files {
		clean := filepath.ToSlash(filepath.Clean(name))
		if strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
			_ = archive.Close()
			_ = file.Close()
			return fmt.Errorf("unsafe bundle path %q", name)
		}
		stream, err := archive.Create(clean)
		if err != nil {
			_ = archive.Close()
			_ = file.Close()
			return err
		}
		if _, err := stream.Write(raw); err != nil {
			_ = archive.Close()
			_ = file.Close()
			return err
		}
	}
	if err := archive.Close(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

// BuildExecutable copies the current tagged interpreter and appends a reviewed
// mini-app payload. This Go-friendly single-file format avoids source
// generation, network access, and host-specific toolchains at build time.
func BuildExecutable(interpreterPath, outputPath string, portable Portable, mode string) error {
	if mode != "console" && mode != "ui" {
		return fmt.Errorf("build mode must be console or ui")
	}
	if report := Validate(portable.App); !report.Valid {
		return fmt.Errorf("%w: %s", ErrInvalid, report.Issues[0].Message)
	}
	if sanitize := Sanitize(portable.App); !sanitize.Clean {
		return fmt.Errorf("%w: %s", ErrReleaseGate, sanitize.Findings[0].Message)
	}
	payload, err := json.Marshal(embeddedPayload{
		Format: bundleFormat, Mode: mode, App: portable.App, Files: portable.Files,
	})
	if err != nil {
		return err
	}
	input, err := os.Open(interpreterPath)
	if err != nil {
		return err
	}
	defer func() { _ = input.Close() }()
	stat, err := input.Stat()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	output, err := os.OpenFile(outputPath, os.O_CREATE|os.O_TRUNC|os.O_RDWR, stat.Mode().Perm())
	if err != nil {
		return err
	}
	success := false
	defer func() {
		_ = output.Close()
		if !success {
			_ = os.Remove(outputPath)
		}
	}()
	if _, err := io.Copy(output, input); err != nil {
		return err
	}
	if err := setWindowsPESubsystem(output, mode); err != nil {
		return err
	}
	if _, err := output.Write(payload); err != nil {
		return err
	}
	var size [8]byte
	binary.LittleEndian.PutUint64(size[:], uint64(len(payload)))
	if _, err := output.Write(size[:]); err != nil {
		return err
	}
	if _, err := output.Write([]byte(embeddedMagicText)); err != nil {
		return err
	}
	if err := output.Sync(); err != nil {
		return err
	}
	success = true
	return output.Close()
}

// setWindowsPESubsystem lets a desktop-tagged Windows interpreter emit either
// a console program (IMAGE_SUBSYSTEM_WINDOWS_CUI) or a windowed program
// (IMAGE_SUBSYSTEM_WINDOWS_GUI) without recompiling the Go runtime.
func setWindowsPESubsystem(file *os.File, mode string) error {
	var dos [64]byte
	if _, err := file.ReadAt(dos[:], 0); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	if dos[0] != 'M' || dos[1] != 'Z' {
		return nil
	}
	peOffset := int64(binary.LittleEndian.Uint32(dos[0x3c:]))
	var signature [4]byte
	if _, err := file.ReadAt(signature[:], peOffset); err != nil {
		return err
	}
	if !bytes.Equal(signature[:], []byte{'P', 'E', 0, 0}) {
		return errors.New("invalid PE signature in interpreter")
	}
	subsystem := uint16(3)
	if mode == "ui" {
		subsystem = 2
	}
	var encoded [2]byte
	binary.LittleEndian.PutUint16(encoded[:], subsystem)
	_, err := file.WriteAt(encoded[:], peOffset+24+68)
	return err
}

func ReadEmbeddedExecutable(path string) (Portable, string, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return Portable{}, "", false, err
	}
	defer func() { _ = file.Close() }()
	stat, err := file.Stat()
	if err != nil {
		return Portable{}, "", false, err
	}
	magic := []byte(embeddedMagicText)
	footerSize := int64(len(magic) + 8)
	if stat.Size() < footerSize {
		return Portable{}, "", false, nil
	}
	footer := make([]byte, footerSize)
	if _, err := file.ReadAt(footer, stat.Size()-footerSize); err != nil {
		return Portable{}, "", false, err
	}
	if !bytes.Equal(footer[8:], magic) {
		return Portable{}, "", false, nil
	}
	payloadSize := int64(binary.LittleEndian.Uint64(footer[:8]))
	if payloadSize <= 0 || payloadSize > 512<<20 || payloadSize+footerSize > stat.Size() {
		return Portable{}, "", true, errors.New("invalid embedded mini-app payload length")
	}
	payloadRaw := make([]byte, payloadSize)
	if _, err := file.ReadAt(payloadRaw, stat.Size()-footerSize-payloadSize); err != nil {
		return Portable{}, "", true, err
	}
	var payload embeddedPayload
	if err := json.Unmarshal(payloadRaw, &payload); err != nil {
		return Portable{}, "", true, err
	}
	if payload.Format != bundleFormat {
		return Portable{}, "", true, errors.New("unsupported embedded mini-app format")
	}
	return Portable{
		App: payload.App, Manifest: BundleManifest{Format: payload.Format, Mode: payload.Mode},
		Files: payload.Files,
	}, payload.Mode, true, nil
}
