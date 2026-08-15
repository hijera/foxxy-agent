//go:build miniapps

package miniapps

import (
	"encoding/json"
	"fmt"
	"regexp"
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
	{"credential assignment", regexp.MustCompile(`(?i)(api[_-]?key|token|password|secret)\s*[:=]\s*["']?[^\s"',}]{8,}`)},
}

// Sanitize scans the complete portable JSON document before release.
func Sanitize(app MiniApp) SanitizationReport {
	raw, _ := json.Marshal(app)
	text := string(raw)
	findings := []SanitizationFinding{}
	for _, pattern := range secretPatterns {
		if pattern.re.MatchString(text) {
			findings = append(findings, SanitizationFinding{
				Path: "$", Severity: "error", Message: fmt.Sprintf("possible %s in portable bundle", pattern.name),
			})
		}
	}
	for i, input := range app.Inputs {
		if input.Type == "secret" && input.Default != nil && strings.TrimSpace(fmt.Sprint(input.Default)) != "" {
			findings = append(findings, SanitizationFinding{
				Path: fmt.Sprintf("inputs[%d].default", i), Severity: "error",
				Message: "secret inputs cannot have persisted defaults",
			})
		}
	}
	return SanitizationReport{Clean: len(findings) == 0, Findings: findings}
}
