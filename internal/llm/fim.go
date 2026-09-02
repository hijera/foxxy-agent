package llm

import "strings"

// FIMTemplate describes how one model family expects a fill-in-the-middle prompt.
//
// Code models are trained with special control tokens that mark the text before
// the hole, the text after it, and where generation starts. Sent through a raw
// completion (RawCompleter) they turn "answer this request" into "continue this
// file", which is a different and much better behaviour for inline completion
// than anything a chat prompt can coax out of the same weights.
type FIMTemplate struct {
	// Family is the human-readable name of the token convention.
	Family string
	// Layout is the prompt with {prefix} and {suffix} placeholders. Some families
	// put the suffix first.
	Layout string
	// FileSep, when set, is the token that introduces one repository file
	// (`<|file_sep|>path\ncontent`), used to hand the model neighbouring files as
	// real files. Families without it get those as comment blocks instead.
	FileSep string
	// RepoName, when set, opens a repository-level prompt (`<|repo_name|>name`).
	RepoName string
	// Stop lists the end-of-generation tokens the model emits when the hole is
	// filled; they double as stop sequences so the raw endpoint returns cleanly.
	Stop []string
}

// FIMFile is a neighbouring file handed to the model alongside the one being
// completed.
type FIMFile struct {
	Path    string
	Content string
}

var fimTemplates = []struct {
	match []string
	tmpl  FIMTemplate
}{
	{
		// Qwen2.5-Coder and Qwen3-Coder; CodeGemma uses the same three tokens.
		match: []string{"qwen", "codegemma"},
		tmpl: FIMTemplate{
			Family:   "qwen",
			Layout:   "<|fim_prefix|>{prefix}<|fim_suffix|>{suffix}<|fim_middle|>",
			FileSep:  "<|file_sep|>",
			RepoName: "<|repo_name|>",
			Stop:     []string{"<|endoftext|>", "<|fim_pad|>", "<|repo_name|>", "<|file_sep|>", "<|file_separator|>"},
		},
	},
	{
		match: []string{"deepseek"},
		tmpl: FIMTemplate{
			Family: "deepseek",
			Layout: "<｜fim▁begin｜>{prefix}<｜fim▁hole｜>{suffix}<｜fim▁end｜>",
			Stop:   []string{"<｜end▁of▁sentence｜>", "<｜fim▁begin｜>"},
		},
	},
	{
		match: []string{"codellama", "code-llama"},
		tmpl: FIMTemplate{
			Family: "codellama",
			Layout: "<PRE> {prefix} <SUF>{suffix} <MID>",
			Stop:   []string{"<EOT>", " <EOT>", "<PRE>"},
		},
	},
	{
		match: []string{"starcoder", "santacoder", "stable-code", "stablecode", "granite"},
		tmpl: FIMTemplate{
			Family:  "starcoder",
			Layout:  "<fim_prefix>{prefix}<fim_suffix>{suffix}<fim_middle>",
			FileSep: "<file_sep>",
			Stop:    []string{"<|endoftext|>", "<file_sep>", "<fim_prefix>"},
		},
	},
	{
		match: []string{"codestral"},
		tmpl: FIMTemplate{
			Family: "codestral",
			Layout: "[SUFFIX]{suffix}[PREFIX]{prefix}",
			Stop:   []string{"[PREFIX]", "[SUFFIX]", "</s>"},
		},
	},
}

// FIMTemplateFor returns the fill-in-the-middle convention for a model id
// (provider prefix included or not), and false when the family is unknown - in
// which case only a chat prompt is safe, because the wrong control tokens are
// just noise to a model that was never trained on them.
func FIMTemplateFor(model string) (FIMTemplate, bool) {
	id := strings.ToLower(strings.TrimSpace(model))
	if i := strings.LastIndexByte(id, '/'); i >= 0 {
		id = id[i+1:]
	}
	for _, entry := range fimTemplates {
		for _, m := range entry.match {
			if strings.Contains(id, m) {
				return entry.tmpl, true
			}
		}
	}
	return FIMTemplate{}, false
}

// Prompt renders the raw prompt: neighbouring files first, then the file being
// completed with the hole marked. commentLeader (for example "//" or "#") is
// used for families without a file separator token, so the extra files arrive
// as comments inside the prefix rather than as tokens the model would try to
// complete.
func (t FIMTemplate) Prompt(path, prefix, suffix string, files []FIMFile, commentLeader string) string {
	var b strings.Builder
	if len(files) > 0 && t.FileSep != "" {
		if t.RepoName != "" {
			b.WriteString(t.RepoName)
			b.WriteString("workspace\n")
		}
		for _, f := range files {
			b.WriteString(t.FileSep)
			b.WriteString(f.Path)
			b.WriteString("\n")
			b.WriteString(strings.TrimRight(f.Content, "\n"))
			b.WriteString("\n")
		}
		b.WriteString(t.FileSep)
		b.WriteString(path)
		b.WriteString("\n")
	} else if len(files) > 0 {
		if commentLeader == "" {
			commentLeader = "//"
		}
		for _, f := range files {
			b.WriteString(commentLeader)
			b.WriteString(" ")
			b.WriteString(f.Path)
			b.WriteString("\n")
			for _, line := range strings.Split(strings.TrimRight(f.Content, "\n"), "\n") {
				b.WriteString(commentLeader)
				b.WriteString(" ")
				b.WriteString(line)
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}
	}
	// The Replacer walks the layout once; text substituted for a placeholder is
	// never rescanned, so a literal "{suffix}" inside the code cannot misfire.
	b.WriteString(strings.NewReplacer("{prefix}", prefix, "{suffix}", suffix).Replace(t.Layout))
	return b.String()
}
