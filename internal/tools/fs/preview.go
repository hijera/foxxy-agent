package fs

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/plans"
	"github.com/hijera/foxxycode-agent/internal/textenc"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// notifyFileEdit invokes the optional Env.OnFileEdit hook after a successful write.
// Safe to call with a nil env or nil hook.
func notifyFileEdit(env *tooling.Env, toolName, absPath string, before, after []byte) {
	NotifyFileEdit(env, toolName, absPath, before, after)
}

// NotifyFileEdit invokes the optional Env.OnFileEdit hook after a successful write.
// Safe to call with a nil env or nil hook.
func NotifyFileEdit(env *tooling.Env, toolName, absPath string, before, after []byte) {
	if env == nil || env.OnFileEdit == nil {
		return
	}
	env.OnFileEdit(toolName, absPath, before, after)
}

// EditPreview computes the resolved absolute path and the before/after content a filesystem
// write tool would produce, without touching disk. It reuses the same transforms as the
// executing tools so the preview matches the eventual write exactly.
//
// sessionDir is the session bundle directory; only plan_write needs it (its target lives
// under <sessionDir>/plans). Pass "" for tools that write into the workspace.
//
// ok is false for tool names that do not produce a content diff. err is returned only when
// the transform itself fails (e.g. an edit whose oldString is not found).
func EditPreview(toolName, argsJSON, cwd, sessionDir string) (absPath string, before, after []byte, ok bool, err error) {
	switch toolName {
	case "write":
		args, perr := tooling.ParseArgs[writeArgs](argsJSON)
		if perr != nil {
			return "", nil, nil, false, perr
		}
		absPath = ResolvePath(args.Path, cwd)
		before, _ = os.ReadFile(absPath)
		encoding := textenc.UTF8
		if before != nil {
			encoding, err = existingTextEncoding(before)
			if err != nil {
				return "", nil, nil, false, err
			}
		}
		after, err = encodeText(args.Content, encoding)
		return absPath, before, after, true, err

	case "edit":
		args, perr := tooling.ParseArgs[editArgs](argsJSON)
		if perr != nil {
			return "", nil, nil, false, perr
		}
		absPath = ResolvePath(args.Path, cwd)
		data, rerr := os.ReadFile(absPath)
		if rerr != nil {
			return "", nil, nil, false, rerr
		}
		content, encoding, derr := decodeText(data)
		if derr != nil {
			return "", nil, nil, false, derr
		}
		out, terr := applyEditToContent(content, args)
		if terr != nil {
			return "", nil, nil, false, terr
		}
		after, terr = encodeText(out, encoding)
		return absPath, data, after, true, terr

	case "apply_patch":
		args, perr := tooling.ParseArgs[applyPatchArgs](argsJSON)
		if perr != nil {
			return "", nil, nil, false, perr
		}
		absPath = ResolvePath(args.Path, cwd)
		data, rerr := os.ReadFile(absPath)
		if rerr != nil {
			return "", nil, nil, false, rerr
		}
		patchBody := strings.TrimSpace(args.Patch)
		if patchBody == "" {
			patchBody = strings.TrimSpace(args.Diff)
		}
		content, encoding, derr := decodeText(data)
		if derr != nil {
			return "", nil, nil, false, derr
		}
		out, aerr := applyPatch(content, patchBody)
		if aerr != nil {
			return "", nil, nil, false, aerr
		}
		after, aerr = encodeText(out, encoding)
		return absPath, data, after, true, aerr

	case "plan_write":
		var a struct {
			Slug    string `json:"slug"`
			Content string `json:"content"`
		}
		if perr := json.Unmarshal([]byte(argsJSON), &a); perr != nil {
			return "", nil, nil, false, perr
		}
		if sessionDir == "" {
			return "", nil, nil, false, nil
		}
		if err = plans.ValidateSlug(strings.TrimSpace(a.Slug)); err != nil {
			return "", nil, nil, false, err
		}
		absPath, err = plans.FilePath(sessionDir, strings.TrimSpace(a.Slug))
		if err != nil {
			return "", nil, nil, false, err
		}
		// doc.Content is the normalized text the local tool writes (frontmatter
		// repairs included), so the preview matches it byte for byte.
		doc, perr := plans.Parse(strings.TrimSpace(a.Slug), a.Content)
		if perr != nil {
			return "", nil, nil, false, perr
		}
		before, _ = os.ReadFile(absPath)
		return absPath, before, []byte(doc.Content), true, nil

	default:
		return "", nil, nil, false, nil
	}
}
