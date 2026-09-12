//go:build miniapps

package miniapps

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/cmdprofile"
	"github.com/hijera/foxxycode-agent/internal/cmdprofile/cmdtest"
)

// fakeManager builds a recording binary named like this platform's package
// manager and puts it on PATH, so DetectManagers finds it.
func fakeManager(t *testing.T) (cmdtest.Fake, string, cmdprofile.InstallSpec) {
	t.Helper()
	var binary, managerID string
	var install cmdprofile.InstallSpec
	switch runtime.GOOS {
	case "windows":
		binary, managerID = "scoop", "scoop"
		install = cmdprofile.InstallSpec{Scoop: "fakeenc"}
	case "darwin":
		binary, managerID = "brew", "brew"
		install = cmdprofile.InstallSpec{Brew: "fakeenc"}
	default:
		binary, managerID = "apt-get", "apt"
		install = cmdprofile.InstallSpec{Apt: "fakeenc"}
	}
	fake, err := cmdtest.Build(t.TempDir(), binary)
	if err != nil {
		t.Fatal(err)
	}
	fake.Setenv(t.Setenv)
	t.Setenv("PATH", filepath.Dir(fake.Binary)+string(os.PathListSeparator)+os.Getenv("PATH"))
	return fake, managerID, install
}

func TestServiceCommandInstallRunsTheDetectedManager(t *testing.T) {
	fake, managerID, install := fakeManager(t)
	profile := cmdprofile.ProfileSpec{
		Name: "fakeenc_convert", Binary: "fakeenc", Permission: "allow",
		Template: []string{"-i", "{input_path}", "{output_path}"},
		Params: []cmdprofile.ParamSpec{
			{Name: "input_path", Type: cmdprofile.ParamFile},
			{Name: "output_path", Type: cmdprofile.ParamFile},
		},
		Install: install,
	}
	managers := cmdprofile.DetectManagers(profile)
	if len(managers) == 0 {
		t.Fatalf("no manager detected (id %s)", managerID)
	}

	store := NewStore(t.TempDir())
	service := NewService(store, NewRunner(store, Executors{}))
	defer service.Close()

	job, err := service.StartCommandInstall(profile, managers[0])
	if err != nil {
		t.Fatalf("StartCommandInstall() = %v", err)
	}
	if job.Kind != JobCommandInstall {
		t.Fatalf("job kind = %q", job.Kind)
	}
	done := waitForJob(t, service, job.ID, func(item AsyncJob) bool { return item.Status.terminal() })
	if done.Status != JobSucceeded {
		t.Fatalf("install job = %+v", done)
	}
	calls, err := fake.Calls()
	if err != nil || len(calls) != 1 {
		t.Fatalf("manager calls = %#v, err %v", calls, err)
	}
	joined := strings.Join(calls[0].Args, " ")
	if !strings.Contains(joined, "install") || !strings.Contains(joined, "fakeenc") {
		t.Fatalf("manager argv = %#v", calls[0].Args)
	}
	if done.Result["manager"] != managers[0].ID {
		t.Fatalf("result = %#v", done.Result)
	}
}

func TestServiceCommandInstallSurfacesManagerFailure(t *testing.T) {
	_, managerID, install := fakeManager(t)
	t.Setenv(cmdtest.EnvExit, "1")
	t.Setenv(cmdtest.EnvStderr, "no bucket for fakeenc")
	profile := cmdprofile.ProfileSpec{
		Name: "fakeenc_convert", Binary: "fakeenc", Permission: "allow",
		Template: []string{"{input_path}"},
		Params:   []cmdprofile.ParamSpec{{Name: "input_path", Type: cmdprofile.ParamFile}},
		Install:  install,
	}
	managers := cmdprofile.DetectManagers(profile)
	if len(managers) == 0 {
		t.Fatalf("no manager detected (id %s)", managerID)
	}
	store := NewStore(t.TempDir())
	service := NewService(store, NewRunner(store, Executors{}))
	defer service.Close()

	job, err := service.StartCommandInstall(profile, managers[0])
	if err != nil {
		t.Fatal(err)
	}
	done := waitForJob(t, service, job.ID, func(item AsyncJob) bool { return item.Status.terminal() })
	if done.Status != JobFailed {
		t.Fatalf("install job = %+v", done)
	}
	if !strings.Contains(done.Error, "no bucket") {
		t.Fatalf("error = %q, want the manager output surfaced", done.Error)
	}
}
