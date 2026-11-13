package session

import "github.com/hijera/foxxycode-agent/internal/idehost"

// LoadIntelliJProjectContext retains the session API; host metadata belongs to idehost.
func LoadIntelliJProjectContext(cwd string) string { return idehost.LoadIntelliJProjectContext(cwd) }

// LoadVSCodeProjectContext retains the session API; host metadata belongs to idehost.
func LoadVSCodeProjectContext(cwd string) string { return idehost.LoadVSCodeProjectContext(cwd) }
