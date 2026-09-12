//go:build miniapps

// Package miniapps contains the optional Mini Apps domain. The package is kept
// independent from HTTP, UI, ACP, and the agent adapters so the same contract
// can be used by the service and by the standalone runner.
package miniapps

import (
	"github.com/hijera/foxxycode-agent/internal/cmdprofile"

	"encoding/json"
	"time"
)

const (
	SchemaVersion = "1.0.0"
	KindMiniApp   = "foxxycode.miniapp"
)

type AppState string

const (
	StateDraft    AppState = "draft"
	StateReleased AppState = "released"
)

// MiniApp is the reviewed, portable runtime document. Authoring evidence is
// deliberately stored separately by Store and is never part of this value.
type MiniApp struct {
	SchemaVersion string         `json:"schema_version"`
	Kind          string         `json:"kind"`
	ID            string         `json:"id"`
	State         AppState       `json:"state"`
	Version       string         `json:"version,omitempty"`
	Revision      string         `json:"revision,omitempty"`
	Metadata      Metadata       `json:"metadata"`
	Requirements  Requirements   `json:"requirements,omitempty"`
	Permissions   Permissions    `json:"permissions,omitempty"`
	Inputs        []Input        `json:"inputs,omitempty"`
	Workflow      []Step         `json:"workflow"`
	Success       SuccessSpec    `json:"success"`
	Outputs       []Output       `json:"outputs,omitempty"`
	Display       DisplaySpec    `json:"display,omitempty"`
	Runtime       RuntimePolicy  `json:"runtime"`
	Extensions    map[string]any `json:"extensions,omitempty"`
}

type Metadata struct {
	Name                     string   `json:"name"`
	Description              string   `json:"description,omitempty"`
	Goal                     string   `json:"goal"`
	Author                   string   `json:"author,omitempty"`
	Tags                     []string `json:"tags,omitempty"`
	EstimatedDurationSeconds int      `json:"estimated_duration_seconds,omitempty"`
	Archived                 bool     `json:"archived,omitempty"`
}

type Requirements struct {
	ModelBindings []ModelBinding `json:"model_bindings,omitempty"`
	// Commands are the command profiles this app carries. They travel with the
	// portable document; running one on a new machine requires a local trust
	// approval bound to the profile hash and the resolved binary path.
	Commands []cmdprofile.ProfileSpec `json:"commands,omitempty"`
}

type ModelBinding struct {
	ID                   string           `json:"id"`
	Selection            string           `json:"selection"`
	Provider             ProviderIdentity `json:"provider,omitempty"`
	Model                string           `json:"model,omitempty"`
	RequiredCapabilities []string         `json:"required_capabilities,omitempty"`
}

type ProviderIdentity struct {
	Type    string `json:"type,omitempty"`
	BaseURL string `json:"base_url,omitempty"`
	Scope   string `json:"scope,omitempty"`
}

type Permissions struct {
	Tools  []string `json:"tools,omitempty"`
	Models []string `json:"models,omitempty"`
	Apps   []string `json:"apps,omitempty"`
}

type Input struct {
	ID           string     `json:"id"`
	Type         string     `json:"type"`
	Title        string     `json:"title"`
	Description  string     `json:"description,omitempty"`
	Required     bool       `json:"required,omitempty"`
	Default      any        `json:"default,omitempty"`
	Validation   Validation `json:"validation,omitempty"`
	UI           InputUI    `json:"ui"`
	VisibleWhen  *Condition `json:"visible_when,omitempty"`
	EnabledWhen  *Condition `json:"enabled_when,omitempty"`
	RequiredWhen *Condition `json:"required_when,omitempty"`
}

type Validation struct {
	Enum           []any    `json:"enum,omitempty"`
	Minimum        *float64 `json:"minimum,omitempty"`
	Maximum        *float64 `json:"maximum,omitempty"`
	MinLength      *int     `json:"min_length,omitempty"`
	MaxLength      *int     `json:"max_length,omitempty"`
	Pattern        string   `json:"pattern,omitempty"`
	Extensions     []string `json:"extensions,omitempty"`
	MediaTypes     []string `json:"media_types,omitempty"`
	MaxFiles       *int     `json:"max_files,omitempty"`
	MaxTotalBytes  *int64   `json:"max_total_bytes,omitempty"`
	MustExist      bool     `json:"must_exist,omitempty"`
	FilesystemKind string   `json:"filesystem_kind,omitempty"`
}

type InputUI struct {
	Control     string `json:"control"`
	Order       int    `json:"order,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
}

type RetryPolicy struct {
	MaxAttempts int `json:"max_attempts,omitempty"`
	DelayMS     int `json:"delay_ms,omitempty"`
}

// Step is a strict v1 tagged union. Fields not belonging to the selected Kind
// are rejected by Validate, which prevents dormant deferred contracts from
// becoming executable by accident.
type Step struct {
	ID             string      `json:"id"`
	Kind           string      `json:"kind"`
	Title          string      `json:"title"`
	When           *Condition  `json:"when,omitempty"`
	TimeoutSeconds int         `json:"timeout_seconds,omitempty"`
	Retry          RetryPolicy `json:"retry,omitempty"`

	Tool         string         `json:"tool,omitempty"`
	Arguments    any            `json:"arguments,omitempty"`
	Prompt       string         `json:"prompt,omitempty"`
	ModelBinding string         `json:"model_binding,omitempty"`
	Tools        []string       `json:"tools,omitempty"`
	MaxTurns     int            `json:"max_turns,omitempty"`
	OutputSchema map[string]any `json:"output_schema,omitempty"`

	Message string `json:"message,omitempty"`
	Details any    `json:"details,omitempty"`

	If   *Condition `json:"if,omitempty"`
	Then []Step     `json:"then,omitempty"`
	Else []Step     `json:"else,omitempty"`

	AppID      string         `json:"app_id,omitempty"`
	AppVersion string         `json:"app_version,omitempty"`
	InputMap   map[string]any `json:"inputs,omitempty"`
}

type Condition struct {
	Op    string      `json:"op"`
	Args  []Condition `json:"args,omitempty"`
	Left  any         `json:"left,omitempty"`
	Right any         `json:"right,omitempty"`
	Value any         `json:"value,omitempty"`
}

type Ref struct {
	Ref string `json:"$ref"`
}

type SuccessSpec struct {
	Mode                string         `json:"mode"`
	Expectations        string         `json:"expectations,omitempty"`
	ExpectedResult      string         `json:"expected_result,omitempty"`
	AcceptanceCriterion string         `json:"acceptance_criterion,omitempty"`
	Checks              []SuccessCheck `json:"checks"`
}

type SuccessCheck struct {
	Kind         string         `json:"kind"`
	Step         string         `json:"step,omitempty"`
	Status       string         `json:"status,omitempty"`
	Value        any            `json:"value,omitempty"`
	Schema       map[string]any `json:"schema,omitempty"`
	Path         any            `json:"path,omitempty"`
	Prompt       string         `json:"prompt,omitempty"`
	ModelBinding string         `json:"model_binding,omitempty"`
}

type Output struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Value    any            `json:"value"`
	Schema   map[string]any `json:"schema,omitempty"`
	Renderer string         `json:"renderer,omitempty"`
	Title    string         `json:"title,omitempty"`
}

type DisplaySpec struct {
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Layout      string   `json:"layout,omitempty"`
	Sections    []string `json:"sections,omitempty"`
}

type RuntimePolicy struct {
	LogScope              string `json:"log_scope"`
	OperatorEventLevel    string `json:"operator_event_level"`
	DiagnosticToolEvents  string `json:"diagnostic_tool_events"`
	PersistAgentReasoning bool   `json:"persist_agent_reasoning"`
}

// SourceEvidence is private authoring data. It is intentionally a different
// type from MiniApp so session provenance cannot accidentally be exported.
// Trace-specific values are owned by trace.go; maps keep this core package
// independent of the extraction implementation.
type SourceEvidence struct {
	SessionID          string                   `json:"source_session_id,omitempty"`
	ScenarioCandidates []TraceScenarioCandidate `json:"scenario_candidates,omitempty"`
	ConfirmedScenario  *TraceConfirmedScenario  `json:"confirmed_scenario,omitempty"`
	SanitizedTrace     *NormalizedTrace         `json:"sanitized_trace,omitempty"`
	AcceptedResult     any                      `json:"accepted_result,omitempty"`
	SourceFixture      map[string]any           `json:"source_fixture,omitempty"`
	FixtureFiles       map[string][]byte        `json:"fixture_files,omitempty"`
	Metrics            map[string]any           `json:"metrics,omitempty"`
	CreatedAt          time.Time                `json:"created_at"`
}

// AuthoringEvidence is the plan's public name for the private evidence file.
type AuthoringEvidence = SourceEvidence

type CatalogEntry struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	State       AppState  `json:"state"`
	Version     string    `json:"version,omitempty"`
	Revision    string    `json:"revision,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	Archived    bool      `json:"archived,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type RunStatus string

const (
	RunPending                RunStatus = "pending"
	RunRunning                RunStatus = "running"
	RunWaitingForInput        RunStatus = "waiting_for_input"
	RunWaitingForConfirmation RunStatus = "waiting_for_confirmation"
	RunSucceeded              RunStatus = "succeeded"
	RunFailed                 RunStatus = "failed"
	RunCancelled              RunStatus = "cancelled"
	RunInterrupted            RunStatus = "interrupted"
)

type StepResult struct {
	ID         string         `json:"id"`
	Status     RunStatus      `json:"status"`
	Outputs    map[string]any `json:"outputs,omitempty"`
	Attempts   int            `json:"attempts,omitempty"`
	Error      string         `json:"error,omitempty"`
	StartedAt  time.Time      `json:"started_at"`
	FinishedAt time.Time      `json:"finished_at"`
}

type RunArtifact struct {
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`
}

type Run struct {
	ID         string                `json:"id"`
	AppID      string                `json:"app_id"`
	Version    string                `json:"version,omitempty"`
	Revision   string                `json:"revision,omitempty"`
	Test       bool                  `json:"test"`
	Status     RunStatus             `json:"status"`
	Inputs     map[string]any        `json:"inputs,omitempty"`
	Outputs    map[string]any        `json:"outputs,omitempty"`
	Steps      map[string]StepResult `json:"steps,omitempty"`
	Artifacts  []RunArtifact         `json:"artifacts,omitempty"`
	Error      string                `json:"error,omitempty"`
	StartedAt  time.Time             `json:"started_at"`
	FinishedAt time.Time             `json:"finished_at"`
	LogPath    string                `json:"log_path,omitempty"`
	EventsPath string                `json:"events_path,omitempty"`
}

type ValidationIssue struct {
	Path     string `json:"path"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type ValidationReport struct {
	Valid  bool              `json:"valid"`
	Issues []ValidationIssue `json:"issues"`
}

// CapabilitySet is optional validation context supplied by the registry and
// configured model catalog. Plain Validate still checks declaration shape.
type CapabilitySet struct {
	Tools  map[string]bool
	Models map[string]bool
	Apps   map[string]bool
}

type DistillationStatus string

const (
	DistillationQueued          DistillationStatus = "queued"
	DistillationAnalyzing       DistillationStatus = "analyzing"
	DistillationWaitingScenario DistillationStatus = "waiting_for_scenario"
	DistillationCompleted       DistillationStatus = "completed"
	DistillationFailed          DistillationStatus = "failed"
	DistillationCancelled       DistillationStatus = "cancelled"
)

type DistillationJob struct {
	ID        string             `json:"id"`
	SessionID string             `json:"session_id"`
	Status    DistillationStatus `json:"status"`
	Phase     string             `json:"phase,omitempty"`
	Progress  int                `json:"progress,omitempty"`
	AppID     string             `json:"app_id,omitempty"`
	Summary   string             `json:"summary,omitempty"`
	Error     string             `json:"error,omitempty"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
}

// cloneJSON is used by the store when crossing the draft/release boundary.
func cloneJSON[T any](in T) (T, error) {
	var out T
	raw, err := json.Marshal(in)
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(raw, &out)
	return out, err
}
