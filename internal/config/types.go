// Package config handles loading and validating agent configuration.
package config

import "github.com/hijera/foxxycode-agent/internal/cmdprofile"

// Config is the root configuration struct.
type Config struct {
	Paths Paths `yaml:"-"`

	Providers    []ProviderConfig  `yaml:"providers"`
	Models       []ModelEntry      `yaml:"models"`
	Agent        Agent             `yaml:"agent"`
	Prompts      Prompts           `yaml:"prompts"`
	Instructions Instructions      `yaml:"instructions"`
	Skills       Skills            `yaml:"skills"`
	Rules        Rules             `yaml:"rules"`
	MCPServers   []MCPServerConfig `yaml:"mcp_servers"`
	MCP          MCP               `yaml:"mcp"`
	Tools        Tools             `yaml:"tools"`
	Logger       Logger            `yaml:"logger"`
	Sessions     Sessions          `yaml:"sessions"`
	Memory       MemoryConfig      `yaml:"memory"`
	Compaction   CompactionConfig  `yaml:"compaction"`
	Title        TitleConfig       `yaml:"title"`
	HTTPServer   HTTPServerConfig  `yaml:"httpserver"`
	Scheduler    SchedulerConfig   `yaml:"scheduler"`
	Gateways     GatewayConfig     `yaml:"gateways"`
	UI           UIConfig          `yaml:"ui"`
	Browser      BrowserConfig     `yaml:"browser"`
	VCS          VCSConfig         `yaml:"vcs"`
	Debug        Debug             `yaml:"debug"`
	// Commands declares operator-approved command profiles: narrow argv-exec
	// tools over one fixed binary each. See internal/cmdprofile.
	Commands []cmdprofile.ProfileSpec `yaml:"commands"`
}
