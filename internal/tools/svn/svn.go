// Package svn exposes Subversion working copy operations as LLM tools. It is the
// SVN counterpart of driving git through run_command: the model gets explicit,
// permission-gated tools instead of assembling raw command lines.
//
// Every tool is registered only when vcs.svn.enabled is on and an svn client is
// installed (see register.go), and every tool refuses to run outside a working
// copy with an explanatory message.
package svn

import (
	"context"
	"fmt"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/svnws"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// client carries the resolved svn options into each tool closure.
type client struct {
	opts svnws.Options
}

// newClient builds the client options from the vcs.svn config section.
func newClient(cfg *config.Config) client {
	if cfg == nil {
		return client{opts: svnws.Options{TimeoutSeconds: config.SVNDefaultTimeoutSeconds}}
	}
	return client{opts: OptionsFor(cfg)}
}

// OptionsFor maps the vcs.svn config section onto svnws options. Exported so the
// HTTP layer describes working copies with exactly the same client settings.
func OptionsFor(cfg *config.Config) svnws.Options {
	if cfg == nil {
		return svnws.Options{TimeoutSeconds: config.SVNDefaultTimeoutSeconds}
	}
	return svnws.Options{
		Binary:         cfg.VCS.SVN.Binary,
		TimeoutSeconds: cfg.VCS.SVN.ResolvedTimeoutSeconds(),
		BranchLookup:   cfg.VCS.SVN.BranchLookupEnabled(),
	}
}

// cwd returns the session working directory.
func cwd(env *tooling.Env) string {
	if env == nil {
		return ""
	}
	return env.CWD
}

// requireWorkingCopy resolves the working copy for the session directory,
// returning a model-facing error message when there is none. Detection ignores
// git entirely, so a folder holding both a git repository and an SVN working
// copy resolves here exactly like a plain SVN checkout.
func (c client) requireWorkingCopy(ctx context.Context, env *tooling.Env) (svnws.Info, error) {
	dir := cwd(env)
	if strings.TrimSpace(dir) == "" {
		return svnws.Info{}, fmt.Errorf("no working directory for this session")
	}
	info := svnws.Describe(ctx, dir, c.opts)
	if !info.Available {
		return info, fmt.Errorf("svn client not found; install Subversion or set vcs.svn.binary")
	}
	if !info.IsSVNRepo {
		return info, fmt.Errorf("not an svn working copy: %s", dir)
	}
	return info, nil
}

// report renders a tool result: svn output when there is any, a short
// confirmation otherwise. Client failures are returned as text so the model can
// read the svn diagnostic and react instead of the turn erroring out.
func report(out string, err error, done string) (string, error) {
	if err != nil {
		return fmt.Sprintf("error: %v", err), nil
	}
	if trimmed := strings.TrimSpace(out); trimmed != "" {
		return trimmed, nil
	}
	return done, nil
}

// pathsArg normalises the optional "paths" argument.
func pathsArg(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// pathsSchema is the shared JSON schema fragment for path lists.
func pathsSchema(description string) map[string]interface{} {
	return map[string]interface{}{
		"type":        "array",
		"items":       map[string]interface{}{"type": "string"},
		"description": description,
	}
}

func objectSchema(props map[string]interface{}, required []string) map[string]interface{} {
	schema := map[string]interface{}{
		"type":       "object",
		"properties": props,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}
