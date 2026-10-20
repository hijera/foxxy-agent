//go:build miniapps

package miniapps

import (
	"testing"

	"github.com/hijera/foxxycode-agent/internal/cmdprofile"
)

// commandProfileApp is a valid app whose single step runs an embedded profile.
func commandProfileApp() MiniApp {
	app := coreValidApp()
	app.Permissions = Permissions{Tools: []string{"cmd_ffmpeg_extract_audio"}}
	app.Workflow = []Step{{
		ID: "extract", Kind: "tool", Title: "Extract audio", Tool: "cmd_ffmpeg_extract_audio",
		Arguments: map[string]any{
			"input_path": Ref{Ref: "inputs.source"}, "codec": "aac", "output_path": "out.m4a",
		},
	}}
	app.Success = SuccessSpec{Mode: "all", Checks: []SuccessCheck{{
		Kind: "step", Step: "extract", Status: string(RunSucceeded),
	}}}
	app.Requirements.Commands = []cmdprofile.ProfileSpec{commandTestProfile()}
	return app
}

func TestValidateAcceptsAnAppWithAnEmbeddedProfile(t *testing.T) {
	report := Validate(commandProfileApp())
	if !report.Valid {
		t.Fatalf("report = %+v", report.Issues)
	}
}

func TestValidateRejectsBrokenCommandDeclarations(t *testing.T) {
	cases := map[string]struct {
		mutate func(*MiniApp)
		path   string
	}{
		"invalid embedded profile": {
			func(app *MiniApp) { app.Requirements.Commands[0].Binary = "" },
			"requirements.commands[0]",
		},
		// Mini Apps run unattended: an embedded ask-profile could never be
		// approved mid-run, so it is rejected at validation time.
		"ask permission": {
			func(app *MiniApp) { app.Requirements.Commands[0].Permission = "ask" },
			"requirements.commands[0].permission",
		},
		"duplicate profile names": {
			func(app *MiniApp) {
				app.Requirements.Commands = append(app.Requirements.Commands, app.Requirements.Commands[0].Clone())
			},
			"requirements.commands[1].name",
		},
		"declared but unused profile stays valid; an undeclared step does not": {
			func(app *MiniApp) { app.Requirements.Commands = nil },
			"workflow[0].tool",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			app := commandProfileApp()
			tc.mutate(&app)
			report := Validate(app)
			if report.Valid {
				t.Fatal("Validate() accepted a broken command declaration")
			}
			if !hasIssuePath(report, tc.path) {
				t.Fatalf("issues = %+v, want one at %s", report.Issues, tc.path)
			}
		})
	}
}
