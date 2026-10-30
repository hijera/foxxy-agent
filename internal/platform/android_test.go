//go:build !windows

// Android paths, modes and symlinks are POSIX ones: these tests hold on every
// host but Windows, where a file has no execute bit and a symlink needs a
// privilege the runner does not have.

package platform

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestIsSystemLinker(t *testing.T) {
	for path, want := range map[string]bool{
		"/system/bin/linker64":                         true,
		"/system/bin/linker":                           true,
		"/apex/com.android.runtime/bin/linker64":       true,
		"/apex/com.android.runtime/bin/linker":         true,
		"/data/data/com.termux/files/usr/bin/linker64": false,
		"/system/bin/sh":                               false,
		"/usr/bin/foxxycode":                           false,
		"":                                             false,
	} {
		if got := isSystemLinker(path); got != want {
			t.Errorf("isSystemLinker(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestLinkerSelf(t *testing.T) {
	const bin = "/data/data/com.termux/files/home/.local/bin/foxxycode"
	const linker = "/apex/com.android.runtime/bin/linker64"
	cases := []struct {
		name string
		exe  string
		args []string
		self string
		ok   bool
	}{
		{name: "started by the kernel", exe: bin, args: []string{"foxxycode", "-v"}},
		{name: "started through the linker by termux-exec", exe: linker, args: []string{bin, "-v"}, self: bin, ok: true},
		{name: "started by hand as linker64 ./foxxycode", exe: "/system/bin/linker64", args: []string{"./foxxycode"}, self: "/data/data/com.termux/files/home/foxxycode", ok: true},
		{name: "no arguments at all", exe: linker, args: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			self, ok := linkerSelf(tc.exe, tc.args, "/data/data/com.termux/files/home")
			if ok != tc.ok || self != tc.self {
				t.Fatalf("linkerSelf = %q, %v; want %q, %v", self, ok, tc.self, tc.ok)
			}
		})
	}
}

func TestTermuxPrefix(t *testing.T) {
	env := func(kv map[string]string) func(string) string {
		return func(name string) string { return kv[name] }
	}
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"TERMUX__PREFIX first", map[string]string{"TERMUX__PREFIX": "/data/user/0/com.termux/files/usr", "PREFIX": "/elsewhere"}, "/data/user/0/com.termux/files/usr"},
		{"then PREFIX", map[string]string{"PREFIX": "/data/data/com.termux.nightly/files/usr/"}, "/data/data/com.termux.nightly/files/usr"},
		{"a relative value is no prefix", map[string]string{"PREFIX": "usr"}, termuxDefaultPrefix},
		{"the app's default", nil, termuxDefaultPrefix},
	}
	for _, tc := range cases {
		if got := termuxPrefix(env(tc.env)); got != tc.want {
			t.Errorf("%s: termuxPrefix = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestTermuxAppDataDirs(t *testing.T) {
	env := map[string]string{
		"TERMUX_APP__DATA_DIR":        "/data/user/0/com.termux",
		"TERMUX_APP__LEGACY_DATA_DIR": "/data/data/com.termux/",
	}
	got := termuxAppDataDirs(func(name string) string { return env[name] }, termuxDefaultPrefix)
	want := []string{"/data/user/0/com.termux", "/data/data/com.termux"}
	if !slices.Equal(got, want) {
		t.Fatalf("termuxAppDataDirs = %q, want %q", got, want)
	}
	// Termux before 0.119.0 exports neither: the prefix names the directory.
	got = termuxAppDataDirs(func(string) string { return "" }, "/data/data/com.termux.nightly/files/usr")
	if !slices.Equal(got, []string{"/data/data/com.termux.nightly"}) {
		t.Fatalf("termuxAppDataDirs without the variables = %q", got)
	}
}

func TestUseTermuxFiles(t *testing.T) {
	prefix := t.TempDir()
	for _, dir := range []string{"etc/tls", "tmp"} {
		if err := os.MkdirAll(filepath.Join(prefix, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(prefix, "etc", "tls", "cert.pem"), []byte("bundle"), 0o644); err != nil {
		t.Fatal(err)
	}

	set := map[string]string{}
	useTermuxFiles(prefix, func(name string) string { return set[name] }, func(name, value string) error {
		set[name] = value
		return nil
	})
	if set["SSL_CERT_FILE"] != filepath.Join(prefix, "etc", "tls", "cert.pem") || set["TMPDIR"] != filepath.Join(prefix, "tmp") {
		t.Fatalf("set = %v", set)
	}

	// What the user exported stays.
	set = map[string]string{"SSL_CERT_FILE": "/sdcard/corp.pem", "TMPDIR": "/data/local/tmp"}
	useTermuxFiles(prefix, func(name string) string { return set[name] }, func(name, value string) error {
		t.Fatalf("overwrote %s with %s", name, value)
		return nil
	})

	// A prefix without the files sets nothing.
	useTermuxFiles(t.TempDir(), func(string) string { return "" }, func(name, value string) error {
		t.Fatalf("set %s=%s for files that are not there", name, value)
		return nil
	})
}

// termuxHost builds a Termux data directory with its prefix in a temporary
// directory and returns the host that runs programs out of it.
func termuxHost(t *testing.T, linkerExec bool) androidHost {
	t.Helper()
	root := t.TempDir()
	prefix := filepath.Join(root, "files", "usr")
	if err := os.MkdirAll(filepath.Join(prefix, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	return androidHost{prefix: prefix, dataDirs: []string{root}, linkerExec: linkerExec}
}

func writeProgram(t *testing.T, path string, body []byte, mode os.FileMode) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, mode); err != nil {
		t.Fatal(err)
	}
	return path
}

var (
	elf64 = append([]byte("\x7fELF\x02\x01\x01"), make([]byte, 57)...)
	elf32 = append([]byte("\x7fELF\x01\x01\x01"), make([]byte, 45)...)
)

func adapted(h androidHost, env []string, path string, args ...string) *exec.Cmd {
	cmd := exec.Command(path, args...)
	cmd.Env = env
	h.adapt(cmd)
	return cmd
}

func TestAdaptLeavesAProgramAndroidRunsAsIs(t *testing.T) {
	h := termuxHost(t, false)
	git := writeProgram(t, filepath.Join(h.prefix, "bin", "git"), elf64, 0o755)
	env := []string{"HOME=/data/data/com.termux/files/home"}
	cmd := adapted(h, env, git, "status")
	if cmd.Path != git || !slices.Equal(cmd.Args, []string{git, "status"}) || !slices.Equal(cmd.Env, env) {
		t.Fatalf("command = %s %q env %q, want it untouched", cmd.Path, cmd.Args, cmd.Env)
	}
}

func TestAdaptRunsA32BitProgramThroughTheMatchingLinker(t *testing.T) {
	h := termuxHost(t, true)
	tool := writeProgram(t, filepath.Join(h.prefix, "bin", "tool32"), elf32, 0o755)
	cmd := adapted(h, nil, tool)
	if cmd.Path != androidLinker32 || !slices.Equal(cmd.Args, []string{tool, tool}) {
		t.Fatalf("command = %s %q, want %s", cmd.Path, cmd.Args, androidLinker32)
	}
}

func TestAdaptStartsASystemProgramDirectlyWithoutFoxxyCodesPath(t *testing.T) {
	h := termuxHost(t, true)
	// Outside the data directory stands for /system/bin: exec is allowed there.
	getprop := writeProgram(t, filepath.Join(t.TempDir(), "system", "bin", "getprop"), elf64, 0o755)
	cmd := adapted(h, []string{envTermuxProcSelfExe + "=/data/data/com.termux/files/usr/bin/foxxycode", "A=b"}, getprop, "ro.build.version.sdk")
	if cmd.Path != getprop || !slices.Equal(cmd.Args, []string{getprop, "ro.build.version.sdk"}) {
		t.Fatalf("command = %s %q, want it started directly", cmd.Path, cmd.Args)
	}
	if !slices.Equal(cmd.Env, []string{"A=b"}) {
		t.Fatalf("env = %q, want FoxxyCode's own %s dropped", cmd.Env, envTermuxProcSelfExe)
	}
}

func TestAdaptRunsAScriptThroughItsInterpreterUnderTheLinker(t *testing.T) {
	h := termuxHost(t, true)
	sh := writeProgram(t, filepath.Join(h.prefix, "bin", "sh"), elf64, 0o755)
	script := writeProgram(t, filepath.Join(h.dataDirs[0], "files", "home", "hook.sh"), []byte("#!/bin/sh -e\necho hook\n"), 0o755)
	cmd := adapted(h, nil, script, "PreToolUse")
	want := []string{"/bin/sh", sh, "-e", script, "PreToolUse"}
	if cmd.Path != androidLinker64 || !slices.Equal(cmd.Args, want) {
		t.Fatalf("command = %s %q, want %s %q", cmd.Path, cmd.Args, androidLinker64, want)
	}
	if !slices.Contains(cmd.Env, envTermuxProcSelfExe+"="+script) {
		t.Fatalf("env does not name the script as the program: %q", cmd.Env)
	}
}

func TestAdaptRunsAScriptWithASystemInterpreterDirectlyUnderTheLinker(t *testing.T) {
	h := termuxHost(t, true)
	systemSh := writeProgram(t, filepath.Join(t.TempDir(), "system", "bin", "sh"), elf64, 0o755)
	script := writeProgram(t, filepath.Join(h.dataDirs[0], "files", "home", "boot.sh"), []byte("#!"+systemSh+"\necho boot\n"), 0o755)
	cmd := adapted(h, nil, script)
	// Android refuses to execute the script file itself, so its interpreter
	// is started with the script as an argument.
	if cmd.Path != systemSh || !slices.Equal(cmd.Args, []string{systemSh, script}) {
		t.Fatalf("command = %s %q, want %s %q", cmd.Path, cmd.Args, systemSh, []string{systemSh, script})
	}
}

func TestAdaptLeavesAScriptWithAWorkingShebangToTheKernel(t *testing.T) {
	h := termuxHost(t, false)
	writeProgram(t, filepath.Join(h.prefix, "bin", "python3"), elf64, 0o755)
	script := writeProgram(t, filepath.Join(h.prefix, "bin", "pip"), []byte("#!"+filepath.Join(h.prefix, "bin", "python3")+"\nimport pip\n"), 0o755)
	cmd := adapted(h, nil, script, "list")
	if cmd.Path != script || !slices.Equal(cmd.Args, []string{script, "list"}) {
		t.Fatalf("command = %s %q, want it untouched", cmd.Path, cmd.Args)
	}
}

func TestAdaptFindsALinuxPathInThePrefix(t *testing.T) {
	h := termuxHost(t, false)
	python := writeProgram(t, filepath.Join(h.prefix, "bin", "python3"), elf64, 0o755)
	cmd := adapted(h, nil, "/usr/bin/python3", "-m", "server")
	if cmd.Path != python || !slices.Equal(cmd.Args, []string{"/usr/bin/python3", "-m", "server"}) {
		t.Fatalf("command = %s %q, want %s with argv[0] kept", cmd.Path, cmd.Args, python)
	}
}

func TestAdaptResolvesARelativePathAgainstDir(t *testing.T) {
	h := termuxHost(t, true)
	dir := filepath.Join(h.dataDirs[0], "files", "home", "project")
	tool := writeProgram(t, filepath.Join(dir, "bin", "tool"), elf64, 0o755)
	cmd := exec.Command("./bin/tool")
	cmd.Dir = dir
	h.adapt(cmd)
	if cmd.Path != androidLinker64 || !slices.Equal(cmd.Args, []string{"./bin/tool", tool}) {
		t.Fatalf("command = %s %q, want the linker on %s", cmd.Path, cmd.Args, tool)
	}
}

func TestAdaptLeavesWhatItCannotRunToStart(t *testing.T) {
	h := termuxHost(t, true)
	home := filepath.Join(h.dataDirs[0], "files", "home")
	dir := filepath.Join(home, "dir")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"missing":        filepath.Join(home, "missing"),
		"directory":      dir,
		"not executable": writeProgram(t, filepath.Join(home, "notes.txt"), elf64, 0o644),
		"no format":      writeProgram(t, filepath.Join(home, "plain.sh"), []byte("echo no shebang\n"), 0o755),
		"endless line":   writeProgram(t, filepath.Join(home, "long.sh"), []byte("#!/bin/sh"+strings.Repeat(" x", 200)), 0o755),
	}
	for name, path := range cases {
		cmd := adapted(h, []string{"A=b"}, path)
		if cmd.Path != path || !slices.Equal(cmd.Args, []string{path}) {
			t.Errorf("%s: command = %s %q, want it left for Start to report", name, cmd.Path, cmd.Args)
		}
	}

	failed := exec.Command("foxxycode-no-such-program-anywhere")
	before := failed.Path
	h.adapt(failed)
	if failed.Err == nil || failed.Path != before {
		t.Fatalf("a lookup error must survive: %s %v", failed.Path, failed.Err)
	}
}

func TestAdaptCommandIsANoOpOffAndroid(t *testing.T) {
	if runtime.GOOS == "android" {
		t.Skip("this host is Android")
	}
	cmd := exec.Command("/usr/bin/env", "true")
	AdaptCommand(cmd)
	if cmd.Path != "/usr/bin/env" || !slices.Equal(cmd.Args, []string{"/usr/bin/env", "true"}) || cmd.Env != nil {
		t.Fatalf("command = %s %q env %q, want it untouched", cmd.Path, cmd.Args, cmd.Env)
	}
	AdaptCommand(nil)
}

func TestExecutableIsTheBinaryTheLinkerRan(t *testing.T) {
	saved := androidSelf
	t.Cleanup(func() { androidSelf = saved })

	androidSelf = ""
	want, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if got, err := Executable(); err != nil || got != want {
		t.Fatalf("Executable() = %q, %v; want os.Executable %q", got, err, want)
	}

	androidSelf = "/data/data/com.termux/files/home/.local/bin/foxxycode"
	if got, err := Executable(); err != nil || got != androidSelf {
		t.Fatalf("Executable() = %q, %v; want %q", got, err, androidSelf)
	}
}
