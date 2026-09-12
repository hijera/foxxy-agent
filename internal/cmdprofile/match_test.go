package cmdprofile

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestTokenizeSimpleCommand(t *testing.T) {
	cases := map[string]struct {
		command string
		want    []string
	}{
		"plain":         {"ffmpeg -i in.mp4 out.mp3", []string{"ffmpeg", "-i", "in.mp4", "out.mp3"}},
		"extra spaces":  {"  ffmpeg   -i  in.mp4 ", []string{"ffmpeg", "-i", "in.mp4"}},
		"double quotes": {`ffmpeg -i "my video.mp4" out.mp3`, []string{"ffmpeg", "-i", "my video.mp4", "out.mp3"}},
		"single quotes": {`convert 'a b.png' out.png`, []string{"convert", "a b.png", "out.png"}},
		"windows path":  {`ffmpeg -i C:\video\in.mp4 out.mp3`, []string{"ffmpeg", "-i", `C:\video\in.mp4`, "out.mp3"}},
		"mixed value":   {"ffmpeg -vf scale=640:-1 out.mp4", []string{"ffmpeg", "-vf", "scale=640:-1", "out.mp4"}},
		"tab separated": {"ffmpeg\t-i\tin.mp4", []string{"ffmpeg", "-i", "in.mp4"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := TokenizeSimpleCommand(tc.command)
			if err != nil {
				t.Fatalf("TokenizeSimpleCommand(%q) error = %v", tc.command, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("TokenizeSimpleCommand(%q) = %#v, want %#v", tc.command, got, tc.want)
			}
		})
	}
}

// Anything the shell would interpret is rejected wholesale: this tokenizer
// exists to prove a command is a single simple invocation, not to emulate sh.
func TestTokenizeSimpleCommandRejectsShellSyntax(t *testing.T) {
	rejected := []string{
		"ffmpeg -i in.mp4 out.mp3 && rm -rf .",
		"ffmpeg -i in.mp4 out.mp3; echo done",
		"cat in.txt | grep x",
		"ffmpeg -i in.mp4 out.mp3 &",
		"echo hi > out.txt",
		"wc -l < in.txt",
		"echo `whoami`",
		"echo $(whoami)",
		"echo $HOME",
		"ffmpeg -i in.mp4\nrm -rf .",
		"echo (test)",
		`echo "unterminated`,
		`echo ab"cd"`,
		`echo \"escaped\"`,
		"",
		"   ",
	}
	for _, command := range rejected {
		if _, err := TokenizeSimpleCommand(command); !errors.Is(err, ErrShellComplex) {
			t.Errorf("TokenizeSimpleCommand(%q) error = %v, want ErrShellComplex", command, err)
		}
	}
}

func TestMatchProfilesBindsTypedParams(t *testing.T) {
	match, err := MatchProfiles("ffmpeg -i in.mp4 -vn -acodec libmp3lame out.mp3", []ProfileSpec{ffmpegSpec()})
	if err != nil {
		t.Fatalf("MatchProfiles() error = %v", err)
	}
	if match == nil {
		t.Fatal("MatchProfiles() found no match")
	}
	want := map[string]string{"input_path": "in.mp4", "codec": "libmp3lame", "output_path": "out.mp3"}
	if !reflect.DeepEqual(match.Params, want) {
		t.Fatalf("params = %#v, want %#v", match.Params, want)
	}
}

func TestMatchProfilesNoMatchCases(t *testing.T) {
	spec := ffmpegSpec()
	cases := map[string]string{
		"different binary":       "convert in.png out.jpg",
		"missing tail token":     "ffmpeg -i in.mp4 -vn -acodec libmp3lame",
		"extra trailing token":   "ffmpeg -i in.mp4 -vn -acodec libmp3lame out.mp3 extra",
		"enum value not allowed": "ffmpeg -i in.mp4 -vn -acodec opus out.mp3",
		"literal mismatch":       "ffmpeg -i in.mp4 -an -acodec libmp3lame out.mp3",
		"dashed captured value":  "ffmpeg -i -badflag -vn -acodec libmp3lame out.mp3",
	}
	for name, command := range cases {
		t.Run(name, func(t *testing.T) {
			match, err := MatchProfiles(command, []ProfileSpec{spec})
			if err != nil {
				t.Fatalf("MatchProfiles() error = %v", err)
			}
			if match != nil {
				t.Fatalf("MatchProfiles(%q) unexpectedly matched: %#v", command, match.Params)
			}
		})
	}
}

func TestMatchProfilesFlagBranching(t *testing.T) {
	spec := ProfileSpec{
		Name: "gzip_pack", Binary: "gzip", Permission: "allow",
		Template: []string{"{keep}", "{level}", "{input_path}"},
		Params: []ParamSpec{
			{Name: "keep", Type: ParamFlag, Literal: "-k"},
			{Name: "level", Type: ParamEnum, Enum: []string{"-1", "-9"}},
			{Name: "input_path", Type: ParamFile},
		},
	}
	// Enum values may not start with '-' per validation, so adjust: use a
	// mixed-token level instead to keep the fixture valid.
	spec.Template = []string{"{keep}", "-{level}", "{input_path}"}
	spec.Params[1] = ParamSpec{Name: "level", Type: ParamEnum, Enum: []string{"1", "9"}}
	if err := spec.Validate(); err != nil {
		t.Fatalf("fixture invalid: %v", err)
	}

	with, err := MatchProfiles("gzip -k -9 data.bin", []ProfileSpec{spec})
	if err != nil || with == nil {
		t.Fatalf("flagged match = %#v, err %v", with, err)
	}
	if with.Params["keep"] != "true" || with.Params["level"] != "9" || with.Params["input_path"] != "data.bin" {
		t.Fatalf("flagged params = %#v", with.Params)
	}

	without, err := MatchProfiles("gzip -1 data.bin", []ProfileSpec{spec})
	if err != nil || without == nil {
		t.Fatalf("unflagged match = %#v, err %v", without, err)
	}
	if without.Params["keep"] != "false" || without.Params["level"] != "1" {
		t.Fatalf("unflagged params = %#v", without.Params)
	}
}

func TestMatchProfilesMixedTokenWithTwoPlaceholders(t *testing.T) {
	spec := ProfileSpec{
		Name: "scale_video", Binary: "ffmpeg", Permission: "allow",
		Template: []string{"-i", "{input_path}", "-vf", "scale={width}:{height}", "{output_path}"},
		Params: []ParamSpec{
			{Name: "input_path", Type: ParamFile},
			{Name: "width", Type: ParamInt},
			{Name: "height", Type: ParamInt},
			{Name: "output_path", Type: ParamFile},
		},
	}
	if err := spec.Validate(); err != nil {
		t.Fatalf("fixture invalid: %v", err)
	}
	match, err := MatchProfiles("ffmpeg -i in.mp4 -vf scale=640:480 out.mp4", []ProfileSpec{spec})
	if err != nil || match == nil {
		t.Fatalf("match = %#v, err %v", match, err)
	}
	if match.Params["width"] != "640" || match.Params["height"] != "480" {
		t.Fatalf("params = %#v", match.Params)
	}
}

func TestBuildArgvSubstitutesAndGuards(t *testing.T) {
	argv, err := BuildArgv(ffmpegSpec(), map[string]string{
		"input_path": "видео с пробелом.mp4", "codec": "aac", "output_path": "out.m4a",
	})
	if err != nil {
		t.Fatalf("BuildArgv() error = %v", err)
	}
	want := []string{"-i", "видео с пробелом.mp4", "-vn", "-acodec", "aac", "out.m4a"}
	if !reflect.DeepEqual(argv, want) {
		t.Fatalf("argv = %#v, want %#v", argv, want)
	}

	if _, err := BuildArgv(ffmpegSpec(), map[string]string{
		"input_path": "-i", "codec": "aac", "output_path": "out.m4a",
	}); err == nil || !strings.Contains(err.Error(), "dash") {
		t.Fatalf("leading-dash value error = %v", err)
	}
	if _, err := BuildArgv(ffmpegSpec(), map[string]string{
		"input_path": "in.mp4", "codec": "opus", "output_path": "out.m4a",
	}); err == nil {
		t.Fatal("out-of-enum value was accepted")
	}
	if _, err := BuildArgv(ffmpegSpec(), map[string]string{
		"codec": "aac", "output_path": "out.m4a",
	}); err == nil || !strings.Contains(err.Error(), "input_path") {
		t.Fatalf("missing required param error = %v", err)
	}
	if _, err := BuildArgv(ffmpegSpec(), map[string]string{
		"input_path": "in\nject.mp4", "codec": "aac", "output_path": "out.m4a",
	}); err == nil {
		t.Fatal("value with a newline was accepted")
	}
}

// VerifyReconstruction is the acceptance gate for both the matcher and the
// LLM generator: the profile is only real if it reproduces the original argv.
func TestVerifyReconstruction(t *testing.T) {
	argv := []string{"ffmpeg", "-i", "in.mp4", "-vn", "-acodec", "aac", "out.m4a"}
	params := map[string]string{"input_path": "in.mp4", "codec": "aac", "output_path": "out.m4a"}
	if err := VerifyReconstruction(ffmpegSpec(), params, argv); err != nil {
		t.Fatalf("VerifyReconstruction() = %v", err)
	}
	// A profile that drops a token must be rejected even when everything else fits.
	broken := ffmpegSpec()
	broken.Template = []string{"-i", "{input_path}", "-acodec", "{codec}", "{output_path}"}
	if err := VerifyReconstruction(broken, params, argv); err == nil {
		t.Fatal("a diverging template passed reconstruction")
	}
	// The binary token participates through name matching, not equality.
	windowsArgv := append([]string{`C:\tools\ffmpeg.EXE`}, argv[1:]...)
	if err := VerifyReconstruction(ffmpegSpec(), params, windowsArgv); err != nil {
		t.Fatalf("binary path form rejected: %v", err)
	}
	if err := VerifyReconstruction(ffmpegSpec(), params, append([]string{"magick"}, argv[1:]...)); err == nil {
		t.Fatal("wrong binary passed reconstruction")
	}
}

// Every successful match must survive its own reconstruction; this property
// holds by construction and is pinned here against matcher regressions.
func TestMatchImpliesReconstruction(t *testing.T) {
	commands := []string{
		"ffmpeg -i in.mp4 -vn -acodec libmp3lame out.mp3",
		`ffmpeg -i "с пробелом.mp4" -vn -acodec aac out.m4a`,
	}
	for _, command := range commands {
		match, err := MatchProfiles(command, []ProfileSpec{ffmpegSpec()})
		if err != nil || match == nil {
			t.Fatalf("MatchProfiles(%q) = %#v, err %v", command, match, err)
		}
		argv, err := TokenizeSimpleCommand(command)
		if err != nil {
			t.Fatal(err)
		}
		if err := VerifyReconstruction(match.Profile, match.Params, argv); err != nil {
			t.Fatalf("match for %q failed reconstruction: %v", command, err)
		}
	}
}
