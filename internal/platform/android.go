package platform

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
)

// Android (Termux) support.
//
// The Android release is this module built with GOOS=android and cgo, linked
// by the NDK against Bionic, for arm64 and x86_64: a position-independent
// executable naming Android's system linker as its interpreter. That is the
// one shape Termux can start where the app may not execute the files of its
// own data directory - Android 10 and later, for a Termux that targets them,
// such as the Google Play build. There termux-exec runs every program as
// `/system/bin/linker64 <path> args`, and the linker refuses the static Linux
// build with `has unexpected e_type: 2`. Bionic gives such a program what libc
// gives any: its own arguments and the system's resolver.
//
// Termux patches its own Go packages for the rest, and a binary built outside
// Termux carries none of those patches, so this file makes the same
// adjustments at run time:
//
//   - Go starts a program with the execve system call, which termux-exec
//     cannot intercept. AdaptCommand does what it would have done.
//   - Started through the linker, /proc/self/exe names the linker; the
//     android init keeps the path of the binary for Executable.
//   - Root certificates and the temporary directory are looked up where
//     Termux keeps them (useTermuxFiles).

const (
	// termuxDefaultPrefix is where the Termux app keeps its packages when the
	// environment names no other prefix.
	termuxDefaultPrefix = "/data/data/com.termux/files/usr"

	// envTermuxProcSelfExe is the path of the program a process was started
	// for, set by termux-exec (and by AdaptCommand) when /proc/self/exe names
	// the system linker instead.
	envTermuxProcSelfExe = "TERMUX_EXEC__PROC_SELF_EXE"

	androidLinker64 = "/system/bin/linker64"
	androidLinker32 = "/system/bin/linker"

	// shebangMax is how much of a file the kernel reads for its #! line.
	shebangMax = 256
)

// Set by the android init when the system linker started this process.
var (
	androidSelf       string // the path of the running binary
	androidLinkerExec bool   // programs in the app data directory go through the linker too
)

var (
	androidHostOnce  sync.Once
	androidHostValue androidHost
)

// Executable returns the path of the running binary: os.Executable, except
// on Android when the system linker started this process, since /proc/self/exe
// names the linker then.
func Executable() (string, error) {
	if androidSelf != "" {
		return androidSelf, nil
	}
	return os.Executable()
}

// AdaptCommand fits cmd to the way this host starts programs. Every place that
// builds an exec.Cmd calls it once Path, Args, Dir and Env are final and
// before Start. It does nothing anywhere but Android.
//
// On Android it does what termux-exec does for Termux's own programs:
//   - a program or a script interpreter named by its Linux path, /bin/... or
//     /usr/bin/..., is taken from the Termux prefix, so a script starting
//     with #!/usr/bin/env runs;
//   - when this process was started through the system linker, a program in
//     the app data directory is started that way too, and a script there
//     through its interpreter, because Android refuses to execute either
//     directly for this app. The child finds its own path in
//     TERMUX_EXEC__PROC_SELF_EXE.
func AdaptCommand(cmd *exec.Cmd) {
	if runtime.GOOS != "android" || cmd == nil {
		return
	}
	currentAndroidHost().adapt(cmd)
}

// androidHost is what AdaptCommand needs to know about the device.
type androidHost struct {
	prefix     string   // the Termux prefix
	dataDirs   []string // the app data directory, under both its names
	linkerExec bool     // this process was started through the system linker
}

func currentAndroidHost() androidHost {
	androidHostOnce.Do(func() {
		prefix := termuxPrefix(os.Getenv)
		androidHostValue = androidHost{
			prefix:     prefix,
			dataDirs:   termuxAppDataDirs(os.Getenv, prefix),
			linkerExec: androidLinkerExec,
		}
	})
	return androidHostValue
}

// termuxPrefix returns the Termux prefix: TERMUX__PREFIX, which the app
// exports from 0.119.0 on, then PREFIX, then the app's default.
func termuxPrefix(getenv func(string) string) string {
	for _, name := range []string{"TERMUX__PREFIX", "PREFIX"} {
		if v := getenv(name); filepath.IsAbs(v) {
			return filepath.Clean(v)
		}
	}
	return termuxDefaultPrefix
}

// termuxAppDataDirs returns the app data directory under the names Termux
// exports (from 0.119.0 on), and the one the prefix sits in:
// <data dir>/files/usr. These are the directories termux-exec starts programs
// from through the linker.
func termuxAppDataDirs(getenv func(string) string, prefix string) []string {
	var dirs []string
	add := func(dir string) {
		if !filepath.IsAbs(dir) {
			return
		}
		dir = filepath.Clean(dir)
		if !slices.Contains(dirs, dir) {
			dirs = append(dirs, dir)
		}
	}
	add(getenv("TERMUX_APP__DATA_DIR"))
	add(getenv("TERMUX_APP__LEGACY_DATA_DIR"))
	if dir, ok := strings.CutSuffix(prefix, "/files/usr"); ok {
		add(dir)
	}
	return dirs
}

// adapt rewrites cmd for the device; see AdaptCommand. A command it cannot
// read - a lookup that failed, a missing or unreadable file, no ELF and no
// shebang - is left for Start to report as it is.
func (h androidHost) adapt(cmd *exec.Cmd) {
	if cmd.Err != nil || cmd.Path == "" {
		return
	}
	argv := cmd.Args
	if len(argv) == 0 {
		argv = []string{cmd.Path}
	}
	program := h.hostPath(absolutePath(cmd.Path, cmd.Dir))
	header, ok := readExecutableHeader(program)
	if !ok {
		return
	}

	if bytes.HasPrefix(header, []byte("\x7fELF")) {
		if h.linkerExec && h.inDataDir(program) {
			cmd.Path = linkerFor(header)
			cmd.Args = append([]string{argv[0], program}, argv[1:]...)
			setChildSelf(cmd, program)
			return
		}
		cmd.Path = program
		setChildSelf(cmd, "")
		return
	}

	shebang, ok := parseShebang(header)
	if !ok {
		return
	}
	interpreter := h.hostPath(absolutePath(shebang.interpreter, cmd.Dir))
	tail := []string{}
	if shebang.arg != "" {
		tail = append(tail, shebang.arg)
	}
	tail = append(append(tail, program), argv[1:]...)

	switch {
	case h.linkerExec && h.inDataDir(interpreter):
		interpreterHeader, ok := readExecutableHeader(interpreter)
		if !ok || !bytes.HasPrefix(interpreterHeader, []byte("\x7fELF")) {
			return
		}
		cmd.Path = linkerFor(interpreterHeader)
		cmd.Args = append([]string{shebang.interpreter, interpreter}, tail...)
		setChildSelf(cmd, program)
	case (h.linkerExec && h.inDataDir(program)) || interpreter != shebang.interpreter:
		// Android refuses to execute the script file itself, or the kernel
		// would not find the interpreter it names: start the interpreter.
		cmd.Path = interpreter
		cmd.Args = append([]string{shebang.interpreter}, tail...)
		setChildSelf(cmd, "")
	default:
		cmd.Path = program
		setChildSelf(cmd, "")
	}
}

// hostPath maps a program named by its Linux path, /bin/... or /usr/bin/..., to
// the Termux build of it in the prefix, when there is one.
func (h androidHost) hostPath(path string) string {
	for _, dir := range []string{"/usr/bin/", "/bin/"} {
		rest, ok := strings.CutPrefix(path, dir)
		if !ok || rest == "" {
			continue
		}
		if candidate := filepath.Join(h.prefix, "bin", rest); isRegularFile(candidate) {
			return candidate
		}
		return path
	}
	return path
}

func (h androidHost) inDataDir(path string) bool {
	for _, dir := range h.dataDirs {
		if path == dir || strings.HasPrefix(path, dir+"/") {
			return true
		}
	}
	return false
}

// linkerFor picks the linker for the class of an ELF header: a 32-bit program
// on a 64-bit device needs the 32-bit linker.
func linkerFor(header []byte) string {
	if len(header) > 4 && header[4] == 1 {
		return androidLinker32
	}
	return androidLinker64
}

// setChildSelf names the child's own path in its environment when the linker
// starts it, and otherwise drops the variable, which would still name
// whatever program this process was started for.
func setChildSelf(cmd *exec.Cmd, self string) {
	env := cmd.Env
	if env == nil {
		if self == "" {
			if _, set := os.LookupEnv(envTermuxProcSelfExe); !set {
				return
			}
		}
		env = cmd.Environ()
	}
	kept := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if !strings.HasPrefix(kv, envTermuxProcSelfExe+"=") {
			kept = append(kept, kv)
		}
	}
	if self != "" {
		kept = append(kept, envTermuxProcSelfExe+"="+self)
	}
	if len(kept) == len(env) && self == "" {
		return
	}
	cmd.Env = kept
}

type shebangLine struct {
	interpreter string // as the script names it
	arg         string // the optional single argument after it
}

// parseShebang reads the #! line of a script the way Linux does: the
// interpreter, then everything up to the end of the line as one argument.
func parseShebang(header []byte) (shebangLine, bool) {
	rest, ok := bytes.CutPrefix(header, []byte("#!"))
	if !ok {
		return shebangLine{}, false
	}
	line, _, found := bytes.Cut(rest, []byte("\n"))
	if !found {
		return shebangLine{}, false
	}
	text := strings.TrimSpace(strings.TrimSuffix(string(line), "\r"))
	interpreter, arg, _ := strings.Cut(text, " ")
	if interpreter == "" {
		return shebangLine{}, false
	}
	return shebangLine{interpreter: interpreter, arg: strings.TrimSpace(arg)}, true
}

// readExecutableHeader returns the first bytes of a regular file someone may
// execute, or false for anything Start should be left to report.
func readExecutableHeader(path string) ([]byte, bool) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return nil, false
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer func() { _ = f.Close() }()
	header := make([]byte, shebangMax)
	n, err := io.ReadFull(f, header)
	if err != nil && err != io.ErrUnexpectedEOF {
		return nil, false
	}
	return header[:n], true
}

// absolutePath resolves a relative program path the way the child will: from
// its working directory, dir, or this process's when dir is empty.
func absolutePath(path, dir string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	base := dir
	if !filepath.IsAbs(base) {
		cwd, err := os.Getwd()
		if err != nil {
			return path
		}
		base = filepath.Join(cwd, base)
	}
	return filepath.Join(base, path)
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
