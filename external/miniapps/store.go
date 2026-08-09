//go:build miniapps

package miniapps

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrNotFound          = errors.New("mini app not found")
	ErrInvalid           = errors.New("invalid mini app")
	ErrReleaseGate       = errors.New("mini app release gate failed")
	ErrVersionExists     = errors.New("mini app version already exists")
	ErrInvalidIdentifier = errors.New("invalid mini app identifier")
	ErrModelExecution    = errors.New("mini app model execution failed")
	jsonPersistenceMu    sync.RWMutex
)

type Store struct {
	root    string
	runRoot string
	mu      sync.Mutex
}

func NewStore(root string) *Store {
	clean := filepath.Clean(root)
	return &Store{root: clean, runRoot: clean}
}

func NewStoreWithRunRoot(root, runRoot string) *Store {
	return &Store{root: filepath.Clean(root), runRoot: filepath.Clean(runRoot)}
}

func (s *Store) Root() string    { return s.root }
func (s *Store) RunRoot() string { return s.runRoot }

func (s *Store) appDir(id string) (string, error) {
	if !portableIDPattern.MatchString(id) {
		return "", ErrInvalidIdentifier
	}
	return filepath.Join(s.root, id), nil
}

func (s *Store) CreateDraft(app MiniApp, source *SourceEvidence) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir, err := s.appDir(app.ID)
	if err != nil {
		return err
	}
	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("%w: %s", ErrVersionExists, app.ID)
	} else if !os.IsNotExist(err) {
		return err
	}
	return s.writeDraftLocked(app, source, true)
}

func (s *Store) PutDraft(id string, app MiniApp) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id != app.ID {
		return fmt.Errorf("%w: path id and document id differ", ErrInvalid)
	}
	return s.writeDraftLocked(app, nil, false)
}

func (s *Store) writeDraftLocked(app MiniApp, source *SourceEvidence, creating bool) error {
	app.State = StateDraft
	app.Version = ""
	app.SchemaVersion = SchemaVersion
	app.Kind = KindMiniApp
	app.Revision = ""
	report := Validate(app)
	if !report.Valid {
		return fmt.Errorf("%w: %s", ErrInvalid, report.Issues[0].Message)
	}
	revision, err := revisionFor(app)
	if err != nil {
		return err
	}
	app.Revision = revision
	dir, err := s.appDir(app.ID)
	if err != nil {
		return err
	}
	if !creating {
		if _, err := os.Stat(dir); err != nil {
			if os.IsNotExist(err) {
				return ErrNotFound
			}
			return err
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "draft"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "authoring"), 0o700); err != nil {
		return err
	}
	if err := writeJSONAtomic(filepath.Join(dir, "draft", "miniapp.json"), app, 0o600); err != nil {
		return err
	}
	if source != nil {
		source.CreatedAt = time.Now().UTC()
		if err := writeJSONAtomic(filepath.Join(dir, "authoring", "source.json"), source, 0o600); err != nil {
			return err
		}
	}
	_ = os.Remove(filepath.Join(dir, "draft", "passing-test.json"))
	return s.writeCatalogLocked(app)
}

func (s *Store) GetDraft(id string) (MiniApp, error) {
	dir, err := s.appDir(id)
	if err != nil {
		return MiniApp{}, err
	}
	var app MiniApp
	if err := readJSON(filepath.Join(dir, "draft", "miniapp.json"), &app); err != nil {
		if os.IsNotExist(err) {
			return MiniApp{}, ErrNotFound
		}
		return MiniApp{}, err
	}
	return app, nil
}

func (s *Store) GetSourceEvidence(id string) (SourceEvidence, error) {
	dir, err := s.appDir(id)
	if err != nil {
		return SourceEvidence{}, err
	}
	var evidence SourceEvidence
	if err := readJSON(filepath.Join(dir, "authoring", "source.json"), &evidence); err != nil {
		if os.IsNotExist(err) {
			return SourceEvidence{}, ErrNotFound
		}
		return SourceEvidence{}, err
	}
	return evidence, nil
}

func (s *Store) GetRelease(id, version string) (MiniApp, error) {
	dir, err := s.appDir(id)
	if err != nil {
		return MiniApp{}, err
	}
	if !validVersion(version) {
		return MiniApp{}, fmt.Errorf("%w: invalid release version", ErrInvalid)
	}
	var app MiniApp
	if err := readJSON(filepath.Join(dir, "releases", version, "miniapp.json"), &app); err != nil {
		if os.IsNotExist(err) {
			return MiniApp{}, ErrNotFound
		}
		return MiniApp{}, err
	}
	return app, nil
}

func (s *Store) List(query string, includeArchived bool) ([]CatalogEntry, error) {
	entries, err := os.ReadDir(s.root)
	if os.IsNotExist(err) {
		return []CatalogEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(strings.TrimSpace(query))
	out := make([]CatalogEntry, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !portableIDPattern.MatchString(entry.Name()) {
			continue
		}
		var item CatalogEntry
		if err := readJSON(filepath.Join(s.root, entry.Name(), "app.json"), &item); err != nil {
			continue
		}
		if item.Archived && !includeArchived {
			continue
		}
		haystack := strings.ToLower(item.ID + " " + item.Name + " " + item.Description + " " + strings.Join(item.Tags, " "))
		if needle != "" && !strings.Contains(haystack, needle) {
			continue
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

func (s *Store) RecordPassingTest(id string, run Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	app, err := s.getDraftLocked(id)
	if err != nil {
		return err
	}
	if !run.Test || run.Status != RunSucceeded || run.Revision != app.Revision {
		return fmt.Errorf("%w: passing test does not match current draft", ErrReleaseGate)
	}
	dir, _ := s.appDir(id)
	return writeJSONAtomic(filepath.Join(dir, "draft", "passing-test.json"), run, 0o600)
}

func (s *Store) Release(id, version string) (MiniApp, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validVersion(version) {
		return MiniApp{}, fmt.Errorf("%w: version must be MAJOR.MINOR.PATCH", ErrInvalid)
	}
	app, err := s.getDraftLocked(id)
	if err != nil {
		return MiniApp{}, err
	}
	if report := Sanitize(app); !report.Clean {
		return MiniApp{}, fmt.Errorf("%w: sanitization found %s", ErrReleaseGate, report.Findings[0].Message)
	}
	dir, _ := s.appDir(id)
	var test Run
	if err := readJSON(filepath.Join(dir, "draft", "passing-test.json"), &test); err != nil ||
		test.Status != RunSucceeded || test.Revision != app.Revision {
		return MiniApp{}, fmt.Errorf("%w: current draft requires a passing test", ErrReleaseGate)
	}
	releaseDir := filepath.Join(dir, "releases", version)
	if _, err := os.Stat(releaseDir); err == nil {
		return MiniApp{}, ErrVersionExists
	}
	released, err := cloneJSON(app)
	if err != nil {
		return MiniApp{}, err
	}
	released.State = StateReleased
	released.Version = version
	if err := os.MkdirAll(releaseDir, 0o755); err != nil {
		return MiniApp{}, err
	}
	if err := writeJSONAtomic(filepath.Join(releaseDir, "miniapp.json"), released, 0o600); err != nil {
		return MiniApp{}, err
	}
	if err := s.writeCatalogLocked(released); err != nil {
		return MiniApp{}, err
	}
	return released, nil
}

func (s *Store) SaveRun(run Run) error {
	return s.SaveRunAtRoot(s.runRoot, run)
}

func (s *Store) SaveRunAtRoot(root string, run Run) error {
	if !portableIDPattern.MatchString(run.AppID) {
		return ErrInvalidIdentifier
	}
	if !portableIDPattern.MatchString(run.ID) {
		return ErrInvalidIdentifier
	}
	runDir := filepath.Join(filepath.Clean(root), run.AppID, "runs", run.ID)
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return err
	}
	return writeJSONAtomic(filepath.Join(runDir, "run.json"), run, 0o600)
}

func (s *Store) SaveDistillation(job DistillationJob) error {
	if !portableIDPattern.MatchString(job.ID) {
		return ErrInvalidIdentifier
	}
	job.UpdatedAt = time.Now().UTC()
	return writeJSONAtomic(filepath.Join(s.root, ".distillations", job.ID+".json"), job, 0o600)
}

func (s *Store) GetDistillation(id string) (DistillationJob, error) {
	if !portableIDPattern.MatchString(id) {
		return DistillationJob{}, ErrInvalidIdentifier
	}
	var job DistillationJob
	if err := readJSON(filepath.Join(s.root, ".distillations", id+".json"), &job); err != nil {
		if os.IsNotExist(err) {
			return DistillationJob{}, ErrNotFound
		}
		return DistillationJob{}, err
	}
	return job, nil
}

func (s *Store) GetRun(appID, runID string) (Run, error) {
	if !portableIDPattern.MatchString(appID) {
		return Run{}, ErrInvalidIdentifier
	}
	if !portableIDPattern.MatchString(runID) {
		return Run{}, ErrInvalidIdentifier
	}
	var run Run
	if err := readJSON(filepath.Join(s.runRoot, appID, "runs", runID, "run.json"), &run); err != nil {
		if os.IsNotExist(err) {
			return Run{}, ErrNotFound
		}
		return Run{}, err
	}
	return run, nil
}

func (s *Store) FindRun(runID string) (Run, error) {
	if !portableIDPattern.MatchString(runID) {
		return Run{}, ErrInvalidIdentifier
	}
	entries, err := os.ReadDir(s.runRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return Run{}, ErrNotFound
		}
		return Run{}, err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !portableIDPattern.MatchString(entry.Name()) {
			continue
		}
		run, err := s.GetRun(entry.Name(), runID)
		if err == nil {
			return run, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return Run{}, err
		}
	}
	return Run{}, ErrNotFound
}

func (s *Store) getDraftLocked(id string) (MiniApp, error) {
	dir, err := s.appDir(id)
	if err != nil {
		return MiniApp{}, err
	}
	var app MiniApp
	if err := readJSON(filepath.Join(dir, "draft", "miniapp.json"), &app); err != nil {
		if os.IsNotExist(err) {
			return MiniApp{}, ErrNotFound
		}
		return MiniApp{}, err
	}
	return app, nil
}

func (s *Store) writeCatalogLocked(app MiniApp) error {
	dir, _ := s.appDir(app.ID)
	item := CatalogEntry{
		ID: app.ID, Name: app.Metadata.Name, Description: app.Metadata.Description,
		State: app.State, Version: app.Version, Revision: app.Revision,
		Tags: app.Metadata.Tags, Archived: app.Metadata.Archived, UpdatedAt: time.Now().UTC(),
	}
	return writeJSONAtomic(filepath.Join(dir, "app.json"), item, 0o600)
}

func revisionFor(app MiniApp) (string, error) {
	app.Revision = ""
	app.Version = ""
	app.State = StateDraft
	raw, err := json.Marshal(app)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:12]), nil
}

func newID(prefix string) string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(buf[:])
}

func validVersion(version string) bool {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, ch := range part {
			if ch < '0' || ch > '9' {
				return false
			}
		}
	}
	return true
}

func writeJSONAtomic(path string, value any, mode os.FileMode) error {
	jsonPersistenceMu.Lock()
	defer jsonPersistenceMu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	var renameErr error
	for attempt := 0; attempt < 20; attempt++ {
		renameErr = os.Rename(name, path)
		if renameErr == nil {
			return nil
		}
		time.Sleep(time.Duration(attempt+1) * time.Millisecond)
	}
	return renameErr
}

func readJSON(path string, value any) error {
	jsonPersistenceMu.RLock()
	defer jsonPersistenceMu.RUnlock()
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	return decoder.Decode(value)
}
