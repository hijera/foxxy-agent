//go:build scheduler

package schedtools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/EvilFreelancer/coddy-agent/external/scheduler/lib"
	"github.com/EvilFreelancer/coddy-agent/internal/config"
	"github.com/EvilFreelancer/coddy-agent/internal/llm"
	"github.com/EvilFreelancer/coddy-agent/internal/tooling"
)

// RegisterTools registers scheduler maintenance tools (requires cfg.Scheduler enabled).
func RegisterTools(reg func(*tooling.Tool), cfg *config.Config) {
	if cfg == nil || !cfg.SchedulerEffectiveEnabled() {
		return
	}
	reg(listTool(cfg))
	reg(readTool(cfg))
	reg(writeTool(cfg))
	reg(deleteTool(cfg))
	reg(validateTool())
}

func listTool(cfg *config.Config) *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name:        ToolList,
			Description: "List scheduler job markdown files under scheduler.dir with description, schedule, path, last and next scheduled times (UTC).",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		AllowedInPlanMode: true,
		Execute: func(ctx context.Context, argsJSON string, env *tooling.Env) (string, error) {
			return execList(cfg)
		},
	}
}

func execList(cfg *config.Config) (string, error) {
	paths, err := scheduler.ListJobMarkdownFiles(cfg.SchedulerScanRoots())
	if err != nil {
		return "", err
	}
	var lines []string
	now := time.Now().UTC()
	for _, p := range paths {
		fm, _, err := scheduler.ParseJobFile(p)
		if err != nil {
			lines = append(lines, fmt.Sprintf("- %s (error: %v)", p, err))
			continue
		}
		sched, err := scheduler.ParseCronUTC(fm.Schedule)
		if err != nil {
			lines = append(lines, fmt.Sprintf("- %s | %s | cron error: %v", p, fm.Description, err))
			continue
		}
		last, _ := scheduler.ReadJobState(scheduler.StatePath(p))
		next := scheduler.NextScheduledUTC(sched, last)
		lastStr := "never"
		if !last.IsZero() {
			lastStr = last.UTC().Format(time.RFC3339)
		}
		nextStr := next.UTC().Format(time.RFC3339)
		if next.After(now) {
			nextStr += " (future)"
		}
		lines = append(lines, fmt.Sprintf("- %s | %s | schedule=%s | last=%s | next=%s", p, fm.Description, fm.Schedule, lastStr, nextStr))
	}
	if len(lines) == 0 {
		return "No scheduler job files found.", nil
	}
	return strings.Join(lines, "\n"), nil
}

func readTool(cfg *config.Config) *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name:        ToolRead,
			Description: "Read one scheduler job file by path relative to scheduler.dir (e.g. backups/nightly.md). Returns YAML frontmatter and markdown body.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{"type": "string", "description": "Relative path within scheduler dirs"},
				},
				"required": []interface{}{"path"},
			},
		},
		AllowedInPlanMode: true,
		Execute: func(ctx context.Context, argsJSON string, env *tooling.Env) (string, error) {
			var in struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal([]byte(argsJSON), &in); err != nil {
				return "", err
			}
			abs, err := resolveUnderSchedulerRoots(strings.TrimSpace(in.Path), cfg.SchedulerScanRoots())
			if err != nil {
				return "", err
			}
			data, err := os.ReadFile(abs)
			if err != nil {
				return "", err
			}
			return string(data), nil
		},
	}
}

func writeTool(cfg *config.Config) *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name:        ToolWrite,
			Description: "Create or replace a scheduler job .md file under scheduler.dir. Content must include YAML frontmatter with description and schedule (5-field crontab UTC) plus markdown instruction body.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path":    map[string]interface{}{"type": "string"},
					"content": map[string]interface{}{"type": "string"},
				},
				"required": []interface{}{"path", "content"},
			},
		},
		RequiresPermission: true,
		Execute: func(ctx context.Context, argsJSON string, env *tooling.Env) (string, error) {
			var in struct {
				Path    string `json:"path"`
				Content string `json:"content"`
			}
			if err := json.Unmarshal([]byte(argsJSON), &in); err != nil {
				return "", err
			}
			rel := strings.TrimSpace(in.Path)
			if rel == "" {
				return "", fmt.Errorf("path required")
			}
			root, err := pickWritableRoot(cfg.SchedulerScanRoots())
			if err != nil {
				return "", err
			}
			if strings.Contains(rel, "..") {
				return "", fmt.Errorf("invalid path")
			}
			abs := filepath.Join(root, filepath.Clean(rel))
			prefix, err := filepath.Abs(root)
			if err != nil {
				return "", err
			}
			ap, err := filepath.Abs(abs)
			if err != nil {
				return "", err
			}
			if !(strings.HasPrefix(ap, prefix+string(filepath.Separator)) || ap == prefix) {
				return "", fmt.Errorf("path escapes scheduler root")
			}
			if _, err := scheduler.ParseJobFromBytes([]byte(in.Content)); err != nil {
				return "", err
			}
			if err := os.MkdirAll(filepath.Dir(ap), 0o755); err != nil {
				return "", err
			}
			if err := os.WriteFile(ap, []byte(in.Content), 0o644); err != nil {
				return "", err
			}
			return fmt.Sprintf("Wrote %s", ap), nil
		},
	}
}

func deleteTool(cfg *config.Config) *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name:        ToolDelete,
			Description: "Delete a scheduler job .md and its sidecar .state and .lock files if present.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{"type": "string"},
				},
				"required": []interface{}{"path"},
			},
		},
		RequiresPermission: true,
		Execute: func(ctx context.Context, argsJSON string, env *tooling.Env) (string, error) {
			var in struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal([]byte(argsJSON), &in); err != nil {
				return "", err
			}
			abs, err := resolveUnderSchedulerRoots(strings.TrimSpace(in.Path), cfg.SchedulerScanRoots())
			if err != nil {
				return "", err
			}
			_ = os.Remove(scheduler.LockPath(abs))
			_ = os.Remove(scheduler.StatePath(abs))
			if err := os.Remove(abs); err != nil {
				return "", err
			}
			return fmt.Sprintf("Deleted %s", abs), nil
		},
	}
}

func validateTool() *tooling.Tool {
	return &tooling.Tool{
		Definition: llm.ToolDefinition{
			Name:        ToolValidate,
			Description: "Validate a 5-field crontab schedule string (UTC) and print next three run times.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"schedule": map[string]interface{}{"type": "string"},
				},
				"required": []interface{}{"schedule"},
			},
		},
		AllowedInPlanMode: true,
		Execute: func(ctx context.Context, argsJSON string, env *tooling.Env) (string, error) {
			var in struct {
				Schedule string `json:"schedule"`
			}
			if err := json.Unmarshal([]byte(argsJSON), &in); err != nil {
				return "", err
			}
			sch, err := scheduler.ParseCronUTC(in.Schedule)
			if err != nil {
				return "", err
			}
			last := scheduler.CronEpoch()
			var out []string
			for i := 0; i < 3; i++ {
				n := sch.Next(last)
				out = append(out, n.UTC().Format(time.RFC3339))
				last = n
			}
			return strings.Join(out, "\n"), nil
		},
	}
}

func resolveUnderSchedulerRoots(rel string, roots []string) (string, error) {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" || strings.Contains(rel, "..") {
		return "", fmt.Errorf("invalid relative path")
	}
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		cand := filepath.Join(root, filepath.FromSlash(rel))
		ar, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		ac, err := filepath.Abs(cand)
		if err != nil {
			continue
		}
		if strings.HasPrefix(ac, ar+string(filepath.Separator)) || ac == ar {
			if st, err := os.Stat(ac); err == nil && !st.IsDir() {
				return ac, nil
			}
		}
	}
	return "", fmt.Errorf("path not found under scheduler.dir")
}

func pickWritableRoot(roots []string) (string, error) {
	for _, r := range roots {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		if fi, err := os.Stat(r); err == nil && fi.IsDir() {
			return r, nil
		}
	}
	return "", fmt.Errorf("no scheduler directory available")
}
