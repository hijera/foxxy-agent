package prompts

import "strings"

// Families returns the ordered family slugs used to select prompt templates.
//
// Model-id patterns take precedence over the provider type because open models
// are frequently served through OpenAI-compatible or
// neuraldeep endpoints, so the provider type alone would misclassify them.
// DeepSeek also keeps the previously selected family as a fallback for modes
// and custom prompt directories that have no DeepSeek variant.
func Families(providerType, modelID string) []string {
	p := strings.ToLower(strings.TrimSpace(providerType))
	m := strings.ToLower(strings.TrimSpace(modelID))

	var families []string
	switch {
	case strings.Contains(m, "gpt-oss"):
		return []string{"gpt-oss"}
	case strings.Contains(m, "gemini"):
		return []string{"gemini"}
	case strings.Contains(m, "gemma"):
		return []string{"gemma"}
	case strings.Contains(m, "qwen"):
		return []string{"qwen"}
	// "moonshot" covers the moonshot-v1-* ids Moonshot's OpenAI-compatible API
	// serves and the moonshotai/ vendor namespace; must win over p == "openai".
	case strings.Contains(m, "kimi"), strings.Contains(m, "moonshot"):
		return []string{"kimi"}
	case strings.Contains(m, "deepseek"):
		families = append(families, "deepseek")
	}

	switch {
	case strings.Contains(m, "claude") || p == "anthropic":
		return append(families, "anthropic")
	case isOpenAIModel(m) || p == "openai":
		return append(families, "openai")
	case p == "neuraldeep":
		return append(families, "neuraldeep")
	default:
		return families
	}
}

// isOpenAIModel reports whether a (lowercased) model id looks like an OpenAI model.
// gpt-oss is handled by the caller before this check, so a plain "gpt" match is safe.
func isOpenAIModel(m string) bool {
	if strings.Contains(m, "gpt") {
		return true
	}
	for _, pfx := range []string{"o1", "o3", "o4"} {
		if strings.HasPrefix(m, pfx) {
			return true
		}
	}
	return false
}
