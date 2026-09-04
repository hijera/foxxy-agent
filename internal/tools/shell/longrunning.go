package shell

import (
	"path/filepath"
	"strings"
)

// Long-running work has no natural end: a dev server, a bundler in watch mode, a
// daemon. The pool must not put a clock on it — see bgtask.Spec.NoTimeout.
//
// Detection is deliberately narrow. A false positive only means the task runs
// until it finishes or background_stop ends it; a false negative kills a dev
// server out from under the turn that started it, which is the failure this
// exists to prevent. So only unmistakable invocations count.

// devServerSubcommands are subcommands that start a server or a watcher when
// passed to a JS package runner or a project CLI.
var devServerSubcommands = map[string]bool{
	"serve":   true,
	"dev":     true,
	"start":   true,
	"watch":   true,
	"preview": true,
	"server":  true,
}

// packageRunners forward their first non-flag argument to a project script, so
// the subcommand after them decides.
var packageRunners = map[string]bool{
	"npm": true, "yarn": true, "pnpm": true, "bun": true, "npx": true,
	"vue-cli-service": true, "webpack": true, "next": true, "nuxt": true,
	"php": true, "rails": true, "artisan": true, "deno": true, "go": true,
	"python": true, "python3": true,
}

// alwaysLongRunning are binaries whose whole purpose is to keep running.
var alwaysLongRunning = map[string]bool{
	"nodemon": true, "vite": true, "http-server": true, "serve": true,
	"live-server": true, "watchexec": true, "air": true,
}

// isLongRunningCommand reports whether cmd starts work that is not expected to
// exit on its own.
func isLongRunningCommand(cmd string) bool {
	for _, segment := range splitShellSegments(cmd) {
		if segmentIsLongRunning(segment) {
			return true
		}
	}
	return false
}

// splitShellSegments breaks a command line on the operators that separate whole
// commands, so `cd frontend && yarn serve` is judged on `yarn serve`.
func splitShellSegments(cmd string) []string {
	replacer := strings.NewReplacer("&&", "\n", "||", "\n", ";", "\n", "|", "\n")
	return strings.Split(replacer.Replace(cmd), "\n")
}

func segmentIsLongRunning(segment string) bool {
	fields := strings.Fields(segment)
	if len(fields) == 0 {
		return false
	}
	// `docker compose up` keeps the stack in the foreground unless detached.
	if isDockerComposeUp(fields) {
		return true
	}
	// A watch flag anywhere in the segment is decisive on its own.
	for _, f := range fields[1:] {
		if f == "--watch" || f == "-w" || strings.HasPrefix(f, "--watch=") {
			return true
		}
	}

	program := normaliseProgram(fields[0])
	if alwaysLongRunning[program] {
		return true
	}
	if !packageRunners[program] {
		return false
	}
	// Look for the subcommand among the arguments. Some runners take a script
	// before it (`php artisan serve`), so every non-flag argument is considered
	// rather than only the first.
	for _, arg := range fields[1:] {
		if strings.HasPrefix(arg, "-") || arg == "run" || arg == "exec" {
			continue
		}
		// Only a bare token counts as a subcommand. A path that merely contains
		// the word — `go build ./cmd/serve`, `cat src/serve/readme.md` — is a
		// destination, not an instruction to keep running.
		if isBareToken(arg) && devServerSubcommands[arg] {
			return true
		}
		if isBareToken(arg) && alwaysLongRunning[arg] {
			return true
		}
		// `python -m http.server` serves a directory until interrupted.
		if strings.HasPrefix(arg, "http.server") {
			return true
		}
	}
	return false
}

// isBareToken reports whether arg is a plain word rather than a path.
func isBareToken(arg string) bool {
	return arg != "" && !strings.ContainsAny(arg, `/\`)
}

// isDockerComposeUp matches `docker compose up` / `docker-compose up` while
// letting the detached form through: `-d` returns immediately.
func isDockerComposeUp(fields []string) bool {
	program := normaliseProgram(fields[0])
	rest := fields[1:]
	switch program {
	case "docker-compose":
	case "docker":
		if len(rest) == 0 || rest[0] != "compose" {
			return false
		}
		rest = rest[1:]
	default:
		return false
	}
	sawUp := false
	for _, arg := range rest {
		if arg == "-d" || arg == "--detach" {
			return false
		}
		if arg == "up" {
			sawUp = true
		}
	}
	return sawUp
}

// normaliseProgram reduces a program reference to its bare name: strips any path
// and, on Windows, the executable suffix.
func normaliseProgram(s string) string {
	s = strings.Trim(strings.TrimSpace(s), `"'`)
	s = filepath.Base(filepath.ToSlash(s))
	s = strings.TrimSuffix(strings.TrimSuffix(s, ".exe"), ".cmd")
	return strings.ToLower(s)
}
