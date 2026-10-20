//go:build miniapps

package miniapps

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type SanitizationFinding struct {
	Path     string `json:"path"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type SanitizationReport struct {
	Clean    bool                  `json:"clean"`
	Findings []SanitizationFinding `json:"findings"`
}

var secretPatterns = []struct {
	name string
	re   *regexp.Regexp
}{
	{"private key", regexp.MustCompile(`-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----`)},
	{"API key", regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{16,}\b`)},
	{"bearer token", regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/-]{16,}=*`)},
	{"credential assignment", regexp.MustCompile(`(?i)(api[_-]?key|access[_-]?token|token|password|secret|cookie)\s*[:=]\s*["']?[A-Za-z0-9._~+/-]{8,}`)},
}

// absolutePathPattern matches user-specific roots. The Windows alternatives are
// quantified rather than doubled: values are scanned after JSON decoding, so a
// drive path arrives with a single separator (`C:\Users\...`) and only bundled
// file bytes can still carry the escaped form.
var absolutePathPattern = regexp.MustCompile(`(?i)(^|[\s"'=:(])(?:/home/|/users/|/private/|/var/folders/|[a-z]:\\+|\\{2,})[^\s"'<>]+`)

// Sanitize scans the complete portable JSON document before release. It does
// not inspect SourceEvidence because evidence is private authoring data and is
// never copied into a released document; callers use SanitizeEvidence for the
// separate evidence/fixture gate.
func Sanitize(app MiniApp) SanitizationReport {
	raw, err := json.Marshal(app)
	if err != nil {
		return SanitizationReport{Clean: false, Findings: []SanitizationFinding{{Path: "$", Severity: "error", Message: "portable document could not be encoded"}}}
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return SanitizationReport{Clean: false, Findings: []SanitizationFinding{{Path: "$", Severity: "error", Message: "portable document could not be decoded"}}}
	}
	findings := make([]SanitizationFinding, 0)
	walkSanitizedValue(decoded, "$", &findings)
	for index, input := range app.Inputs {
		if input.Type == "secret" && input.Default != nil && strings.TrimSpace(fmt.Sprint(input.Default)) != "" {
			findings = append(findings, SanitizationFinding{
				Path: fmt.Sprintf("inputs[%d].default", index), Severity: "error",
				Message: "secret inputs cannot have persisted defaults",
			})
		}
	}
	return finalizeSanitization(findings)
}

// SanitizeEvidence checks private authoring evidence for retained credentials.
// Session provenance and source paths are expected in this private document;
// Sanitize rejects them if they cross into the portable MiniApp instead.
func SanitizeEvidence(evidence SourceEvidence) SanitizationReport {
	raw, err := json.Marshal(evidence)
	if err != nil {
		return SanitizationReport{Clean: false, Findings: []SanitizationFinding{{Path: "$", Severity: "error", Message: "authoring evidence could not be encoded"}}}
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return SanitizationReport{Clean: false, Findings: []SanitizationFinding{{Path: "$", Severity: "error", Message: "authoring evidence could not be decoded"}}}
	}
	findings := make([]SanitizationFinding, 0)
	walkEvidenceValue(decoded, "$", &findings)
	fileReport := SanitizeFiles(evidence.FixtureFiles)
	findings = append(findings, fileReport.Findings...)
	return finalizeSanitization(findings)
}

func walkEvidenceValue(value any, path string, findings *[]SanitizationFinding) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			childPath := path + "." + key
			if isSensitiveKey(key) && !isRedactedEvidenceValue(child) {
				*findings = append(*findings, SanitizationFinding{Path: childPath, Severity: "error", Message: "unredacted credential in authoring evidence"})
			}
			walkEvidenceValue(child, childPath, findings)
		}
	case []any:
		for index, child := range typed {
			walkEvidenceValue(child, fmt.Sprintf("%s[%d]", path, index), findings)
		}
	case string:
		for _, pattern := range secretPatterns {
			if pattern.re.MatchString(typed) {
				*findings = append(*findings, SanitizationFinding{Path: path, Severity: "error", Message: "possible " + pattern.name + " in authoring evidence"})
			}
		}
	}
}

func isRedactedEvidenceValue(value any) bool {
	text := strings.TrimSpace(strings.ToLower(fmt.Sprint(value)))
	return text == "" || text == "<nil>" || text == "[redacted]" || text == "redacted"
}

// SanitizeFiles scans bundled or fixture bytes without retaining their
// contents. Paths are included in findings so release review can remove the
// offending artifact without exposing the secret itself.
func SanitizeFiles(files map[string][]byte) SanitizationReport {
	findings := make([]SanitizationFinding, 0)
	for name, raw := range files {
		text := string(raw)
		for _, pattern := range secretPatterns {
			if pattern.re.MatchString(text) {
				findings = append(findings, SanitizationFinding{Path: "files." + name, Severity: "error", Message: "possible " + pattern.name + " in bundled content"})
			}
		}
		if absolutePathPattern.MatchString(text) {
			findings = append(findings, SanitizationFinding{Path: "files." + name, Severity: "error", Message: "absolute user-specific path in bundled content"})
		}
	}
	return finalizeSanitization(findings)
}

func walkSanitizedValue(value any, path string, findings *[]SanitizationFinding) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			childPath := path + "." + key
			keyName := strings.ToLower(key)
			if isSensitiveKey(keyName) && !isRedactedEvidenceValue(child) {
				*findings = append(*findings, SanitizationFinding{Path: childPath, Severity: "error", Message: "unredacted credential in portable document"})
			}
			if isProvenanceKey(keyName) {
				*findings = append(*findings, SanitizationFinding{Path: childPath, Severity: "error", Message: "source-session provenance is not allowed in a portable document"})
			}
			walkSanitizedValue(child, childPath, findings)
		}
	case []any:
		for index, child := range typed {
			walkSanitizedValue(child, fmt.Sprintf("%s[%d]", path, index), findings)
		}
	case string:
		for _, pattern := range secretPatterns {
			if pattern.re.MatchString(typed) {
				*findings = append(*findings, SanitizationFinding{Path: path, Severity: "error", Message: "possible " + pattern.name + " in portable document"})
			}
		}
		if absolutePathPattern.MatchString(typed) {
			*findings = append(*findings, SanitizationFinding{Path: path, Severity: "error", Message: "absolute user-specific path in portable document"})
		}
	}
}

func isProvenanceKey(key string) bool {
	switch key {
	case "source_session_id", "session_id", "transcript", "raw_transcript", "conversation", "source_trace":
		return true
	default:
		return false
	}
}

func finalizeSanitization(findings []SanitizationFinding) SanitizationReport {
	seen := make(map[string]bool, len(findings))
	unique := findings[:0]
	for _, finding := range findings {
		key := finding.Path + "\x00" + finding.Message
		if seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, finding)
	}
	sort.SliceStable(unique, func(left, right int) bool {
		if unique[left].Path == unique[right].Path {
			return unique[left].Message < unique[right].Message
		}
		return unique[left].Path < unique[right].Path
	})
	return SanitizationReport{Clean: len(unique) == 0, Findings: unique}
}
