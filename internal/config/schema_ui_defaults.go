package config

func boolPtr(v bool) *bool { return &v }

// SchemaExampleConfigJSON returns representative defaults for JSON Schema "default"
// and UI placeholders. It is not loaded as a real config; values mirror applyDefaults
// and field semantics where possible.
//
// Each value is the placeholder of its own field, not part of a coherent row:
// attachNodeDefaults walks the tree and attaches a "default" per property. So
// the temperature here is what a new models[] row offers for temperature
// whatever model it names - including a reasoning id such as the one below,
// which would not send it (see the reasoning branch in internal/llm/openai.go).
func SchemaExampleConfigJSON() *ConfigJSON {
	perProviderEnabled := true
	compactionEnabled := true
	compactionKeepRecent := CompactionDefaultKeepRecentTurns
	skillsAutoDiscovery := true
	planNoSelfRun := false
	subagentsEnabled := true
	subagentsMaxDepth := SubagentsDefaultMaxDepth
	titleEnabled := true
	autocompleteEnabled := false
	autocompleteMultiLine := true
	autocompleteRelatedFiles := AutocompleteDefaultRelatedFiles
	browserHeadless := true
	svnEnabled := true
	svnBranchLookup := true
	loopGuard := true
	loopToolRepeatLimit := AgentDefaultLoopToolRepeatLimit
	loopStreamRepeatCycles := AgentDefaultLoopStreamRepeatCycles
	loopToolCycleRepeats := AgentDefaultLoopToolCycleRepeats
	loopNudgeMax := AgentDefaultLoopNudgeMax
	waitForLimitResetMaxMS := AgentDefaultWaitForLimitResetMaxMS
	llmRetryMax := AgentDefaultLLMRetryMax
	llmFirstTokenTimeoutMS := AgentDefaultLLMFirstTokenTimeoutMS
	llmStallTimeoutMS := AgentDefaultLLMStallTimeoutMS
	llmStallRetry := true
	llmStallRetryMaxWaitMS := AgentDefaultLLMStallRetryMaxWaitMS
	return &ConfigJSON{
		Providers: []ProviderJSON{
			{Name: "openai", Type: "openai", APIBase: "", APIKey: ""},
		},
		Models: []ModelJSON{
			{
				Model:            "openai/gpt-5.6-terra",
				MaxTokens:        4096,
				Temperature:      0.2,
				MaxContextTokens: 0,
			},
		},
		Agent: AgentJSON{
			Model:                  "openai/gpt-5.6-terra",
			MaxTurns:               AgentDefaultMaxTurns,
			MaxTokensPerTurn:       AgentDefaultMaxTokensPerTurn,
			LLMRetryMax:            &llmRetryMax,
			LLMRetryBaseMS:         AgentDefaultLLMRetryBaseMS,
			LLMFirstTokenTimeoutMS: &llmFirstTokenTimeoutMS,
			LLMStallTimeoutMS:      &llmStallTimeoutMS,
			LLMStallRetry:          &llmStallRetry,
			LLMStallRetryDelaysMS:  append([]int(nil), agentDefaultLLMStallRetryDelaysMS...),
			LLMStallRetryMaxWaitMS: &llmStallRetryMaxWaitMS,
			LoopGuard:              &loopGuard,
			LoopToolRepeatLimit:    &loopToolRepeatLimit,
			LoopStreamRepeatCycles: &loopStreamRepeatCycles,
			LoopToolCycleRepeats:   &loopToolCycleRepeats,
			LoopStuckAction:        AgentDefaultLoopStuckAction,
			LoopNudgeMax:           &loopNudgeMax,
			WaitForLimitResetMaxMS: &waitForLimitResetMaxMS,
		},
		Autocomplete: AutocompleteJSON{
			Enabled:        &autocompleteEnabled,
			Model:          "",
			Mode:           AutocompleteModeAuto,
			Temperature:    0,
			MaxTokens:      AutocompleteDefaultMaxTokens,
			TimeoutMS:      AutocompleteDefaultTimeoutMS,
			DebounceMS:     AutocompleteDefaultDebounceMS,
			Trigger:        AutocompleteTriggerAuto,
			MultiLine:      &autocompleteMultiLine,
			MaxPrefixBytes: AutocompleteDefaultMaxPrefixBytes,
			MaxSuffixBytes: AutocompleteDefaultMaxSuffixBytes,
			RelatedFiles:   &autocompleteRelatedFiles,
		},
		Prompts: PromptsJSON{
			Dir:         "",
			AgentPrompt: "agent.md",
			PlanPrompt:  "plan.md",
			AskPrompt:   "ask.md",
			PerProvider: &PerProviderPromptsJSON{Enabled: &perProviderEnabled},
		},
		Instructions: InstructionsJSON{
			Files: []string{"AGENTS.md"},
		},
		Skills: SkillsJSON{
			Dirs: []string{
				"~/.agents/skills",
				"${FOXXYCODE_HOME}/skills",
				"${CWD}/.foxxycode/skills",
			},
			Sources:       []string{},
			AutoDiscovery: &skillsAutoDiscovery,
		},
		MCPServers: []MCPServerJSON{},
		MCP:        MCPJSON{ProjectTrust: ProjectTrustAsk},
		Tools: ToolsJSON{
			PermissionMode:   PermModeAsk,
			CommandAllowlist: nil,
			PlanNoSelfRun:    &planNoSelfRun,
		},
		Subagents: SubagentsJSON{
			Enabled:               &subagentsEnabled,
			Dirs:                  DefaultSubagentDirs(),
			ProjectTrust:          SubagentsProjectTrustAsk,
			MaxConcurrent:         SubagentsDefaultMaxConcurrent,
			MaxDepth:              &subagentsMaxDepth,
			DefaultTimeoutSeconds: SubagentsDefaultTimeoutSeconds,
			MaxTurns:              0,
		},
		Logger: LoggerJSON{
			Level:    LogLevelInfo,
			Outputs:  []string{LogOutputStderr},
			File:     "",
			Format:   "text",
			Rotation: LoggerRotationJSON{MaxSizeMB: 0, MaxFiles: 0},
		},
		Sessions: SessionsJSON{Dir: ""},
		Memory: MemoryJSON{
			Enabled:          false,
			Model:            "",
			Dir:              "",
			RecallMaxTurns:   6,
			PersistMaxTurns:  12,
			CopilotMaxTokens: 4096,
			MaxSearchHits:    8,
		},
		Compaction: CompactionJSON{
			Engine:           CompactionEngineCoddy,
			Enabled:          &compactionEnabled,
			Model:            "",
			ThresholdPercent: CompactionDefaultThresholdCoddy,
			KeepRecentTurns:  &compactionKeepRecent,
			MaxTokens:        CompactionDefaultMaxTokens,
		},
		Title: TitleJSON{
			Enabled:   &titleEnabled,
			Model:     "",
			MaxTokens: TitleDefaultMaxTokens,
		},
		Hooks: HooksJSON{
			Enabled:               boolPtr(true),
			Files:                 DefaultHookFiles(),
			ProjectTrust:          ProjectTrustAsk,
			DefaultTimeoutSeconds: HooksDefaultTimeoutSeconds,
			StopLoopLimit:         HooksDefaultStopLoopLimit,
			MaxOutputChars:        HooksDefaultMaxOutputChars,
		},
		Scheduler: SchedulerJSON{
			Enabled:        false,
			Dir:            "${FOXXYCODE_HOME}/scheduler",
			MaxQueue:       10,
			Timeout:        "30m",
			RetainSessions: 5,
		},
		Gateways: GatewaysJSON{
			Telegram: TelegramGatewayJSON{
				Enabled:          false,
				Token:            "${TELEGRAM_BOT_TOKEN}",
				RichMessages:     true,
				DefaultAccess:    string(AccessAll),
				DefaultIsolation: string(IsolationIndividual),
			},
		},
		UI: UIJSON{SendMode: UISendModeEnter},
		Browser: BrowserJSON{
			Enabled:        false,
			Headless:       &browserHeadless,
			ExecutablePath: "",
			TimeoutSeconds: BrowserDefaultTimeoutSeconds,
		},
		VCS: VCSJSON{
			SVN: SVNJSON{
				Enabled:        &svnEnabled,
				Binary:         "",
				TimeoutSeconds: SVNDefaultTimeoutSeconds,
				BranchLookup:   &svnBranchLookup,
			},
		},
	}
}
