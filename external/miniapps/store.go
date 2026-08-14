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
	ErrRevisionConflict  = errors.New("mini app draft revision conflict")
	ErrReleaseApproval   = errors.New("explicit mini app release approval is required")
	jsonPersistenceMu    sync.RWMutex
)

// Store persists catalog/draft/release state below root and run records below
// runRoot. A process-local mutex plus atomic rename keeps readers from seeing
// partially written JSON; the revision token provides cross-request conflict
// detection for editors.
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
	if _, err := os.Stat(filepath.Join(dir, "draft", "miniapp.json")); err == nil {
		return fmt.Errorf("%w: %s", ErrVersionExists, app.ID)
	} else if !os.IsNotExist(err) && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return s.writeDraftLocked(app, source, true)
}

// PutDraft keeps the original simple editor API while binding every update to
// the revision returned by GetDraft. A missing revision on an existing draft
// is therefore a conflict rather than an unconditional overwrite.
func (s *Store) PutDraft(id string, app MiniApp) error {
	_, err := s.UpdateDraft(id, app.Revision, app)
	return err
}

// UpdateDraft writes a new revision only when expectedRevision still names the
// current draft. It returns the normalized app including its new revision.
func (s *Store) UpdateDraft(id, expectedRevision string, app MiniApp) (MiniApp, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id != app.ID {
		return MiniApp{}, fmt.Errorf("%w: path id and document id differ", ErrInvalid)
	}
	current, err := s.getDraftLocked(id)
	if err != nil {
		return MiniApp{}, err
	}
	if expectedRevision == "" || expectedRevision != current.Revision {
		return MiniApp{}, fmt.Errorf("%w: expected %q, current %q", ErrRevisionConflict, expectedRevision, current.Revision)
	}
	if err := s.writeDraftLocked(app, nil, false); err != nil {
		return MiniApp{}, err
	}
	return s.getDraftLocked(id)
}

func (s *Store) writeDraftLocked(app MiniApp, source *SourceEvidence, creating bool) error {
	app.State = StateDraft
	app.Version = ""
	app.SchemaVersion = SchemaVersion
	app.Kind = KindMiniApp
	app.Revision = ""
	if report := Validate(app); !report.Valid {
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
		if _, err := os.Stat(filepath.Join(dir, "draft", "miniapp.json")); err != nil {
			if os.IsNotExist(err) {
				return ErrNotFound
			}
			return err
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "draft", "authoring"), 0o700); err != nil {
		return err
	}
	if err := writeJSONAtomic(filepath.Join(dir, "draft", "miniapp.json"), app, 0o600); err != nil {
		return err
	}
	if source != nil {
		source.CreatedAt = time.Now().UTC()
		if err := writeJSONAtomic(filepath.Join(dir, "draft", "authoring", "evidence.json"), source, 0o600); err != nil {
			return err
		}
		if source.SourceFixture != nil {
			if err := writeJSONAtomic(filepath.Join(dir, "draft", "authoring", "fixture.json"), source.SourceFixture, 0o600); err != nil {
				return err
			}
		}
	}
	// Passing tests and sanitization reports are revision-bound. Any draft
	// mutation invalidates both artifacts before the catalog is updated.
	_ = os.Remove(filepath.Join(dir, "draft", "passing-test.json"))
	_ = os.Remove(filepath.Join(dir, "draft", "authoring", "sanitization.json"))
	return s.writeCatalogLocked(app)
}

func (s *Store) GetDraft(id string) (MiniApp, error) {
	dir, err := s.appDir(id)
	if err != nil {
		return MiniApp{}, err
	}
	return readApp(filepath.Join(dir, "draft", "miniapp.json"))
}

func (s *Store) GetSourceEvidence(id string) (SourceEvidence, error) {
	dir, err := s.appDir(id)
	if err != nil {
		return SourceEvidence{}, err
	}
	var evidence SourceEvidence
	if err := readJSON(filepath.Join(dir, "draft", "authoring", "evidence.json"), &evidence); err != nil {
		if os.IsNotExist(err) {
			return SourceEvidence{}, ErrNotFound
		}
		return SourceEvidence{}, err
	}
	return evidence, nil
}

func (s *Store) SaveRepairProposal(id string, proposal RepairProposal) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir, err := s.appDir(id)
	if err != nil {
		return err
	}
	if proposal.AppID != id || !portableIDPattern.MatchString(proposal.ID) || strings.TrimSpace(proposal.BaseRevision) == "" {
		return fmt.Errorf("%w: invalid repair proposal identity", ErrInvalid)
	}
	app, err := s.getDraftLocked(id)
	if err != nil {
		return err
	}
	if proposal.BaseRevision != app.Revision {
		return fmt.Errorf("%w: proposal revision %q, current %q", ErrRevisionConflict, proposal.BaseRevision, app.Revision)
	}
	existing, err := s.listRepairProposalsLocked(dir)
	if err != nil {
		return err
	}
	attempts := 0
	for _, item := range existing {
		if item.BaseRevision == proposal.BaseRevision && item.ID != proposal.ID {
			attempts++
		}
	}
	if attempts >= MaxRepairProposals {
		return fmt.Errorf("%w: repair proposal limit of %d reached for revision %s", ErrReleaseGate, MaxRepairProposals, proposal.BaseRevision)
	}
	return writeJSONAtomic(filepath.Join(dir, "draft", "authoring", "patches", proposal.ID+".json"), proposal, 0o600)
}

func (s *Store) listRepairProposalsLocked(dir string) ([]RepairProposal, error) {
	entries, err := os.ReadDir(filepath.Join(dir, "draft", "authoring", "patches"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	items := make([]RepairProposal, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		var proposal RepairProposal
		if err := readJSON(filepath.Join(dir, "draft", "authoring", "patches", entry.Name()), &proposal); err == nil {
			items = append(items, proposal)
		}
	}
	return items, nil
}

func (s *Store) GetRepairProposal(id, proposalID string) (RepairProposal, error) {
	dir, err := s.appDir(id)
	if err != nil {
		return RepairProposal{}, err
	}
	if !portableIDPattern.MatchString(proposalID) {
		return RepairProposal{}, ErrInvalidIdentifier
	}
	var proposal RepairProposal
	if err := readJSON(filepath.Join(dir, "draft", "authoring", "patches", proposalID+".json"), &proposal); err != nil {
		if os.IsNotExist(err) {
			return RepairProposal{}, ErrNotFound
		}
		return RepairProposal{}, err
	}
	if proposal.AppID != id || proposal.ID != proposalID {
		return RepairProposal{}, fmt.Errorf("%w: repair proposal identity mismatch", ErrInvalid)
	}
	return proposal, nil
}

func (s *Store) ListRepairProposals(id string) ([]RepairProposal, error) {
	dir, err := s.appDir(id)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(dir, "draft", "authoring", "patches"))
	if os.IsNotExist(err) {
		return []RepairProposal{}, nil
	}
	if err != nil {
		return nil, err
	}
	proposals := make([]RepairProposal, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		proposalID := strings.TrimSuffix(entry.Name(), ".json")
		proposal, err := s.GetRepairProposal(id, proposalID)
		if err == nil {
			proposals = append(proposals, proposal)
		}
	}
	sort.Slice(proposals, func(i, j int) bool { return proposals[i].ID < proposals[j].ID })
	return proposals, nil
}

func (s *Store) GetRelease(id, version string) (MiniApp, error) {
	dir, err := s.appDir(id)
	if err != nil {
		return MiniApp{}, err
	}
	if !validSemanticVersion(version) {
		return MiniApp{}, fmt.Errorf("%w: invalid release version", ErrInvalid)
	}
	return readApp(filepath.Join(dir, "releases", version, "miniapp.json"))
}

func readApp(path string) (MiniApp, error) {
	var app MiniApp
	if err := readJSON(path, &app); err != nil {
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
		if err := readJSON(filepath.Join(s.root, entry.Name(), "catalog.json"), &item); err != nil {
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
	if run.AppID != id || run.ID == "" || !run.Test || run.Status != RunSucceeded || run.Revision != app.Revision {
		return fmt.Errorf("%w: passing test does not match current draft", ErrReleaseGate)
	}
	dir, _ := s.appDir(id)
	return writeJSONAtomic(filepath.Join(dir, "draft", "passing-test.json"), run, 0o600)
}

// ReleaseOptions makes human approval and an optional editor revision check
// explicit. The variadic form preserves source compatibility with older
// callers while making a two-argument release fail closed.
type ReleaseOptions struct {
	Approved         bool   `json:"approved"`
	ExpectedRevision string `json:"expected_revision,omitempty"`
}

func (s *Store) Release(id, version string, approvals ...ReleaseOptions) (MiniApp, error) {
	options := ReleaseOptions{}
	if len(approvals) > 0 {
		options = approvals[0]
	}
	return s.ReleaseWithOptions(id, version, options)
}

func (s *Store) ReleaseWithOptions(id, version string, options ReleaseOptions) (MiniApp, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !options.Approved {
		return MiniApp{}, ErrReleaseApproval
	}
	if strings.TrimSpace(options.ExpectedRevision) == "" {
		return MiniApp{}, fmt.Errorf("%w: expected revision is required", ErrRevisionConflict)
	}
	if !validSemanticVersion(version) {
		return MiniApp{}, fmt.Errorf("%w: version must be MAJOR.MINOR.PATCH", ErrInvalid)
	}
	app, err := s.getDraftLocked(id)
	if err != nil {
		return MiniApp{}, err
	}
	if options.ExpectedRevision != app.Revision {
		return MiniApp{}, fmt.Errorf("%w: expected %q, current %q", ErrRevisionConflict, options.ExpectedRevision, app.Revision)
	}
	report := Sanitize(app)
	if evidence, evidenceErr := s.getSourceEvidenceLocked(id); evidenceErr == nil {
		evidenceReport := SanitizeEvidence(evidence)
		if !evidenceReport.Clean {
			report.Clean = false
			report.Findings = append(report.Findings, evidenceReport.Findings...)
		}
	} else if !errors.Is(evidenceErr, ErrNotFound) {
		return MiniApp{}, evidenceErr
	}
	if dir, dirErr := s.appDir(id); dirErr == nil {
		// Persist the exact report shown to the author, including a clean report,
		// so release review is reproducible for the current revision.
		_ = writeJSONAtomic(filepath.Join(dir, "draft", "authoring", "sanitization.json"), report, 0o600)
	}
	if !report.Clean {
		return MiniApp{}, fmt.Errorf("%w: sanitization found %s", ErrReleaseGate, report.Findings[0].Message)
	}
	dir, _ := s.appDir(id)
	var test Run
	if err := readJSON(filepath.Join(dir, "draft", "passing-test.json"), &test); err != nil ||
		test.AppID != id || test.Test != true || test.Status != RunSucceeded || test.Revision != app.Revision {
		return MiniApp{}, fmt.Errorf("%w: current draft requires a passing test", ErrReleaseGate)
	}
	releaseDir := filepath.Join(dir, "releases", version)
	if _, err := os.Stat(releaseDir); err == nil {
		return MiniApp{}, ErrVersionExists
	} else if !os.IsNotExist(err) {
		return MiniApp{}, err
	}
	latest, err := s.latestReleaseVersionLocked(dir)
	if err != nil {
		return MiniApp{}, err
	}
	if latest != "" && compareSemanticVersions(version, latest) <= 0 {
		return MiniApp{}, fmt.Errorf("%w: release version must increase after %s", ErrReleaseGate, latest)
	}
	released, err := cloneJSON(app)
	if err != nil {
		return MiniApp{}, err
	}
	released.State = StateReleased
	released.Version = version
	releasesDir := filepath.Dir(releaseDir)
	if err := os.MkdirAll(releasesDir, 0o755); err != nil {
		return MiniApp{}, err
	}
	temporaryReleaseDir, err := os.MkdirTemp(releasesDir, ".release-"+version+"-*")
	if err != nil {
		return MiniApp{}, err
	}
	defer func() { _ = os.RemoveAll(temporaryReleaseDir) }()
	if err := writeJSONAtomic(filepath.Join(temporaryReleaseDir, "miniapp.json"), released, 0o600); err != nil {
		return MiniApp{}, err
	}
	if err := os.Rename(temporaryReleaseDir, releaseDir); err != nil {
		return MiniApp{}, err
	}
	if err := s.writeCatalogLocked(released); err != nil {
		return MiniApp{}, err
	}
	return released, nil
}

func (s *Store) getSourceEvidenceLocked(id string) (SourceEvidence, error) {
	dir, err := s.appDir(id)
	if err != nil {
		return SourceEvidence{}, err
	}
	var evidence SourceEvidence
	if err := readJSON(filepath.Join(dir, "draft", "authoring", "evidence.json"), &evidence); err != nil {
		if os.IsNotExist(err) {
			return SourceEvidence{}, ErrNotFound
		}
		return SourceEvidence{}, err
	}
	return evidence, nil
}

func (s *Store) latestReleaseVersionLocked(dir string) (string, error) {
	entries, err := os.ReadDir(filepath.Join(dir, "releases"))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	latest := ""
	for _, entry := range entries {
		if !entry.IsDir() || !validSemanticVersion(entry.Name()) {
			continue
		}
		if latest == "" || compareSemanticVersions(entry.Name(), latest) > 0 {
			latest = entry.Name()
		}
	}
	return latest, nil
}

func (s *Store) SaveRun(run Run) error {
	return s.SaveRunAtRoot(s.runRoot, run)
}

func (s *Store) SaveRunAtRoot(root string, run Run) error {
	if !portableIDPattern.MatchString(run.AppID) || !portableIDPattern.MatchString(run.ID) {
		return ErrInvalidIdentifier
	}
	runDir := filepath.Join(filepath.Clean(root), run.AppID, "runs", run.ID)
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return err
	}
	return writeJSONAtomic(filepath.Join(runDir, "run.json"), run, 0o600)
}

func (s *Store) GetRun(appID, runID string) (Run, error) {
	if !portableIDPattern.MatchString(appID) || !portableIDPattern.MatchString(runID) {
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
	if os.IsNotExist(err) {
		return Run{}, ErrNotFound
	}
	if err != nil {
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

func (s *Store) getDraftLocked(id string) (MiniApp, error) {
	dir, err := s.appDir(id)
	if err != nil {
		return MiniApp{}, err
	}
	return readApp(filepath.Join(dir, "draft", "miniapp.json"))
}

func (s *Store) writeCatalogLocked(app MiniApp) error {
	dir, err := s.appDir(app.ID)
	if err != nil {
		return err
	}
	item := CatalogEntry{
		ID: app.ID, Name: app.Metadata.Name, Description: app.Metadata.Description,
		State: app.State, Version: app.Version, Revision: app.Revision,
		Tags: app.Metadata.Tags, Archived: app.Metadata.Archived, UpdatedAt: time.Now().UTC(),
	}
	// Draft and release metadata coexist. Editing a draft after release must not
	// hide the latest immutable version from the operator catalog.
	if app.State == StateDraft {
		var existing CatalogEntry
		if err := readJSON(filepath.Join(dir, "catalog.json"), &existing); err == nil && existing.State == StateReleased && existing.Version != "" {
			item.State = StateReleased
			item.Version = existing.Version
		}
	}
	return writeJSONAtomic(filepath.Join(dir, "catalog.json"), item, 0o600)
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
	var buffer [8]byte
	if _, err := rand.Read(buffer[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(buffer[:])
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
	temporary, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer func() { _ = os.Remove(temporaryName) }()
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	var renameErr error
	for attempt := 0; attempt < 20; attempt++ {
		renameErr = replaceFile(temporaryName, path)
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
