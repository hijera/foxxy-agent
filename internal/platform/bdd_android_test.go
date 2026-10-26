//go:build !windows

// The scenarios lay a Termux tree out with execute bits and symlinks, which a
// Windows host cannot reproduce; see android_test.go.

package platform

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

// androidFeatureState is one scenario of features/android_termux.feature. The
// Termux tree lives in a temporary directory laid out like the app's data
// directory, so the decisions the scenarios check are the ones FoxxyCode takes on
// a device, made on whatever host runs the tests.
type androidFeatureState struct {
	root string // stands for /data/data/com.termux
	host androidHost

	kernelExe string   // what /proc/self/exe names
	args      []string // os.Args as Bionic passed them to main
	self      string   // the path FoxxyCode found for its own binary

	cmd    *exec.Cmd
	script string // the script the scenario installed
}

const androidFeatureCommand = "echo from bash"

func (s *androidFeatureState) reset() {
	if s.root != "" {
		_ = os.RemoveAll(s.root)
	}
	*s = androidFeatureState{}
}

// termuxTree creates the app data directory with an empty prefix and home.
func (s *androidFeatureState) termuxTree() error {
	if s.root != "" {
		return nil
	}
	root, err := os.MkdirTemp("", "foxxycode-termux-*")
	if err != nil {
		return err
	}
	s.root = root
	prefix := filepath.Join(root, "files", "usr")
	for _, dir := range []string{"bin", "etc", "lib", "tmp"} {
		if err := os.MkdirAll(filepath.Join(prefix, dir), 0o755); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "files", "home", ".local", "bin"), 0o755); err != nil {
		return err
	}
	s.host = androidHost{prefix: prefix, dataDirs: []string{root}}
	return nil
}

func (s *androidFeatureState) foxxycodePath() string {
	return filepath.Join(s.root, "files", "home", ".local", "bin", "foxxycode")
}

func (s *androidFeatureState) prefixBin(name string) string {
	return filepath.Join(s.host.prefix, "bin", name)
}

func (s *androidFeatureState) linkerStartedFoxxyCodeWith(args string) error {
	if err := s.termuxTree(); err != nil {
		return err
	}
	// termux-exec runs the linker on the program's path, and Bionic starts
	// main with the arguments after the linker's own.
	s.kernelExe = "/apex/com.android.runtime/bin/linker64"
	s.args = append([]string{s.foxxycodePath()}, strings.Fields(args)...)
	return nil
}

func (s *androidFeatureState) foxxycodeLooksForItsBinary() error {
	self, ok := linkerSelf(s.kernelExe, s.args, filepath.Join(s.root, "files", "home"))
	if !ok {
		return fmt.Errorf("a launch through %s was not recognised", s.kernelExe)
	}
	s.self = self
	return nil
}

func (s *androidFeatureState) itKeepsItsArguments(want string) error {
	if got := s.args[1:]; !slices.Equal(got, strings.Fields(want)) {
		return fmt.Errorf("arguments = %q, want %q", got, strings.Fields(want))
	}
	return nil
}

func (s *androidFeatureState) itFindsTheBinaryNotTheLinker() error {
	if s.self != s.foxxycodePath() {
		return fmt.Errorf("own path = %q, want %q", s.self, s.foxxycodePath())
	}
	return nil
}

func (s *androidFeatureState) linkerStartedFoxxyCode() error {
	if err := s.termuxTree(); err != nil {
		return err
	}
	s.host.linkerExec = true
	return nil
}

func (s *androidFeatureState) kernelStartedFoxxyCode() error {
	return s.termuxTree()
}

// androidELF writes the start of a 64-bit ELF: all a decision reads is the
// magic and the class.
func androidELF(path string) error {
	header := append([]byte("\x7fELF\x02\x01\x01"), make([]byte, 57)...)
	return os.WriteFile(path, header, 0o755)
}

func (s *androidFeatureState) bashIsInstalled() error {
	return androidELF(s.prefixBin("bash"))
}

// npxIsInstalled lays npx out the way the nodejs package does: a symlink in
// bin to a script under lib whose shebang names the Linux path of env.
func (s *androidFeatureState) npxIsInstalled(shebang string) error {
	if err := androidELF(s.prefixBin("env")); err != nil {
		return err
	}
	script := filepath.Join(s.host.prefix, "lib", "node_modules", "npm", "bin", "npx-cli.js")
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(script, []byte(shebang+"\nrequire('../lib/cli.js')(process)\n"), 0o755); err != nil {
		return err
	}
	s.script = s.prefixBin("npx")
	return os.Symlink(filepath.Join("..", "lib", "node_modules", "npm", "bin", "npx-cli.js"), s.script)
}

// start builds the command the way the spawn sites do and adapts it. The
// environment carries the path termux-exec gave FoxxyCode itself, which a child
// must not inherit.
func (s *androidFeatureState) start(path string, args ...string) {
	s.cmd = exec.Command(path, args...)
	s.cmd.Env = []string{"HOME=" + filepath.Join(s.root, "files", "home"), envTermuxProcSelfExe + "=" + s.foxxycodePath()}
	s.host.adapt(s.cmd)
}

func (s *androidFeatureState) foxxycodeStartsBash() error {
	s.start(s.prefixBin("bash"), "-c", androidFeatureCommand)
	return nil
}

func (s *androidFeatureState) foxxycodeStartsNpx() error {
	s.start(s.script, "-y", "@modelcontextprotocol/server-filesystem")
	return nil
}

func (s *androidFeatureState) runsTheLinkerOnBash() error {
	want := []string{s.prefixBin("bash"), s.prefixBin("bash"), "-c", androidFeatureCommand}
	if s.cmd.Path != androidLinker64 || !slices.Equal(s.cmd.Args, want) {
		return fmt.Errorf("command = %s %q, want %s %q", s.cmd.Path, s.cmd.Args, androidLinker64, want)
	}
	return nil
}

func (s *androidFeatureState) bashLearnsItsPath() error {
	return s.childSelfIs(s.prefixBin("bash"))
}

func (s *androidFeatureState) childSelfIs(want string) error {
	var got []string
	for _, kv := range s.cmd.Env {
		if name, value, ok := strings.Cut(kv, "="); ok && name == envTermuxProcSelfExe {
			got = append(got, value)
		}
	}
	if len(got) != 1 || got[0] != want {
		return fmt.Errorf("%s in the child's environment = %q, want only %q", envTermuxProcSelfExe, got, want)
	}
	return nil
}

func (s *androidFeatureState) runsTheLinkerOnEnv() error {
	if s.cmd.Path != androidLinker64 || len(s.cmd.Args) < 2 || s.cmd.Args[0] != "/usr/bin/env" || s.cmd.Args[1] != s.prefixBin("env") {
		return fmt.Errorf("command = %s %q, want %s /usr/bin/env %s ...", s.cmd.Path, s.cmd.Args, androidLinker64, s.prefixBin("env"))
	}
	return nil
}

func (s *androidFeatureState) runsEnvDirectly() error {
	if s.cmd.Path != s.prefixBin("env") || len(s.cmd.Args) < 1 || s.cmd.Args[0] != "/usr/bin/env" {
		return fmt.Errorf("command = %s %q, want %s with argv[0] /usr/bin/env", s.cmd.Path, s.cmd.Args, s.prefixBin("env"))
	}
	return s.childSelfIsAbsent()
}

func (s *androidFeatureState) childSelfIsAbsent() error {
	for _, kv := range s.cmd.Env {
		if strings.HasPrefix(kv, envTermuxProcSelfExe+"=") {
			return fmt.Errorf("the child inherits %q", kv)
		}
	}
	return nil
}

func (s *androidFeatureState) envReceivesNodeAndTheScript() error {
	want := []string{"node", s.script, "-y", "@modelcontextprotocol/server-filesystem"}
	i := slices.Index(s.cmd.Args, "node")
	if i < 0 || !slices.Equal(s.cmd.Args[i:], want) {
		return fmt.Errorf("arguments = %q, want them to end with %q", s.cmd.Args, want)
	}
	return nil
}

func TestAndroidTermuxFeature(t *testing.T) {
	s := &androidFeatureState{}
	t.Cleanup(s.reset)

	suite := godog.TestSuite{
		ScenarioInitializer: func(sc *godog.ScenarioContext) {
			sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
				s.reset()
				return ctx, nil
			})
			sc.Step(`^the system linker started FoxxyCode with the arguments "([^"]*)"$`, s.linkerStartedFoxxyCodeWith)
			sc.Step(`^FoxxyCode looks for its own binary$`, s.foxxycodeLooksForItsBinary)
			sc.Step(`^it finds the binary the linker was given rather than the linker$`, s.itFindsTheBinaryNotTheLinker)
			sc.Step(`^its arguments are still "([^"]*)"$`, s.itKeepsItsArguments)
			sc.Step(`^the system linker started FoxxyCode$`, s.linkerStartedFoxxyCode)
			sc.Step(`^the kernel started FoxxyCode directly$`, s.kernelStartedFoxxyCode)
			sc.Step(`^bash is installed in the Termux prefix$`, s.bashIsInstalled)
			sc.Step(`^npx is installed in the Termux prefix with the shebang "([^"]*)"$`, s.npxIsInstalled)
			sc.Step(`^FoxxyCode starts bash for a command$`, s.foxxycodeStartsBash)
			sc.Step(`^FoxxyCode starts npx for an MCP server$`, s.foxxycodeStartsNpx)
			sc.Step(`^the command runs the system linker on bash$`, s.runsTheLinkerOnBash)
			sc.Step(`^bash learns its own path from TERMUX_EXEC__PROC_SELF_EXE$`, s.bashLearnsItsPath)
			sc.Step(`^the command runs the system linker on the env of the Termux prefix$`, s.runsTheLinkerOnEnv)
			sc.Step(`^the command runs the env of the Termux prefix directly$`, s.runsEnvDirectly)
			sc.Step(`^env receives node and the path of the script$`, s.envReceivesNodeAndTheScript)
		},
		Options: &godog.Options{
			Format: "progress",
			Paths:  []string{"../../features/android_termux.feature"},
		},
	}
	if suite.Run() != 0 {
		t.Fatal("android termux feature failed")
	}
}
