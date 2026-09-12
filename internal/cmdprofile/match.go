package cmdprofile

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// ErrShellComplex marks a command line this package refuses to interpret: it
// carries shell syntax (operators, substitution, escapes) and therefore is not
// one simple invocation of one binary. Callers surface it as an actionable
// "cannot be distilled into a command profile" message.
var ErrShellComplex = errors.New("command uses shell syntax and is not a single simple invocation")

// shellMetaCharacters is the reject set for the raw command string, checked
// before any tokenization. The single-character set subsumes the two-character
// operators (&&, ||, >>, 2>). '$' also covers variable and arithmetic
// expansion; parentheses cover subshells and functions.
const shellMetaCharacters = "|&;<>`$()\n\r\x00"

// TokenizeSimpleCommand splits a simple command line into argv tokens.
//
// It is deliberately not a shell parser. Quotes (single or double) group a
// whole token and are stripped; they must open at a token boundary and close
// at one. There is no escape processing: a backslash is a plain character
// (Windows paths), but a backslash immediately before a quote is rejected
// because its meaning would depend on shell dialect.
func TokenizeSimpleCommand(command string) ([]string, error) {
	if strings.ContainsAny(command, shellMetaCharacters) {
		return nil, fmt.Errorf("%w: shell operator in %q", ErrShellComplex, command)
	}
	if strings.Contains(command, `\"`) || strings.Contains(command, `\'`) {
		return nil, fmt.Errorf("%w: escaped quote in %q", ErrShellComplex, command)
	}
	var tokens []string
	rest := command
	for {
		rest = strings.TrimLeft(rest, " \t")
		if rest == "" {
			break
		}
		switch rest[0] {
		case '"', '\'':
			quote := rest[0]
			end := strings.IndexByte(rest[1:], quote)
			if end < 0 {
				return nil, fmt.Errorf("%w: unterminated quote", ErrShellComplex)
			}
			token := rest[1 : 1+end]
			rest = rest[2+end:]
			if rest != "" && rest[0] != ' ' && rest[0] != '\t' {
				return nil, fmt.Errorf("%w: quote closes inside a token", ErrShellComplex)
			}
			tokens = append(tokens, token)
		default:
			end := strings.IndexAny(rest, " \t")
			if end < 0 {
				end = len(rest)
			}
			token := rest[:end]
			rest = rest[end:]
			if strings.ContainsAny(token, `"'`) {
				return nil, fmt.Errorf("%w: quote opens inside a token", ErrShellComplex)
			}
			tokens = append(tokens, token)
		}
	}
	if len(tokens) == 0 || tokens[0] == "" {
		return nil, fmt.Errorf("%w: empty command", ErrShellComplex)
	}
	return tokens, nil
}

// BinaryMatches reports whether an argv[0] token names the profile's binary:
// base names are compared with executable extensions stripped, and
// case-insensitively on Windows.
func BinaryMatches(token, binary string) bool {
	return normalizeBinaryName(token) == normalizeBinaryName(binary)
}

func normalizeBinaryName(name string) string {
	// filepath.Base is host-specific; strip both separator conventions so a
	// Windows-style token normalizes on any platform.
	base := filepath.Base(strings.ReplaceAll(name, `\`, `/`))
	lower := strings.ToLower(base)
	for _, ext := range []string{".exe", ".bat", ".cmd"} {
		if strings.HasSuffix(lower, ext) {
			base = base[:len(base)-len(ext)]
			lower = lower[:len(lower)-len(ext)]
			break
		}
	}
	if runtime.GOOS == "windows" {
		return lower
	}
	return base
}

// Match is a successful binding of a simple command line to one profile.
// Params values are strings; flags are represented as "true"/"false".
type Match struct {
	Profile ProfileSpec
	Params  map[string]string
}

// MatchProfiles tokenizes a simple command line and tries each profile in
// order; the first profile whose template binds the full argv wins. A binding
// that fails its own reconstruction is discarded — a matcher bug can therefore
// never produce a divergent profile. (nil, nil) means "no profile fits";
// ErrShellComplex propagates so callers can distinguish "too complex".
func MatchProfiles(command string, profiles []ProfileSpec) (*Match, error) {
	argv, err := TokenizeSimpleCommand(command)
	if err != nil {
		return nil, err
	}
	for _, profile := range profiles {
		if !BinaryMatches(argv[0], profile.Binary) {
			continue
		}
		params, ok := matchTemplate(profile, argv[1:])
		if !ok {
			continue
		}
		if err := VerifyReconstruction(profile, params, argv); err != nil {
			continue
		}
		return &Match{Profile: profile, Params: params}, nil
	}
	return nil, nil
}

// matchTemplate unifies the template against the argv tail. Flags branch the
// search (present/absent); profiles are small, so the bounded backtracking is
// cheap.
func matchTemplate(profile ProfileSpec, tail []string) (map[string]string, bool) {
	params := make(map[string]string, len(profile.Params))
	specs := make(map[string]ParamSpec, len(profile.Params))
	for _, param := range profile.Params {
		specs[param.Name] = param
	}
	if !matchTokens(profile.Template, tail, specs, params) {
		return nil, false
	}
	return params, true
}

func matchTokens(template, tail []string, specs map[string]ParamSpec, params map[string]string) bool {
	if len(template) == 0 {
		return len(tail) == 0
	}
	token := template[0]
	segments, err := parseTemplateToken(token)
	if err != nil {
		return false
	}
	if len(segments) == 1 && segments[0].placeholder {
		if param, ok := specs[segments[0].text]; ok && param.Type == ParamFlag {
			// Branch: flag present (literal token consumed) or absent.
			if len(tail) > 0 && tail[0] == param.Literal {
				params[param.Name] = "true"
				if matchTokens(template[1:], tail[1:], specs, params) {
					return true
				}
			}
			params[param.Name] = "false"
			return matchTokens(template[1:], tail, specs, params)
		}
	}
	if len(tail) == 0 {
		return false
	}
	captured, ok := captureToken(segments, tail[0], specs)
	if !ok {
		return false
	}
	for name, value := range captured {
		params[name] = value
	}
	return matchTokens(template[1:], tail[1:], specs, params)
}

// captureToken extracts placeholder values from one argv token by anchoring on
// the template token's literal segments. With two placeholders the middle
// literal splits on its first occurrence; any split satisfying the anchors
// reconstructs the same token, so the choice cannot produce a divergent match.
func captureToken(segments []tokenSegment, token string, specs map[string]ParamSpec) (map[string]string, bool) {
	captured := map[string]string{}
	rest := token
	for index := 0; index < len(segments); index++ {
		segment := segments[index]
		if !segment.placeholder {
			if !strings.HasPrefix(rest, segment.text) {
				return nil, false
			}
			rest = rest[len(segment.text):]
			continue
		}
		// Determine the terminating literal (if any) after this placeholder.
		value := rest
		if index+1 < len(segments) {
			next := segments[index+1].text
			cut := strings.Index(rest, next)
			if cut < 0 {
				return nil, false
			}
			value = rest[:cut]
			rest = rest[cut:]
		} else {
			rest = ""
		}
		param, declared := specs[segment.text]
		if !declared {
			return nil, false
		}
		if err := validateParamValue(param, value, dashAllowedByToken(segments)); err != nil {
			return nil, false
		}
		captured[param.Name] = value
	}
	if rest != "" {
		return nil, false
	}
	return captured, true
}

// dashAllowedByToken reports whether captured values in this token may start
// with a dash: only when the token's own leading literal already starts with
// one, i.e. the author wrote the option syntax into the template.
func dashAllowedByToken(segments []tokenSegment) bool {
	return len(segments) > 0 && !segments[0].placeholder && strings.HasPrefix(segments[0].text, "-")
}

// validateParamValue enforces the parameter's type constraints on a value that
// is about to enter an argv.
func validateParamValue(param ParamSpec, value string, dashAllowed bool) error {
	if strings.ContainsAny(value, "\x00\n\r") {
		return fmt.Errorf("param %q: value must not contain control characters", param.Name)
	}
	if !dashAllowed && strings.HasPrefix(value, "-") {
		return fmt.Errorf("param %q: value %q must not start with a dash", param.Name, value)
	}
	switch param.Type {
	case ParamFile:
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("param %q: a path is required", param.Name)
		}
	case ParamEnum:
		for _, allowed := range param.Enum {
			if value == allowed {
				return nil
			}
		}
		return fmt.Errorf("param %q: value %q is not one of the allowed values", param.Name, value)
	case ParamInt:
		number, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("param %q: %q is not an integer", param.Name, value)
		}
		if param.Min != nil && number < *param.Min {
			return fmt.Errorf("param %q: %d is below the minimum %d", param.Name, number, *param.Min)
		}
		if param.Max != nil && number > *param.Max {
			return fmt.Errorf("param %q: %d is above the maximum %d", param.Name, number, *param.Max)
		}
	case ParamString:
		pattern, err := compileParamPattern(param.Pattern)
		if err != nil {
			return fmt.Errorf("param %q: %w", param.Name, err)
		}
		if !pattern.MatchString(value) {
			return fmt.Errorf("param %q: value %q does not match the declared pattern", param.Name, value)
		}
	case ParamFlag:
		if value != "true" && value != "false" {
			return fmt.Errorf("param %q: flag value must be true or false", param.Name)
		}
	default:
		return fmt.Errorf("param %q: unknown type %q", param.Name, param.Type)
	}
	return nil
}

// BuildArgv substitutes params into the template and returns the argv tail
// (the binary itself excluded). Every value passes the same validation the
// matcher applies, so the two directions cannot diverge.
func BuildArgv(spec ProfileSpec, params map[string]string) ([]string, error) {
	argv := make([]string, 0, len(spec.Template))
	for _, token := range spec.Template {
		segments, err := parseTemplateToken(token)
		if err != nil {
			return nil, err
		}
		if len(segments) == 1 && segments[0].placeholder {
			if param, ok := findParam(spec, segments[0].text); ok && param.Type == ParamFlag {
				if params[param.Name] == "true" {
					argv = append(argv, param.Literal)
				}
				continue
			}
		}
		dashAllowed := dashAllowedByToken(segments)
		var builder strings.Builder
		for _, segment := range segments {
			if !segment.placeholder {
				builder.WriteString(segment.text)
				continue
			}
			param, declared := findParam(spec, segment.text)
			if !declared {
				return nil, fmt.Errorf("template references undeclared param %q", segment.text)
			}
			value, present := params[param.Name]
			if !present {
				return nil, fmt.Errorf("param %q is required", param.Name)
			}
			if err := validateParamValue(param, value, dashAllowed); err != nil {
				return nil, err
			}
			builder.WriteString(value)
		}
		argv = append(argv, builder.String())
	}
	return argv, nil
}

func findParam(spec ProfileSpec, name string) (ParamSpec, bool) {
	for _, param := range spec.Params {
		if param.Name == name {
			return param, true
		}
	}
	return ParamSpec{}, false
}

// VerifyReconstruction is the deterministic acceptance gate shared by the
// matcher and the LLM profile generator: building the argv from the profile
// and params must reproduce the original argv exactly (the binary token via
// name matching, the tail verbatim).
func VerifyReconstruction(spec ProfileSpec, params map[string]string, originalArgv []string) error {
	if len(originalArgv) == 0 {
		return errors.New("original argv is empty")
	}
	if !BinaryMatches(originalArgv[0], spec.Binary) {
		return fmt.Errorf("binary %q does not name %q", originalArgv[0], spec.Binary)
	}
	rebuilt, err := BuildArgv(spec, params)
	if err != nil {
		return err
	}
	tail := originalArgv[1:]
	if len(rebuilt) != len(tail) {
		return fmt.Errorf("reconstructed argv has %d tokens, original has %d", len(rebuilt), len(tail))
	}
	for index := range rebuilt {
		if rebuilt[index] != tail[index] {
			return fmt.Errorf("reconstructed token %d is %q, original is %q", index, rebuilt[index], tail[index])
		}
	}
	return nil
}
