//go:build http && miniapps

package httpserver

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hijera/foxxycode-agent/external/miniapps"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

const miniAppsJSONLimit = 2 << 20

type miniAppRunRequest struct {
	JobID   string
	AppID   string
	Version string
	Test    bool
	Inputs  map[string]any
}

type miniAppsHTTPState struct {
	store     *miniapps.Store
	service   *miniapps.Service
	assistant *miniapps.ProviderModelExecutor

	mu   sync.Mutex
	runs map[string]miniAppRunRequest
	jobs map[string]struct{}
}

func (s *Server) miniAppsHTTPState() *miniAppsHTTPState {
	s.miniAppsMu.Lock()
	defer s.miniAppsMu.Unlock()
	if state, ok := s.miniAppsState.(*miniAppsHTTPState); ok && state != nil {
		return state
	}
	cfg := s.activeCfg()
	home := ""
	if cfg != nil {
		home = strings.TrimSpace(cfg.Paths.Home)
	}
	if home == "" {
		home = strings.TrimSpace(s.defaultCWD)
	}
	if home == "" {
		home = os.TempDir()
	}
	root := filepath.Join(home, "miniapps")
	runRoot := filepath.Join(home, "apps")
	store := miniapps.NewStoreWithRunRoot(root, runRoot)
	// The executors resolve the configuration per call rather than closing over
	// the one that happened to be live here, so a provider, key, model, or MCP
	// change saved in Settings (ReplaceConfig) applies to the next Mini App step
	// and to the next assistant reply. Only the store stays pinned: it owns
	// persisted jobs and running workspaces, which a mid-flight home change
	// cannot follow.
	source := miniapps.ConfigSource(s.activeCfg)
	modelExecutor := miniapps.NewLiveProviderModelExecutor(source)
	runner := miniapps.NewRunner(store, miniapps.Executors{
		Tool:  miniapps.NewLiveBuiltinToolExecutor(source),
		Model: modelExecutor,
		Agent: miniapps.NewLiveReActAgentExecutor(source),
	}).WithWorkspaceRoot(runRoot)
	state := &miniAppsHTTPState{
		store: store, service: miniapps.NewService(store, runner), assistant: modelExecutor,
		runs: make(map[string]miniAppRunRequest), jobs: make(map[string]struct{}),
	}
	s.miniAppsState = state
	return state
}

func (s *Server) registerMiniAppsRoutes() {
	s.mux.HandleFunc("POST /foxxycode/sessions/{id}/miniapps/distill", s.miniAppsDistillPost)
	s.mux.HandleFunc("GET /foxxycode/miniapp-distillations/{job_id}", s.miniAppsDistillationGet)
	s.mux.HandleFunc("GET /foxxycode/miniapp-distillations/{job_id}/events", s.miniAppsDistillationEvents)
	s.mux.HandleFunc("POST /foxxycode/miniapp-distillations/{job_id}/scenario", s.miniAppsDistillationScenarioPost)
	s.mux.HandleFunc("POST /foxxycode/miniapp-distillations/{job_id}/cancel", s.miniAppsDistillationCancelPost)

	s.mux.HandleFunc("GET /foxxycode/miniapps", s.miniAppsCatalogGet)
	s.mux.HandleFunc("POST /foxxycode/miniapps", s.miniAppsCatalogPost)
	s.mux.HandleFunc("GET /foxxycode/miniapps/{id}", s.miniAppsGet)
	s.mux.HandleFunc("GET /foxxycode/miniapps/{id}/versions/{version}", s.miniAppsReleaseGet)
	s.mux.HandleFunc("PATCH /foxxycode/miniapps/{id}", s.miniAppsPatch)
	s.mux.HandleFunc("GET /foxxycode/miniapps/{id}/draft", s.miniAppsDraftGet)
	s.mux.HandleFunc("PUT /foxxycode/miniapps/{id}/draft", s.miniAppsDraftPut)
	s.mux.HandleFunc("POST /foxxycode/miniapps/{id}/assistant", s.miniAppsAssistantPost)
	s.mux.HandleFunc("GET /foxxycode/miniapps/{id}/authoring/source", s.miniAppsAuthoringSourceGet)
	s.mux.HandleFunc("POST /foxxycode/miniapps/{id}/authoring/patches", s.miniAppsAuthoringPatchPost)
	s.mux.HandleFunc("POST /foxxycode/miniapps/{id}/authoring/patches/{patch_id}/accept", s.miniAppsAuthoringPatchAcceptPost)
	s.mux.HandleFunc("POST /foxxycode/miniapps/{id}/validate", s.miniAppsValidatePost)
	s.mux.HandleFunc("POST /foxxycode/miniapps/{id}/sanitize", s.miniAppsSanitizePost)
	s.mux.HandleFunc("POST /foxxycode/miniapps/{id}/release", s.miniAppsReleasePost)
	s.mux.HandleFunc("GET /foxxycode/miniapps/{id}/runs", s.miniAppsRunHistoryGet)
	s.mux.HandleFunc("POST /foxxycode/miniapps/{id}/test-runs", s.miniAppsTestRunPost)
	s.mux.HandleFunc("POST /foxxycode/miniapps/{id}/versions/{version}/runs", s.miniAppsReleaseRunPost)

	s.mux.HandleFunc("GET /foxxycode/miniapp-runs/{run_id}", s.miniAppsRunGet)
	s.mux.HandleFunc("GET /foxxycode/miniapp-runs/{run_id}/events", s.miniAppsRunEvents)
	s.mux.HandleFunc("POST /foxxycode/miniapp-runs/{run_id}/confirmation", s.miniAppsRunConfirmationPost)
	s.mux.HandleFunc("POST /foxxycode/miniapp-runs/{run_id}/cancel", s.miniAppsRunCancelPost)
}

func (s *Server) foxxycodeCapabilitiesGet(w http.ResponseWriter, r *http.Request) {
	writeMiniAppsJSON(w, http.StatusOK, map[string]any{
		"object":       "foxxycode.capabilities",
		"capabilities": map[string]bool{"miniapps": true},
	})
}

func (s *Server) miniAppsDistillPost(w http.ResponseWriter, r *http.Request) {
	id, ok := s.miniAppsSessionID(w, r)
	if !ok {
		return
	}
	if s.mgr == nil || s.mgr.SessionTurnActiveInProcess(id) {
		writeMiniAppsError(w, http.StatusConflict, "session_busy", "session has an active turn")
		return
	}
	fs := s.mgr.FileStore()
	if fs == nil || !fs.HasPersistedSnapshot(id) {
		writeMiniAppsError(w, http.StatusNotFound, "not_found", "session not found")
		return
	}
	snapshot, err := fs.ReadSnapshot(id)
	if err != nil {
		writeMiniAppsError(w, http.StatusNotFound, "not_found", "session not found")
		return
	}
	if session.TurnLockHeld(snapshot.Dir) {
		writeMiniAppsError(w, http.StatusConflict, "session_busy", "session has an active turn")
		return
	}
	var body struct {
		Title  string `json:"title"`
		Author string `json:"author"`
	}
	if err := decodeMiniAppsJSON(w, r, &body); err != nil {
		return
	}
	evidence, fixtureFiles := readMiniAppsTraceEvidence(snapshot.Dir, snapshot.Messages)
	job, err := s.miniAppsHTTPState().service.StartDistillation(miniapps.DistillInput{
		SessionID: id, Title: body.Title, Author: body.Author,
		Messages: snapshot.Messages, Evidence: evidence,
		FixtureFiles: fixtureFiles,
		TurnActive:   false,
	})
	if err != nil {
		s.writeMiniAppsServiceError(w, err)
		return
	}
	s.trackMiniAppsJob(job.ID)
	writeMiniAppsJSON(w, http.StatusAccepted, job)
}

func (s *Server) miniAppsSessionID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := strings.TrimSpace(r.PathValue("id"))
	if err := session.ValidateFolderSessionID(id); err != nil {
		writeMiniAppsError(w, http.StatusBadRequest, "invalid_session_id", err.Error())
		return "", false
	}
	if header := strings.TrimSpace(r.Header.Get("X-FoxxyCode-Session-ID")); header != "" && header != id {
		writeMiniAppsError(w, http.StatusBadRequest, "session_id_mismatch", "X-FoxxyCode-Session-ID does not match path id")
		return "", false
	}
	return id, true
}

func (s *Server) miniAppsDistillationGet(w http.ResponseWriter, r *http.Request) {
	jobID := strings.TrimSpace(r.PathValue("job_id"))
	if !validMiniAppsID(jobID) {
		writeMiniAppsError(w, http.StatusBadRequest, "invalid_job_id", "invalid job id")
		return
	}
	job, err := s.miniAppsHTTPState().service.GetJob(jobID)
	if err != nil {
		s.writeMiniAppsServiceError(w, err)
		return
	}
	writeMiniAppsJSON(w, http.StatusOK, job)
}

func (s *Server) miniAppsDistillationScenarioPost(w http.ResponseWriter, r *http.Request) {
	jobID := strings.TrimSpace(r.PathValue("job_id"))
	if !validMiniAppsID(jobID) {
		writeMiniAppsError(w, http.StatusBadRequest, "invalid_job_id", "invalid job id")
		return
	}
	var raw json.RawMessage
	if err := decodeMiniAppsJSON(w, r, &raw); err != nil {
		return
	}
	var envelope struct {
		Scenario json.RawMessage `json:"scenario"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		writeMiniAppsError(w, http.StatusBadRequest, "invalid_json", "invalid scenario request")
		return
	}
	scenarioRaw := envelope.Scenario
	if len(scenarioRaw) == 0 {
		scenarioRaw = raw
	}
	var candidate struct {
		CandidateID     string   `json:"candidate_id"`
		ID              string   `json:"id"`
		Task            string   `json:"task"`
		AcceptedOutcome string   `json:"accepted_outcome"`
		ActionIndexes   []int    `json:"action_indexes"`
		Boundaries      []string `json:"boundaries"`
	}
	if err := json.Unmarshal(scenarioRaw, &candidate); err != nil {
		writeMiniAppsError(w, http.StatusBadRequest, "invalid_scenario", "invalid scenario selection")
		return
	}
	selection := miniapps.TraceScenarioSelection{CandidateID: candidate.CandidateID}
	if selection.CandidateID == "" {
		selection.CandidateID = candidate.ID
	}
	if candidate.Task != "" || candidate.AcceptedOutcome != "" || len(candidate.ActionIndexes) > 0 || len(candidate.Boundaries) > 0 {
		selection.Correction = &miniapps.TraceScenarioCorrection{CandidateID: selection.CandidateID, Task: candidate.Task,
			AcceptedOutcome: candidate.AcceptedOutcome, ActionIndexes: candidate.ActionIndexes, Boundaries: candidate.Boundaries}
	}
	job, err := s.miniAppsHTTPState().service.ConfirmScenario(jobID, selection)
	if err != nil {
		s.writeMiniAppsServiceError(w, err)
		return
	}
	writeMiniAppsJSON(w, http.StatusAccepted, job)
}

func (s *Server) miniAppsDistillationCancelPost(w http.ResponseWriter, r *http.Request) {
	jobID := strings.TrimSpace(r.PathValue("job_id"))
	if !validMiniAppsID(jobID) {
		writeMiniAppsError(w, http.StatusBadRequest, "invalid_job_id", "invalid job id")
		return
	}
	job, err := s.miniAppsHTTPState().service.Cancel(jobID)
	if err != nil {
		s.writeMiniAppsServiceError(w, err)
		return
	}
	writeMiniAppsJSON(w, http.StatusOK, job)
}

func (s *Server) miniAppsCatalogGet(w http.ResponseWriter, r *http.Request) {
	items, err := s.miniAppsHTTPState().store.List(r.URL.Query().Get("q"), r.URL.Query().Get("include_archived") == "true")
	if err != nil {
		s.writeMiniAppsServiceError(w, err)
		return
	}
	writeMiniAppsJSON(w, http.StatusOK, map[string]any{"object": "list", "items": items, "apps": items})
}

func (s *Server) miniAppsCatalogPost(w http.ResponseWriter, r *http.Request) {
	var app miniapps.MiniApp
	if err := decodeMiniAppsJSON(w, r, &app); err != nil {
		return
	}
	if err := s.miniAppsHTTPState().store.CreateDraft(app, nil); err != nil {
		s.writeMiniAppsServiceError(w, err)
		return
	}
	created, err := s.miniAppsHTTPState().store.GetDraft(app.ID)
	if err != nil {
		s.writeMiniAppsServiceError(w, err)
		return
	}
	w.Header().Set("Location", "/foxxycode/miniapps/"+created.ID)
	writeMiniAppsJSON(w, http.StatusCreated, created)
}

func (s *Server) miniAppsGet(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	app, err := s.miniAppsHTTPState().store.GetDraft(id)
	if err != nil {
		s.writeMiniAppsServiceError(w, err)
		return
	}
	writeMiniAppsJSON(w, http.StatusOK, app)
}

func (s *Server) miniAppsReleaseGet(w http.ResponseWriter, r *http.Request) {
	app, err := s.miniAppsHTTPState().store.GetRelease(strings.TrimSpace(r.PathValue("id")), strings.TrimSpace(r.PathValue("version")))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "invalid release version") {
			writeMiniAppsError(w, http.StatusBadRequest, "invalid_version", "invalid release version")
			return
		}
		s.writeMiniAppsServiceError(w, err)
		return
	}
	writeMiniAppsJSON(w, http.StatusOK, app)
}

func (s *Server) miniAppsPatch(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	current, err := s.miniAppsHTTPState().store.GetDraft(id)
	if err != nil {
		s.writeMiniAppsServiceError(w, err)
		return
	}
	var patch struct {
		Metadata            *miniapps.Metadata `json:"metadata"`
		Archived            *bool              `json:"archived"`
		ExpectedRevision    string             `json:"revision"`
		ExpectedRevisionAlt string             `json:"expected_revision"`
	}
	if err := decodeMiniAppsJSON(w, r, &patch); err != nil {
		return
	}
	if header := strings.Trim(strings.TrimSpace(r.Header.Get("If-Match")), "\""); header != "" {
		patch.ExpectedRevision = header
	} else if patch.ExpectedRevision == "" {
		patch.ExpectedRevision = patch.ExpectedRevisionAlt
	}
	if patch.Metadata != nil {
		current.Metadata = *patch.Metadata
	}
	if patch.Archived != nil {
		current.Metadata.Archived = *patch.Archived
	}
	updated, err := s.miniAppsHTTPState().store.UpdateDraft(id, patch.ExpectedRevision, current)
	if err != nil {
		s.writeMiniAppsServiceError(w, err)
		return
	}
	writeMiniAppsJSON(w, http.StatusOK, updated)
}

func (s *Server) miniAppsDraftGet(w http.ResponseWriter, r *http.Request) {
	app, err := s.miniAppsHTTPState().store.GetDraft(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		s.writeMiniAppsServiceError(w, err)
		return
	}
	writeMiniAppsJSON(w, http.StatusOK, app)
}

func (s *Server) miniAppsDraftPut(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var app miniapps.MiniApp
	if err := decodeMiniAppsJSON(w, r, &app); err != nil {
		return
	}
	if app.ID == "" {
		app.ID = id
	}
	expected := strings.Trim(strings.TrimSpace(r.Header.Get("If-Match")), "\"")
	if expected == "" {
		expected = app.Revision
	}
	updated, err := s.miniAppsHTTPState().store.UpdateDraft(id, expected, app)
	if err != nil {
		s.writeMiniAppsServiceError(w, err)
		return
	}
	writeMiniAppsJSON(w, http.StatusOK, updated)
}

func (s *Server) miniAppsAssistantPost(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	state := s.miniAppsHTTPState()
	stored, err := state.store.GetDraft(id)
	if err != nil {
		s.writeMiniAppsServiceError(w, err)
		return
	}
	var body struct {
		Message string                           `json:"message"`
		History []miniapps.DraftAssistantMessage `json:"history"`
		Draft   *miniapps.MiniApp                `json:"draft"`
	}
	if err := decodeMiniAppsJSON(w, r, &body); err != nil {
		return
	}
	draft := stored
	if body.Draft != nil {
		if body.Draft.ID != "" && body.Draft.ID != id {
			writeMiniAppsError(w, http.StatusBadRequest, "draft_id_mismatch", "draft id does not match the URL")
			return
		}
		// A supplied editor snapshot must name the revision it was taken from.
		// Defaulting a missing one to the stored revision would let a stale
		// snapshot pass the staleness check it exists to fail.
		if strings.TrimSpace(body.Draft.Revision) == "" {
			writeMiniAppsError(w, http.StatusBadRequest, "revision_required", "draft revision is required")
			return
		}
		if body.Draft.Revision != stored.Revision {
			writeMiniAppsError(w, http.StatusConflict, "revision_conflict", "draft revision is stale")
			return
		}
		draft = *body.Draft
		if draft.ID == "" {
			draft.ID = id
		}
	}
	result, err := state.assistant.AssistDraft(r.Context(), miniapps.DraftAssistantRequest{
		Draft: draft, History: body.History, Prompt: body.Message,
	})
	if err != nil {
		writeMiniAppsError(w, http.StatusUnprocessableEntity, "assistant_error", err.Error())
		return
	}
	writeMiniAppsJSON(w, http.StatusOK, result)
}

func (s *Server) miniAppsAuthoringSourceGet(w http.ResponseWriter, r *http.Request) {
	evidence, err := s.miniAppsHTTPState().store.GetSourceEvidence(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		s.writeMiniAppsServiceError(w, err)
		return
	}
	writeMiniAppsJSON(w, http.StatusOK, miniAppsPublicSourceEvidence(evidence))
}

func miniAppsPublicSourceEvidence(evidence miniapps.SourceEvidence) map[string]any {
	// Fixture bytes are private runner inputs and must not be copied into an
	// HTTP response. Return only a deterministic manifest for authoring review.
	fixtureFiles := evidence.FixtureFiles
	evidence.FixtureFiles = nil
	raw, _ := json.Marshal(evidence)
	public := make(map[string]any)
	_ = json.Unmarshal(raw, &public)
	paths := make([]string, 0, len(fixtureFiles))
	for path := range fixtureFiles {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	manifest := make([]map[string]any, 0, len(paths))
	for _, path := range paths {
		content := fixtureFiles[path]
		sum := sha256.Sum256(content)
		manifest = append(manifest, map[string]any{"path": path, "sha256": hex.EncodeToString(sum[:]), "size_bytes": len(content)})
	}
	public["fixture_files"] = manifest
	return public
}

func (s *Server) miniAppsAuthoringPatchPost(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	app, err := s.miniAppsHTTPState().store.GetDraft(id)
	if err != nil {
		s.writeMiniAppsServiceError(w, err)
		return
	}
	var body struct {
		Report miniapps.VerificationReport `json:"report"`
	}
	if err := decodeMiniAppsJSON(w, r, &body); err != nil {
		return
	}
	proposals := miniapps.GenerateRepairProposals(app, body.Report)
	for _, proposal := range proposals {
		if err := s.miniAppsHTTPState().store.SaveRepairProposal(id, proposal); err != nil {
			s.writeMiniAppsServiceError(w, err)
			return
		}
	}
	writeMiniAppsJSON(w, http.StatusOK, map[string]any{"items": proposals, "patches": proposals})
}

func (s *Server) miniAppsAuthoringPatchAcceptPost(w http.ResponseWriter, r *http.Request) {
	id, patchID := strings.TrimSpace(r.PathValue("id")), strings.TrimSpace(r.PathValue("patch_id"))
	state := s.miniAppsHTTPState()
	proposal, err := state.store.GetRepairProposal(id, patchID)
	if err != nil {
		s.writeMiniAppsServiceError(w, err)
		return
	}
	if err := miniapps.AcceptRepair(&proposal); err != nil {
		writeMiniAppsError(w, http.StatusUnprocessableEntity, "invalid_patch", err.Error())
		return
	}
	if err := state.store.SaveRepairProposal(id, proposal); err != nil {
		s.writeMiniAppsServiceError(w, err)
		return
	}
	updated, err := miniapps.ApplyRepair(state.store, proposal)
	if err != nil {
		s.writeMiniAppsServiceError(w, err)
		return
	}
	writeMiniAppsJSON(w, http.StatusOK, updated)
}

func (s *Server) miniAppsValidatePost(w http.ResponseWriter, r *http.Request) {
	app, err := s.miniAppsHTTPState().store.GetDraft(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		s.writeMiniAppsServiceError(w, err)
		return
	}
	report := miniapps.Validate(app)
	writeMiniAppsJSON(w, http.StatusOK, report)
}

func (s *Server) miniAppsSanitizePost(w http.ResponseWriter, r *http.Request) {
	appID := strings.TrimSpace(r.PathValue("id"))
	state := s.miniAppsHTTPState()
	app, err := state.store.GetDraft(appID)
	if err != nil {
		s.writeMiniAppsServiceError(w, err)
		return
	}
	document := miniapps.Sanitize(app)
	evidence, evidenceErr := state.store.GetSourceEvidence(appID)
	if evidenceErr != nil && !errors.Is(evidenceErr, miniapps.ErrNotFound) {
		s.writeMiniAppsServiceError(w, evidenceErr)
		return
	}
	private := miniapps.SanitizationReport{Clean: true}
	if evidenceErr == nil {
		private = miniapps.SanitizeEvidence(evidence)
	}
	findings := append([]miniapps.SanitizationFinding(nil), document.Findings...)
	for _, finding := range private.Findings {
		finding.Path = "evidence." + finding.Path
		findings = append(findings, finding)
	}
	writeMiniAppsJSON(w, http.StatusOK, map[string]any{
		"clean":    document.Clean && private.Clean,
		"findings": findings,
		"document": document,
		"evidence": private,
	})
}

func (s *Server) miniAppsReleasePost(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var body struct {
		Version          string `json:"version"`
		Approved         bool   `json:"approved"`
		ExpectedRevision string `json:"expected_revision"`
	}
	if err := decodeMiniAppsJSON(w, r, &body); err != nil {
		return
	}
	if body.ExpectedRevision == "" {
		body.ExpectedRevision = strings.Trim(strings.TrimSpace(r.Header.Get("If-Match")), "\"")
	}
	app, err := s.miniAppsHTTPState().store.ReleaseWithOptions(id, body.Version, miniapps.ReleaseOptions{Approved: body.Approved, ExpectedRevision: body.ExpectedRevision})
	if err != nil {
		s.writeMiniAppsServiceError(w, err)
		return
	}
	writeMiniAppsJSON(w, http.StatusCreated, app)
}

func (s *Server) miniAppsTestRunPost(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var body struct {
		Inputs    map[string]any              `json:"inputs"`
		Decisions *miniapps.OperatorDecisions `json:"decisions"`
	}
	if err := decodeMiniAppsJSON(w, r, &body); err != nil {
		return
	}
	job, err := s.miniAppsHTTPState().service.StartTestRun(id, body.Inputs, body.Decisions)
	if err != nil {
		s.writeMiniAppsServiceError(w, err)
		return
	}
	s.trackMiniAppsRun(job, miniAppRunRequest{JobID: job.ID, AppID: id, Test: true, Inputs: body.Inputs})
	writeMiniAppsJSON(w, http.StatusAccepted, job)
}

func (s *Server) miniAppsReleaseRunPost(w http.ResponseWriter, r *http.Request) {
	id, version := strings.TrimSpace(r.PathValue("id")), strings.TrimSpace(r.PathValue("version"))
	var body struct {
		Inputs    map[string]any              `json:"inputs"`
		Decisions *miniapps.OperatorDecisions `json:"decisions"`
	}
	if err := decodeMiniAppsJSON(w, r, &body); err != nil {
		return
	}
	job, err := s.miniAppsHTTPState().service.StartReleaseRun(id, version, body.Inputs, body.Decisions)
	if err != nil {
		s.writeMiniAppsServiceError(w, err)
		return
	}
	s.trackMiniAppsRun(job, miniAppRunRequest{JobID: job.ID, AppID: id, Version: version, Inputs: body.Inputs})
	writeMiniAppsJSON(w, http.StatusAccepted, job)
}

func (s *Server) trackMiniAppsRun(job miniapps.AsyncJob, req miniAppRunRequest) {
	state := s.miniAppsHTTPState()
	state.mu.Lock()
	state.runs[job.ID] = req
	state.jobs[job.ID] = struct{}{}
	state.mu.Unlock()
}

func (s *Server) trackMiniAppsJob(jobID string) {
	state := s.miniAppsHTTPState()
	state.mu.Lock()
	state.jobs[jobID] = struct{}{}
	state.mu.Unlock()
}

// miniAppsDrain propagates HTTP server shutdown to in-flight optional jobs.
func (s *Server) miniAppsDrain() {
	s.miniAppsMu.Lock()
	state, ok := s.miniAppsState.(*miniAppsHTTPState)
	s.miniAppsMu.Unlock()
	if !ok || state == nil {
		return
	}
	state.service.Close()
}

func (s *Server) miniAppsRunGet(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimSpace(r.PathValue("run_id"))
	if !validMiniAppsID(runID) {
		writeMiniAppsError(w, http.StatusBadRequest, "invalid_run_id", "invalid run id")
		return
	}
	if job, err := s.miniAppsHTTPState().service.GetJob(runID); err == nil && (job.Kind == miniapps.JobTestRun || job.Kind == miniapps.JobReleaseRun) {
		writeMiniAppsJSON(w, http.StatusOK, job)
		return
	}
	if job, ok := s.miniAppsJobForRun(runID); ok {
		writeMiniAppsJSON(w, http.StatusOK, job)
		return
	}
	run, err := s.miniAppsHTTPState().store.FindRun(runID)
	if err != nil {
		s.writeMiniAppsServiceError(w, err)
		return
	}
	writeMiniAppsJSON(w, http.StatusOK, miniAppsPublicRun(run))
}

func (s *Server) miniAppsJobForRun(runID string) (miniapps.AsyncJob, bool) {
	state := s.miniAppsHTTPState()
	state.mu.Lock()
	jobIDs := make([]string, 0, len(state.runs))
	for jobID := range state.runs {
		jobIDs = append(jobIDs, jobID)
	}
	state.mu.Unlock()
	for _, jobID := range jobIDs {
		job, err := state.service.GetJob(jobID)
		if err == nil && job.RunID == runID {
			return job, true
		}
	}
	return miniapps.AsyncJob{}, false
}

func (s *Server) miniAppsRunHistoryGet(w http.ResponseWriter, r *http.Request) {
	appID := strings.TrimSpace(r.PathValue("id"))
	if !validMiniAppsID(appID) {
		writeMiniAppsError(w, http.StatusBadRequest, "invalid_identifier", "invalid Mini App id")
		return
	}
	state := s.miniAppsHTTPState()
	entries, err := os.ReadDir(filepath.Join(state.store.RunRoot(), appID, "runs"))
	if os.IsNotExist(err) {
		writeMiniAppsJSON(w, http.StatusOK, map[string]any{"items": []miniapps.Run{}})
		return
	}
	if err != nil {
		s.writeMiniAppsServiceError(w, err)
		return
	}
	runs := make([]miniapps.Run, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !validMiniAppsID(entry.Name()) {
			continue
		}
		run, readErr := state.store.GetRun(appID, entry.Name())
		if readErr == nil {
			runs = append(runs, miniAppsPublicRun(run))
		}
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].StartedAt.After(runs[j].StartedAt) })
	writeMiniAppsJSON(w, http.StatusOK, map[string]any{"items": runs, "runs": runs})
}

func (s *Server) miniAppsRunCancelPost(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimSpace(r.PathValue("run_id"))
	if !validMiniAppsID(runID) {
		writeMiniAppsError(w, http.StatusBadRequest, "invalid_run_id", "invalid run id")
		return
	}
	if job, err := s.miniAppsHTTPState().service.GetJob(runID); err == nil && (job.Kind == miniapps.JobTestRun || job.Kind == miniapps.JobReleaseRun) {
		updated, cancelErr := s.miniAppsHTTPState().service.Cancel(runID)
		if cancelErr != nil {
			s.writeMiniAppsServiceError(w, cancelErr)
			return
		}
		writeMiniAppsJSON(w, http.StatusOK, updated)
		return
	}
	job, ok := s.miniAppsJobForRun(runID)
	if !ok {
		writeMiniAppsError(w, http.StatusNotFound, "not_found", "run not found")
		return
	}
	updated, err := s.miniAppsHTTPState().service.Cancel(job.ID)
	if err != nil {
		s.writeMiniAppsServiceError(w, err)
		return
	}
	writeMiniAppsJSON(w, http.StatusOK, updated)
}

func (s *Server) miniAppsRunConfirmationPost(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimSpace(r.PathValue("run_id"))
	if !validMiniAppsID(runID) {
		writeMiniAppsError(w, http.StatusBadRequest, "invalid_run_id", "invalid run id")
		return
	}
	var body struct {
		Approved       bool   `json:"approved"`
		ConfirmationID string `json:"confirmation_id"`
	}
	if err := decodeMiniAppsJSON(w, r, &body); err != nil {
		return
	}
	state := s.miniAppsHTTPState()
	if job, err := state.service.GetJob(runID); err == nil && (job.Kind == miniapps.JobTestRun || job.Kind == miniapps.JobReleaseRun) {
		if !body.Approved {
			cancelled, cancelErr := state.service.Cancel(runID)
			if cancelErr != nil {
				s.writeMiniAppsServiceError(w, cancelErr)
				return
			}
			writeMiniAppsJSON(w, http.StatusOK, cancelled)
			return
		}
		decisions, decisionsErr := s.miniAppsConfirmationDecisions(state, job, body.ConfirmationID)
		if decisionsErr != nil {
			writeMiniAppsError(w, http.StatusUnprocessableEntity, "invalid_confirmation", decisionsErr.Error())
			return
		}
		resumed, confirmErr := state.service.ConfirmRun(runID, decisions)
		if confirmErr != nil {
			s.writeMiniAppsServiceError(w, confirmErr)
			return
		}
		writeMiniAppsJSON(w, http.StatusAccepted, resumed)
		return
	}
	job, found := s.miniAppsJobForRun(runID)
	if !found {
		writeMiniAppsError(w, http.StatusNotFound, "not_found", "run not found")
		return
	}
	if !body.Approved {
		cancelled, err := state.service.Cancel(job.ID)
		if err != nil {
			s.writeMiniAppsServiceError(w, err)
			return
		}
		writeMiniAppsJSON(w, http.StatusOK, cancelled)
		return
	}
	decisions, decisionsErr := s.miniAppsConfirmationDecisions(state, job, body.ConfirmationID)
	if decisionsErr != nil {
		writeMiniAppsError(w, http.StatusUnprocessableEntity, "invalid_confirmation", decisionsErr.Error())
		return
	}
	resumed, err := state.service.ConfirmRun(job.ID, decisions)
	if err != nil {
		s.writeMiniAppsServiceError(w, err)
		return
	}
	writeMiniAppsJSON(w, http.StatusAccepted, resumed)
}

// miniAppsConfirmationDecisions resolves the operator's confirmation to the
// persisted waiting step. The run API intentionally does not expose a
// confirmation id in AsyncJob, so an empty client id is accepted only when
// there is exactly one unambiguous waiting step.
func (s *Server) miniAppsConfirmationDecisions(state *miniAppsHTTPState, job miniapps.AsyncJob, confirmationID string) (*miniapps.OperatorDecisions, error) {
	confirmationID = strings.TrimSpace(confirmationID)
	if confirmationID != "" {
		return &miniapps.OperatorDecisions{Confirmations: map[string]bool{confirmationID: true}}, nil
	}
	if state == nil || strings.TrimSpace(job.RunID) == "" {
		return nil, errors.New("confirmation id is required while the run state is unavailable")
	}
	run, err := state.store.FindRun(job.RunID)
	if err != nil {
		return nil, fmt.Errorf("load waiting run: %w", err)
	}
	waiting := make([]string, 0, 1)
	for id, step := range run.Steps {
		if step.Status == miniapps.RunWaitingForConfirmation {
			waiting = append(waiting, id)
		}
	}
	if len(waiting) != 1 {
		return nil, fmt.Errorf("confirmation is ambiguous: %d waiting steps", len(waiting))
	}
	return &miniapps.OperatorDecisions{Confirmations: map[string]bool{waiting[0]: true}}, nil
}

func (s *Server) miniAppsDistillationEvents(w http.ResponseWriter, r *http.Request) {
	jobID := strings.TrimSpace(r.PathValue("job_id"))
	if !validMiniAppsID(jobID) {
		writeMiniAppsError(w, http.StatusBadRequest, "invalid_job_id", "invalid job id")
		return
	}
	state := s.miniAppsHTTPState()
	_, err := state.service.GetJob(jobID)
	if err != nil {
		s.writeMiniAppsServiceError(w, err)
		return
	}
	streamMiniAppsSSE(r, w, parseMiniAppsAfter(r), func(after uint64) ([]miniapps.JobEvent, bool, error) {
		current, getErr := state.service.GetJob(jobID)
		if getErr != nil {
			return nil, false, getErr
		}
		events, eventsErr := state.service.Events(jobID, after)
		if eventsErr != nil {
			return nil, false, eventsErr
		}
		return events, miniAppsJobTerminal(current.Status), nil
	}, func(event miniapps.JobEvent) ([]byte, error) {
		return json.Marshal(event)
	})
}

func miniAppsJobTerminal(status miniapps.JobStatus) bool {
	switch status {
	case miniapps.JobSucceeded, miniapps.JobFailed, miniapps.JobCancelled, miniapps.JobInterrupted:
		return true
	default:
		return false
	}
}

func (s *Server) miniAppsRunEvents(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimSpace(r.PathValue("run_id"))
	if !validMiniAppsID(runID) {
		writeMiniAppsError(w, http.StatusBadRequest, "invalid_run_id", "invalid run id")
		return
	}
	state := s.miniAppsHTTPState()
	if job, err := state.service.GetJob(runID); err == nil && (job.Kind == miniapps.JobTestRun || job.Kind == miniapps.JobReleaseRun) {
		streamMiniAppsSSE(r, w, parseMiniAppsAfter(r), func(after uint64) ([]miniapps.JobEvent, bool, error) {
			current, getErr := state.service.GetJob(runID)
			if getErr != nil {
				return nil, false, getErr
			}
			events, eventsErr := state.service.Events(runID, after)
			if eventsErr != nil {
				return nil, false, eventsErr
			}
			return events, miniAppsJobTerminal(current.Status), nil
		}, func(event miniapps.JobEvent) ([]byte, error) { return json.Marshal(event) })
		return
	}
	_, err := state.store.FindRun(runID)
	if err != nil {
		s.writeMiniAppsServiceError(w, err)
		return
	}
	streamMiniAppsSSE(r, w, parseMiniAppsAfter(r), func(after uint64) ([]miniapps.RunEvent, bool, error) {
		current, findErr := state.store.FindRun(runID)
		if findErr != nil {
			return nil, false, findErr
		}
		events := readMiniAppsRunEvents(current.EventsPath)
		if after >= uint64(len(events)) {
			events = nil
		} else if after > 0 {
			events = events[after:]
		}
		terminal := current.Status == miniapps.RunSucceeded || current.Status == miniapps.RunFailed || current.Status == miniapps.RunCancelled || current.Status == miniapps.RunInterrupted
		return events, terminal, nil
	}, func(event miniapps.RunEvent) ([]byte, error) { return json.Marshal(event) })
}

func parseMiniAppsAfter(r *http.Request) uint64 {
	raw := strings.TrimSpace(r.URL.Query().Get("after"))
	if raw == "" {
		raw = strings.TrimSpace(r.Header.Get("Last-Event-ID"))
	}
	var value uint64
	_, _ = fmt.Sscanf(raw, "%d", &value)
	return value
}

func streamMiniAppsSSE[T any](r *http.Request, w http.ResponseWriter, after uint64, next func(uint64) ([]T, bool, error), encode func(T) ([]byte, error)) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, _ := w.(http.Flusher)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		events, terminal, err := next(after)
		if err != nil {
			return
		}
		for _, event := range events {
			data, encodeErr := encode(event)
			if encodeErr != nil {
				continue
			}
			after++
			_, _ = fmt.Fprintf(w, "id: %d\ndata: %s\n\n", after, data)
		}
		if flusher != nil {
			flusher.Flush()
		}
		if terminal {
			_, _ = io.WriteString(w, "event: done\ndata: [DONE]\n\n")
			if flusher != nil {
				flusher.Flush()
			}
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func readMiniAppsRunEvents(path string) []miniapps.RunEvent {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	var events []miniapps.RunEvent
	scanner := bufio.NewScanner(io.LimitReader(file, 4<<20))
	for scanner.Scan() {
		var event miniapps.RunEvent
		if json.Unmarshal(scanner.Bytes(), &event) == nil {
			events = append(events, event)
		}
	}
	return events
}

func readMiniAppsTraceEvidence(dir string, messages []llm.Message) ([]miniapps.TraceCallEvidence, map[string][]byte) {
	ids, _ := session.ListToolCalls(dir)
	sort.Strings(ids)
	evidence := make([]miniapps.TraceCallEvidence, 0, len(ids))
	for _, id := range ids {
		meta, metaErr := session.ReadToolCallMeta(dir, id)
		if metaErr != nil || meta == nil {
			continue
		}
		args, _ := session.ReadToolCallArgs(dir, id)
		result, resultErr := session.ReadToolCallResult(dir, id)
		status := miniapps.TraceActionStatus(strings.ToLower(strings.TrimSpace(meta.Status)))
		switch status {
		case "completed", "complete", "success", "successful":
			status = miniapps.TraceActionSucceeded
		case "error", "failed", "failure":
			status = miniapps.TraceActionFailed
		case "denied", "rejected":
			status = miniapps.TraceActionDenied
		case "cancelled", "canceled":
			status = miniapps.TraceActionCancelled
		}
		item := miniapps.TraceCallEvidence{ID: id, CallID: id, ToolCallID: id, Name: meta.Name, Kind: meta.Kind,
			Status: status, Arguments: args, StartedAt: meta.StartedAt, FinishedAt: meta.FinishedAt}
		if resultErr == nil {
			item.Result = result
			if (item.Status == "" || item.Status == "in_progress" || item.Status == "pending") && strings.TrimSpace(result) != "" {
				item.Status = persistedTraceResultStatus(result)
			}
		}
		evidence = append(evidence, item)
	}
	evidence = appendPersistedMessageEvidence(evidence, messages)
	callTurns := miniAppsTraceCallTurns(messages)
	fixtureFiles := addMiniAppsDiffArtifacts(dir, evidence, callTurns)
	return evidence, fixtureFiles
}

func appendPersistedMessageEvidence(evidence []miniapps.TraceCallEvidence, messages []llm.Message) []miniapps.TraceCallEvidence {
	seen := make(map[string]bool, len(evidence))
	for _, item := range evidence {
		if id := strings.TrimSpace(item.ID); id != "" {
			seen[id] = true
		}
	}
	results := make(map[string]string)
	for _, message := range messages {
		if message.Role == llm.RoleTool && strings.TrimSpace(message.ToolCallID) != "" {
			results[message.ToolCallID] = message.Content
		}
	}
	for _, message := range messages {
		if message.Role != llm.RoleAssistant {
			continue
		}
		for _, call := range message.ToolCalls {
			id := strings.TrimSpace(call.ID)
			if id == "" || seen[id] {
				continue
			}
			result := results[id]
			status := miniapps.TraceActionMissing
			if strings.TrimSpace(result) != "" {
				status = persistedTraceResultStatus(result)
			}
			evidence = append(evidence, miniapps.TraceCallEvidence{ID: id, CallID: id, ToolCallID: id, Name: call.Name,
				Status: status, Arguments: call.InputJSON, Result: result})
			seen[id] = true
		}
	}
	return evidence
}

func persistedTraceResultStatus(result string) miniapps.TraceActionStatus {
	lower := strings.ToLower(result)
	if strings.Contains(lower, "permission denied") || strings.Contains(lower, "forbidden") || strings.Contains(lower, "rejected") {
		return miniapps.TraceActionDenied
	}
	if strings.Contains(lower, "error") || strings.Contains(lower, "failed") || strings.Contains(lower, "failure") {
		return miniapps.TraceActionFailed
	}
	return miniapps.TraceActionSucceeded
}

func miniAppsTraceCallTurns(messages []llm.Message) map[string]int {
	turns := make(map[string]int)
	turn := 0
	for _, message := range messages {
		if message.Role == llm.RoleUser {
			turn++
		}
		if message.Role != llm.RoleAssistant {
			continue
		}
		for _, call := range message.ToolCalls {
			if strings.TrimSpace(call.ID) != "" && turn > 0 {
				turns[call.ID] = turn
			}
		}
	}
	return turns
}

func addMiniAppsDiffArtifacts(dir string, evidence []miniapps.TraceCallEvidence, callTurns map[string]int) map[string][]byte {
	fixtureFiles := make(map[string][]byte)
	if len(evidence) == 0 {
		return fixtureFiles
	}
	turns, err := session.ListStoredTurnDiffs(dir)
	if err != nil {
		return fixtureFiles
	}
	artifactsByTurn := make(map[int]map[string]miniapps.TraceArtifact)
	contentsByTurn := make(map[int]map[string][]byte)
	for _, turn := range turns {
		diff, err := session.LoadWorkspaceDiff(dir, turn)
		if err != nil || diff == nil {
			continue
		}
		for _, change := range diff.Changes {
			if change.After == nil || !safeMiniAppsRelativePath(change.Path) {
				continue
			}
			path := filepath.ToSlash(filepath.Clean(change.Path))
			artifacts := artifactsByTurn[turn]
			if artifacts == nil {
				artifacts = make(map[string]miniapps.TraceArtifact)
				artifactsByTurn[turn] = artifacts
			}
			contents := contentsByTurn[turn]
			if contents == nil {
				contents = make(map[string][]byte)
				contentsByTurn[turn] = contents
			}
			if _, exists := artifacts[path]; exists {
				continue
			}
			sum := sha256.Sum256(change.After.Content)
			artifacts[path] = miniapps.TraceArtifact{Path: path, SHA256: hex.EncodeToString(sum[:]), SizeBytes: int64(len(change.After.Content))}
			contents[path] = append([]byte(nil), change.After.Content...)
		}
	}
	for _, turn := range turns {
		artifacts := artifactsByTurn[turn]
		if len(artifacts) == 0 {
			continue
		}
		target := -1
		for index := len(evidence) - 1; index >= 0; index-- {
			id := strings.TrimSpace(evidence[index].ID)
			if id == "" {
				id = strings.TrimSpace(evidence[index].CallID)
			}
			mappedTurn, mapped := callTurns[id]
			if !mapped {
				mappedTurn, mapped = callTurns[strings.TrimSpace(evidence[index].CallID)]
			}
			if evidence[index].Status == miniapps.TraceActionSucceeded && mapped && mappedTurn == turn {
				target = index
				break
			}
		}
		if target < 0 {
			continue
		}
		paths := make([]string, 0, len(artifacts))
		for path := range artifacts {
			paths = append(paths, path)
		}
		sort.Strings(paths)
		for _, path := range paths {
			evidence[target].Artifacts = append(evidence[target].Artifacts, artifacts[path])
			if _, exists := fixtureFiles[path]; !exists {
				fixtureFiles[path] = append([]byte(nil), contentsByTurn[turn][path]...)
			}
		}
	}
	return fixtureFiles
}

func safeMiniAppsRelativePath(path string) bool {
	path = filepath.Clean(strings.TrimSpace(path))
	return path != "." && !filepath.IsAbs(path) && path != ".." && !strings.HasPrefix(path, ".."+string(filepath.Separator))
}

func miniAppsPublicRun(run miniapps.Run) miniapps.Run {
	run.LogPath, run.EventsPath = "", ""
	return run
}

func validMiniAppsID(id string) bool {
	if len(id) < 3 || len(id) > 64 {
		return false
	}
	for i, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			if i == 0 || i == len(id)-1 {
				if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
					continue
				}
				return false
			}
			continue
		}
		return false
	}
	return true
}

func decodeMiniAppsJSON(w http.ResponseWriter, r *http.Request, target any) error {
	if r.Body == nil {
		writeMiniAppsError(w, http.StatusBadRequest, "invalid_json", "request body is required")
		return errors.New("request body is required")
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, miniAppsJSONLimit))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeMiniAppsError(w, http.StatusBadRequest, "invalid_json", "invalid JSON request body")
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		writeMiniAppsError(w, http.StatusBadRequest, "invalid_json", "request body must contain one JSON value")
		if err == nil {
			return errors.New("request body contains multiple JSON values")
		}
		return err
	}
	return nil
}

func writeMiniAppsJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeMiniAppsError(w http.ResponseWriter, status int, code, message string) {
	writeMiniAppsJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func (s *Server) writeMiniAppsServiceError(w http.ResponseWriter, err error) {
	status, code := http.StatusInternalServerError, "internal_error"
	lower := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, miniapps.ErrNotFound):
		status, code = http.StatusNotFound, "not_found"
	case errors.Is(err, miniapps.ErrInvalidIdentifier):
		status, code = http.StatusBadRequest, "invalid_identifier"
	case errors.Is(err, miniapps.ErrRevisionConflict):
		status, code = http.StatusConflict, "revision_conflict"
	case errors.Is(err, miniapps.ErrVersionExists):
		status, code = http.StatusConflict, "version_exists"
	case errors.Is(err, miniapps.ErrReleaseApproval):
		status, code = http.StatusUnprocessableEntity, "approval_required"
	case errors.Is(err, miniapps.ErrReleaseGate), errors.Is(err, miniapps.ErrInvalid):
		status, code = http.StatusUnprocessableEntity, "validation_failed"
	case strings.Contains(lower, "not waiting"), strings.Contains(lower, "already terminal"), strings.Contains(lower, "is not a run"):
		status, code = http.StatusConflict, "conflict"
	case strings.Contains(lower, "scenario"), strings.Contains(lower, "confirmation"):
		status, code = http.StatusUnprocessableEntity, "validation_failed"
	case strings.Contains(lower, "busy"):
		status, code = http.StatusConflict, "conflict"
	}
	message := err.Error()
	if status >= 500 {
		message = "internal error"
		if s != nil && s.log != nil {
			s.log.Error("miniapps HTTP operation failed", "error", err)
		}
	}
	writeMiniAppsError(w, status, code, message)
}
