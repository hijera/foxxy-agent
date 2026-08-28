// Package cmdprofile implements command profiles: narrow, operator-reviewed
// wrappers that execute one fixed binary with an argv template and typed
// parameters. Execution is argv-style (exec.CommandContext) and never touches
// a shell, so shell metacharacters in parameter values are plain characters.
//
// The package is the single source of truth for the profile shape and its
// validation. It deliberately imports nothing above internal/platform, the
// same layering as internal/svnws, so both internal/config and
// external/miniapps can depend on it.
package cmdprofile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// ParamType enumerates the parameter kinds a template slot can carry. There is
// deliberately no free-form string type without a pattern: every value that
// reaches the argv is constrained by construction.
type ParamType string

const (
	// ParamFile is a filesystem path. The parameter name must end in _path,
	// _file, or _dir so both the Mini Apps containment walk and the trace
	// input classifier recognize it.
	ParamFile ParamType = "file"
	// ParamEnum restricts the value to a fixed list.
	ParamEnum ParamType = "enum"
	// ParamInt is an integer with optional bounds.
	ParamInt ParamType = "int"
	// ParamFlag toggles a fixed literal token on or off.
	ParamFlag ParamType = "flag"
	// ParamString requires an explicit pattern; it is compiled anchored, so
	// the pattern describes the whole value.
	ParamString ParamType = "string"
)

// Profile permission levels (вариант Б): ask prompts in chat like any
// permission-bearing tool; allow runs without a prompt and is the only level
// Mini Apps accept for unattended runs.
const (
	PermissionAsk   = "ask"
	PermissionAllow = "allow"
)

// DefaultTimeoutSeconds bounds a profile run when neither the profile nor the
// caller sets a timeout.
const DefaultTimeoutSeconds = 120

// maxTimeoutSeconds keeps a profile from disabling the watchdog entirely.
const maxTimeoutSeconds = 3600

// ParamSpec declares one typed parameter referenced from the template as
// {name}. JSON tags define the portable document shape.
type ParamSpec struct {
	Name string    `json:"name" yaml:"name"`
	Type ParamType `json:"type" yaml:"type"`
	// Enum lists the allowed values (enum type only).
	Enum []string `json:"enum,omitempty" yaml:"enum,omitempty"`
	// Min and Max bound int parameters when set.
	Min *int `json:"min,omitempty" yaml:"min,omitempty"`
	Max *int `json:"max,omitempty" yaml:"max,omitempty"`
	// Pattern constrains string parameters. It is compiled as ^(?:pattern)$.
	Pattern string `json:"pattern,omitempty" yaml:"pattern,omitempty"`
	// Literal is the token a true flag emits (flag type only). It is the one
	// place a leading dash is legitimate, because the author wrote it.
	Literal string `json:"literal,omitempty" yaml:"literal,omitempty"`
	// Required is accepted for schema symmetry. Non-flag parameters are always
	// required in v1 (an optional value inside a template token would make
	// matching ambiguous); flags are inherently optional.
	Required    *bool  `json:"required,omitempty" yaml:"required,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

// InstallSpec names the profile's package in known package managers. Fixed
// fields rather than a map: the config path walker cannot address map keys.
type InstallSpec struct {
	Winget string `json:"winget,omitempty" yaml:"winget,omitempty"`
	Scoop  string `json:"scoop,omitempty" yaml:"scoop,omitempty"`
	Brew   string `json:"brew,omitempty" yaml:"brew,omitempty"`
	Apt    string `json:"apt,omitempty" yaml:"apt,omitempty"`
	Dnf    string `json:"dnf,omitempty" yaml:"dnf,omitempty"`
}

// ProfileSpec is one command profile. It is both the config shape (mirrored in
// internal/config) and the portable Mini App document shape
// (requirements.commands), which is why Binary is a bare name by convention:
// absolute paths are machine-specific and the release sanitizer rejects them
// in portable documents.
type ProfileSpec struct {
	Name        string `json:"name" yaml:"name"`
	Binary      string `json:"binary" yaml:"binary"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	// Permission is ask or allow; empty resolves to ask (the safe default).
	Permission     string      `json:"permission,omitempty" yaml:"permission,omitempty"`
	TimeoutSeconds int         `json:"timeout_seconds,omitempty" yaml:"timeout_seconds,omitempty"`
	Template       []string    `json:"template" yaml:"template"`
	Params         []ParamSpec `json:"params,omitempty" yaml:"params,omitempty"`
	Install        InstallSpec `json:"install,omitempty" yaml:"install,omitempty"`
}

// Clone returns a deep copy, so config DTO round-trips and document embedding
// never share slice backing arrays with the original.
func (s ProfileSpec) Clone() ProfileSpec {
	out := s
	out.Template = append([]string(nil), s.Template...)
	out.Params = append([]ParamSpec(nil), s.Params...)
	for index := range out.Params {
		out.Params[index].Enum = append([]string(nil), s.Params[index].Enum...)
		if s.Params[index].Min != nil {
			value := *s.Params[index].Min
			out.Params[index].Min = &value
		}
		if s.Params[index].Max != nil {
			value := *s.Params[index].Max
			out.Params[index].Max = &value
		}
		if s.Params[index].Required != nil {
			value := *s.Params[index].Required
			out.Params[index].Required = &value
		}
	}
	return out
}

// CloneSpecs deep-copies a profile list.
func CloneSpecs(specs []ProfileSpec) []ProfileSpec {
	if specs == nil {
		return nil
	}
	out := make([]ProfileSpec, len(specs))
	for index := range specs {
		out[index] = specs[index].Clone()
	}
	return out
}

// ToolName is the registry name for the profile's tool. The cmd_ prefix keeps
// profile tools out of every builtin namespace; profile-name validation keeps
// the result clear of the MCP "__" separator and the Mini Apps
// _create/_update/_delete denylist.
func (s ProfileSpec) ToolName() string { return "cmd_" + s.Name }

// ResolvedPermission returns the effective permission level, defaulting to ask.
func (s ProfileSpec) ResolvedPermission() string {
	if strings.EqualFold(strings.TrimSpace(s.Permission), PermissionAllow) {
		return PermissionAllow
	}
	return PermissionAsk
}

// ResolvedTimeout returns the run timeout in seconds.
func (s ProfileSpec) ResolvedTimeout() int {
	if s.TimeoutSeconds > 0 {
		return s.TimeoutSeconds
	}
	return DefaultTimeoutSeconds
}

var (
	profileNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)
	paramNamePattern   = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
	installPattern     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)
)

// forbiddenNameSuffixes would collide with the Mini Apps interactive-tool
// denylist, which blocks any tool name ending in these.
var forbiddenNameSuffixes = []string{"_create", "_update", "_delete"}

// forbiddenParamSubstrings are markers the Mini Apps trace classifier treats
// as source-specific, secret, or environmental. A parameter carrying one would
// silently lose its operator-input status during distillation, or invite
// secret material into an argv.
var forbiddenParamSubstrings = []string{
	"source", "fixture", "session",
	"token", "secret", "password", "passwd", "credential",
	"api_key", "apikey", "private_key", "auth",
	"cwd", "env", "workspace", "working_dir", "home_dir",
}

// Validate checks the whole profile. It returns the first problem found; the
// config loader surfaces it with the profile name attached.
func (s *ProfileSpec) Validate() error {
	name := strings.TrimSpace(s.Name)
	if !profileNamePattern.MatchString(name) || strings.Contains(name, "__") {
		return fmt.Errorf("profile name %q must match %s and must not contain %q", s.Name, profileNamePattern, "__")
	}
	for _, suffix := range forbiddenNameSuffixes {
		if strings.HasSuffix(name, suffix) {
			return fmt.Errorf("profile name %q must not end with the reserved suffix %q", name, suffix)
		}
	}
	if err := validateBinaryName(s.Binary); err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(s.Permission)) {
	case "", PermissionAsk, PermissionAllow:
	default:
		return fmt.Errorf("profile %s: permission must be %q or %q", name, PermissionAsk, PermissionAllow)
	}
	if s.TimeoutSeconds < 0 || s.TimeoutSeconds > maxTimeoutSeconds {
		return fmt.Errorf("profile %s: timeout_seconds must be between 0 and %d", name, maxTimeoutSeconds)
	}
	if len(s.Template) == 0 {
		return fmt.Errorf("profile %s: template must not be empty", name)
	}
	params := make(map[string]ParamSpec, len(s.Params))
	for index := range s.Params {
		param := s.Params[index]
		if err := validateParamSpec(param); err != nil {
			return fmt.Errorf("profile %s: %w", name, err)
		}
		if _, exists := params[param.Name]; exists {
			return fmt.Errorf("profile %s: duplicate param name %q", name, param.Name)
		}
		params[param.Name] = param
	}
	if err := validateTemplate(s.Template, params); err != nil {
		return fmt.Errorf("profile %s: %w", name, err)
	}
	if err := validateInstall(s.Install); err != nil {
		return fmt.Errorf("profile %s: %w", name, err)
	}
	return nil
}

func validateBinaryName(binary string) error {
	binary = strings.TrimSpace(binary)
	if binary == "" {
		return fmt.Errorf("binary is required")
	}
	if strings.ContainsAny(binary, " \t\r\n\x00") {
		return fmt.Errorf("binary %q must not contain whitespace", binary)
	}
	lower := strings.ToLower(binary)
	// CreateProcess launches batch files through cmd.exe, whose argument
	// re-parsing is a known injection vector, so they are banned outright.
	if strings.HasSuffix(lower, ".bat") || strings.HasSuffix(lower, ".cmd") {
		return fmt.Errorf("binary %q: batch files are not allowed", binary)
	}
	if strings.ContainsAny(binary, `/\`) && !isAbsolutePath(binary) {
		return fmt.Errorf("binary %q must be a bare name resolved on PATH or an absolute path", binary)
	}
	return nil
}

// isAbsolutePath accepts both platform conventions regardless of the host, so
// a profile authored on Windows validates on Linux and vice versa.
func isAbsolutePath(path string) bool {
	if filepath.IsAbs(path) {
		return true
	}
	if strings.HasPrefix(path, "/") {
		return true
	}
	// Windows drive path checked textually for cross-platform validation.
	if len(path) >= 3 && path[1] == ':' && (path[2] == '\\' || path[2] == '/') {
		return true
	}
	return false
}

func validateParamSpec(param ParamSpec) error {
	if !paramNamePattern.MatchString(param.Name) || strings.Contains(param.Name, "__") {
		return fmt.Errorf("param name %q must match %s", param.Name, paramNamePattern)
	}
	for _, marker := range forbiddenParamSubstrings {
		if strings.Contains(param.Name, marker) {
			return fmt.Errorf("param name %q must not contain %q (reserved trace-classification marker)", param.Name, marker)
		}
	}
	if param.Type != ParamFlag && param.Literal != "" {
		return fmt.Errorf("param %q: literal is only valid on flag params", param.Name)
	}
	if param.Type != ParamFlag && param.Required != nil && !*param.Required {
		return fmt.Errorf("param %q: only flag params may be non-required", param.Name)
	}
	switch param.Type {
	case ParamFile:
		if !strings.HasSuffix(param.Name, "_path") && !strings.HasSuffix(param.Name, "_file") && !strings.HasSuffix(param.Name, "_dir") {
			return fmt.Errorf("param %q: file params must be named *_path, *_file, or *_dir", param.Name)
		}
	case ParamEnum:
		if len(param.Enum) == 0 {
			return fmt.Errorf("param %q: enum params need at least one value", param.Name)
		}
		for _, value := range param.Enum {
			if value == "" || strings.HasPrefix(value, "-") || strings.ContainsAny(value, " \t\r\n\x00") {
				return fmt.Errorf("param %q: enum value %q must be non-empty, without whitespace or a leading dash", param.Name, value)
			}
		}
	case ParamInt:
		if param.Min != nil && param.Max != nil && *param.Min > *param.Max {
			return fmt.Errorf("param %q: min must not exceed max", param.Name)
		}
	case ParamFlag:
		if strings.TrimSpace(param.Literal) == "" {
			return fmt.Errorf("param %q: flag params require a literal token", param.Name)
		}
		if strings.ContainsAny(param.Literal, " \t\r\n\x00") {
			return fmt.Errorf("param %q: flag literal must not contain whitespace", param.Name)
		}
	case ParamString:
		if strings.TrimSpace(param.Pattern) == "" {
			return fmt.Errorf("param %q: string params require a pattern", param.Name)
		}
		if _, err := compileParamPattern(param.Pattern); err != nil {
			return fmt.Errorf("param %q: pattern does not compile: %w", param.Name, err)
		}
	default:
		return fmt.Errorf("param %q: unknown type %q", param.Name, param.Type)
	}
	return nil
}

// compileParamPattern anchors the author's pattern so it always describes the
// whole value; forgetting ^...$ cannot weaken a constraint.
func compileParamPattern(pattern string) (*regexp.Regexp, error) {
	return regexp.Compile("^(?:" + pattern + ")$")
}

func validateInstall(install InstallSpec) error {
	for manager, coordinate := range map[string]string{
		"winget": install.Winget, "scoop": install.Scoop,
		"brew": install.Brew, "apt": install.Apt, "dnf": install.Dnf,
	} {
		if coordinate == "" {
			continue
		}
		if !installPattern.MatchString(coordinate) {
			return fmt.Errorf("install.%s coordinate %q must match %s", manager, coordinate, installPattern)
		}
	}
	return nil
}

func validateTemplate(template []string, params map[string]ParamSpec) error {
	used := make(map[string]bool, len(params))
	for _, token := range template {
		if strings.ContainsAny(token, "\x00\n\r") {
			return fmt.Errorf("template token %q must not contain control characters", token)
		}
		segments, err := parseTemplateToken(token)
		if err != nil {
			return err
		}
		placeholders := 0
		for _, segment := range segments {
			if !segment.placeholder {
				continue
			}
			placeholders++
			param, declared := params[segment.text]
			if !declared {
				return fmt.Errorf("template references undeclared param %q", segment.text)
			}
			if used[segment.text] {
				return fmt.Errorf("param %q appears in the template more than once", segment.text)
			}
			used[segment.text] = true
			if param.Type == ParamFlag && len(segments) != 1 {
				return fmt.Errorf("flag param %q must be a whole template token", segment.text)
			}
		}
		if placeholders > 2 {
			return fmt.Errorf("template token %q carries more than two placeholders", token)
		}
	}
	for name := range params {
		if !used[name] {
			return fmt.Errorf("param %q is declared but never used in the template", name)
		}
	}
	return nil
}

// tokenSegment is one literal or placeholder piece of a template token.
type tokenSegment struct {
	text        string
	placeholder bool
}

// parseTemplateToken splits a template token into literal and {placeholder}
// segments. Adjacent placeholders without a separating literal are rejected:
// there would be no anchor to split a captured value on.
func parseTemplateToken(token string) ([]tokenSegment, error) {
	var segments []tokenSegment
	rest := token
	for {
		open := strings.IndexByte(rest, '{')
		if open < 0 {
			if rest != "" {
				segments = append(segments, tokenSegment{text: rest})
			}
			break
		}
		closing := strings.IndexByte(rest[open:], '}')
		if closing < 0 {
			return nil, fmt.Errorf("template token %q has an unclosed placeholder", token)
		}
		if open > 0 {
			segments = append(segments, tokenSegment{text: rest[:open]})
		}
		name := rest[open+1 : open+closing]
		if name == "" {
			return nil, fmt.Errorf("template token %q has an empty placeholder", token)
		}
		if len(segments) > 0 && segments[len(segments)-1].placeholder {
			return nil, fmt.Errorf("template token %q has adjacent placeholders without a literal separator", token)
		}
		segments = append(segments, tokenSegment{text: name, placeholder: true})
		rest = rest[open+closing+1:]
	}
	if len(segments) == 0 {
		return nil, fmt.Errorf("template token must not be empty")
	}
	return segments, nil
}

// CanonicalHash is the profile's identity for trust decisions: sha256 over the
// spec's canonical JSON encoding. Any edit to any field yields a new hash and
// therefore a fresh trust prompt. The encoding (Go struct field order) is
// pinned by a golden test.
func CanonicalHash(spec ProfileSpec) (string, error) {
	raw, err := json.Marshal(spec)
	if err != nil {
		return "", fmt.Errorf("encode profile for hashing: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
