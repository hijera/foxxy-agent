package shell

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ValidateReadOnlyCommand accepts a deliberately small set of inspection
// commands for Ask mode. It is a policy boundary, not a general shell parser:
// compound commands, redirections, substitutions, and dynamic command
// execution are rejected before the shell starts.
func ValidateReadOnlyCommand(command string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return fmt.Errorf("empty command")
	}
	if hasUnsafeShellSyntax(command) {
		return fmt.Errorf("compound commands, redirections, and substitutions are unavailable in Ask mode")
	}

	fields := strings.Fields(command)
	if len(fields) == 0 {
		return fmt.Errorf("empty command")
	}
	if err := validateCommandToken(fields[0]); err != nil {
		return err
	}
	name := commandBase(fields[0])
	args := fields[1:]

	switch name {
	case "git":
		return validateReadOnlyGit(args)
	case "go":
		return validateReadOnlyGo(args)
	case "rg":
		if hasAnyArgPrefix(args, "--pre", "--hostname-bin") {
			return fmt.Errorf("rg preprocessors are unavailable in Ask mode")
		}
		return nil
	case "ag":
		if hasAnyArgPrefix(args, "--pager") {
			return fmt.Errorf("ag pagers are unavailable in Ask mode")
		}
		return nil
	case "tree":
		if hasArg(args, "-o") || hasAnyArgPrefix(args, "--output") {
			return fmt.Errorf("tree output flags are unavailable in Ask mode")
		}
		return nil
	case "read", "dir", "ls", "pwd", "cat", "head", "tail", "wc",
		"which", "type", "file", "du", "df",
		"uname", "whoami", "printenv", "grep", "findstr", "uniq",
		"cut", "tr", "jq", "stat", "realpath", "readlink",
		"get-childitem", "get-content", "get-item", "get-location", "get-process",
		"get-service", "get-date", "get-ciminstance", "test-path",
		"resolve-path", "join-path", "split-path", "select-string",
		"out-string", "convertto-json", "write-output":
		return nil
	default:
		return fmt.Errorf("command %q is not in the Ask read-only allowlist", fields[0])
	}
}

func hasUnsafeShellSyntax(command string) bool {
	if strings.ContainsAny(command, "\r\n|;&`><()") {
		return true
	}
	lower := strings.ToLower(command)
	return strings.Contains(lower, "$(") ||
		strings.Contains(lower, "${") ||
		strings.Contains(lower, ">(") ||
		strings.Contains(lower, "<(")
}

func validateCommandToken(raw string) error {
	token := strings.Trim(raw, `"'`)
	if token == "" || strings.ContainsAny(token, `/\:`) {
		return fmt.Errorf("explicit command paths are unavailable in Ask mode")
	}
	return nil
}

func commandBase(raw string) string {
	raw = strings.Trim(raw, `"'`)
	base := filepath.Base(raw)
	if i := strings.LastIndexAny(base, `/\`); i >= 0 {
		base = base[i+1:]
	}
	base = strings.ToLower(base)
	return strings.TrimSuffix(base, ".exe")
}

func validateReadOnlyGit(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("git requires an explicit read-only subcommand in Ask mode")
	}
	if hasAnyArgPrefix(args, "--output", "--exec-path", "--ext-diff", "--textconv", "--show-signature") {
		return fmt.Errorf("git output, filters, and helper-program flags are unavailable in Ask mode")
	}
	sub := strings.ToLower(args[0])
	switch sub {
	case "status", "log", "show", "diff", "blame", "rev-parse", "rev-list",
		"ls-files", "ls-tree", "shortlog", "describe",
		"name-rev":
		return nil
	case "cat-file":
		if hasAnyArgPrefix(args[1:], "--filters", "--textconv") {
			break
		}
		return nil
	case "stash":
		if len(args) >= 2 && strings.EqualFold(args[1], "list") {
			return nil
		}
	case "tag":
		if hasArg(args[1:], "-l") || hasArg(args[1:], "--list") {
			return nil
		}
	case "branch":
		if hasArg(args[1:], "--list") || hasArg(args[1:], "-a") || hasArg(args[1:], "-r") {
			return nil
		}
	case "remote":
		if hasArg(args[1:], "-v") || hasArg(args[1:], "--verbose") {
			return nil
		}
	case "config":
		if hasArg(args[1:], "--list") || hasArg(args[1:], "-l") ||
			hasAnyArgPrefix(args[1:], "--get", "--show-origin", "--show-scope") {
			return nil
		}
	}
	return fmt.Errorf("git subcommand %q may modify state and is unavailable in Ask mode", sub)
}

func validateReadOnlyGo(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("go requires an explicit read-only subcommand in Ask mode")
	}
	sub := strings.ToLower(args[0])
	switch sub {
	case "version", "doc":
		return nil
	case "env":
		if hasArg(args[1:], "-w") || hasArg(args[1:], "-u") {
			return fmt.Errorf("go env write flags are unavailable in Ask mode")
		}
		return nil
	default:
		return fmt.Errorf("go subcommand %q may execute code or modify state and is unavailable in Ask mode", sub)
	}
}

func hasArg(args []string, want string) bool {
	for _, arg := range args {
		if strings.EqualFold(arg, want) {
			return true
		}
	}
	return false
}

func hasAnyArgPrefix(args []string, prefixes ...string) bool {
	for _, arg := range args {
		lower := strings.ToLower(arg)
		for _, prefix := range prefixes {
			prefix = strings.ToLower(prefix)
			if lower == prefix || strings.HasPrefix(lower, prefix+"=") {
				return true
			}
		}
	}
	return false
}
