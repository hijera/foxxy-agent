//go:build miniapps

package miniapps

import (
	"strings"
	"testing"
)

func TestSanitizeRejectsSecretsAndSourceSpecificData(t *testing.T) {
	app := coreValidApp()
	app.Inputs = append(app.Inputs, Input{
		ID: "token", Type: "secret", Title: "Token", Default: "sk-1234567890123456",
		UI: InputUI{Control: "password"},
	})
	app.Workflow[0].Arguments = map[string]any{
		"token": "Bearer abcdefghijklmnop",
		"key":   "-----BEGIN PRIVATE KEY-----",
		"path":  "/home/alice/private.txt",
	}
	app.Extensions = map[string]any{"source_session_id": "session-123", "transcript": "private transcript"}
	report := Sanitize(app)
	if report.Clean || len(report.Findings) < 4 {
		t.Fatalf("unsafe app was clean: %+v", report)
	}
	joined := ""
	for _, finding := range report.Findings {
		joined += finding.Path + " " + finding.Message + "\n"
	}
	for _, needle := range []string{"secret", "private", "path", "session", "transcript"} {
		if !strings.Contains(strings.ToLower(joined), needle) {
			t.Errorf("sanitizer findings do not mention %q: %s", needle, joined)
		}
	}
}

func TestSanitizeAllowsSecretHandlesWithoutDefaults(t *testing.T) {
	app := coreValidApp()
	app.Inputs = append(app.Inputs, Input{
		ID: "token", Type: "secret", Title: "Token", UI: InputUI{Control: "password"},
	})
	report := Sanitize(app)
	if !report.Clean {
		t.Fatalf("secret handle should be exportable: %+v", report.Findings)
	}
}

func TestSanitizeRejectsSensitiveExtensionKeysWithoutAssignmentSyntax(t *testing.T) {
	app := coreValidApp()
	app.Extensions = map[string]any{"password": "hunter2"}
	report := Sanitize(app)
	if report.Clean {
		t.Fatal("plain credential under a sensitive key was accepted")
	}
}

func TestSanitizeEvidenceAllowsPrivateProvenanceButRejectsCredentials(t *testing.T) {
	evidence := SourceEvidence{
		SessionID:     "session-private",
		SourceFixture: map[string]any{"path": "/home/alice/source.txt", "token": "[REDACTED]"},
	}
	if report := SanitizeEvidence(evidence); !report.Clean {
		t.Fatalf("private provenance should be allowed in evidence: %+v", report.Findings)
	}
	evidence.SourceFixture["token"] = "actual-secret-value"
	if report := SanitizeEvidence(evidence); report.Clean {
		t.Fatal("unredacted evidence credential was accepted")
	}
	evidence.SourceFixture["token"] = "[REDACTED]"
	evidence.FixtureFiles = map[string][]byte{"source.env": []byte("API_KEY=definitely-secret-value\n")}
	if report := SanitizeEvidence(evidence); report.Clean {
		t.Fatal("credential in private fixture bytes was accepted")
	}
}
