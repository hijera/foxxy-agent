package cmdprofile

import (
	"errors"
	"os/exec"
	"reflect"
	"runtime"
	"testing"
)

// stubLookPath makes only the named managers resolvable for one test.
func stubLookPath(t *testing.T, available ...string) {
	t.Helper()
	previous := managerLookPath
	managerLookPath = func(name string) (string, error) {
		for _, ok := range available {
			if name == ok {
				return "/fake/" + name, nil
			}
		}
		return "", errors.New("not found")
	}
	t.Cleanup(func() { managerLookPath = previous })
}

func TestDetectManagersUsesDeclaredCoordinatesOnly(t *testing.T) {
	spec := ffmpegSpec() // declares winget, scoop, apt, brew — not dnf
	switch runtime.GOOS {
	case "windows":
		stubLookPath(t, "winget", "scoop")
		managers := DetectManagers(spec)
		if len(managers) != 2 {
			t.Fatalf("managers = %#v", managers)
		}
		if managers[0].ID != "winget" || managers[0].Package != "Gyan.FFmpeg" {
			t.Fatalf("winget row = %#v", managers[0])
		}
		want := []string{"winget", "install", "--id", "Gyan.FFmpeg", "-e",
			"--accept-source-agreements", "--accept-package-agreements"}
		if !reflect.DeepEqual(managers[0].Argv, want) {
			t.Fatalf("winget argv = %#v", managers[0].Argv)
		}
		if managers[1].ID != "scoop" || !reflect.DeepEqual(managers[1].Argv, []string{"scoop", "install", "ffmpeg"}) {
			t.Fatalf("scoop row = %#v", managers[1])
		}
	case "darwin":
		stubLookPath(t, "brew")
		managers := DetectManagers(spec)
		if len(managers) != 1 || managers[0].ID != "brew" {
			t.Fatalf("managers = %#v", managers)
		}
	default:
		stubLookPath(t, "apt-get", "dnf")
		managers := DetectManagers(spec)
		// dnf is installed but the profile declares no dnf coordinate.
		if len(managers) != 1 || managers[0].ID != "apt" {
			t.Fatalf("managers = %#v", managers)
		}
		if !reflect.DeepEqual(managers[0].Argv, []string{"apt-get", "install", "-y", "ffmpeg"}) {
			t.Fatalf("apt argv = %#v", managers[0].Argv)
		}
	}
}

func TestDetectManagersEmptyWhenNothingInstalled(t *testing.T) {
	stubLookPath(t) // nothing resolvable
	if managers := DetectManagers(ffmpegSpec()); len(managers) != 0 {
		t.Fatalf("managers = %#v, want none", managers)
	}
}

// The seam must default to the real lookup so production code detects real
// managers; the stub above only narrows it within a test.
func TestManagerLookPathDefaultsToExec(t *testing.T) {
	if reflect.ValueOf(managerLookPath).Pointer() != reflect.ValueOf(exec.LookPath).Pointer() {
		t.Fatal("managerLookPath does not default to exec.LookPath")
	}
}
