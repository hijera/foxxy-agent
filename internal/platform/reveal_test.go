package platform

import (
	"reflect"
	"testing"
)

func TestRevealFileArgv(t *testing.T) {
	for _, tc := range []struct {
		name string
		goos string
		path string
		want []string
	}{
		{name: "macOS selects file", goos: "darwin", path: "/workspace/report.txt", want: []string{"open", "-R", "/workspace/report.txt"}},
		{name: "Linux opens containing folder", goos: "linux", path: "/workspace/report.txt", want: []string{"xdg-open", "/workspace"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := revealFileArgv(tc.goos, tc.path)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("argv = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestRevealFileArgvWindowsUsesShellExplorer(t *testing.T) {
	t.Setenv("SystemRoot", `C:\Win`)
	t.Setenv("WINDIR", `D:\Ignore`)
	got, err := revealFileArgv("windows", `C:\temp\artifact.txt`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{`C:\Win\explorer.exe`, `/select,C:\temp\artifact.txt`}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argv = %#v, want %#v", got, want)
	}
	// No Windows directory variables at all: the conventional location.
	t.Setenv("SystemRoot", "")
	t.Setenv("WINDIR", "")
	got, err = revealFileArgv("windows", `C:\temp\artifact.txt`)
	if err != nil {
		t.Fatal(err)
	}
	want = []string{`C:\Windows\explorer.exe`, `/select,C:\temp\artifact.txt`}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argv = %#v, want %#v", got, want)
	}
}

func TestRevealFileArgvRejectsUnsupportedPlatform(t *testing.T) {
	if _, err := revealFileArgv("plan9", "/workspace/report.txt"); err == nil {
		t.Fatal("unsupported platform was accepted")
	}
}
