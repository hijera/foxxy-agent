package llm

import (
	"strings"
	"testing"
)

func TestFIMTemplateForKnownFamilies(t *testing.T) {
	cases := map[string]string{
		"neuraldeep/qwen3.6-35b-a3b":      "qwen",
		"local/Qwen2.5-Coder-7B-Instruct": "qwen",
		"openai/deepseek-coder-6.7b":      "deepseek",
		"local/codellama-13b":             "codellama",
		"local/starcoder2-15b":            "starcoder",
		"local/granite-8b-code":           "starcoder",
		"mistral/codestral-latest":        "codestral",
		"google/codegemma-7b":             "qwen",
		"local/stable-code-instruct-3b":   "starcoder",
		"openai/gpt-4o":                   "",
		"anthropic/claude-sonnet-4-5":     "",
		"neuraldeep/gpt-oss-120b":         "",
		"neuraldeep/kimi-k2.6":            "",
		"neuraldeep/gemma-4-31b":          "",
	}
	for model, want := range cases {
		tmpl, ok := FIMTemplateFor(model)
		if want == "" {
			if ok {
				t.Errorf("%s: expected no FIM template, got %s", model, tmpl.Family)
			}
			continue
		}
		if !ok || tmpl.Family != want {
			t.Errorf("%s: family = %q (ok=%v), want %q", model, tmpl.Family, ok, want)
		}
	}
}

func TestFIMPromptQwenLayoutAndFiles(t *testing.T) {
	tmpl, _ := FIMTemplateFor("qwen3-coder")
	got := tmpl.Prompt("main.go", "func add(a, b int) int {\n\treturn ", "\n}",
		[]FIMFile{{Path: "util.go", Content: "package main\n\nfunc helper() {}\n"}}, "//")

	want := "<|repo_name|>workspace\n" +
		"<|file_sep|>util.go\npackage main\n\nfunc helper() {}\n" +
		"<|file_sep|>main.go\n" +
		"<|fim_prefix|>func add(a, b int) int {\n\treturn <|fim_suffix|>\n}<|fim_middle|>"
	if got != want {
		t.Fatalf("prompt mismatch:\n got %q\nwant %q", got, want)
	}
}

func TestFIMPromptWithoutFileSepUsesComments(t *testing.T) {
	tmpl, _ := FIMTemplateFor("deepseek-coder")
	got := tmpl.Prompt("a.py", "x = ", "\n", []FIMFile{{Path: "b.py", Content: "def f():\n    pass"}}, "#")
	if !strings.HasPrefix(got, "# b.py\n# def f():\n#     pass\n\n<｜fim▁begin｜>x = ") {
		t.Fatalf("comment prelude missing: %q", got)
	}
	if !strings.HasSuffix(got, "<｜fim▁hole｜>\n<｜fim▁end｜>") {
		t.Fatalf("layout tail wrong: %q", got)
	}
}

func TestFIMPromptSuffixFirstFamily(t *testing.T) {
	tmpl, _ := FIMTemplateFor("codestral")
	got := tmpl.Prompt("a.go", "PRE", "SUF", nil, "")
	if got != "[SUFFIX]SUF[PREFIX]PRE" {
		t.Fatalf("codestral layout = %q", got)
	}
}

// A literal placeholder inside the code must not be expanded twice.
func TestFIMPromptDoesNotRescanSubstitutedText(t *testing.T) {
	tmpl, _ := FIMTemplateFor("qwen")
	got := tmpl.Prompt("a.go", "s := \"{suffix}\"", "", nil, "")
	if !strings.Contains(got, "<|fim_prefix|>s := \"{suffix}\"<|fim_suffix|><|fim_middle|>") {
		t.Fatalf("placeholder inside code was rewritten: %q", got)
	}
}
