package agent

import (
	"strings"
	"testing"
)

func TestLanguageDirective(t *testing.T) {
	tests := []struct {
		name    string
		locale  string
		wantSub string
	}{
		{name: "russian", locale: "ru", wantSub: "Russian"},
		{name: "english", locale: "en", wantSub: "English"},
		{name: "auto detect empty", locale: "", wantSub: "same language the user writes"},
		{name: "auto detect whitespace", locale: "   ", wantSub: "same language the user writes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := languageDirective(tt.locale)
			if !strings.HasPrefix(got, "## Response language") {
				t.Fatalf("directive should start with the heading, got %q", got)
			}
			if !strings.Contains(got, tt.wantSub) {
				t.Fatalf("directive for locale %q = %q, want substring %q", tt.locale, got, tt.wantSub)
			}
		})
	}
}
