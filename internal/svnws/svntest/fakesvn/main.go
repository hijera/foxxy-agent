// Command fakesvn is a stand-in Subversion client used by FoxxyCode tests. It
// answers the subset of svn commands the agent issues, keeps a small repository
// model in a JSON state file, and records every invocation so tests can assert
// on the exact command line that was built.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/svnws/svntest"
)

func main() {
	args := os.Args[1:]
	cwd, _ := os.Getwd()
	record(cwd, args)

	sub, rest := splitSubcommand(args)
	if sub == "" {
		fail("svn: E205000: Please specify a subcommand")
	}
	state, err := loadState()
	if err != nil {
		fail("svn: E200001: fake state unavailable: " + err.Error())
	}
	if msg, ok := state.Fail[sub]; ok && msg != "" {
		fail(msg)
	}

	switch sub {
	case "info":
		cmdInfo(state, cwd)
	case "status":
		requireWC(state, cwd)
		fmt.Println(state.Status)
	case "diff":
		requireWC(state, cwd)
		fmt.Println(state.Diff)
	case "log":
		requireWC(state, cwd)
		fmt.Println(state.Log)
	case "list":
		cmdList(state, rest)
	case "add":
		requireWC(state, cwd)
		for _, p := range positionals(rest) {
			fmt.Printf("A         %s\n", p)
		}
	case "revert":
		requireWC(state, cwd)
		for _, p := range positionals(rest) {
			fmt.Printf("Reverted '%s'\n", p)
		}
	case "resolve":
		requireWC(state, cwd)
		for _, p := range positionals(rest) {
			fmt.Printf("Resolved conflicted state of '%s'\n", p)
		}
	case "update":
		cmdUpdate(state, cwd)
	case "commit":
		cmdCommit(state, cwd, rest)
	case "switch":
		cmdSwitch(state, cwd, rest)
	case "checkout":
		cmdCheckout(state, cwd, rest)
	case "merge":
		cmdMerge(state, cwd, rest)
	default:
		fail(fmt.Sprintf("svn: E205000: Unknown subcommand: '%s'", sub))
	}
}

// splitSubcommand skips global flags and returns the subcommand plus its arguments.
func splitSubcommand(args []string) (string, []string) {
	for i, a := range args {
		if strings.HasPrefix(a, "-") {
			continue
		}
		return a, args[i+1:]
	}
	return "", nil
}

// positionals returns the arguments after the "--" separator, or every non-flag
// argument when no separator is present.
func positionals(args []string) []string {
	for i, a := range args {
		if a == "--" {
			return args[i+1:]
		}
	}
	var out []string
	skipValue := false
	for _, a := range args {
		if skipValue {
			skipValue = false
			continue
		}
		if strings.HasPrefix(a, "-") {
			skipValue = a == "--message" || a == "--revision" || a == "--accept" || a == "--limit"
			continue
		}
		out = append(out, a)
	}
	return out
}

func record(cwd string, args []string) {
	path := strings.TrimSpace(os.Getenv(svntest.EnvLog))
	if path == "" {
		return
	}
	buf, err := json.Marshal(svntest.Call{Dir: cwd, Args: args})
	if err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = f.Write(append(buf, '\n'))
}

func statePath() string { return strings.TrimSpace(os.Getenv(svntest.EnvState)) }

func loadState() (svntest.State, error) {
	var s svntest.State
	path := statePath()
	if path == "" {
		return s, fmt.Errorf("%s is not set", svntest.EnvState)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return s, err
	}
	if s.WorkingCopies == nil {
		s.WorkingCopies = map[string]*svntest.WC{}
	}
	return s, nil
}

func saveState(s svntest.State) {
	buf, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(statePath(), buf, 0o644)
}

// resolveWC finds the working copy covering dir: the nearest registered
// ancestor, unless dir (or a directory between them) is marked unversioned.
func resolveWC(s svntest.State, dir string) (string, *svntest.WC) {
	dir = normalize(dir)
	for _, u := range s.Unversioned {
		if underOrEqual(dir, normalize(u)) {
			return "", nil
		}
	}
	current := dir
	for {
		if wc, ok := s.WorkingCopies[current]; ok {
			return current, wc
		}
		for path, wc := range s.WorkingCopies {
			if normalize(path) == current {
				return normalize(path), wc
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", nil
		}
		current = parent
	}
}

func normalize(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		abs = p
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	return filepath.Clean(abs)
}

func underOrEqual(dir, ancestor string) bool {
	if dir == ancestor {
		return true
	}
	rel, err := filepath.Rel(ancestor, dir)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func requireWC(s svntest.State, dir string) (string, *svntest.WC) {
	root, wc := resolveWC(s, dir)
	if wc == nil {
		fail(fmt.Sprintf("svn: warning: W155007: '%s' is not a working copy", dir))
	}
	return root, wc
}

func cmdInfo(s svntest.State, cwd string) {
	root, wc := requireWC(s, cwd)
	url := s.RepositoryRoot + "/" + wc.Branch
	fmt.Printf(`<?xml version="1.0" encoding="UTF-8"?>
<info>
<entry path="." revision="%d" kind="dir">
<url>%s</url>
<relative-url>^/%s</relative-url>
<repository>
<root>%s</root>
<uuid>%s</uuid>
</repository>
<wc-info>
<wcroot-abspath>%s</wcroot-abspath>
<schedule>normal</schedule>
<depth>infinity</depth>
</wc-info>
<commit revision="%d">
<author>dev</author>
<date>2026-07-01T10:00:00.000000Z</date>
</commit>
</entry>
</info>
`, wc.Revision, url, wc.Branch, s.RepositoryRoot, s.UUID, root, wc.Revision)
}

func cmdList(s svntest.State, rest []string) {
	target := ""
	if p := positionals(rest); len(p) > 0 {
		target = p[0]
	}
	var names []string
	switch target {
	case s.RepositoryRoot:
		names = []string{"trunk", "branches", "tags"}
	case s.RepositoryRoot + "/branches":
		for _, b := range s.Branches {
			if strings.HasPrefix(b, "branches/") {
				names = append(names, strings.TrimPrefix(b, "branches/"))
			}
		}
	default:
		fail(fmt.Sprintf("svn: E170000: URL '%s' non-existent in revision", target))
	}
	fmt.Println(`<?xml version="1.0" encoding="UTF-8"?>`)
	fmt.Printf("<lists>\n<list path=\"%s\">\n", target)
	for _, n := range names {
		fmt.Printf("<entry kind=\"dir\">\n<name>%s</name>\n</entry>\n", n)
	}
	fmt.Print("</list>\n</lists>\n")
}

func cmdUpdate(s svntest.State, cwd string) {
	_, wc := requireWC(s, cwd)
	wc.Revision++
	saveState(s)
	fmt.Printf("Updating '.':\nAt revision %d.\n", wc.Revision)
}

func cmdCommit(s svntest.State, cwd string, rest []string) {
	_, wc := requireWC(s, cwd)
	paths := positionals(rest)
	if len(paths) == 0 {
		fail("svn: E205000: fake commit requires explicit paths")
	}
	wc.Revision++
	saveState(s)
	for _, p := range paths {
		fmt.Printf("Sending        %s\n", p)
	}
	fmt.Printf("Transmitting file data .\nCommitted revision %d.\n", wc.Revision)
}

func cmdSwitch(s svntest.State, cwd string, rest []string) {
	_, wc := requireWC(s, cwd)
	paths := positionals(rest)
	if len(paths) == 0 {
		fail("svn: E205000: switch requires a URL")
	}
	branch, ok := branchFromURL(s, paths[0])
	if !ok {
		fail(fmt.Sprintf("svn: E170000: URL '%s' does not exist", paths[0]))
	}
	wc.Branch = branch
	wc.Revision++
	saveState(s)
	fmt.Printf("Updated to revision %d.\n", wc.Revision)
}

func cmdCheckout(s svntest.State, cwd string, rest []string) {
	paths := positionals(rest)
	if len(paths) < 2 {
		fail("svn: E205000: checkout requires a URL and a destination")
	}
	branch, ok := branchFromURL(s, paths[0])
	if !ok {
		fail(fmt.Sprintf("svn: E170000: URL '%s' does not exist", paths[0]))
	}
	dest := paths[1]
	if !filepath.IsAbs(dest) {
		dest = filepath.Join(cwd, dest)
	}
	if err := os.MkdirAll(filepath.Join(dest, ".svn"), 0o755); err != nil {
		fail("svn: E155000: " + err.Error())
	}
	rev := 1
	for _, wc := range s.WorkingCopies {
		if wc.Revision > rev {
			rev = wc.Revision
		}
	}
	s.WorkingCopies[normalize(dest)] = &svntest.WC{Branch: branch, Revision: rev}
	saveState(s)
	fmt.Printf("Checked out revision %d.\n", rev)
}

func cmdMerge(s svntest.State, cwd string, rest []string) {
	requireWC(s, cwd)
	paths := positionals(rest)
	if len(paths) == 0 {
		fail("svn: E205000: merge requires a source")
	}
	fmt.Printf("--- Merging differences between repository URLs into '.':\nU    src/main.go\n--- Recording mergeinfo for merge of %s into '.':\n U   .\n", paths[0])
}

func branchFromURL(s svntest.State, url string) (string, bool) {
	prefix := s.RepositoryRoot + "/"
	if !strings.HasPrefix(url, prefix) {
		return "", false
	}
	branch := strings.Trim(strings.TrimPrefix(url, prefix), "/")
	for _, b := range s.Branches {
		if b == branch {
			return branch, true
		}
	}
	return "", false
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}
