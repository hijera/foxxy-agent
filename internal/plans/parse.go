package plans

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	frontmatterDelim = "---"
	frontmatterClose = "\n" + frontmatterDelim
)

// frontmatterKeys are the only top-level keys a plan frontmatter block may hold.
// A leading YAML mapping limited to these is treated as frontmatter that lost its fences.
var frontmatterKeys = map[string]bool{"name": true, "overview": true, "todos": true}

// NormalizeContent repairs plan content whose YAML frontmatter was written without the
// --- fences. Models (notably the Qwen family) often emit the keys directly, which would
// otherwise parse as a plan with no name, no overview, no todos and the raw YAML as body.
// Content that already opens with --- and content with no leading frontmatter block are
// returned unchanged.
func NormalizeContent(raw string) string {
	s := strings.TrimPrefix(raw, string(rune(0xFEFF)))
	if strings.HasPrefix(strings.TrimSpace(s), frontmatterDelim) {
		return raw
	}
	head, rest := splitLeadingYAMLBlock(s)
	if head == "" {
		return raw
	}
	var probe map[string]interface{}
	if err := yaml.Unmarshal([]byte(head), &probe); err != nil || len(probe) == 0 {
		return raw
	}
	for k := range probe {
		if !frontmatterKeys[strings.ToLower(strings.TrimSpace(k))] {
			return raw
		}
	}
	var out strings.Builder
	out.WriteString(frontmatterDelim)
	out.WriteByte('\n')
	out.WriteString(strings.TrimRight(head, "\n"))
	out.WriteByte('\n')
	out.WriteString(frontmatterDelim)
	out.WriteByte('\n')
	if body := strings.TrimLeft(rest, "\r\n"); body != "" {
		out.WriteString(body)
	}
	return out.String()
}

// splitLeadingYAMLBlock returns the leading run of non-blank lines that look like a YAML
// mapping (a `key:` line, or a continuation indented under one), plus the remainder.
// Returns "" when the text does not start with a `key:` line.
func splitLeadingYAMLBlock(s string) (head, rest string) {
	lines := strings.Split(s, "\n")
	end := -1
	for i, ln := range lines {
		t := strings.TrimSpace(ln)
		if t == "" {
			break
		}
		indented := ln != strings.TrimLeft(ln, " \t")
		if !indented && !isYAMLMappingKeyLine(t) {
			if i == 0 {
				return "", s
			}
			break
		}
		if i == 0 && indented {
			return "", s
		}
		end = i
	}
	if end < 0 {
		return "", s
	}
	return strings.Join(lines[:end+1], "\n"), strings.Join(lines[end+1:], "\n")
}

// isYAMLMappingKeyLine reports whether a trimmed line opens a YAML mapping entry.
func isYAMLMappingKeyLine(t string) bool {
	if strings.HasPrefix(t, "#") || strings.HasPrefix(t, "-") {
		return false
	}
	idx := strings.Index(t, ":")
	if idx <= 0 {
		return false
	}
	if idx+1 < len(t) && t[idx+1] != ' ' && t[idx+1] != '\t' {
		return false
	}
	key := strings.TrimSpace(t[:idx])
	return key != "" && !strings.ContainsAny(key, " \t")
}

// Parse reads a .plan.md file into a Document. Content is normalized first, so a
// frontmatter block written without fences still yields name, overview and todos.
func Parse(slug, raw string) (*Document, error) {
	if err := ValidateSlug(slug); err != nil {
		return nil, err
	}
	raw = NormalizeContent(raw)
	fm, body, err := splitFrontmatter(raw)
	if err != nil {
		return nil, err
	}
	var meta Frontmatter
	if strings.TrimSpace(fm) != "" {
		if err := yaml.Unmarshal([]byte(fm), &meta); err != nil {
			return nil, fmt.Errorf("plan frontmatter: %w", err)
		}
	}
	name := strings.TrimSpace(meta.Name)
	if name == "" {
		name = defaultPlanName
	}
	return &Document{
		Slug:     slug,
		Name:     name,
		Overview: strings.TrimSpace(meta.Overview),
		Todos:    []TodoItem(meta.Todos),
		Body:     strings.TrimRight(body, "\n"),
		Content:  raw,
	}, nil
}

// Format builds file content from parts.
func Format(fm Frontmatter, body string) string {
	var buf bytes.Buffer
	buf.WriteString(frontmatterDelim)
	buf.WriteByte('\n')
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	_ = enc.Encode(fm)
	_ = enc.Close()
	buf.WriteString(frontmatterDelim)
	buf.WriteByte('\n')
	body = strings.TrimRight(body, "\n")
	if body != "" {
		buf.WriteString(body)
		buf.WriteByte('\n')
	}
	return buf.String()
}

// DefaultContent returns starter content for a new plan.
func DefaultContent(slug, name string) string {
	n := strings.TrimSpace(name)
	if n == "" {
		n = defaultPlanName
	}
	return Format(Frontmatter{
		Name:     n,
		Overview: "",
		Todos:    nil,
	}, "## Summary\n\n(Describe the goal.)\n\n## Steps\n\n1. \n")
}

func splitFrontmatter(raw string) (yamlPart, body string, err error) {
	raw = strings.TrimPrefix(raw, "\uFEFF")
	s := strings.TrimSpace(raw)
	if !strings.HasPrefix(s, frontmatterDelim) {
		return "", strings.TrimRight(raw, "\n"), nil
	}
	rest := strings.TrimPrefix(s, frontmatterDelim)
	rest = strings.TrimLeft(rest, "\r\n")
	idx := strings.Index(rest, frontmatterClose)
	if idx < 0 {
		return "", "", fmt.Errorf("plan frontmatter: missing closing %s", frontmatterDelim)
	}
	yamlPart = strings.TrimSpace(rest[:idx])
	body = strings.TrimLeft(rest[idx+len(frontmatterClose):], "\r\n")
	return yamlPart, body, nil
}
