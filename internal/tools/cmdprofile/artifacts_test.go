package cmdprofile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/cmdprofile"
)

func TestCommandArtifactEvidenceRefusesUnconfinedAndOversizedFiles(t *testing.T) {
	workspace := t.TempDir()
	outside := filepath.Join(t.TempDir(), "private.txt")
	if err := os.WriteFile(outside, []byte("outside data"), 0o600); err != nil {
		t.Fatal(err)
	}
	large := filepath.Join(workspace, "large.bin")
	file, err := os.Create(large)
	if err != nil {
		t.Fatal(err)
	}
	err = file.Truncate(maxRecordedArtifactBytes + 1)
	_ = file.Close()
	if err != nil {
		t.Fatal(err)
	}
	spec := cmdprofile.ProfileSpec{Params: []cmdprofile.ParamSpec{{Name: "output_path", Type: cmdprofile.ParamFile}}}
	cases := map[string]string{"absolute escape": outside, "relative escape": "../private.txt", "oversized": "large.bin"}
	if os.Symlink(outside, filepath.Join(workspace, "linked.txt")) == nil {
		cases["symlink"] = "linked.txt"
	}
	for name, path := range cases {
		t.Run(name, func(t *testing.T) {
			result, err := recordCommandArtifacts(spec, map[string]string{"output_path": path}, workspace, "ordinary stdout")
			if err != nil || result != "ordinary stdout" {
				t.Fatalf("unsafe output recorded: result=%q err=%v", result, err)
			}
		})
	}
}
