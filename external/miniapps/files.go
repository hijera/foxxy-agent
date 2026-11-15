//go:build miniapps

package miniapps

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const MaxUploadBytes int64 = 256 << 20
const maxUploadStorageBytes int64 = 512 << 20
const uploadLifetime = 24 * time.Hour

var ErrUploadTooLarge = errors.New("upload exceeds the file or storage limit")

// Upload is private, app/input/revision-bound staging data. Browser inputs carry
// only its opaque id; filenames never select a server-side source path.
type Upload struct {
	ID        string    `json:"id"`
	AppID     string    `json:"app_id"`
	InputID   string    `json:"input_id"`
	Revision  string    `json:"revision,omitempty"`
	Version   string    `json:"version,omitempty"`
	Name      string    `json:"name"`
	MediaType string    `json:"media_type"`
	SizeBytes int64     `json:"size_bytes"`
	SHA256    string    `json:"sha256"`
	CreatedAt time.Time `json:"created_at"`
}

type RunFile struct {
	ID        string `json:"id"`
	OutputID  string `json:"output_id"`
	Name      string `json:"name"`
	MediaType string `json:"media_type"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
	Renderer  string `json:"renderer,omitempty"`
	Title     string `json:"title,omitempty"`
	Path      string `json:"path,omitempty"`
}

func fileInput(app MiniApp, id string) (Input, error) {
	for _, input := range app.Inputs {
		if input.ID == id && (input.Type == "file" || input.Type == "files") {
			return input, nil
		}
	}
	return Input{}, fmt.Errorf("%w: input %q is not a file input", ErrInvalid, id)
}

func (s *Store) uploadDir(appID, id string) (string, error) {
	dir, err := s.appDir(appID)
	if err != nil {
		return "", err
	}
	if !portableIDPattern.MatchString(id) {
		return "", ErrInvalidIdentifier
	}
	return filepath.Join(dir, "uploads", id), nil
}

func (s *Store) PutUpload(app MiniApp, inputID, name string, source io.Reader) (Upload, error) {
	input, err := fileInput(app, inputID)
	if err != nil {
		return Upload{}, err
	}
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00\r\n:") || len(name) > 240 {
		return Upload{}, fmt.Errorf("%w: invalid upload filename", ErrInvalid)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	upload := Upload{ID: newID("upload"), AppID: app.ID, InputID: inputID, Revision: app.Revision, Version: app.Version, Name: name, CreatedAt: time.Now().UTC()}
	dir, err := s.uploadDir(app.ID, upload.ID)
	if err != nil {
		return Upload{}, err
	}
	entries, err := os.ReadDir(filepath.Dir(dir))
	if err != nil && !os.IsNotExist(err) {
		return Upload{}, err
	}
	var used int64
	for _, entry := range entries {
		if !entry.IsDir() || !portableIDPattern.MatchString(entry.Name()) {
			continue
		}
		var previous Upload
		path := filepath.Join(filepath.Dir(dir), entry.Name())
		if readJSON(filepath.Join(path, "upload.json"), &previous) != nil {
			continue
		}
		if time.Since(previous.CreatedAt) > uploadLifetime {
			_ = os.RemoveAll(path)
			continue
		}
		used += previous.SizeBytes
	}
	limit := min(MaxUploadBytes, maxUploadStorageBytes-used)
	if input.Validation.MaxTotalBytes != nil {
		limit = min(limit, *input.Validation.MaxTotalBytes)
	}
	if limit <= 0 {
		return Upload{}, fmt.Errorf("%w: upload storage limit reached", ErrUploadTooLarge)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Upload{}, err
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(dir)
		}
	}()
	file, err := os.OpenFile(filepath.Join(dir, "content"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return Upload{}, err
	}
	hash := sha256.New()
	upload.SizeBytes, err = io.Copy(io.MultiWriter(file, hash), io.LimitReader(source, limit+1))
	closeErr := file.Close()
	if err != nil {
		return Upload{}, err
	}
	if closeErr != nil {
		return Upload{}, closeErr
	}
	if upload.SizeBytes > limit {
		return Upload{}, fmt.Errorf("%w: upload exceeds %d bytes", ErrUploadTooLarge, limit)
	}
	upload.SHA256 = hex.EncodeToString(hash.Sum(nil))
	content, err := os.Open(filepath.Join(dir, "content"))
	if err != nil {
		return Upload{}, err
	}
	var prefix [512]byte
	n, readErr := content.Read(prefix[:])
	_ = content.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return Upload{}, readErr
	}
	upload.MediaType = http.DetectContentType(prefix[:n])
	if err := validateUpload(input, upload); err != nil {
		return Upload{}, err
	}
	if err := writeJSONAtomic(filepath.Join(dir, "upload.json"), upload, 0o600); err != nil {
		return Upload{}, err
	}
	keep = true
	return upload, nil
}

func validateUpload(input Input, upload Upload) error {
	if input.Validation.MaxTotalBytes != nil && upload.SizeBytes > *input.Validation.MaxTotalBytes {
		return fmt.Errorf("%w: file exceeds input byte limit", ErrInvalid)
	}
	if len(input.Validation.Extensions) > 0 {
		found := false
		for _, extension := range input.Validation.Extensions {
			if strings.EqualFold(filepath.Ext(upload.Name), extension) {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("%w: file extension is not accepted by %s", ErrInvalid, input.Title)
		}
	}
	if len(input.Validation.MediaTypes) > 0 {
		found := false
		media := strings.Split(upload.MediaType, ";")[0]
		for _, accepted := range input.Validation.MediaTypes {
			if media == accepted || strings.HasSuffix(accepted, "/*") && strings.HasPrefix(media, strings.TrimSuffix(accepted, "*")) {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("%w: file content type is not accepted by %s", ErrInvalid, input.Title)
		}
	}
	return nil
}

func (s *Store) locateUpload(appID, id string) (string, bool, error) {
	staged, err := s.uploadDir(appID, id)
	if err != nil {
		return "", false, err
	}
	appDir, err := s.appDir(appID)
	if err != nil {
		return "", false, err
	}
	pinned := filepath.Join(appDir, "draft", "authoring", "uploads", id)
	if _, err := os.Stat(filepath.Join(pinned, "upload.json")); err == nil {
		return pinned, true, nil
	}
	return staged, false, nil
}

func (s *Store) UploadForInput(app MiniApp, input Input, id string) (Upload, error) {
	dir, pinned, err := s.locateUpload(app.ID, id)
	if err != nil {
		return Upload{}, err
	}
	var upload Upload
	if err := readJSON(filepath.Join(dir, "upload.json"), &upload); err != nil {
		return Upload{}, fmt.Errorf("%w: upload is unavailable; select the file again", ErrInvalid)
	}
	if upload.ID != id || upload.AppID != app.ID || upload.InputID != input.ID || upload.Revision != app.Revision || upload.Version != app.Version || !pinned && time.Since(upload.CreatedAt) > uploadLifetime {
		return Upload{}, fmt.Errorf("%w: upload belongs to another input or revision, or expired; select the file again", ErrInvalid)
	}
	return upload, validateUpload(input, upload)
}

type uploadCopy struct {
	upload Upload
	path   string
}

func (r *Runner) prepareUploadedInputs(app MiniApp, values map[string]any) ([]uploadCopy, error) {
	var copies []uploadCopy
	for _, input := range app.Inputs {
		if input.Type != "file" && input.Type != "files" {
			continue
		}
		if value, ok := values[input.ID]; !ok || value == nil || value == "" {
			continue
		}
		items := []any{values[input.ID]}
		if input.Type == "files" {
			if many, ok := values[input.ID].([]any); ok {
				items = many
			} else {
				continue
			}
		}
		var paths []any
		var total int64
		maximum := 16
		if input.Type == "file" {
			maximum = 1
		}
		if input.Validation.MaxFiles != nil {
			maximum = min(maximum, *input.Validation.MaxFiles)
		}
		if len(items) > maximum {
			return nil, fmt.Errorf("%w: too many files for %s", ErrInvalid, input.ID)
		}
		for _, item := range items {
			ref, ok := item.(map[string]any)
			if !ok {
				paths = append(paths, item)
				continue
			}
			id, _ := ref["upload_id"].(string)
			store, ok := r.store.(*Store)
			if !ok {
				return nil, errors.New("upload storage unavailable")
			}
			upload, err := store.UploadForInput(app, input, id)
			if err != nil {
				return nil, err
			}
			path := filepath.ToSlash(filepath.Join("inputs", input.ID, upload.ID, upload.Name))
			copies = append(copies, uploadCopy{upload: upload, path: path})
			paths = append(paths, path)
			total += upload.SizeBytes
		}
		if input.Validation.MaxTotalBytes != nil && total > *input.Validation.MaxTotalBytes {
			return nil, fmt.Errorf("%w: total input file size exceeds limit", ErrInvalid)
		}
		if input.Type == "files" {
			values[input.ID] = paths
		} else if len(paths) > 0 {
			values[input.ID] = paths[0]
		}
	}
	return copies, nil
}

func (r *Runner) copyUploads(ctx context.Context, workspace string, copies []uploadCopy) error {
	if len(copies) == 0 {
		return nil
	}
	store := r.store.(*Store)
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	for _, copy := range copies {
		if err := ctx.Err(); err != nil {
			return err
		}
		dir, _, err := store.locateUpload(copy.upload.AppID, copy.upload.ID)
		if err != nil {
			return err
		}
		sourceRoot, err := os.OpenRoot(dir)
		if err != nil {
			return err
		}
		source, err := sourceRoot.Open("content")
		_ = sourceRoot.Close()
		if err != nil {
			return err
		}
		if err := root.MkdirAll(filepath.Dir(copy.path), 0o700); err != nil {
			_ = source.Close()
			return err
		}
		dest, err := root.OpenFile(copy.path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			_ = source.Close()
			return err
		}
		hash := sha256.New()
		n, err := io.Copy(io.MultiWriter(dest, hash), io.LimitReader(source, copy.upload.SizeBytes+1))
		_ = source.Close()
		closeErr := dest.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if n != copy.upload.SizeBytes || hex.EncodeToString(hash.Sum(nil)) != copy.upload.SHA256 {
			return errors.New("uploaded file integrity check failed")
		}
	}
	return ctx.Err()
}

// OpenRunFile exposes only a manifest entry from a successful persisted run.
// os.Root confines the open even if a command replaces an output with a link.
func (s *Store) OpenRunFile(runID, fileID string) (*os.File, RunFile, error) {
	run, err := s.FindRun(runID)
	if err != nil {
		return nil, RunFile{}, err
	}
	if run.Status != RunSucceeded {
		return nil, RunFile{}, ErrNotFound
	}
	for _, item := range run.Files {
		if item.ID != fileID {
			continue
		}
		root, err := os.OpenRoot(filepath.Join(s.runRoot, run.AppID, "runs", run.ID, "workspace"))
		if err != nil {
			return nil, RunFile{}, err
		}
		defer func() { _ = root.Close() }()
		info, err := root.Lstat(item.Path)
		if err != nil {
			return nil, RunFile{}, err
		}
		if !info.Mode().IsRegular() {
			return nil, RunFile{}, ErrNotFound
		}
		file, err := root.Open(item.Path)
		if err != nil {
			return nil, RunFile{}, err
		}
		hash := sha256.New()
		n, err := io.Copy(hash, io.LimitReader(file, item.SizeBytes+1))
		if err != nil || n != item.SizeBytes || hex.EncodeToString(hash.Sum(nil)) != item.SHA256 {
			_ = file.Close()
			return nil, RunFile{}, errors.New("result file changed after the run")
		}
		_, err = file.Seek(0, io.SeekStart)
		if err != nil {
			_ = file.Close()
			return nil, RunFile{}, err
		}
		return file, item, nil
	}
	return nil, RunFile{}, ErrNotFound
}

func (e *runExecution) resultFiles(outputs map[string]any) ([]RunFile, error) {
	var files []RunFile
	root, err := os.OpenRoot(e.workspace)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	var total int64
	for _, output := range e.app.Outputs {
		if output.Type != "file" {
			continue
		}
		path, ok := outputs[output.ID].(string)
		if !ok || path == "" || filepath.IsAbs(path) || strings.Contains(path, "\\") {
			return nil, fmt.Errorf("output %s must name a workspace-relative file", output.ID)
		}
		path = filepath.Clean(path)
		if path == ".." || strings.HasPrefix(path, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("output %s escapes the run workspace", output.ID)
		}
		info, err := root.Lstat(path)
		if err != nil {
			return nil, fmt.Errorf("output %s: %w", output.ID, err)
		}
		if !info.Mode().IsRegular() || info.Size() > MaxUploadBytes || total+info.Size() > maxUploadStorageBytes {
			return nil, fmt.Errorf("output %s is not a regular file within the size limit", output.ID)
		}
		file, err := root.Open(path)
		if err != nil {
			return nil, err
		}
		var prefix [512]byte
		n, readErr := file.Read(prefix[:])
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			_ = file.Close()
			return nil, readErr
		}
		_, err = file.Seek(0, io.SeekStart)
		if err != nil {
			_ = file.Close()
			return nil, err
		}
		hash := sha256.New()
		size, err := io.Copy(hash, io.LimitReader(file, MaxUploadBytes+1))
		_ = file.Close()
		if err != nil {
			return nil, err
		}
		if size != info.Size() {
			return nil, errors.New("output changed while collecting it")
		}
		digest := hex.EncodeToString(hash.Sum(nil))
		id := sha256.Sum256([]byte(output.ID + "\x00" + digest))
		files = append(files, RunFile{ID: "file-" + hex.EncodeToString(id[:12]), OutputID: output.ID, Name: redactString(filepath.Base(path), e.secretVals), Path: filepath.ToSlash(path), MediaType: http.DetectContentType(prefix[:n]), SizeBytes: size, SHA256: digest, Renderer: output.Renderer, Title: redactString(output.Title, e.secretVals)})
		total += size
	}
	return files, nil
}

func (s *Store) DeleteUpload(appID, id string) error {
	dir, err := s.uploadDir(appID, id)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return os.RemoveAll(dir)
}

// RecordFileInputs keeps the operator-selected verification uploads private.
// A later test can reuse those exact bytes without reading the source workspace.
func (s *Store) RecordFileInputs(app MiniApp, inputs map[string]any) error {
	selected := map[string]any{}
	for _, input := range app.Inputs {
		if input.Type != "file" && input.Type != "files" {
			continue
		}
		if value, ok := inputs[input.ID]; ok {
			if _, ok := value.(map[string]any); ok {
				selected[input.ID] = value
			}
			if _, ok := value.([]any); ok {
				selected[input.ID] = value
			}
		}
	}
	if len(selected) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.getDraftLocked(app.ID)
	if err != nil {
		return err
	}
	if current.Revision != app.Revision {
		return ErrRevisionConflict
	}
	source, err := s.GetSourceEvidence(app.ID)
	if err != nil {
		return err
	}
	for _, input := range app.Inputs {
		value, exists := selected[input.ID]
		if !exists {
			continue
		}
		items := []any{value}
		if many, ok := value.([]any); ok {
			items = many
		}
		for _, item := range items {
			ref, ok := item.(map[string]any)
			if !ok {
				continue
			}
			id, _ := ref["upload_id"].(string)
			upload, err := s.UploadForInput(app, input, id)
			if err != nil {
				return err
			}
			sourceDir, pinned, err := s.locateUpload(app.ID, id)
			if err != nil {
				return err
			}
			if pinned {
				continue
			}
			appDir, err := s.appDir(app.ID)
			if err != nil {
				return err
			}
			destination := filepath.Join(appDir, "draft", "authoring", "uploads", id)
			if err := os.MkdirAll(destination, 0o700); err != nil {
				return err
			}
			src, err := os.Open(filepath.Join(sourceDir, "content"))
			if err != nil {
				return err
			}
			dst, err := os.OpenFile(filepath.Join(destination, "content"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
			if err != nil {
				_ = src.Close()
				return err
			}
			hash := sha256.New()
			n, err := io.Copy(io.MultiWriter(dst, hash), io.LimitReader(src, upload.SizeBytes+1))
			_ = src.Close()
			closeErr := dst.Close()
			if err != nil {
				return err
			}
			if closeErr != nil {
				return closeErr
			}
			if n != upload.SizeBytes || hex.EncodeToString(hash.Sum(nil)) != upload.SHA256 {
				return errors.New("verification upload changed")
			}
			if err := writeJSONAtomic(filepath.Join(destination, "upload.json"), upload, 0o600); err != nil {
				return err
			}
		}
	}
	source.FixtureInputs = selected
	dir, err := s.appDir(app.ID)
	if err != nil {
		return err
	}
	if err := writeJSONAtomic(filepath.Join(dir, "draft", "authoring", "evidence.json"), source, 0o600); err != nil {
		return err
	}
	keep := map[string]bool{}
	for _, value := range selected {
		items := []any{value}
		if many, ok := value.([]any); ok {
			items = many
		}
		for _, item := range items {
			if ref, ok := item.(map[string]any); ok {
				if id, ok := ref["upload_id"].(string); ok {
					keep[id] = true
				}
			}
		}
	}
	pinnedRoot := filepath.Join(dir, "draft", "authoring", "uploads")
	entries, _ := os.ReadDir(pinnedRoot)
	for _, entry := range entries {
		if portableIDPattern.MatchString(entry.Name()) && !keep[entry.Name()] {
			_ = os.RemoveAll(filepath.Join(pinnedRoot, entry.Name()))
		}
	}
	return nil
}
