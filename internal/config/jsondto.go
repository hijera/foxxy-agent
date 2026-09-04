package config

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ConfigJSON is the JSON shape for GET/PUT /foxxycode/config (snake_case keys match YAML).
type ConfigJSON struct {
	Providers    []ProviderJSON   `json:"providers,omitempty"`
	Models       []ModelJSON      `json:"models,omitempty"`
	Agent        AgentJSON        `json:"agent,omitempty"`
	Autocomplete AutocompleteJSON `json:"autocomplete,omitempty"`
	Prompts      PromptsJSON      `json:"prompts,omitempty"`
	Instructions InstructionsJSON `json:"instructions,omitempty"`
	Skills       SkillsJSON       `json:"skills,omitempty"`
	MCPServers   []MCPServerJSON  `json:"mcp_servers,omitempty"`
	MCP          MCPJSON          `json:"mcp,omitempty"`
	Tools        ToolsJSON        `json:"tools,omitempty"`
	Logger       LoggerJSON       `json:"logger,omitempty"`
	Sessions     SessionsJSON     `json:"sessions,omitempty"`
	Memory       MemoryJSON       `json:"memory,omitempty"`
	Compaction   CompactionJSON   `json:"compaction,omitempty"`
	Title        TitleJSON        `json:"title,omitempty"`
	HTTPServer   HTTPServerJSON   `json:"httpserver,omitempty"`
	Scheduler    SchedulerJSON    `json:"scheduler,omitempty"`
	Gateways     GatewaysJSON     `json:"gateways,omitempty"`
	UI           UIJSON           `json:"ui,omitempty"`
	Browser      BrowserJSON      `json:"browser,omitempty"`
	VCS          VCSJSON          `json:"vcs,omitempty"`
	Debug        DebugJSON        `json:"debug,omitempty"`
}

// VCSJSON mirrors VCSConfig for JSON APIs.
type VCSJSON struct {
	SVN SVNJSON `json:"svn,omitempty"`
}

// SVNJSON mirrors SVNConfig for JSON APIs. Enabled and BranchLookup are pointers
// so an unset value round-trips as "use default" (true) rather than false.
type SVNJSON struct {
	Enabled        *bool  `json:"enabled,omitempty"`
	Binary         string `json:"binary,omitempty"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
	BranchLookup   *bool  `json:"branch_lookup,omitempty"`
}

// BrowserJSON mirrors BrowserConfig for JSON APIs. Headless is a pointer so an unset
// value round-trips as "use default" (true) rather than an explicit false.
type BrowserJSON struct {
	Enabled        bool   `json:"enabled,omitempty"`
	Headless       *bool  `json:"headless,omitempty"`
	ExecutablePath string `json:"executable_path,omitempty"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
}

// UIJSON mirrors UIConfig for JSON APIs.
type UIJSON struct {
	Enabled    *bool  `json:"enabled,omitempty"`
	Locale     string `json:"locale,omitempty"`
	SendMode   string `json:"send_mode,omitempty"`
	StatusLine *bool  `json:"status_line,omitempty"`
}

// GatewaysJSON mirrors GatewayConfig for JSON APIs.
type GatewaysJSON struct {
	Telegram TelegramGatewayJSON `json:"telegram,omitempty"`
}

// TelegramGatewayJSON mirrors TelegramGatewayConfig.
type TelegramGatewayJSON struct {
	Enabled          bool                    `json:"enabled,omitempty"`
	Token            string                  `json:"token,omitempty"`
	Proxy            string                  `json:"proxy,omitempty"`
	RichMessages     bool                    `json:"rich_messages,omitempty"`
	Admins           []int64                 `json:"admins,omitempty"`
	DefaultAccess    string                  `json:"default_access,omitempty"`
	DefaultIsolation string                  `json:"default_isolation,omitempty"`
	UserGroups       []TelegramUserGroupJSON `json:"user_groups,omitempty"`
	Chats            []TelegramChatJSON      `json:"chats,omitempty"`
}

// TelegramUserGroupJSON mirrors TelegramUserGroup.
type TelegramUserGroupJSON struct {
	Name    string  `json:"name"`
	UserIDs []int64 `json:"user_ids,omitempty"`
}

// TelegramChatJSON mirrors TelegramChatConfig.
type TelegramChatJSON struct {
	ChatID    int64  `json:"chat_id"`
	Isolation string `json:"isolation,omitempty"`
	Access    string `json:"access,omitempty"`
}

// InstructionsJSON mirrors Instructions for JSON APIs.
type InstructionsJSON struct {
	Files []string `json:"files,omitempty"`
}

// ProviderJSON mirrors ProviderConfig for JSON APIs.
// Field order and types must match ProviderConfig (direct struct conversion is used below).
type ProviderJSON struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	APIBase       string `json:"api_base,omitempty"`
	APIKey        string `json:"api_key,omitempty"`
	APIKeyCommand string `json:"api_key_command,omitempty"`
	Proxy         string `json:"proxy,omitempty"`
	TimeoutMS     int    `json:"timeout_ms,omitempty"`
}

// ModelJSON mirrors ModelEntry for JSON APIs.
// Field order and types must match ModelEntry (direct struct conversion is used below).
type ModelJSON struct {
	Model            string   `json:"model"`
	MaxTokens        int      `json:"max_tokens"`
	Temperature      float64  `json:"temperature"`
	MaxContextTokens int      `json:"max_context_tokens,omitempty"`
	Multimodal       bool     `json:"multimodal,omitempty"`
	ReasoningLevels  []string `json:"reasoning_levels,omitempty"`
	ReasoningDefault string   `json:"reasoning_default,omitempty"`
	// Stream keeps the unset/explicit distinction of ModelEntry.Stream: a settings
	// round trip must not turn an omitted key into an explicit false.
	Stream *bool `json:"stream,omitempty"`
}

// AgentJSON mirrors Agent for JSON APIs. Pointer fields keep the unset/explicit
// distinction where an explicit 0 means something different from "unset": the
// loop-guard counters, llm_retry_max (0 disables retries), and
// llm_first_token_timeout_ms (0 disables the silence guard).
type AgentJSON struct {
	Model                  string `json:"model"`
	MaxTurns               int    `json:"max_turns,omitempty"`
	MaxTokensPerTurn       int    `json:"max_tokens_per_turn,omitempty"`
	LLMRetryMax            *int   `json:"llm_retry_max,omitempty"`
	LLMRetryBaseMS         int    `json:"llm_retry_base_ms,omitempty"`
	LLMMinIntervalMS       int    `json:"llm_min_interval_ms,omitempty"`
	LLMFirstTokenTimeoutMS *int   `json:"llm_first_token_timeout_ms,omitempty"`
	LoopGuard              *bool  `json:"loop_guard,omitempty"`
	LoopToolRepeatLimit    *int   `json:"loop_tool_repeat_limit,omitempty"`
	LoopStreamRepeatCycles *int   `json:"loop_stream_repeat_cycles,omitempty"`
	LoopNudgeMax           *int   `json:"loop_nudge_max,omitempty"`
}

// PromptsJSON mirrors Prompts for JSON APIs.
type PromptsJSON struct {
	Dir         string                  `json:"dir,omitempty"`
	AgentPrompt string                  `json:"agent_prompt,omitempty"`
	PlanPrompt  string                  `json:"plan_prompt,omitempty"`
	PerProvider *PerProviderPromptsJSON `json:"per_provider,omitempty"`
}

// PerProviderPromptsJSON mirrors PerProviderPrompts. Enabled is a pointer so an
// unset value round-trips as "use default" rather than an explicit false.
type PerProviderPromptsJSON struct {
	Enabled *bool `json:"enabled,omitempty"`
}

// SkillsJSON mirrors Skills for JSON APIs.
type SkillsJSON struct {
	Dirs          []string `json:"dirs,omitempty"`
	Sources       []string `json:"sources,omitempty"`
	AutoDiscovery *bool    `json:"auto_discovery,omitempty"`
}

// MCPServerJSON mirrors MCPServerConfig for JSON APIs.
type MCPServerJSON struct {
	Type               string           `json:"type,omitempty"`
	Name               string           `json:"name"`
	Command            string           `json:"command,omitempty"`
	Args               []string         `json:"args,omitempty"`
	Env                []EnvVarJSON     `json:"env,omitempty"`
	URL                string           `json:"url,omitempty"`
	Headers            []HTTPHeaderJSON `json:"headers,omitempty"`
	InsecureSkipVerify bool             `json:"insecure_skip_verify,omitempty"`
	Disabled           bool             `json:"disabled,omitempty"`
	DisabledTools      []string         `json:"disabled_tools,omitempty"`
}

// EnvVarJSON mirrors EnvVarConfig.
type EnvVarJSON struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// HTTPHeaderJSON mirrors HTTPHeaderConfig.
type HTTPHeaderJSON struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// MCPJSON mirrors MCP for JSON APIs.
type MCPJSON struct {
	ProjectTrust string `json:"project_trust,omitempty"`
}

// ToolsJSON mirrors Tools for JSON APIs.
type ToolsJSON struct {
	PermissionMode          string   `json:"permission_mode,omitempty"`
	CommandAllowlist        []string `json:"command_allowlist,omitempty"`
	PlanNoSelfRun           *bool    `json:"plan_no_self_run,omitempty"`
	AskDisableExtendedTools bool     `json:"ask_disable_extended_tools,omitempty"`
	// omitempty does not apply to structs; all-nil limits serialize as {}.
	OutputLimits ToolOutputLimitsJSON `json:"output_limits"`
	Background   ToolBackgroundJSON   `json:"background"`
}

// ToolBackgroundJSON mirrors ToolBackground for JSON APIs.
type ToolBackgroundJSON struct {
	Enabled               *bool `json:"enabled,omitempty"`
	MaxConcurrent         int   `json:"max_concurrent,omitempty"`
	DefaultTimeoutSeconds int   `json:"default_timeout_seconds,omitempty"`
	MaxTimeoutSeconds     int   `json:"max_timeout_seconds,omitempty"`
	OutputBufferBytes     int   `json:"output_buffer_bytes,omitempty"`
}

type ToolOutputLimitsJSON struct {
	Read          *int `json:"read,omitempty"`
	Grep          *int `json:"grep,omitempty"`
	Glob          *int `json:"glob,omitempty"`
	PrintTree     *int `json:"print_tree,omitempty"`
	RunCommand    *int `json:"run_command,omitempty"`
	SSHRunCommand *int `json:"ssh_run_command,omitempty"`
	WebFetch      *int `json:"webfetch,omitempty"`
	WebSearch     *int `json:"websearch,omitempty"`
	Default       *int `json:"default,omitempty"`
}

// LoggerJSON mirrors Logger for JSON APIs.
type LoggerJSON struct {
	Level    string             `json:"level,omitempty"`
	Outputs  []string           `json:"outputs,omitempty"`
	File     string             `json:"file,omitempty"`
	Format   string             `json:"format,omitempty"`
	Rotation LoggerRotationJSON `json:"rotation,omitempty"`
}

// DebugJSON mirrors Debug for JSON APIs. CaptureLLM is a pointer so an unset
// value round-trips as "follow Enabled" rather than false.
type DebugJSON struct {
	Enabled    bool  `json:"enabled"`
	CaptureLLM *bool `json:"capture_llm,omitempty"`
}

// LoggerRotationJSON mirrors LoggerRotation.
type LoggerRotationJSON struct {
	MaxSizeMB int `json:"max_size_mb,omitempty"`
	MaxFiles  int `json:"max_files,omitempty"`
}

// SessionsJSON mirrors Sessions.
type SessionsJSON struct {
	Dir string `json:"dir,omitempty"`
}

// MemoryJSON mirrors MemoryConfig.
type MemoryJSON struct {
	Enabled          bool   `json:"enabled,omitempty"`
	Model            string `json:"model,omitempty"`
	Dir              string `json:"dir,omitempty"`
	RecallMaxTurns   int    `json:"recall_max_turns,omitempty"`
	PersistMaxTurns  int    `json:"persist_max_turns,omitempty"`
	CopilotMaxTokens int    `json:"copilot_max_tokens,omitempty"`
	MaxSearchHits    int    `json:"max_search_hits,omitempty"`
}

// CompactionJSON mirrors CompactionConfig. Enabled is a pointer so an unset value round-trips as
// "use default" (true) rather than an explicit false; KeepRecentTurns is a pointer so an explicit
// 0 (keep nothing verbatim) round-trips distinctly from unset.
type CompactionJSON struct {
	Engine           string `json:"engine,omitempty"`
	Enabled          *bool  `json:"enabled,omitempty"`
	Model            string `json:"model,omitempty"`
	ThresholdPercent int    `json:"threshold_percent,omitempty"`
	KeepRecentTurns  *int   `json:"keep_recent_turns,omitempty"`
	MaxTokens        int    `json:"max_tokens,omitempty"`
	// omitempty does not apply to structs; unset eviction serializes as {}.
	ResultEviction ResultEvictionJSON `json:"result_eviction"`
}

type ResultEvictionJSON struct {
	Enabled        *bool `json:"enabled,omitempty"`
	KeepRecent     *int  `json:"keep_recent,omitempty"`
	MinResultBytes *int  `json:"min_result_bytes,omitempty"`
}

// AutocompleteJSON mirrors AutocompleteConfig. Enabled and MultiLine are pointers so an unset
// value round-trips as "use the default" rather than an explicit false. Note that Enabled defaults
// to false here, unlike the other optional sections: suggestions cost tokens per keystroke.
type AutocompleteJSON struct {
	Enabled        *bool   `json:"enabled,omitempty"`
	Model          string  `json:"model,omitempty"`
	Mode           string  `json:"mode,omitempty"`
	Temperature    float64 `json:"temperature,omitempty"`
	MaxTokens      int     `json:"max_tokens,omitempty"`
	TimeoutMS      int     `json:"timeout_ms,omitempty"`
	DebounceMS     int     `json:"debounce_ms,omitempty"`
	Trigger        string  `json:"trigger,omitempty"`
	MultiLine      *bool   `json:"multi_line,omitempty"`
	MaxPrefixBytes int     `json:"max_prefix_bytes,omitempty"`
	MaxSuffixBytes int     `json:"max_suffix_bytes,omitempty"`
	RelatedFiles   *int    `json:"related_files,omitempty"`
}

// TitleJSON mirrors TitleConfig. Enabled is a pointer so an unset value round-trips as
// "use default" (true) rather than an explicit false.
type TitleJSON struct {
	Enabled   *bool  `json:"enabled,omitempty"`
	Model     string `json:"model,omitempty"`
	MaxTokens int    `json:"max_tokens,omitempty"`
}

// HTTPServerJSON mirrors HTTPServerConfig. AuthToken is write-only: ConfigToJSONDTO never
// populates it (redacted), reporting only whether one is set via AuthConfigured.
type HTTPServerJSON struct {
	Host           string `json:"host,omitempty"`
	Port           int    `json:"port,omitempty"`
	AuthToken      string `json:"auth_token,omitempty"`
	AuthConfigured bool   `json:"auth_configured,omitempty"`
	PublicDocs     bool   `json:"public_docs,omitempty"`
	// StreamTicketsOnly mirrors HTTPServerConfig.StreamTicketsOnly.
	StreamTicketsOnly bool             `json:"stream_tickets_only,omitempty"`
	AllowInsecure     bool             `json:"allow_insecure,omitempty"`
	CORS              HTTPCORSJSON     `json:"cors,omitempty"`
	Remotes           []HTTPRemoteJSON `json:"remotes,omitempty"`
}

// HTTPCORSJSON mirrors HTTPCORSConfig.
type HTTPCORSJSON struct {
	Enabled        bool     `json:"enabled,omitempty"`
	AllowedOrigins []string `json:"allowed_origins,omitempty"`
}

// HTTPRemoteJSON mirrors HTTPRemote.
type HTTPRemoteJSON struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// SchedulerJSON mirrors SchedulerConfig.
type SchedulerJSON struct {
	Enabled        bool   `json:"enabled,omitempty"`
	Dir            string `json:"dir,omitempty"`
	MaxQueue       int    `json:"max_queue,omitempty"`
	Timeout        string `json:"timeout,omitempty"`
	RetainSessions int    `json:"retain_sessions,omitempty"`
}

// ConfigToJSONDTO copies a loaded Config into ConfigJSON (for GET /foxxycode/config).
func ConfigToJSONDTO(c *Config) *ConfigJSON {
	if c == nil {
		return &ConfigJSON{}
	}
	out := &ConfigJSON{}
	for _, p := range c.Providers {
		out.Providers = append(out.Providers, ProviderJSON(p))
	}
	for _, m := range c.Models {
		out.Models = append(out.Models, ModelJSON(m))
	}
	out.Agent = AgentJSON{
		Model:                  c.Agent.Model,
		MaxTurns:               c.Agent.MaxTurns,
		MaxTokensPerTurn:       c.Agent.MaxTokensPerTurn,
		LLMRetryMax:            c.Agent.LLMRetryMax,
		LLMRetryBaseMS:         c.Agent.LLMRetryBaseMS,
		LLMMinIntervalMS:       c.Agent.LLMMinIntervalMS,
		LLMFirstTokenTimeoutMS: c.Agent.LLMFirstTokenTimeoutMS,
		LoopGuard:              c.Agent.LoopGuard,
		LoopToolRepeatLimit:    c.Agent.LoopToolRepeatLimit,
		LoopStreamRepeatCycles: c.Agent.LoopStreamRepeatCycles,
		LoopNudgeMax:           c.Agent.LoopNudgeMax,
	}
	out.Prompts = PromptsJSON{
		Dir: c.Prompts.Dir, AgentPrompt: c.Prompts.AgentPrompt, PlanPrompt: c.Prompts.PlanPrompt,
	}
	if c.Prompts.PerProvider.Enabled != nil {
		out.Prompts.PerProvider = &PerProviderPromptsJSON{Enabled: c.Prompts.PerProvider.Enabled}
	}
	out.Instructions = InstructionsJSON{Files: append([]string(nil), c.Instructions.Files...)}
	out.Skills = SkillsJSON{
		Dirs:          append([]string(nil), c.Skills.Dirs...),
		Sources:       append([]string(nil), c.Skills.Sources...),
		AutoDiscovery: c.Skills.AutoDiscovery,
	}
	for _, s := range c.MCPServers {
		mj := MCPServerJSON{
			Type: s.Type, Name: s.Name, Command: s.Command,
			Args: append([]string(nil), s.Args...), URL: s.URL,
			InsecureSkipVerify: s.InsecureSkipVerify,
			Disabled:           s.Disabled,
			DisabledTools:      append([]string(nil), s.DisabledTools...),
		}
		for _, e := range s.Env {
			mj.Env = append(mj.Env, EnvVarJSON(e))
		}
		for _, h := range s.Headers {
			mj.Headers = append(mj.Headers, HTTPHeaderJSON(h))
		}
		out.MCPServers = append(out.MCPServers, mj)
	}
	out.MCP = MCPJSON{ProjectTrust: c.MCP.ResolvedProjectTrust()}
	out.Tools = ToolsJSON{
		PermissionMode:          c.Tools.ResolvedPermMode(),
		CommandAllowlist:        append([]string(nil), c.Tools.CommandAllowlist...),
		PlanNoSelfRun:           c.Tools.PlanNoSelfRun,
		AskDisableExtendedTools: c.Tools.AskDisableExtendedTools,
		OutputLimits: ToolOutputLimitsJSON{
			Read: c.Tools.OutputLimits.Read, Grep: c.Tools.OutputLimits.Grep,
			Glob: c.Tools.OutputLimits.Glob, PrintTree: c.Tools.OutputLimits.PrintTree,
			RunCommand: c.Tools.OutputLimits.RunCommand, SSHRunCommand: c.Tools.OutputLimits.SSHRunCommand,
			WebFetch: c.Tools.OutputLimits.WebFetch, WebSearch: c.Tools.OutputLimits.WebSearch,
			Default: c.Tools.OutputLimits.Default,
		},
		Background: ToolBackgroundJSON{
			Enabled:               c.Tools.Background.Enabled,
			MaxConcurrent:         c.Tools.Background.MaxConcurrent,
			DefaultTimeoutSeconds: c.Tools.Background.DefaultTimeoutSeconds,
			MaxTimeoutSeconds:     c.Tools.Background.MaxTimeoutSeconds,
			OutputBufferBytes:     c.Tools.Background.OutputBufferBytes,
		},
	}
	out.Logger = LoggerJSON{
		Level: c.Logger.Level, Outputs: append([]string(nil), c.Logger.Outputs...),
		File: c.Logger.File, Format: c.Logger.Format,
		Rotation: LoggerRotationJSON{MaxSizeMB: c.Logger.Rotation.MaxSizeMB, MaxFiles: c.Logger.Rotation.MaxFiles},
	}
	out.Debug = DebugJSON{Enabled: c.Debug.Enabled, CaptureLLM: c.Debug.CaptureLLM}
	out.Sessions = SessionsJSON{Dir: c.Sessions.Dir}
	out.Memory = MemoryJSON{
		Enabled: c.Memory.Enabled, Model: c.Memory.Model, Dir: c.Memory.Dir,
		RecallMaxTurns: c.Memory.RecallMaxTurns, PersistMaxTurns: c.Memory.PersistMaxTurns,
		CopilotMaxTokens: c.Memory.CopilotMaxTokens, MaxSearchHits: c.Memory.MaxSearchHits,
	}
	out.Compaction = CompactionJSON{
		Engine: c.Compaction.Engine, Enabled: c.Compaction.Enabled, Model: c.Compaction.Model,
		ThresholdPercent: c.Compaction.ThresholdPercent, KeepRecentTurns: c.Compaction.KeepRecentTurns,
		MaxTokens: c.Compaction.MaxTokens,
		ResultEviction: ResultEvictionJSON{
			Enabled: c.Compaction.ResultEviction.Enabled, KeepRecent: c.Compaction.ResultEviction.KeepRecent,
			MinResultBytes: c.Compaction.ResultEviction.MinResultBytes,
		},
	}
	out.Title = TitleJSON{
		Enabled: c.Title.Enabled, Model: c.Title.Model, MaxTokens: c.Title.MaxTokens,
	}
	out.Autocomplete = AutocompleteJSON{
		Enabled:        c.Autocomplete.Enabled,
		Model:          c.Autocomplete.Model,
		Mode:           c.Autocomplete.Mode,
		Temperature:    c.Autocomplete.Temperature,
		MaxTokens:      c.Autocomplete.MaxTokens,
		TimeoutMS:      c.Autocomplete.TimeoutMS,
		DebounceMS:     c.Autocomplete.DebounceMS,
		Trigger:        c.Autocomplete.Trigger,
		MultiLine:      c.Autocomplete.MultiLine,
		MaxPrefixBytes: c.Autocomplete.MaxPrefixBytes,
		MaxSuffixBytes: c.Autocomplete.MaxSuffixBytes,
		RelatedFiles:   c.Autocomplete.RelatedFiles,
	}
	out.HTTPServer = HTTPServerJSON{
		Host:              c.HTTPServer.Host,
		Port:              c.HTTPServer.Port,
		PublicDocs:        c.HTTPServer.PublicDocs,
		StreamTicketsOnly: c.HTTPServer.StreamTicketsOnly,
		AllowInsecure:     c.HTTPServer.AllowInsecure,
		// AuthToken is intentionally redacted; report only whether one is configured.
		AuthConfigured: strings.TrimSpace(c.HTTPServer.AuthToken) != "",
		CORS: HTTPCORSJSON{
			Enabled:        c.HTTPServer.CORS.Enabled,
			AllowedOrigins: append([]string(nil), c.HTTPServer.CORS.AllowedOrigins...),
		},
	}
	for _, rm := range c.HTTPServer.Remotes {
		out.HTTPServer.Remotes = append(out.HTTPServer.Remotes, HTTPRemoteJSON(rm))
	}
	out.Scheduler = SchedulerJSON{
		Enabled: c.Scheduler.Enabled, Dir: c.Scheduler.Dir, MaxQueue: c.Scheduler.MaxQueue,
		Timeout: c.Scheduler.Timeout, RetainSessions: c.Scheduler.RetainSessions,
	}
	tg := c.Gateways.Telegram
	tgJSON := TelegramGatewayJSON{
		Enabled: tg.Enabled, Token: tg.Token, Proxy: tg.Proxy, RichMessages: tg.RichMessages,
		Admins:           append([]int64(nil), tg.Admins...),
		DefaultAccess:    string(tg.DefaultAccess),
		DefaultIsolation: string(tg.DefaultIsolation),
	}
	for _, g := range tg.UserGroups {
		tgJSON.UserGroups = append(tgJSON.UserGroups, TelegramUserGroupJSON{
			Name: g.Name, UserIDs: append([]int64(nil), g.UserIDs...),
		})
	}
	for _, ch := range tg.Chats {
		tgJSON.Chats = append(tgJSON.Chats, TelegramChatJSON{
			ChatID: ch.ChatID, Isolation: string(ch.Isolation), Access: string(ch.Access),
		})
	}
	out.Gateways = GatewaysJSON{Telegram: tgJSON}
	out.UI = UIJSON{
		Enabled: c.UI.Enabled, Locale: c.UI.Locale,
		SendMode: c.UI.SendMode, StatusLine: c.UI.StatusLine,
	}
	out.Browser = BrowserJSON{
		Enabled: c.Browser.Enabled, Headless: c.Browser.Headless,
		ExecutablePath: c.Browser.ExecutablePath, TimeoutSeconds: c.Browser.TimeoutSeconds,
	}
	out.VCS = VCSJSON{SVN: SVNJSON{
		Enabled: c.VCS.SVN.Enabled, Binary: c.VCS.SVN.Binary,
		TimeoutSeconds: c.VCS.SVN.TimeoutSeconds, BranchLookup: c.VCS.SVN.BranchLookup,
	}}
	return out
}

// JSONDTOToConfig maps JSON DTO into a new Config (Paths must be set by caller before validate).
func JSONDTOToConfig(j *ConfigJSON, paths Paths) *Config {
	cfg := &Config{Paths: paths}
	if j == nil {
		return cfg
	}
	for _, p := range j.Providers {
		cfg.Providers = append(cfg.Providers, ProviderConfig(p))
	}
	for _, m := range j.Models {
		cfg.Models = append(cfg.Models, ModelEntry(m))
	}
	cfg.Agent = Agent{
		Model:                  j.Agent.Model,
		MaxTurns:               j.Agent.MaxTurns,
		MaxTokensPerTurn:       j.Agent.MaxTokensPerTurn,
		LLMRetryMax:            j.Agent.LLMRetryMax,
		LLMRetryBaseMS:         j.Agent.LLMRetryBaseMS,
		LLMMinIntervalMS:       j.Agent.LLMMinIntervalMS,
		LLMFirstTokenTimeoutMS: j.Agent.LLMFirstTokenTimeoutMS,
		LoopGuard:              j.Agent.LoopGuard,
		LoopToolRepeatLimit:    j.Agent.LoopToolRepeatLimit,
		LoopStreamRepeatCycles: j.Agent.LoopStreamRepeatCycles,
		LoopNudgeMax:           j.Agent.LoopNudgeMax,
	}
	cfg.Prompts = Prompts{
		Dir: j.Prompts.Dir, AgentPrompt: j.Prompts.AgentPrompt, PlanPrompt: j.Prompts.PlanPrompt,
	}
	if j.Prompts.PerProvider != nil {
		cfg.Prompts.PerProvider = PerProviderPrompts{Enabled: j.Prompts.PerProvider.Enabled}
	}
	cfg.Instructions = Instructions{Files: append([]string(nil), j.Instructions.Files...)}
	cfg.Skills = Skills{
		Dirs:          append([]string(nil), j.Skills.Dirs...),
		Sources:       append([]string(nil), j.Skills.Sources...),
		AutoDiscovery: j.Skills.AutoDiscovery,
	}
	for _, s := range j.MCPServers {
		mc := MCPServerConfig{
			Type: s.Type, Name: s.Name, Command: s.Command,
			Args: append([]string(nil), s.Args...), URL: s.URL,
			InsecureSkipVerify: s.InsecureSkipVerify,
			Disabled:           s.Disabled,
			DisabledTools:      append([]string(nil), s.DisabledTools...),
		}
		for _, e := range s.Env {
			mc.Env = append(mc.Env, EnvVarConfig(e))
		}
		for _, h := range s.Headers {
			mc.Headers = append(mc.Headers, HTTPHeaderConfig(h))
		}
		cfg.MCPServers = append(cfg.MCPServers, mc)
	}
	cfg.MCP = MCP{ProjectTrust: j.MCP.ProjectTrust}
	cfg.Tools = Tools{
		PermissionMode:          j.Tools.PermissionMode,
		CommandAllowlist:        append([]string(nil), j.Tools.CommandAllowlist...),
		PlanNoSelfRun:           j.Tools.PlanNoSelfRun,
		AskDisableExtendedTools: j.Tools.AskDisableExtendedTools,
		OutputLimits: ToolOutputLimits{
			Read: j.Tools.OutputLimits.Read, Grep: j.Tools.OutputLimits.Grep,
			Glob: j.Tools.OutputLimits.Glob, PrintTree: j.Tools.OutputLimits.PrintTree,
			RunCommand: j.Tools.OutputLimits.RunCommand, SSHRunCommand: j.Tools.OutputLimits.SSHRunCommand,
			WebFetch: j.Tools.OutputLimits.WebFetch, WebSearch: j.Tools.OutputLimits.WebSearch,
			Default: j.Tools.OutputLimits.Default,
		},
		Background: ToolBackground{
			Enabled:               j.Tools.Background.Enabled,
			MaxConcurrent:         j.Tools.Background.MaxConcurrent,
			DefaultTimeoutSeconds: j.Tools.Background.DefaultTimeoutSeconds,
			MaxTimeoutSeconds:     j.Tools.Background.MaxTimeoutSeconds,
			OutputBufferBytes:     j.Tools.Background.OutputBufferBytes,
		},
	}
	cfg.Logger = Logger{
		Level: j.Logger.Level, Outputs: append([]string(nil), j.Logger.Outputs...),
		File: j.Logger.File, Format: j.Logger.Format,
		Rotation: LoggerRotation{
			MaxSizeMB: j.Logger.Rotation.MaxSizeMB, MaxFiles: j.Logger.Rotation.MaxFiles,
		},
	}
	cfg.Debug = Debug{Enabled: j.Debug.Enabled, CaptureLLM: j.Debug.CaptureLLM}
	cfg.Sessions = Sessions{Dir: j.Sessions.Dir}
	cfg.Memory = MemoryConfig{
		Enabled: j.Memory.Enabled, Model: j.Memory.Model, Dir: j.Memory.Dir,
		RecallMaxTurns: j.Memory.RecallMaxTurns, PersistMaxTurns: j.Memory.PersistMaxTurns,
		CopilotMaxTokens: j.Memory.CopilotMaxTokens, MaxSearchHits: j.Memory.MaxSearchHits,
	}
	cfg.Compaction = CompactionConfig{
		Engine: j.Compaction.Engine, Enabled: j.Compaction.Enabled, Model: j.Compaction.Model,
		ThresholdPercent: j.Compaction.ThresholdPercent, KeepRecentTurns: j.Compaction.KeepRecentTurns,
		MaxTokens: j.Compaction.MaxTokens,
		ResultEviction: ResultEviction{
			Enabled: j.Compaction.ResultEviction.Enabled, KeepRecent: j.Compaction.ResultEviction.KeepRecent,
			MinResultBytes: j.Compaction.ResultEviction.MinResultBytes,
		},
	}
	cfg.Title = TitleConfig{
		Enabled: j.Title.Enabled, Model: j.Title.Model, MaxTokens: j.Title.MaxTokens,
	}
	cfg.Autocomplete = AutocompleteConfig{
		Enabled:        j.Autocomplete.Enabled,
		Model:          j.Autocomplete.Model,
		Mode:           j.Autocomplete.Mode,
		Temperature:    j.Autocomplete.Temperature,
		MaxTokens:      j.Autocomplete.MaxTokens,
		TimeoutMS:      j.Autocomplete.TimeoutMS,
		DebounceMS:     j.Autocomplete.DebounceMS,
		Trigger:        j.Autocomplete.Trigger,
		MultiLine:      j.Autocomplete.MultiLine,
		MaxPrefixBytes: j.Autocomplete.MaxPrefixBytes,
		MaxSuffixBytes: j.Autocomplete.MaxSuffixBytes,
		RelatedFiles:   j.Autocomplete.RelatedFiles,
	}
	cfg.HTTPServer = HTTPServerConfig{
		Host:              j.HTTPServer.Host,
		Port:              j.HTTPServer.Port,
		AuthToken:         j.HTTPServer.AuthToken,
		PublicDocs:        j.HTTPServer.PublicDocs,
		StreamTicketsOnly: j.HTTPServer.StreamTicketsOnly,
		AllowInsecure:     j.HTTPServer.AllowInsecure,
		CORS: HTTPCORSConfig{
			Enabled:        j.HTTPServer.CORS.Enabled,
			AllowedOrigins: append([]string(nil), j.HTTPServer.CORS.AllowedOrigins...),
		},
	}
	for _, rm := range j.HTTPServer.Remotes {
		cfg.HTTPServer.Remotes = append(cfg.HTTPServer.Remotes, HTTPRemote(rm))
	}
	cfg.Scheduler = SchedulerConfig{
		Enabled: j.Scheduler.Enabled, Dir: j.Scheduler.Dir, MaxQueue: j.Scheduler.MaxQueue,
		Timeout: j.Scheduler.Timeout, RetainSessions: j.Scheduler.RetainSessions,
	}
	jt := j.Gateways.Telegram
	tg := TelegramGatewayConfig{
		Enabled: jt.Enabled, Token: jt.Token, Proxy: jt.Proxy, RichMessages: jt.RichMessages,
		Admins:           append([]int64(nil), jt.Admins...),
		DefaultAccess:    AccessLevel(jt.DefaultAccess),
		DefaultIsolation: IsolationMode(jt.DefaultIsolation),
	}
	for _, g := range jt.UserGroups {
		tg.UserGroups = append(tg.UserGroups, TelegramUserGroup{
			Name: g.Name, UserIDs: append([]int64(nil), g.UserIDs...),
		})
	}
	for _, ch := range jt.Chats {
		tg.Chats = append(tg.Chats, TelegramChatConfig{
			ChatID: ch.ChatID, Isolation: IsolationMode(ch.Isolation), Access: AccessLevel(ch.Access),
		})
	}
	cfg.Gateways = GatewayConfig{Telegram: tg}
	cfg.UI = UIConfig{
		Enabled: j.UI.Enabled, Locale: j.UI.Locale,
		SendMode: j.UI.SendMode, StatusLine: j.UI.StatusLine,
	}
	cfg.Browser = BrowserConfig{
		Enabled: j.Browser.Enabled, Headless: j.Browser.Headless,
		ExecutablePath: j.Browser.ExecutablePath, TimeoutSeconds: j.Browser.TimeoutSeconds,
	}
	cfg.VCS = VCSConfig{SVN: SVNConfig{
		Enabled: j.VCS.SVN.Enabled, Binary: j.VCS.SVN.Binary,
		TimeoutSeconds: j.VCS.SVN.TimeoutSeconds, BranchLookup: j.VCS.SVN.BranchLookup,
	}}
	return cfg
}

// ParseAndValidateConfigJSON unmarshals JSON into ConfigJSON, maps to Config, applies defaults and validates.
func ParseAndValidateConfigJSON(data []byte, paths Paths) (*Config, error) {
	return ParseConfigJSONPreservingSecrets(data, paths, nil)
}

// ParseConfigJSONPreservingSecrets is like ParseAndValidateConfigJSON but, when current is
// non-nil, carries write-only secrets that GET /foxxycode/config redacts (currently the
// httpserver auth token) from current into the incoming config when the payload omitted them.
// This lets the UI save an edited, redacted config without wiping tokens it never received.
func ParseConfigJSONPreservingSecrets(data []byte, paths Paths, current *Config) (*Config, error) {
	var j ConfigJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return nil, fmt.Errorf("json: %w", err)
	}
	cfg := JSONDTOToConfig(&j, paths)
	preserveRedactedSecrets(cfg, current)
	applyDefaults(cfg)
	if err := validateSubconfigs(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// preserveRedactedSecrets copies redacted, write-only secrets from current into next when next
// left them empty. GET /foxxycode/config never returns these, so a plain round-trip would drop them.
func preserveRedactedSecrets(next, current *Config) {
	if next == nil || current == nil {
		return
	}
	if strings.TrimSpace(next.HTTPServer.AuthToken) == "" && strings.TrimSpace(current.HTTPServer.AuthToken) != "" {
		next.HTTPServer.AuthToken = current.HTTPServer.AuthToken
	}
}

// MarshalConfigYAML serializes cfg to YAML bytes for disk (Paths is omitted via yaml:"-" on field).
// Always-literal secret fields (proxy URLs) are "$"-escaped so the load-time expansion pass restores
// them verbatim instead of resolving "$WORD"/"$N" fragments to empty environment variables.
func MarshalConfigYAML(cfg *Config) ([]byte, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	return yaml.Marshal(escapeYAMLSecrets(cfg))
}

// escapeYAMLSecrets returns a copy of cfg with always-literal proxy URLs "$"-escaped for disk.
// It copies only what it mutates (the Providers slice and the gateway proxy string), leaving the
// caller's in-memory *Config untouched — the live config keeps the real, unescaped values.
func escapeYAMLSecrets(cfg *Config) *Config {
	out := *cfg
	if len(cfg.Providers) > 0 {
		out.Providers = make([]ProviderConfig, len(cfg.Providers))
		copy(out.Providers, cfg.Providers)
		for i := range out.Providers {
			out.Providers[i].Proxy = escapeYAMLDollar(out.Providers[i].Proxy)
		}
	}
	out.Gateways.Telegram.Proxy = escapeYAMLDollar(cfg.Gateways.Telegram.Proxy)
	return &out
}
