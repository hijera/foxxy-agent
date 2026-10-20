//go:build http && miniapps

package httpserver

import (
	"encoding/json"

	"net/http"
	"strings"

	"github.com/hijera/foxxycode-agent/external/miniapps"
	"github.com/hijera/foxxycode-agent/internal/cmdprofile"
)

// miniAppsCommandsHome resolves the home directory the trust store lives in —
// the same derivation miniAppsHTTPState uses for the app store root.
func (s *Server) miniAppsCommandsHome() string {
	cfg := s.activeCfg()
	if cfg != nil {
		if home := strings.TrimSpace(cfg.Paths.Home); home != "" {
			return home
		}
	}
	return strings.TrimSpace(s.defaultCWD)
}

// miniAppCommandStatus is one row of the commands panel: what the profile
// declares, what it resolves to here, and whether this machine trusts it.
type miniAppCommandStatus struct {
	Name         string                  `json:"name"`
	Binary       string                  `json:"binary"`
	Description  string                  `json:"description,omitempty"`
	Permission   string                  `json:"permission"`
	Hash         string                  `json:"hash"`
	ResolvedPath string                  `json:"resolved_path,omitempty"`
	Installed    bool                    `json:"installed"`
	Trusted      bool                    `json:"trusted"`
	Source       string                  `json:"source"`
	Managers     []miniAppCommandManager `json:"managers,omitempty"`
}

type miniAppCommandManager struct {
	ID      string `json:"id"`
	Package string `json:"package"`
	Command string `json:"command"`
}

// commandStatusFor builds the status row for one profile.
func (s *Server) commandStatusFor(profile cmdprofile.ProfileSpec, source string) miniAppCommandStatus {
	status := miniAppCommandStatus{
		Name: profile.Name, Binary: profile.Binary, Description: profile.Description,
		Permission: profile.ResolvedPermission(), Source: source,
	}
	if hash, err := cmdprofile.CanonicalHash(profile); err == nil {
		status.Hash = hash
	}
	if resolved, err := cmdprofile.ResolveBinary(profile, ""); err == nil {
		status.ResolvedPath = resolved
		status.Installed = true
	}
	if status.Installed {
		status.Trusted = s.commandProfileTrustedHTTP(profile, status.Hash, status.ResolvedPath)
	}
	for _, manager := range cmdprofile.DetectManagers(profile) {
		status.Managers = append(status.Managers, miniAppCommandManager{
			ID: manager.ID, Package: manager.Package, Command: strings.Join(manager.Argv, " "),
		})
	}
	return status
}

// commandProfileTrustedHTTP mirrors the executor's trust rule: a config-hash
// match (declared or portable form) or a recorded approval for this binary.
func (s *Server) commandProfileTrustedHTTP(profile cmdprofile.ProfileSpec, hash, resolved string) bool {
	cfg := s.activeCfg()
	if cfg != nil {
		for _, declared := range cfg.Commands {
			if declaredHash, err := cmdprofile.CanonicalHash(declared); err == nil && declaredHash == hash {
				return true
			}
			if portableHash, err := cmdprofile.CanonicalHash(declared.Portable()); err == nil && portableHash == hash {
				return true
			}
		}
	}
	_ = profile
	return cmdprofile.NewTrustStore(s.miniAppsCommandsHome()).Trusted(hash, resolved)
}

// miniAppsCommandsGet reports the command profiles a draft depends on.
func (s *Server) miniAppsCommandsGet(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	app, err := s.miniAppsHTTPState().store.GetDraft(id)
	if err != nil {
		s.writeMiniAppsServiceError(w, err)
		return
	}
	items := make([]miniAppCommandStatus, 0, len(app.Requirements.Commands))
	for _, profile := range app.Requirements.Commands {
		items = append(items, s.commandStatusFor(profile, "document"))
	}
	writeMiniAppsJSON(w, http.StatusOK, map[string]any{"items": items})
}

// miniAppsCommandTrustPost records the operator's approval for one embedded
// profile, binding its content hash to the binary path resolved right now.
func (s *Server) miniAppsCommandTrustPost(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	name := strings.TrimSpace(r.PathValue("name"))
	app, err := s.miniAppsHTTPState().store.GetDraft(id)
	if err != nil {
		s.writeMiniAppsServiceError(w, err)
		return
	}
	var profile *cmdprofile.ProfileSpec
	for index := range app.Requirements.Commands {
		if app.Requirements.Commands[index].Name == name {
			profile = &app.Requirements.Commands[index]
			break
		}
	}
	if profile == nil {
		writeMiniAppsError(w, http.StatusNotFound, "not_found", "the app declares no such command profile")
		return
	}
	var body struct {
		Approved bool `json:"approved"`
	}
	if err := decodeMiniAppsJSON(w, r, &body); err != nil {
		return
	}
	if !body.Approved {
		writeMiniAppsError(w, http.StatusBadRequest, "approval_required", "trust requires approved: true")
		return
	}
	resolved, err := cmdprofile.ResolveBinary(*profile, "")
	if err != nil {
		writeMiniAppsError(w, http.StatusConflict, "binary_missing",
			"the command binary is not installed, so there is no path to bind the approval to")
		return
	}
	hash, err := cmdprofile.CanonicalHash(*profile)
	if err != nil {
		s.writeMiniAppsServiceError(w, err)
		return
	}
	if err := cmdprofile.NewTrustStore(s.miniAppsCommandsHome()).Record(hash, resolved); err != nil {
		s.writeMiniAppsServiceError(w, err)
		return
	}
	writeMiniAppsJSON(w, http.StatusOK, s.commandStatusFor(*profile, "document"))
}

// recordCommandTrustFromConfirmation persists the trust approval carried by a
// command_profile confirmation BEFORE the run resumes. The decision alone is
// inert for a tool step: it is the trust record that clears the executor's
// gate, so a write failure must abort the resume or the job would ping-pong
// back to waiting_for_confirmation forever.
func (s *Server) recordCommandTrustFromConfirmation(job miniapps.AsyncJob) error {
	if job.Confirmation == nil {
		return nil
	}
	details, ok := job.Confirmation.Details.(map[string]any)
	if !ok || details["kind"] != "command_profile" {
		return nil
	}
	hash, _ := details["hash"].(string)
	binary, _ := details["binary"].(string)
	if hash == "" || binary == "" {
		return nil
	}
	return cmdprofile.NewTrustStore(s.miniAppsCommandsHome()).Record(hash, binary)
}

// miniAppsCommandInstallPost launches an async package-manager install for one
// embedded profile. The manager id selects among the DETECTED options for the
// declared coordinates; the request can never name a package. The approved
// flag is the operator's explicit intent for this system-mutating action.
func (s *Server) miniAppsCommandInstallPost(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	name := strings.TrimSpace(r.PathValue("name"))
	app, err := s.miniAppsHTTPState().store.GetDraft(id)
	if err != nil {
		s.writeMiniAppsServiceError(w, err)
		return
	}
	profile, declared := findAppCommandProfile(app, name)
	if !declared {
		writeMiniAppsError(w, http.StatusNotFound, "not_found", "the app declares no such command profile")
		return
	}
	var body struct {
		Manager  string `json:"manager"`
		Approved bool   `json:"approved"`
	}
	if err := decodeMiniAppsJSON(w, r, &body); err != nil {
		return
	}
	if !body.Approved {
		writeMiniAppsError(w, http.StatusBadRequest, "approval_required", "installing software requires approved: true")
		return
	}
	var selected *cmdprofile.Manager
	for _, manager := range cmdprofile.DetectManagers(profile) {
		if manager.ID == strings.TrimSpace(body.Manager) {
			candidate := manager
			selected = &candidate
			break
		}
	}
	if selected == nil {
		writeMiniAppsError(w, http.StatusConflict, "manager_unavailable",
			"the requested package manager is not detected on this machine or the profile declares no coordinate for it")
		return
	}
	job, err := s.miniAppsHTTPState().service.StartCommandInstall(profile, *selected)
	if err != nil {
		s.writeMiniAppsServiceError(w, err)
		return
	}
	writeMiniAppsJSON(w, http.StatusAccepted, job)
}

func findAppCommandProfile(app miniapps.MiniApp, name string) (cmdprofile.ProfileSpec, bool) {
	for _, profile := range app.Requirements.Commands {
		if profile.Name == name {
			return profile.Clone(), true
		}
	}
	return cmdprofile.ProfileSpec{}, false
}

func (s *Server) miniAppsCommandInstallGet(w http.ResponseWriter, r *http.Request) {
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
	if job.Kind != miniapps.JobCommandInstall {
		writeMiniAppsError(w, http.StatusNotFound, "not_found", "install job not found")
		return
	}
	writeMiniAppsJSON(w, http.StatusOK, job)
}

func (s *Server) miniAppsCommandInstallEvents(w http.ResponseWriter, r *http.Request) {
	jobID := strings.TrimSpace(r.PathValue("job_id"))
	if !validMiniAppsID(jobID) {
		writeMiniAppsError(w, http.StatusBadRequest, "invalid_job_id", "invalid job id")
		return
	}
	state := s.miniAppsHTTPState()
	if job, err := state.service.GetJob(jobID); err != nil || job.Kind != miniapps.JobCommandInstall {
		writeMiniAppsError(w, http.StatusNotFound, "not_found", "install job not found")
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
	}, func(event miniapps.JobEvent) ([]byte, error) { return json.Marshal(event) })
}
