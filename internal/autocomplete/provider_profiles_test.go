package autocomplete

import (
	"testing"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/llm"
)

func TestAutocompleteKeepsProviderProfileCLILoginIsolation(t *testing.T) {
	cfg := &config.Config{
		Providers: []config.ProviderConfig{{Name: "codex", Type: "codex"}, {Name: "work", Type: "codex"}},
		Models:    []config.ModelEntry{{Model: "codex/gpt-5"}, {Model: "work/gpt-5"}},
	}
	for _, tc := range []struct {
		model   string
		blocked bool
	}{{"codex/gpt-5", false}, {"work/gpt-5", true}} {
		t.Run(tc.model, func(t *testing.T) {
			var got llm.ProviderInput
			s := &runner{agentProviderFactory: func(in llm.ProviderInput) (llm.Provider, error) { got = in; return nil, nil }}
			_, err := s.autocompleteProvider(cfg, &config.AutocompleteConfig{Model: tc.model}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got.NoCLILogin != tc.blocked {
				t.Fatalf("NoCLILogin = %v, want %v", got.NoCLILogin, tc.blocked)
			}
		})
	}
}
