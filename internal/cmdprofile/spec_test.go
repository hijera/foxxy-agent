package cmdprofile

import (
	"strings"
	"testing"
)

func boolPtr(v bool) *bool { return &v }

// ffmpegSpec is the canonical fixture used across the package tests.
func ffmpegSpec() ProfileSpec {
	return ProfileSpec{
		Name:        "ffmpeg_extract_audio",
		Binary:      "ffmpeg",
		Description: "Extract the audio track from a video file.",
		Permission:  "allow",
		Template:    []string{"-i", "{input_path}", "-vn", "-acodec", "{codec}", "{output_path}"},
		Params: []ParamSpec{
			{Name: "input_path", Type: ParamFile},
			{Name: "codec", Type: ParamEnum, Enum: []string{"libmp3lame", "aac"}},
			{Name: "output_path", Type: ParamFile},
		},
		Install: InstallSpec{Winget: "Gyan.FFmpeg", Scoop: "ffmpeg", Apt: "ffmpeg", Brew: "ffmpeg"},
	}
}

func TestProfileSpecValidateAcceptsTheCanonicalProfile(t *testing.T) {
	spec := ffmpegSpec()
	if err := spec.Validate(); err != nil {
		t.Fatalf("Validate() = %v", err)
	}
	if got := spec.ToolName(); got != "cmd_ffmpeg_extract_audio" {
		t.Fatalf("ToolName() = %q", got)
	}
	if got := spec.ResolvedPermission(); got != PermissionAllow {
		t.Fatalf("ResolvedPermission() = %q", got)
	}
}

func TestProfileSpecResolvedPermissionDefaultsToAsk(t *testing.T) {
	spec := ffmpegSpec()
	spec.Permission = ""
	if got := spec.ResolvedPermission(); got != PermissionAsk {
		t.Fatalf("ResolvedPermission() = %q, want ask", got)
	}
}

func TestProfileSpecValidateRejections(t *testing.T) {
	cases := map[string]struct {
		mutate func(*ProfileSpec)
		want   string
	}{
		"empty name":        {func(s *ProfileSpec) { s.Name = "" }, "name"},
		"uppercase name":    {func(s *ProfileSpec) { s.Name = "FFmpeg" }, "name"},
		"name with dash":    {func(s *ProfileSpec) { s.Name = "ff-mpeg" }, "name"},
		"double underscore": {func(s *ProfileSpec) { s.Name = "ff__mpeg" }, "name"},
		// The miniapps denylist blocks tool names ending _create/_update/_delete,
		// so such a profile would validate here and then fail every run.
		"create suffix":        {func(s *ProfileSpec) { s.Name = "thumb_create" }, "suffix"},
		"update suffix":        {func(s *ProfileSpec) { s.Name = "meta_update" }, "suffix"},
		"delete suffix":        {func(s *ProfileSpec) { s.Name = "temp_delete" }, "suffix"},
		"empty binary":         {func(s *ProfileSpec) { s.Binary = "" }, "binary"},
		"relative path binary": {func(s *ProfileSpec) { s.Binary = "tools/ffmpeg" }, "binary"},
		"batch binary":         {func(s *ProfileSpec) { s.Binary = "convert.bat" }, "binary"},
		"cmd binary":           {func(s *ProfileSpec) { s.Binary = "convert.CMD" }, "binary"},
		"binary with space":    {func(s *ProfileSpec) { s.Binary = "ff mpeg" }, "binary"},
		"unknown permission":   {func(s *ProfileSpec) { s.Permission = "always" }, "permission"},
		"negative timeout":     {func(s *ProfileSpec) { s.TimeoutSeconds = -1 }, "timeout"},
		"huge timeout":         {func(s *ProfileSpec) { s.TimeoutSeconds = 100000 }, "timeout"},
		"empty template":       {func(s *ProfileSpec) { s.Template = nil }, "template"},
		"undeclared placeholder": {
			func(s *ProfileSpec) { s.Template = append(s.Template, "{bitrate}") }, "bitrate",
		},
		"param missing from template": {
			func(s *ProfileSpec) {
				s.Params = append(s.Params, ParamSpec{Name: "rate_value", Type: ParamInt})
			}, "rate_value",
		},
		"file param without path suffix": {
			func(s *ProfileSpec) {
				s.Params[0].Name = "input"
				s.Template[1] = "{input}"
			}, "_path",
		},
		"param named like a secret": {
			func(s *ProfileSpec) {
				s.Params[1] = ParamSpec{Name: "token_value", Type: ParamEnum, Enum: []string{"a"}}
				s.Template[4] = "{token_value}"
			}, "token",
		},
		"param named source": {
			func(s *ProfileSpec) {
				s.Params[0].Name = "source_path"
				s.Template[1] = "{source_path}"
			}, "source",
		},
		"enum without values": {
			func(s *ProfileSpec) { s.Params[1].Enum = nil }, "enum",
		},
		"enum value with leading dash": {
			func(s *ProfileSpec) { s.Params[1].Enum = []string{"-vn"} }, "enum",
		},
		"string without pattern": {
			func(s *ProfileSpec) {
				s.Params[1] = ParamSpec{Name: "codec", Type: ParamString}
			}, "pattern",
		},
		"string with broken pattern": {
			func(s *ProfileSpec) {
				s.Params[1] = ParamSpec{Name: "codec", Type: ParamString, Pattern: "["}
			}, "pattern",
		},
		"flag without literal": {
			func(s *ProfileSpec) {
				s.Params = append(s.Params, ParamSpec{Name: "overwrite", Type: ParamFlag})
				s.Template = append(s.Template, "{overwrite}")
			}, "literal",
		},
		"literal on non-flag": {
			func(s *ProfileSpec) { s.Params[1].Literal = "-y" }, "literal",
		},
		"optional non-flag param": {
			func(s *ProfileSpec) { s.Params[1].Required = boolPtr(false) }, "required",
		},
		"unknown param type": {
			func(s *ProfileSpec) { s.Params[1].Type = "text" }, "type",
		},
		"min above max": {
			func(s *ProfileSpec) {
				one, zero := 1, 0
				s.Params[1] = ParamSpec{Name: "rate_value", Type: ParamInt, Min: &one, Max: &zero}
				s.Template[4] = "{rate_value}"
			}, "min",
		},
		"install coordinate with leading dash": {
			func(s *ProfileSpec) { s.Install.Winget = "-Gyan.FFmpeg" }, "install",
		},
		"install coordinate with space": {
			func(s *ProfileSpec) { s.Install.Apt = "ffmpeg extra" }, "install",
		},
		"adjacent placeholders in one token": {
			func(s *ProfileSpec) {
				s.Params = append(s.Params, ParamSpec{Name: "rate_value", Type: ParamInt})
				s.Template = append(s.Template, "{rate_value}{codec}")
			}, "placeholder",
		},
		"duplicate param name": {
			func(s *ProfileSpec) {
				s.Params = append(s.Params, ParamSpec{Name: "codec", Type: ParamEnum, Enum: []string{"x"}})
			}, "duplicate",
		},
		"flag inside a mixed token": {
			func(s *ProfileSpec) {
				s.Params = append(s.Params, ParamSpec{Name: "loud", Type: ParamFlag, Literal: "-y"})
				s.Template = append(s.Template, "x={loud}")
			}, "flag",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			spec := ffmpegSpec()
			tc.mutate(&spec)
			err := spec.Validate()
			if err == nil {
				t.Fatalf("Validate() accepted an invalid spec")
			}
			if !strings.Contains(strings.ToLower(err.Error()), tc.want) {
				t.Fatalf("Validate() = %v, want mention of %q", err, tc.want)
			}
		})
	}
}

func TestProfileSpecAllowsAbsoluteBinary(t *testing.T) {
	spec := ffmpegSpec()
	spec.Binary = `C:\tools\ffmpeg\ffmpeg.exe`
	if err := spec.Validate(); err != nil {
		t.Fatalf("absolute Windows binary rejected: %v", err)
	}
	spec.Binary = "/usr/bin/ffmpeg"
	if err := spec.Validate(); err != nil {
		t.Fatalf("absolute POSIX binary rejected: %v", err)
	}
}

// The hash is the identity a trust approval is bound to: any edit must change
// it, and the encoding must never drift between releases.
func TestCanonicalHashIsStableAndEditSensitive(t *testing.T) {
	base, err := CanonicalHash(ffmpegSpec())
	if err != nil {
		t.Fatal(err)
	}
	// Pinned encoding: a drift here silently invalidates every recorded trust
	// approval, so changing it is a deliberate, breaking decision.
	const golden = "eb3ca310dba0677f6b5a9bc1bdaf5823c162a2901620ee03cf68fde80fe20531"
	if base != golden {
		t.Fatalf("canonical hash drifted: %s", base)
	}
	again, err := CanonicalHash(ffmpegSpec())
	if err != nil {
		t.Fatal(err)
	}
	if base != again {
		t.Fatal("hash is not deterministic")
	}
	edited := ffmpegSpec()
	edited.Template[0] = "-loglevel"
	other, err := CanonicalHash(edited)
	if err != nil {
		t.Fatal(err)
	}
	if other == base {
		t.Fatal("template edit did not change the hash")
	}
	edited = ffmpegSpec()
	edited.Permission = PermissionAsk
	other, err = CanonicalHash(edited)
	if err != nil {
		t.Fatal(err)
	}
	if other == base {
		t.Fatal("permission edit did not change the hash")
	}
}
