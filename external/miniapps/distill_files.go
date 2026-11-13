//go:build miniapps

package miniapps

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/cmdprofile"
)

func inputReference(value any) string {
	var ref string
	switch value := value.(type) {
	case Ref:
		ref = value.Ref
	case map[string]any:
		ref, _ = value["$ref"].(string)
	}
	return strings.TrimPrefix(ref, "inputs.")
}

// enrichCommandForms preserves parameter types when a recorded command becomes
// a reusable form. Only conventional output names are inferred as destinations;
// other file parameters remain explicit uploads for the author to review.
func enrichCommandForms(app *MiniApp, specs map[string]TraceInputSpec) {
	for _, step := range app.Workflow {
		profile, ok := commandProfileByToolName(app.Requirements.Commands, step.Tool)
		if !ok {
			continue
		}
		args, ok := step.Arguments.(map[string]any)
		if !ok {
			continue
		}
		for _, param := range profile.Params {
			id := inputReference(args[param.Name])
			for i := range app.Inputs {
				field := &app.Inputs[i]
				if field.ID != id {
					continue
				}
				observed := specs[id].ObservedValue
				switch param.Type {
				case cmdprofile.ParamFile:
					output := param.Name == "output_path" || param.Name == "output_file" || param.Name == "destination_path" || param.Name == "destination_file" || param.Name == "target_path" || param.Name == "target_file"
					if output {
						field.Type, field.UI.Control = "string", "text"
						filename := path.Base(strings.ReplaceAll(fmt.Sprint(observed), "\\", "/"))
						if filename == "." || filename == "/" || filename == "<nil>" {
							filename = "result.bin"
						}
						field.Default = filename
						renderer := "download"
						switch strings.ToLower(path.Ext(filename)) {
						case ".mp3", ".wav", ".m4a", ".ogg", ".flac":
							renderer = "audio"
						case ".png", ".jpg", ".jpeg", ".webp", ".gif":
							renderer = "image"
						case ".mp4", ".webm":
							renderer = "video"
						}
						app.Outputs = append(app.Outputs, Output{ID: fmt.Sprintf("file-%d", len(app.Outputs)), Type: "file", Value: Ref{Ref: "inputs." + field.ID}, Renderer: renderer, Title: filename})
					} else if !strings.HasSuffix(param.Name, "_dir") {
						field.Type, field.UI.Control = "file", "file"
						field.Default = nil
						if extension := path.Ext(fmt.Sprint(observed)); extension != "" {
							field.Validation.Extensions = []string{strings.ToLower(extension)}
						}
					}
				case cmdprofile.ParamEnum:
					field.Type, field.UI.Control = "enum", "select"
					for _, option := range param.Enum {
						field.Validation.Enum = append(field.Validation.Enum, option)
						if fmt.Sprint(observed) == option {
							field.Default = option
						}
					}
				case cmdprofile.ParamInt:
					field.Type, field.UI.Control = "integer", "number"
					if n, err := strconv.Atoi(fmt.Sprint(observed)); err == nil {
						field.Default = n
					}
					if param.Min != nil {
						n := float64(*param.Min)
						field.Validation.Minimum = &n
					}
					if param.Max != nil {
						n := float64(*param.Max)
						field.Validation.Maximum = &n
					}
				case cmdprofile.ParamFlag:
					field.Type, field.UI.Control = "boolean", "checkbox"
					field.Default = fmt.Sprint(observed) == "true"
				case cmdprofile.ParamString:
					field.Type, field.UI.Control = "string", "text"
					field.Validation.Pattern = "^(?:" + param.Pattern + ")$"
				}
			}
		}
	}
}
