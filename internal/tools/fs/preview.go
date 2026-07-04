package fs

import (
	"os"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// notifyFileEdit invokes the optional Env.OnFileEdit hook after a successful write.
// Safe to call with a nil env or nil hook.
func notifyFileEdit(env *tooling.Env, toolName, absPath string, before, after []byte) {
	if env == nil || env.OnFileEdit == nil {
		return
	}
	env.OnFileEdit(toolName, absPath, before, after)
}

// EditPreview computes the resolved absolute path and the before/after content a filesystem
// write tool would produce, without touching disk. It reuses the same transforms as the
// executing tools so the preview matches the eventual write exactly.
//
// ok is false for tool names that do not produce a content diff. err is returned only when
// the transform itself fails (e.g. an edit whose oldString is not found).
func EditPreview(toolName, argsJSON, cwd string) (absPath string, before, after []byte, ok bool, err error) {
	switch toolName {
	case "write":
		args, perr := tooling.ParseArgs[writeArgs](argsJSON)
		if perr != nil {
			return "", nil, nil, false, perr
		}
		absPath = ResolvePath(args.Path, cwd)
		before, _ = os.ReadFile(absPath)
		return absPath, before, []byte(args.Content), true, nil

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
		out, terr := applyEditToContent(string(data), args)
		if terr != nil {
			return "", nil, nil, false, terr
		}
		return absPath, data, []byte(out), true, nil

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
		out, aerr := applyPatch(string(data), patchBody)
		if aerr != nil {
			return "", nil, nil, false, aerr
		}
		return absPath, data, []byte(out), true, nil

	default:
		return "", nil, nil, false, nil
	}
}
