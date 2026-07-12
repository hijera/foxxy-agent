package rules

import (
	"os"
	"path/filepath"
	"strings"
)

// foxxyRulesRoots are the top-level project rule roots, highest precedence first.
// Both a single file and a directory of markdown files are accepted at each root.
var foxxyRulesRoots = []string{".foxxyrules", ".foxyrules"}

// FoxxyRulesProvider loads a top-level .foxxyrules / .foxyrules rule root,
// accepting either a single file or a directory of markdown files (cline-style
// .clinerules). A single file without frontmatter is always loaded; with
// frontmatter it honors globs / alwaysApply like any other rule.
type FoxxyRulesProvider struct {
	rootRel string
}

func (p *FoxxyRulesProvider) ID() Source { return SourceFoxxyCode }

func (p *FoxxyRulesProvider) RulesRoot() string { return p.rootRel }

func (p *FoxxyRulesProvider) Load(root string) ([]*Rule, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return loadMarkdownRulesFromRoot(root, SourceFoxxyCode)
	}
	data, err := os.ReadFile(root)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(string(data)) == "" {
		return nil, nil
	}
	r, err := parseRuleFile(root, SourceFoxxyCode, data)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(r.Name) == "" {
		// filepath.Ext(".foxxyrules") == ".foxxyrules", so the stem is empty;
		// give the dotfile a stable @mention name.
		r.Name = strings.TrimPrefix(filepath.Base(root), ".")
	}
	return []*Rule{r}, nil
}
