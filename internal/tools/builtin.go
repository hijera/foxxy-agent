package tools

import "github.com/hijera/foxxy-agent/internal/tooling"

// BuiltinTool describes a built-in capability exposed as exactly one [*Tool].
// Packages under internal/tools (fs, todo, shell, and similar) expose constructors that return
// *tooling.Tool. Implement [*tooling.Builtin] optionally for uniform adapters.
type BuiltinTool = tooling.Builtin
