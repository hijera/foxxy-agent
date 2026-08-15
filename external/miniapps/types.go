//go:build miniapps

// Package miniapps implements the portable FoxxyCode mini-app format, storage,
// validation, and interpreter. It is linked only into builds tagged miniapps.
package miniapps

import (
	"encoding/json"
	"time"
)

const (
	SchemaVersion = "1.0.0"
	KindMiniApp   = "foxxycode.miniapp"
	VMVersion     = "foxxy-vm/1"
)

type AppState string

const (
	StateDraft    AppState = "draft"
	StateReleased AppState = "released"
)

// MiniApp is the canonical portable JSON document.
type MiniApp struct {
	SchemaVersion string         `json:"schema_version"`
	Kind          string         `json:"kind"`
	ID            string         `json:"id"`
	Version       string         `json:"version,omitempty"`
	State         AppState       `json:"state"`
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
	Description              string   `json:"description"`
	Goal                     string   `json:"goal"`
	Author                   string   `json:"author,omitempty"`
	Tags                     []string `json:"tags,omitempty"`
	EstimatedDurationSeconds int      `json:"estimated_duration_seconds,omitempty"`
	Archived                 bool     `json:"archived,omitempty"`
}

type Requirements struct {
	Engine        string         `json:"engine,omitempty"`
	Platforms     []string       `json:"platforms,omitempty"`
	Executables   []Executable   `json:"executables,omitempty"`
	Runtimes      []Runtime      `json:"runtimes,omitempty"`
	ModelBindings []ModelBinding `json:"model_bindings,omitempty"`
	Skills        []AssetBinding `json:"skills,omitempty"`
	MCPServers    []AssetBinding `json:"mcp_servers,omitempty"`
}

type Executable struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type Runtime struct {
	ID        string           `json:"id"`
	Version   string           `json:"version"`
	Provision *ProvisionPolicy `json:"provision,omitempty"`
}

type ProvisionPolicy struct {
	Mode          string `json:"mode"`
	Interaction   string `json:"interaction"`
	URL           string `json:"url,omitempty"`
	SHA256        string `json:"sha256,omitempty"`
	SizeBytes     int64  `json:"size_bytes,omitempty"`
	InstallScript string `json:"install_script,omitempty"`
}

type AssetBinding struct {
	ID      string `json:"id"`
	Version string `json:"version,omitempty"`
	Path    string `json:"path,omitempty"`
}

type ModelBinding struct {
	ID                   string            `json:"id"`
	LogicalModel         string            `json:"logical_model,omitempty"`
	Selection            string            `json:"selection"`
	Provider             ProviderIdentity  `json:"provider,omitempty"`
	Model                string            `json:"model,omitempty"`
	Credentials          CredentialBinding `json:"credentials,omitempty"`
	RequiredCapabilities []string          `json:"required_capabilities,omitempty"`
	LocalBootstrap       *LocalBootstrap   `json:"local_bootstrap,omitempty"`
}

type ProviderIdentity struct {
	Type    string `json:"type,omitempty"`
	BaseURL string `json:"base_url,omitempty"`
	Scope   string `json:"scope,omitempty"`
	Adapter string `json:"adapter,omitempty"`
}

type CredentialBinding struct {
	Source string `json:"source,omitempty"`
}

type LocalBootstrap struct {
	Connect        bool   `json:"connect,omitempty"`
	Start          string `json:"start,omitempty"`
	EnsureModel    string `json:"ensure_model,omitempty"`
	Load           string `json:"load,omitempty"`
	StorageScope   string `json:"storage_scope,omitempty"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
}

type Permissions struct {
	Filesystem []FilesystemPermission `json:"filesystem,omitempty"`
	Network    []NetworkPermission    `json:"network,omitempty"`
	Commands   []CommandPermission    `json:"commands,omitempty"`
	Models     []string               `json:"models,omitempty"`
	Skills     []string               `json:"skills,omitempty"`
	MCPServers []string               `json:"mcp_servers,omitempty"`
}

type FilesystemPermission struct {
	Access string `json:"access"`
	Path   any    `json:"path"`
}

type NetworkPermission struct {
	Host    string   `json:"host"`
	Methods []string `json:"methods,omitempty"`
}

type CommandPermission struct {
	Executable string `json:"executable"`
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

// Step uses kind-specific fields so the entire workflow remains editable JSON.
type Step struct {
	ID    string     `json:"id"`
	Kind  string     `json:"kind"`
	Title string     `json:"title"`
	When  *Condition `json:"when,omitempty"`
	// If is the selector of a branch step. It is separate from When, which
	// gates every step kind: a branch whose When is false is skipped whole,
	// while a branch whose If is false runs its Else steps.
	If             *Condition               `json:"if,omitempty"`
	TimeoutSeconds int                      `json:"timeout_seconds,omitempty"`
	Retry          RetryPolicy              `json:"retry,omitempty"`
	Language       string                   `json:"language,omitempty"`
	Entry          string                   `json:"entry,omitempty"`
	Imports        map[string]ProgramImport `json:"imports,omitempty"`
	Functions      map[string][]Instruction `json:"functions,omitempty"`
	Limits         ProgramLimits            `json:"limits,omitempty"`
	OutputSchema   map[string]any           `json:"output_schema,omitempty"`
	Prompt         string                   `json:"prompt,omitempty"`
	ModelBinding   string                   `json:"model_binding,omitempty"`
	Command        string                   `json:"command,omitempty"`
	Args           []any                    `json:"args,omitempty"`
	Environment    map[string]any           `json:"environment,omitempty"`
	WorkingDir     any                      `json:"working_directory,omitempty"`
	Script         string                   `json:"script,omitempty"`
	ScriptLanguage string                   `json:"script_language,omitempty"`
	Request        *APIRequest              `json:"request,omitempty"`
	Operation      string                   `json:"operation,omitempty"`
	Path           any                      `json:"path,omitempty"`
	Value          any                      `json:"value,omitempty"`
	Message        string                   `json:"message,omitempty"`
	Details        any                      `json:"details,omitempty"`
	Then           []Step                   `json:"then,omitempty"`
	Else           []Step                   `json:"else,omitempty"`
	AppID          string                   `json:"app_id,omitempty"`
	AppVersion     string                   `json:"app_version,omitempty"`
	InputMap       map[string]any           `json:"inputs,omitempty"`
	Durable        bool                     `json:"durable,omitempty"`
	Resumable      bool                     `json:"resumable,omitempty"`
}

type APIRequest struct {
	Method  string         `json:"method"`
	URL     string         `json:"url"`
	Headers map[string]any `json:"headers,omitempty"`
	Body    any            `json:"body,omitempty"`
}

type Program struct {
	Language     string                   `json:"language"`
	Entry        string                   `json:"entry"`
	Imports      map[string]ProgramImport `json:"imports,omitempty"`
	Functions    map[string][]Instruction `json:"functions"`
	Limits       ProgramLimits            `json:"limits"`
	OutputSchema map[string]any           `json:"output_schema,omitempty"`
}

type ProgramImport struct {
	Capability   string         `json:"capability"`
	InputSchema  map[string]any `json:"input_schema,omitempty"`
	OutputSchema map[string]any `json:"output_schema,omitempty"`
}

type Instruction struct {
	Op  string `json:"op"`
	Arg any    `json:"arg,omitempty"`
}

type ProgramLimits struct {
	Instructions    int   `json:"instructions"`
	WallTimeSeconds int   `json:"wall_time_seconds,omitempty"`
	HeapBytes       int64 `json:"heap_bytes,omitempty"`
	StackDepth      int   `json:"stack_depth"`
	CallDepth       int   `json:"call_depth"`
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
	Export   bool           `json:"export,omitempty"`
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

type SourceEvidence struct {
	SessionID      string    `json:"session_id,omitempty"`
	AcceptedResult string    `json:"accepted_result,omitempty"`
	SanitizedUser  string    `json:"sanitized_user,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type CatalogEntry struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	State       AppState  `json:"state"`
	Version     string    `json:"version,omitempty"`
	Revision    string    `json:"revision,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	Archived    bool      `json:"archived,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type RunStatus string

const (
	RunPending   RunStatus = "pending"
	RunRunning   RunStatus = "running"
	RunSucceeded RunStatus = "succeeded"
	RunFailed    RunStatus = "failed"
	RunCancelled RunStatus = "cancelled"
)

type StepResult struct {
	ID         string         `json:"id"`
	Status     RunStatus      `json:"status"`
	Outputs    map[string]any `json:"outputs,omitempty"`
	Error      string         `json:"error,omitempty"`
	StartedAt  time.Time      `json:"started_at"`
	FinishedAt time.Time      `json:"finished_at"`
}

type Run struct {
	ID         string                `json:"id"`
	AppID      string                `json:"app_id"`
	Version    string                `json:"version,omitempty"`
	Revision   string                `json:"revision,omitempty"`
	Test       bool                  `json:"test"`
	Status     RunStatus             `json:"status"`
	Inputs     map[string]any        `json:"inputs"`
	Outputs    map[string]any        `json:"outputs,omitempty"`
	Steps      map[string]StepResult `json:"steps,omitempty"`
	Error      string                `json:"error,omitempty"`
	StartedAt  time.Time             `json:"started_at"`
	FinishedAt time.Time             `json:"finished_at"`
	LogPath    string                `json:"log_path,omitempty"`
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

type DistillationStatus string

const (
	DistillationQueued    DistillationStatus = "queued"
	DistillationAnalyzing DistillationStatus = "analyzing"
	DistillationCompleted DistillationStatus = "completed"
	DistillationFailed    DistillationStatus = "failed"
	DistillationCancelled DistillationStatus = "cancelled"
)

type DistillationJob struct {
	ID        string             `json:"id"`
	SessionID string             `json:"session_id"`
	Status    DistillationStatus `json:"status"`
	Phase     string             `json:"phase"`
	Progress  int                `json:"progress"`
	AppID     string             `json:"app_id,omitempty"`
	Summary   string             `json:"summary,omitempty"`
	Error     string             `json:"error,omitempty"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
}

func cloneJSON[T any](in T) (T, error) {
	var out T
	data, err := json.Marshal(in)
	if err != nil {
		return out, err
	}
	err = json.Unmarshal(data, &out)
	return out, err
}
