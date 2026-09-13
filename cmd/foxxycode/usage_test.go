package main

import (
	"bytes"
	"os"
	"slices"
	"strings"
	"testing"
)

// topLevelCommands is what `foxxycode <command>` dispatches. The packaging rule ties
// three files to this set - the usage text, the man page and the shell
// completions - and nothing else keeps them together, so the list is asserted
// against all of them here, in both directions: a command the usage text has
// and a completion lacks, and a name a completion still offers after the
// binary stopped answering it (`http` and `gateway` after `serve` took over,
// issue #188), both fail.
//
// Adding a command means adding it below, which then fails until the usage
// text, the man page and the completions carry it too.
var topLevelCommands = []string{
	"acp", "cli", "http", "desktop", "gateway", "serve", "sessions", "skills", "plugin",
	"mcp", "codex", "providers", "rules", "agents", "hooks", "update",
}

// serveVerbs control a daemon that is already running. They are subcommands of
// `serve` rather than commands of their own, so the top-level assertions above
// say nothing about them - and the same three files still have to carry them.
var serveVerbs = []string{"status", "stop", "restart"}

func TestUsageListsEveryCommand(t *testing.T) {
	assertSameSet(t, "the usage text", usageCommands(t), "topLevelCommands", topLevelCommands)
}

func TestPackagingFilesTrackTheCommandSet(t *testing.T) {
	completions := readRepoFile(t, "../../packaging/completions/foxxycode.bash")
	zsh := readRepoFile(t, "../../packaging/completions/foxxycode.zsh")
	man := readRepoFile(t, "../../packaging/man/foxxycode.1")
	usage := usageCommands(t)

	// The bash completion enumerates the commands on one line, and the zsh one
	// in its commands=( ... ) array, so those are the things to assert against
	// - the word appearing anywhere else in the file (a per-command flag case,
	// a comment) proves nothing. Both are compared as sets with the usage
	// text, so a name lingering after its command went is caught as well.
	assertSameSet(t, "packaging/completions/foxxycode.bash", commandWords(t, completions), "the usage text", usage)
	assertSameSet(t, "packaging/completions/foxxycode.zsh", zshCommandWords(t, zsh), "the usage text", usage)
	for _, cmd := range usage {
		if !strings.Contains(man, "\n.B "+cmd+"\n") && !strings.Contains(man, "\n.B \""+cmd+" ") && !strings.Contains(man, "\n.BI \""+cmd+" ") {
			t.Errorf("packaging/man/foxxycode.1 does not document %q", cmd)
		}
	}
}

// usageCommands derives the command set from the usage text itself: the word
// after the program name on every usage line, minus the flag forms and the
// bare invocation. The test binary is the program name here.
func usageCommands(t *testing.T) []string {
	t.Helper()
	var buf bytes.Buffer
	printUsage(&buf)
	var out []string
	for _, line := range strings.Split(buf.String(), "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), os.Args[0]+" ")
		if !ok {
			continue
		}
		word := strings.Fields(rest)[0]
		if strings.HasPrefix(word, "-") || strings.HasPrefix(word, "(") || slices.Contains(out, word) {
			continue
		}
		out = append(out, word)
	}
	if len(out) == 0 {
		t.Fatal("no command lines found in the usage text")
	}
	return out
}

// assertSameSet reports every name one side has and the other lacks.
func assertSameSet(t *testing.T, gotName string, got []string, wantName string, want []string) {
	t.Helper()
	for _, w := range want {
		if !slices.Contains(got, w) {
			t.Errorf("%s does not offer %q, which %s lists", gotName, w, wantName)
		}
	}
	for _, g := range got {
		if !slices.Contains(want, g) {
			t.Errorf("%s offers %q, which %s does not list", gotName, g, wantName)
		}
	}
}

func TestPackagingFilesTrackTheServeVerbs(t *testing.T) {
	usage := func() string {
		var buf bytes.Buffer
		printUsage(&buf)
		return buf.String()
	}()
	completions := readRepoFile(t, "../../packaging/completions/foxxycode.bash")
	zsh := readRepoFile(t, "../../packaging/completions/foxxycode.zsh")
	man := readRepoFile(t, "../../packaging/man/foxxycode.1")

	for _, verb := range serveVerbs {
		if !strings.Contains(usage, verb) {
			t.Errorf("usage does not mention `serve %s`", verb)
		}
		if !strings.Contains(completions, verb) {
			t.Errorf("packaging/completions/foxxycode.bash does not offer `serve %s`", verb)
		}
		if !strings.Contains(zsh, verb) {
			t.Errorf("packaging/completions/foxxycode.zsh does not offer `serve %s`", verb)
		}
		if !strings.Contains(man, "serve "+verb) {
			t.Errorf("packaging/man/foxxycode.1 does not document `serve %s`", verb)
		}
	}
	// The background form is what the verbs are about, so it is asserted with
	// them rather than left to a reader to notice it went missing.
	for name, body := range map[string]string{
		"the usage text":                       usage,
		"packaging/completions/foxxycode.bash": completions,
		"packaging/completions/foxxycode.zsh":  zsh,
		"packaging/man/foxxycode.1":            man,
	} {
		if !strings.Contains(body, "daemon") {
			t.Errorf("%s does not mention `serve --daemon`", name)
		}
	}
}

// commandWords reads the words of the bash completion's commands= line.
func commandWords(t *testing.T, completions string) []string {
	t.Helper()
	for _, line := range strings.Split(completions, "\n") {
		_, rest, ok := strings.Cut(line, `commands="`)
		if !ok {
			continue
		}
		list, _, ok := strings.Cut(rest, `"`)
		if !ok {
			t.Fatalf("unterminated commands= line: %s", line)
		}
		return strings.Fields(list)
	}
	t.Fatal("packaging/completions/foxxycode.bash has no commands= line")
	return nil
}

// zshCommandWords reads the 'name:description' entries of the zsh completion's
// commands=( ... ) array.
func zshCommandWords(t *testing.T, zsh string) []string {
	t.Helper()
	_, body, ok := strings.Cut(zsh, "commands=(")
	if !ok {
		t.Fatal("packaging/completions/foxxycode.zsh has no commands=( array")
	}
	// The array ends at a ")" of its own, not at the first one in the file: a
	// command description may carry parentheses ("messenger gateway (Telegram)").
	var out []string
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimSpace(line) == ")" {
			break
		}
		entry := strings.Trim(strings.TrimSpace(line), "'")
		name, _, ok := strings.Cut(entry, ":")
		if !ok || name == "" {
			continue
		}
		out = append(out, name)
	}
	return out
}

// assertCompletionArmsOffer checks that every command of checkFlagCommands is
// offered flag by its own case arm in both completions. The flag also sits in
// the list offered after the bare binary, so counting it across the file would
// pass with an arm missing.
func assertCompletionArmsOffer(t *testing.T, flag string) {
	t.Helper()
	for _, file := range completionFiles {
		script := readRepoFile(t, file)
		for _, cmd := range checkFlagCommands {
			if arm := completionArm(t, file, script, cmd); !strings.Contains(arm, flag) {
				t.Errorf("%s does not offer %s for %s:\n%s", file, flag, cmd, arm)
			}
		}
	}
}

// completionArm returns the case arm of a completion script that handles cmd:
// from the label naming it ("http)", "cli|acp)") to the ";;" that closes it.
func completionArm(t *testing.T, file, script, cmd string) string {
	t.Helper()
	lines := strings.Split(strings.ReplaceAll(script, "\r\n", "\n"), "\n")
	for i, line := range lines {
		label, ok := strings.CutSuffix(strings.TrimSpace(line), ")")
		if !ok || !slices.Contains(strings.Split(label, "|"), cmd) {
			continue
		}
		var arm []string
		for _, body := range lines[i+1:] {
			if strings.TrimSpace(body) == ";;" {
				return strings.Join(arm, "\n")
			}
			arm = append(arm, body)
		}
		t.Fatalf("%s: the %s arm is not closed by ;;", file, cmd)
	}
	t.Fatalf("%s has no case arm for %s", file, cmd)
	return ""
}

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(rel)
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

// checkFlagCommands are the subcommands that take -t / --test-config and
// --dry-run, besides the bare binary. http is the fork's own addition: it is
// the command the editor plugins start.
var checkFlagCommands = []string{"cli", "acp", "http", "serve"}

// completionFiles are the shell completions the packages install.
var completionFiles = []string{
	"../../packaging/completions/foxxycode.bash",
	"../../packaging/completions/foxxycode.zsh",
}

// TestPackagingFilesCarryTheConfigTestFlag ties -t / --test-config to the same
// three files: the flag is offered on the console, on acp, on http and on
// serve, so the completions must list it for each of them and the man page
// must explain it.
func TestPackagingFilesCarryTheConfigTestFlag(t *testing.T) {
	var buf bytes.Buffer
	printUsage(&buf)
	if !strings.Contains(buf.String(), "--test-config") {
		t.Error("usage does not mention --test-config")
	}
	assertCompletionArmsOffer(t, "--test-config")
	man := readRepoFile(t, "../../packaging/man/foxxycode.1")
	if !strings.Contains(man, `\-\-test\-config`) {
		t.Error("packaging/man/foxxycode.1 does not document --test-config")
	}
}

// TestPackagingFilesCarryTheDryRunFlag does for --dry-run what the test above
// does for --test-config.
func TestPackagingFilesCarryTheDryRunFlag(t *testing.T) {
	var buf bytes.Buffer
	printUsage(&buf)
	if !strings.Contains(buf.String(), "--dry-run") {
		t.Error("usage does not mention --dry-run")
	}
	assertCompletionArmsOffer(t, "--dry-run")
	if !strings.Contains(readRepoFile(t, "../../packaging/man/foxxycode.1"), `\-\-dry\-run`) {
		t.Error("packaging/man/foxxycode.1 does not document --dry-run")
	}
}
