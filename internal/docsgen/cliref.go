package docsgen

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// cliSection is one help screen of the binary: the arguments that print it
// and the heading it gets in the reference.
type cliSection struct {
	Title string
	Args  []string
}

// cliSections lists the help screens that make up the CLI reference. Only
// commands that answer --help with their own text are here; the verbs that
// print a one-line usage are already in the synopsis.
var cliSections = []cliSection{
	{"foxxycode", []string{"--help"}},
	{"foxxycode cli", []string{"cli", "--help"}},
	{"foxxycode acp", []string{"acp", "--help"}},
	{"foxxycode http", []string{"http", "--help"}},
	{"foxxycode serve", []string{"serve", "--help"}},
	{"foxxycode serve status | stop | restart", []string{"serve", "status", "--help"}},
	{"foxxycode sessions list", []string{"sessions", "list", "--help"}},
	{"foxxycode sessions export", []string{"sessions", "export", "--help"}},
	{"foxxycode providers", []string{"providers", "list", "--help"}},
	{"foxxycode plugin", []string{"plugin", "--help"}},
	{"foxxycode update", []string{"update", "--help"}},
}

// BuildFoxxyCode compiles the binary the reference is generated from into dir.
func BuildFoxxyCode(root, dir, tags string) (string, error) {
	out := filepath.Join(dir, "foxxycode")
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	args := []string{"build"}
	if tags != "" {
		args = append(args, "-tags="+tags)
	}
	args = append(args, "-o", out, "./cmd/foxxycode")
	cmd := exec.Command("go", args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build: %w\n%s", err, b)
	}
	return out, nil
}

// CLIReference renders the generated block of docs/reference/cli.md by
// running every help screen of binary. Exit codes are ignored: the flag
// package exits 2 after printing its usage, and that text is the point. The
// block opens with the build tags the binary was made with, because the
// synopsis and the console flags differ between builds.
func CLIReference(binary, tags string) (string, error) {
	var b strings.Builder
	if tags == "" {
		b.WriteString("Help screens of the default build (no build tags).\n\n")
	} else {
		fmt.Fprintf(&b, "Help screens of a binary built with `-tags=%s`, the set the release binaries carry.\n\n", tags)
	}
	home := os.TempDir()
	for i, s := range cliSections {
		cmd := exec.Command(binary, s.Args...)
		cmd.Env = append(os.Environ(), "NO_COLOR=1", "TERM=dumb", "FOXXYCODE_HOME="+home)
		out, _ := cmd.CombinedOutput()
		text := normalizeHelp(string(out), binary, home)
		if text == "" {
			return "", fmt.Errorf("%s printed nothing", strings.Join(append([]string{"foxxycode"}, s.Args...), " "))
		}
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "### %s\n\n```text\n%s\n```\n", s.Title, text)
	}
	return b.String(), nil
}

// normalizeHelp makes a help screen independent of the machine that printed
// it: CRLF from a Windows console becomes LF, the path the binary was run
// from becomes its name, and the temporary home the run used becomes the
// documented default, so a reference generated on Windows matches the one CI
// generates on Linux.
func normalizeHelp(out, binary, home string) string {
	text := strings.ReplaceAll(out, "\r\n", "\n")
	text = strings.TrimRight(text, "\n")
	for _, b := range []string{binary, strings.TrimSuffix(binary, ".exe"), filepath.Base(binary)} {
		if b != "" && b != "foxxycode" {
			text = strings.ReplaceAll(text, b, "foxxycode")
		}
	}
	if home != "" {
		text = strings.ReplaceAll(text, home, "~/.foxxycode")
	}
	return text
}
