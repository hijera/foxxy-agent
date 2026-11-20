package prompts_test

import (
	"slices"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/prompts"
)

func TestFamilies(t *testing.T) {
	cases := []struct {
		name     string
		provider string
		model    string
		want     []string
	}{
		{"anthropic by model", "", "claude-3-5-sonnet", []string{"anthropic"}},
		{"anthropic by opus id", "anthropic", "claude-opus-4-8", []string{"anthropic"}},
		{"anthropic by provider only", "anthropic", "", []string{"anthropic"}},
		{"openai gpt-4o", "openai", "gpt-4o", []string{"openai"}},
		{"openai o3-mini", "openai", "o3-mini", []string{"openai"}},
		{"openai o1 by model only", "", "o1-preview", []string{"openai"}},
		{"openai by provider only", "openai", "some-tuned", []string{"openai"}},
		{"gemini", "", "gemini-2.5-pro", []string{"gemini"}},
		{"gemma", "", "gemma-4-27b-it", []string{"gemma"}},
		{"qwen with space", "neuraldeep", "qwen 3.6", []string{"qwen"}},
		{"gpt-oss beats openai provider", "openai", "gpt-oss-120b", []string{"gpt-oss"}},
		{"gpt-oss via neuraldeep", "neuraldeep", "gpt-oss-120b", []string{"gpt-oss"}},
		{"kimi k2.6 via neuraldeep", "neuraldeep", "kimi-k2.6", []string{"kimi"}},
		{"kimi k2.7 code via neuraldeep", "neuraldeep", "kimi-k2.7-code", []string{"kimi"}},
		{"kimi k2.7 via neuraldeep", "neuraldeep", "kimi-k2.7", []string{"kimi"}},
		{"kimi k2.8 via neuraldeep", "neuraldeep", "kimi-k2.8", []string{"kimi"}},
		{"kimi k3 via neuraldeep", "neuraldeep", "kimi-k3", []string{"kimi"}},
		{"kimi k3.1 via neuraldeep", "neuraldeep", "kimi-k3.1", []string{"kimi"}},
		{"kimi k3.1 pro via neuraldeep", "neuraldeep", "kimi-k3.1-pro", []string{"kimi"}},
		{"kimi via openai compatible API", "openai", "kimi-k2-0905-preview", []string{"kimi"}},
		{"kimi model namespace and case", "openai", "  moonshotai/Kimi-K2-Instruct  ", []string{"kimi"}},
		{"moonshot ids without kimi", "openai", "moonshot-v1-128k", []string{"kimi"}},
		{"moonshot short ids", "openai", "moonshot-v1-8k", []string{"kimi"}},
		{"deepseek by model id", "", "deepseek-coder-6.7b", []string{"deepseek"}},
		{"deepseek distill qwen keeps deepseek first", "neuraldeep", "deepseek-r1-distill-qwen-32b", []string{"deepseek", "qwen"}},
		{"deepseek distill qwen via openai", "openai", "deepseek-r1-distill-qwen-7b", []string{"deepseek", "qwen"}},
		{"deepseek distill llama falls back to the provider family", "neuraldeep", "deepseek-r1-distill-llama-8b", []string{"deepseek", "neuraldeep"}},
		{"deepseek v2 via neuraldeep", "neuraldeep", "deepseek-v2", []string{"deepseek", "neuraldeep"}},
		{"deepseek v3 via neuraldeep", "neuraldeep", "deepseek-v3", []string{"deepseek", "neuraldeep"}},
		{"deepseek r1 via neuraldeep", "neuraldeep", "deepseek-r1", []string{"deepseek", "neuraldeep"}},
		{"deepseek before openai", "openai", "deepseek-chat", []string{"deepseek", "openai"}},
		{"neuraldeep local fallback", "neuraldeep", "some-local-model", []string{"neuraldeep"}},
		{"case-insensitive", "", "Claude-Sonnet", []string{"anthropic"}},
		{"empty is fallback", "", "", nil},
		{"unknown is fallback", "unknown", "mistral-large-latest", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := prompts.Families(tc.provider, tc.model); !slices.Equal(got, tc.want) {
				t.Errorf("Families(%q, %q) = %v, want %v", tc.provider, tc.model, got, tc.want)
			}
		})
	}
}
