//go:build http && miniapps

package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hijera/foxxycode-agent/external/miniapps"
)

func miniAppsCapability() bool { return true }

func (s *Server) registerMiniAppRoutes() {
	s.mux.HandleFunc("POST /foxxycode/sessions/{id}/miniapps/distill", s.foxxycodeMiniAppDistillPost)
	s.mux.HandleFunc("GET /foxxycode/miniapp-distillations/{job_id}", s.foxxycodeMiniAppDistillationGet)
	s.mux.HandleFunc("GET /foxxycode/miniapps", s.foxxycodeMiniAppsList)
	s.mux.HandleFunc("POST /foxxycode/miniapps", s.foxxycodeMiniAppsPost)
	s.mux.HandleFunc("POST /foxxycode/miniapps/import", s.foxxycodeMiniAppsPost)
	s.mux.HandleFunc("GET /foxxycode/miniapps/{id}", s.foxxycodeMiniAppGet)
	s.mux.HandleFunc("PATCH /foxxycode/miniapps/{id}", s.foxxycodeMiniAppPatch)
	s.mux.HandleFunc("GET /foxxycode/miniapps/{id}/draft", s.foxxycodeMiniAppDraftGet)
	s.mux.HandleFunc("PUT /foxxycode/miniapps/{id}/draft", s.foxxycodeMiniAppDraftPut)
	s.mux.HandleFunc("GET /foxxycode/miniapps/{id}/authoring/source", s.foxxycodeMiniAppSourceGet)
	s.mux.HandleFunc("POST /foxxycode/miniapps/{id}/model-binding", s.foxxycodeMiniAppModelBindingPost)
	s.mux.HandleFunc("POST /foxxycode/miniapps/{id}/authoring/chat", s.foxxycodeMiniAppAuthoringChatPost)
	s.mux.HandleFunc("POST /foxxycode/miniapps/{id}/expected-result", s.foxxycodeMiniAppExpectedResultPost)
	s.mux.HandleFunc("POST /foxxycode/miniapps/{id}/validate", s.foxxycodeMiniAppValidatePost)
	s.mux.HandleFunc("POST /foxxycode/miniapps/{id}/sanitize", s.foxxycodeMiniAppSanitizePost)
	s.mux.HandleFunc("POST /foxxycode/miniapps/{id}/test-runs", s.foxxycodeMiniAppTestRunPost)
	s.mux.HandleFunc("POST /foxxycode/miniapps/{id}/release", s.foxxycodeMiniAppReleasePost)
	s.mux.HandleFunc("GET /foxxycode/miniapps/{id}/export", s.foxxycodeMiniAppExportGet)
	s.mux.HandleFunc("POST /foxxycode/miniapps/{id}/versions/{version}/runs", s.foxxycodeMiniAppReleaseRunPost)
	s.mux.HandleFunc("GET /foxxycode/miniapp-runs/{run_id}", s.foxxycodeMiniAppRunGet)
}

func (s *Server) miniAppStore() *miniapps.Store {
	home := ""
	if cfg := s.activeCfg(); cfg != nil {
		home = strings.TrimSpace(cfg.Paths.Home)
	}
	if home == "" {
		home = s.defaultCWD
	}
	return miniapps.NewStoreWithRunRoot(filepath.Join(home, "miniapps"), filepath.Join(home, "apps"))
}

func (s *Server) miniAppRunner() *miniapps.Runner {
	executor := miniapps.NewConfigModelExecutor(s.activeCfg(), miniapps.ProviderFactory(s.agentProviderFactory))
	return miniapps.NewRunner(s.miniAppStore(), executor).WithLocalWorkspace(s.defaultCWD)
}

func (s *Server) foxxycodeMiniAppDistillPost(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.PathValue("id"))
	files := s.mgr.FileStore()
	if files == nil {
		s.miniAppWriteError(w, errors.New("session persistence is unavailable"))
		return
	}
	snapshot, err := files.ReadSnapshot(sessionID)
	if err != nil {
		s.miniAppWriteError(w, miniapps.ErrNotFound)
		return
	}
	job := miniapps.DistillationJob{
		ID:        "distill-" + strings.TrimPrefix(time.Now().UTC().Format("20060102t150405.000000000"), "0"),
		SessionID: sessionID, Status: miniapps.DistillationQueued, Phase: "queued",
		Progress: 0, CreatedAt: time.Now().UTC(),
	}
	store := s.miniAppStore()
	if err := store.SaveDistillation(job); err != nil {
		s.miniAppWriteError(w, err)
		return
	}
	s.bgWG.Add(1)
	go func() {
		defer s.bgWG.Done()
		job.Status, job.Phase, job.Progress = miniapps.DistillationAnalyzing, "analyzing_session", 20
		_ = store.SaveDistillation(job)
		var binding *miniapps.ModelBinding
		modelRef := snapshot.Meta.SelectedModelID
		if modelRef == "" && s.activeCfg() != nil {
			modelRef = s.activeCfg().Agent.Model
		}
		if candidate, bindErr := miniapps.BindingForConfiguredModel(s.activeCfg(), modelRef, "primary"); bindErr == nil {
			binding = candidate
		}
		app, evidence, distillErr := miniapps.Distill(miniapps.DistillInput{
			SessionID: sessionID, Title: snapshot.Meta.Title, Author: os.Getenv("USERNAME"),
			Messages: snapshot.Messages, ModelBinding: binding,
		})
		if distillErr == nil {
			distillErr = store.CreateDraft(app, &evidence)
		}
		if distillErr != nil {
			job.Status, job.Phase, job.Error = miniapps.DistillationFailed, "failed", distillErr.Error()
		} else {
			job.Status, job.Phase, job.Progress = miniapps.DistillationCompleted, "draft_ready", 100
			job.AppID, job.Summary = app.ID, miniapps.DistillationSummary(app)
		}
		_ = store.SaveDistillation(job)
	}()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Location", "/foxxycode/miniapp-distillations/"+job.ID)
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(job)
}

func (s *Server) foxxycodeMiniAppDistillationGet(w http.ResponseWriter, r *http.Request) {
	job, err := s.miniAppStore().GetDistillation(strings.TrimSpace(r.PathValue("job_id")))
	if err != nil {
		s.miniAppWriteError(w, err)
		return
	}
	writeMiniAppJSON(w, http.StatusOK, job)
}

func (s *Server) foxxycodeMiniAppsList(w http.ResponseWriter, r *http.Request) {
	items, err := s.miniAppStore().List(r.URL.Query().Get("q"),
		strings.EqualFold(r.URL.Query().Get("include_archived"), "true"))
	if err != nil {
		s.miniAppWriteError(w, err)
		return
	}
	writeMiniAppJSON(w, http.StatusOK, map[string]interface{}{
		"object": "foxxycode.miniapp.list", "items": items,
	})
}

func (s *Server) foxxycodeMiniAppsPost(w http.ResponseWriter, r *http.Request) {
	var app miniapps.MiniApp
	if err := decodeMiniAppJSON(r, &app); err != nil {
		s.miniAppWriteError(w, err)
		return
	}
	if err := s.miniAppStore().CreateDraft(app, nil); err != nil {
		s.miniAppWriteError(w, err)
		return
	}
	w.Header().Set("Location", "/foxxycode/miniapps/"+app.ID+"/draft")
	writeMiniAppJSON(w, http.StatusCreated, app)
}

func (s *Server) foxxycodeMiniAppGet(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	items, err := s.miniAppStore().List(id, true)
	if err != nil {
		s.miniAppWriteError(w, err)
		return
	}
	for _, item := range items {
		if item.ID == id {
			writeMiniAppJSON(w, http.StatusOK, item)
			return
		}
	}
	s.miniAppWriteError(w, miniapps.ErrNotFound)
}

func (s *Server) foxxycodeMiniAppPatch(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	app, err := s.miniAppStore().GetDraft(id)
	if err != nil {
		s.miniAppWriteError(w, err)
		return
	}
	var patch struct {
		Name        *string   `json:"name"`
		Description *string   `json:"description"`
		Author      *string   `json:"author"`
		Tags        *[]string `json:"tags"`
		Archived    *bool     `json:"archived"`
	}
	if err := decodeMiniAppJSON(r, &patch); err != nil {
		s.miniAppWriteError(w, err)
		return
	}
	if patch.Name != nil {
		app.Metadata.Name = *patch.Name
	}
	if patch.Description != nil {
		app.Metadata.Description = *patch.Description
	}
	if patch.Author != nil {
		app.Metadata.Author = *patch.Author
	}
	if patch.Tags != nil {
		app.Metadata.Tags = *patch.Tags
	}
	if patch.Archived != nil {
		app.Metadata.Archived = *patch.Archived
	}
	if err := s.miniAppStore().PutDraft(id, app); err != nil {
		s.miniAppWriteError(w, err)
		return
	}
	writeMiniAppJSON(w, http.StatusOK, app)
}

func (s *Server) foxxycodeMiniAppDraftGet(w http.ResponseWriter, r *http.Request) {
	app, err := s.miniAppStore().GetDraft(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		s.miniAppWriteError(w, err)
		return
	}
	writeMiniAppJSON(w, http.StatusOK, app)
}

func (s *Server) foxxycodeMiniAppSourceGet(w http.ResponseWriter, r *http.Request) {
	evidence, err := s.miniAppStore().GetSourceEvidence(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		s.miniAppWriteError(w, err)
		return
	}
	writeMiniAppJSON(w, http.StatusOK, evidence)
}

func (s *Server) foxxycodeMiniAppDraftPut(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var app miniapps.MiniApp
	if err := decodeMiniAppJSON(r, &app); err != nil {
		s.miniAppWriteError(w, err)
		return
	}
	if err := s.miniAppStore().PutDraft(id, app); err != nil {
		s.miniAppWriteError(w, err)
		return
	}
	app, _ = s.miniAppStore().GetDraft(id)
	writeMiniAppJSON(w, http.StatusOK, app)
}

func (s *Server) foxxycodeMiniAppModelBindingPost(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var request struct {
		ModelRef string            `json:"model_ref"`
		Draft    *miniapps.MiniApp `json:"draft,omitempty"`
	}
	if err := decodeMiniAppJSON(r, &request); err != nil {
		s.miniAppWriteError(w, err)
		return
	}
	app, err := s.miniAppDraftFromRequest(id, request.Draft)
	if err != nil {
		s.miniAppWriteError(w, err)
		return
	}
	binding, err := miniapps.BindingForConfiguredModel(s.activeCfg(), request.ModelRef, "primary")
	if err != nil {
		s.miniAppWriteError(w, fmt.Errorf("%w: %v", miniapps.ErrInvalid, err))
		return
	}
	app = miniapps.ApplyPrimaryModelBinding(app, *binding)
	if err := s.miniAppStore().PutDraft(id, app); err != nil {
		s.miniAppWriteError(w, err)
		return
	}
	stored, err := s.miniAppStore().GetDraft(id)
	if err != nil {
		s.miniAppWriteError(w, err)
		return
	}
	writeMiniAppJSON(w, http.StatusOK, stored)
}

func (s *Server) foxxycodeMiniAppAuthoringChatPost(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var request struct {
		Message string                   `json:"message"`
		History []miniapps.AuthoringTurn `json:"history,omitempty"`
		Draft   *miniapps.MiniApp        `json:"draft,omitempty"`
	}
	if err := decodeMiniAppJSON(r, &request); err != nil {
		s.miniAppWriteError(w, err)
		return
	}
	app, err := s.miniAppDraftFromRequest(id, request.Draft)
	if err != nil {
		s.miniAppWriteError(w, err)
		return
	}
	binding, found := primaryMiniAppModelBinding(app)
	if !found {
		if len(app.Requirements.ModelBindings) > 0 {
			binding = app.Requirements.ModelBindings[0]
		} else {
			configured, bindErr := miniapps.BindingForConfiguredModel(s.activeCfg(), "", "primary")
			if bindErr != nil {
				s.miniAppWriteError(w, fmt.Errorf("%w: %v", miniapps.ErrInvalid, bindErr))
				return
			}
			binding = *configured
			app = miniapps.ApplyPrimaryModelBinding(app, binding)
		}
	}
	executor := miniapps.NewConfigModelExecutor(
		s.activeCfg(),
		miniapps.ProviderFactory(s.agentProviderFactory),
	)
	result, err := miniapps.EditDraftWithAssistant(
		r.Context(),
		app,
		miniapps.AuthoringRequest{Message: request.Message, History: request.History},
		binding,
		executor,
	)
	if err != nil {
		if errors.Is(err, miniapps.ErrInvalid) {
			s.miniAppWriteError(w, err)
		} else {
			s.miniAppWriteError(w, fmt.Errorf("%w: %v", miniapps.ErrModelExecution, err))
		}
		return
	}
	if err := s.miniAppStore().PutDraft(id, result.App); err != nil {
		s.miniAppWriteError(w, err)
		return
	}
	result.App, err = s.miniAppStore().GetDraft(id)
	if err != nil {
		s.miniAppWriteError(w, err)
		return
	}
	writeMiniAppJSON(w, http.StatusOK, result)
}

func (s *Server) miniAppDraftFromRequest(id string, supplied *miniapps.MiniApp) (miniapps.MiniApp, error) {
	if supplied == nil {
		return s.miniAppStore().GetDraft(id)
	}
	if supplied.ID != id {
		return miniapps.MiniApp{}, fmt.Errorf("%w: path id and document id differ", miniapps.ErrInvalid)
	}
	return *supplied, nil
}

func primaryMiniAppModelBinding(app miniapps.MiniApp) (miniapps.ModelBinding, bool) {
	for _, binding := range app.Requirements.ModelBindings {
		if binding.ID == "primary" {
			return binding, true
		}
	}
	return miniapps.ModelBinding{}, false
}

func (s *Server) foxxycodeMiniAppExpectedResultPost(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var request struct {
		Expectations string            `json:"expectations"`
		Draft        *miniapps.MiniApp `json:"draft,omitempty"`
	}
	if err := decodeMiniAppJSON(r, &request); err != nil {
		s.miniAppWriteError(w, err)
		return
	}
	var app miniapps.MiniApp
	var err error
	if request.Draft != nil {
		app = *request.Draft
		if app.ID != id {
			s.miniAppWriteError(w, fmt.Errorf("%w: path id and document id differ", miniapps.ErrInvalid))
			return
		}
	} else {
		app, err = s.miniAppStore().GetDraft(id)
		if err != nil {
			s.miniAppWriteError(w, err)
			return
		}
	}

	binding, err := s.miniAppExpectedResultBinding(app)
	if err != nil {
		s.miniAppWriteError(w, fmt.Errorf("%w: %v", miniapps.ErrInvalid, err))
		return
	}
	executor := miniapps.NewConfigModelExecutor(
		s.activeCfg(),
		miniapps.ProviderFactory(s.agentProviderFactory),
	)
	suggestion, err := miniapps.GenerateExpectedResult(
		r.Context(),
		app,
		request.Expectations,
		binding,
		executor,
	)
	if err != nil {
		if errors.Is(err, miniapps.ErrInvalid) {
			s.miniAppWriteError(w, err)
		} else {
			s.miniAppWriteError(w, fmt.Errorf("%w: %v", miniapps.ErrModelExecution, err))
		}
		return
	}
	app = miniapps.ApplyExpectedResult(app, suggestion, binding)
	if err := s.miniAppStore().PutDraft(id, app); err != nil {
		s.miniAppWriteError(w, err)
		return
	}
	stored, err := s.miniAppStore().GetDraft(id)
	if err != nil {
		s.miniAppWriteError(w, err)
		return
	}
	writeMiniAppJSON(w, http.StatusOK, miniapps.ExpectedResultGeneration{
		App: stored, Suggestion: suggestion,
	})
}

func (s *Server) miniAppExpectedResultBinding(app miniapps.MiniApp) (miniapps.ModelBinding, error) {
	if binding, found := primaryMiniAppModelBinding(app); found {
		return binding, nil
	}
	for _, step := range app.Workflow {
		if step.Kind == "agent" && strings.TrimSpace(step.ModelBinding) != "" {
			for _, binding := range app.Requirements.ModelBindings {
				if binding.ID == step.ModelBinding {
					return binding, nil
				}
			}
		}
	}
	if len(app.Requirements.ModelBindings) > 0 {
		return app.Requirements.ModelBindings[0], nil
	}
	binding, err := miniapps.BindingForConfiguredModel(s.activeCfg(), "", "acceptance-model")
	if err != nil {
		return miniapps.ModelBinding{}, err
	}
	return *binding, nil
}

func (s *Server) foxxycodeMiniAppValidatePost(w http.ResponseWriter, r *http.Request) {
	app, err := s.miniAppStore().GetDraft(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		s.miniAppWriteError(w, err)
		return
	}
	writeMiniAppJSON(w, http.StatusOK, miniapps.Validate(app))
}

func (s *Server) foxxycodeMiniAppSanitizePost(w http.ResponseWriter, r *http.Request) {
	app, err := s.miniAppStore().GetDraft(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		s.miniAppWriteError(w, err)
		return
	}
	writeMiniAppJSON(w, http.StatusOK, miniapps.Sanitize(app))
}

type miniAppRunRequest struct {
	Inputs        map[string]any  `json:"inputs"`
	Confirmations map[string]bool `json:"confirmations,omitempty"`
	Version       string          `json:"version,omitempty"`
}

func (s *Server) foxxycodeMiniAppTestRunPost(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	var request miniAppRunRequest
	if err := decodeMiniAppJSON(r, &request); err != nil {
		s.miniAppWriteError(w, err)
		return
	}
	run, err := s.miniAppRunner().RunDraft(r.Context(), id, request.Inputs,
		&miniapps.OperatorDecisions{Confirmations: request.Confirmations})
	if err != nil {
		writeMiniAppJSON(w, http.StatusUnprocessableEntity, run)
		return
	}
	if err := s.miniAppStore().RecordPassingTest(id, run); err != nil {
		s.miniAppWriteError(w, err)
		return
	}
	writeMiniAppJSON(w, http.StatusOK, run)
}

func (s *Server) foxxycodeMiniAppReleasePost(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Version string `json:"version"`
	}
	if err := decodeMiniAppJSON(r, &request); err != nil {
		s.miniAppWriteError(w, err)
		return
	}
	app, err := s.miniAppStore().Release(strings.TrimSpace(r.PathValue("id")), request.Version)
	if err != nil {
		s.miniAppWriteError(w, err)
		return
	}
	writeMiniAppJSON(w, http.StatusCreated, app)
}

func (s *Server) foxxycodeMiniAppReleaseRunPost(w http.ResponseWriter, r *http.Request) {
	var request miniAppRunRequest
	if err := decodeMiniAppJSON(r, &request); err != nil {
		s.miniAppWriteError(w, err)
		return
	}
	run, err := s.miniAppRunner().RunRelease(r.Context(), strings.TrimSpace(r.PathValue("id")),
		strings.TrimSpace(r.PathValue("version")), request.Inputs,
		&miniapps.OperatorDecisions{Confirmations: request.Confirmations})
	if err != nil {
		writeMiniAppJSON(w, http.StatusUnprocessableEntity, run)
		return
	}
	writeMiniAppJSON(w, http.StatusOK, run)
}

func (s *Server) foxxycodeMiniAppRunGet(w http.ResponseWriter, r *http.Request) {
	store := s.miniAppStore()
	runID := strings.TrimSpace(r.PathValue("run_id"))
	run, err := store.FindRun(runID)
	if errors.Is(err, miniapps.ErrNotFound) {
		localRunRoot := filepath.Join(s.defaultCWD, ".foxxycode", "apps")
		run, err = miniapps.NewStoreWithRunRoot(store.Root(), localRunRoot).FindRun(runID)
	}
	if err != nil {
		s.miniAppWriteError(w, err)
		return
	}
	writeMiniAppJSON(w, http.StatusOK, run)
}

func (s *Server) foxxycodeMiniAppExportGet(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	version := strings.TrimSpace(r.URL.Query().Get("version"))
	var app miniapps.MiniApp
	var err error
	if version == "" {
		app, err = s.miniAppStore().GetDraft(id)
	} else {
		app, err = s.miniAppStore().GetRelease(id, version)
	}
	if err != nil {
		s.miniAppWriteError(w, err)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-miniapp.json"`, app.ID))
	writeMiniAppJSON(w, http.StatusOK, app)
}

func decodeMiniAppJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 16<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: invalid JSON: %v", miniapps.ErrInvalid, err)
	}
	return nil
}

func writeMiniAppJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (s *Server) miniAppWriteError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, miniapps.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, miniapps.ErrInvalid), errors.Is(err, miniapps.ErrInvalidIdentifier):
		status = http.StatusBadRequest
	case errors.Is(err, miniapps.ErrReleaseGate):
		status = http.StatusConflict
	case errors.Is(err, miniapps.ErrVersionExists):
		status = http.StatusConflict
	case errors.Is(err, miniapps.ErrModelExecution):
		status = http.StatusUnprocessableEntity
	}
	message := err.Error()
	if status == http.StatusInternalServerError {
		s.log.Error("miniapps", "error", err)
		message = "internal error"
	}
	writeMiniAppJSON(w, status, map[string]interface{}{"error": map[string]string{"message": message}})
}
