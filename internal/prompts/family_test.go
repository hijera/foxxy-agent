package prompts_test

import (
	"testing"

	"github.com/hijera/foxxycode-agent/internal/prompts"
)

func TestFamily(t *testing.T) {
	cases := []struct {
		name     string
		provider string
		model    string
		want     string
	}{
		{"anthropic by model", "", "claude-3-5-sonnet", "anthropic"},
		{"anthropic by opus id", "anthropic", "claude-opus-4-8", "anthropic"},
		{"anthropic by provider only", "anthropic", "", "anthropic"},
		{"openai gpt-4o", "openai", "gpt-4o", "openai"},
		{"openai o3-mini", "openai", "o3-mini", "openai"},
		{"openai o1 by model only", "", "o1-preview", "openai"},
		{"openai by provider only", "openai", "some-tuned", "openai"},
		{"gemini", "", "gemini-2.5-pro", "gemini"},
		{"gemma", "", "gemma-4-27b-it", "gemma"},
		{"qwen with space", "neuraldeep", "qwen 3.6", "qwen"},
		{"gpt-oss beats openai provider", "openai", "gpt-oss-120b", "gpt-oss"},
		{"gpt-oss via neuraldeep", "neuraldeep", "gpt-oss-120b", "gpt-oss"},
		{"kimi k2.6 via neuraldeep", "neuraldeep", "kimi-k2.6", "kimi"},
		{"kimi k2.7 code via neuraldeep", "neuraldeep", "kimi-k2.7-code", "kimi"},
		{"kimi k2.7 via neuraldeep", "neuraldeep", "kimi-k2.7", "kimi"},
		{"kimi k2.8 via neuraldeep", "neuraldeep", "kimi-k2.8", "kimi"},
		{"kimi k3 via neuraldeep", "neuraldeep", "kimi-k3", "kimi"},
		{"kimi k3.1 via neuraldeep", "neuraldeep", "kimi-k3.1", "kimi"},
		{"kimi k3.1 pro via neuraldeep", "neuraldeep", "kimi-k3.1-pro", "kimi"},
		{"kimi via openai compatible API", "openai", "kimi-k2-0905-preview", "kimi"},
		{"kimi model namespace and case", "openai", "  moonshotai/Kimi-K2-Instruct  ", "kimi"},
		{"moonshot ids without kimi", "openai", "moonshot-v1-128k", "kimi"},
		{"moonshot short ids", "openai", "moonshot-v1-8k", "kimi"},
		{"neuraldeep local fallback", "neuraldeep", "some-local-model", "neuraldeep"},
		{"case-insensitive", "", "Claude-Sonnet", "anthropic"},
		{"empty is fallback", "", "", ""},
		{"unknown is fallback", "unknown", "mistral-large-latest", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := prompts.Family(tc.provider, tc.model); got != tc.want {
				t.Errorf("Family(%q, %q) = %q, want %q", tc.provider, tc.model, got, tc.want)
			}
		})
	}
}
