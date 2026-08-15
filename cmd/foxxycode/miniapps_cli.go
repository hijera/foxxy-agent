//go:build miniapps

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/hijera/foxxycode-agent/external/miniapps"
	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/desktop"
)

func miniAppsUsage() string {
	return fmt.Sprintf(`  %s miniapps validate <miniapp.json|bundle>
  %s miniapps inspect <miniapp.json|bundle>
  %s miniapps requirements <miniapp.json|bundle>
  %s miniapps run <miniapp.json|bundle> [--input inputs.json] [--confirm confirmations.json] [--ui]
  %s miniapps bundle <miniapp.json> --output app.foxxyapp [--files bundle-root]
  %s miniapps build <miniapp.json|bundle> --output app[.exe] --mode console|ui [--log-scope local|global]`,
		os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0])
}

func miniAppsHomeDirs() []string { return []string{"miniapps", "apps"} }

func runMiniAppsCommand(args []string) (bool, error) {
	if len(args) == 0 || args[0] != "miniapps" {
		return false, nil
	}
	if len(args) < 2 {
		return true, errors.New("usage: foxxycode miniapps validate|inspect|requirements|run|bundle|build")
	}
	switch args[1] {
	case "validate":
		return true, miniAppValidateCLI(args[2:])
	case "inspect":
		return true, miniAppInspectCLI(args[2:])
	case "requirements":
		return true, miniAppRequirementsCLI(args[2:])
	case "run":
		return true, miniAppRunCLI(args[2:])
	case "bundle":
		return true, miniAppBundleCLI(args[2:])
	case "build":
		return true, miniAppBuildCLI(args[2:])
	default:
		return true, fmt.Errorf("unknown miniapps command %q", args[1])
	}
}

func miniAppValidateCLI(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: foxxycode miniapps validate <miniapp.json|bundle>")
	}
	portable, err := miniapps.LoadPortable(args[0])
	if err != nil {
		return err
	}
	report := miniapps.Validate(portable.App)
	sanitization := miniapps.Sanitize(portable.App)
	out := map[string]any{"validation": report, "sanitization": sanitization}
	_ = json.NewEncoder(os.Stdout).Encode(out)
	if !report.Valid || !sanitization.Clean {
		return errors.New("mini app is not releasable")
	}
	return nil
}

func miniAppInspectCLI(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: foxxycode miniapps inspect <miniapp.json|bundle>")
	}
	portable, err := miniapps.LoadPortable(args[0])
	if err != nil {
		return err
	}
	return writeIndentedJSON(os.Stdout, map[string]any{
		"app": portable.App, "manifest": portable.Manifest,
		"bundle_files": sortedKeys(portable.Files),
	})
}

func miniAppRequirementsCLI(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: foxxycode miniapps requirements <miniapp.json|bundle>")
	}
	portable, err := miniapps.LoadPortable(args[0])
	if err != nil {
		return err
	}
	return writeIndentedJSON(os.Stdout, portable.App.Requirements)
}

func miniAppRunCLI(args []string) error {
	fs := flag.NewFlagSet("miniapps run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	inputPath := fs.String("input", "", "JSON object with operator inputs; use - for stdin")
	confirmationPath := fs.String("confirm", "", "JSON object of explicit step confirmations")
	uiMode := fs.Bool("ui", false, "open the generated operator form")
	configPath := fs.String("config", "", "path to config.yaml")
	home := fs.String("home", "", "FoxxyCode home directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: foxxycode miniapps run <miniapp.json|bundle> [--input inputs.json] [--confirm confirmations.json] [--ui]")
	}
	portable, err := miniapps.LoadPortable(fs.Arg(0))
	if err != nil {
		return err
	}
	return executePortableCLI(portable, *inputPath, *confirmationPath, *uiMode, *configPath, *home)
}

func miniAppBundleCLI(args []string) error {
	fs := flag.NewFlagSet("miniapps bundle", flag.ContinueOnError)
	output := fs.String("output", "", "output .foxxyapp path")
	filesRoot := fs.String("files", "", "directory whose relative files are copied into the bundle")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 || strings.TrimSpace(*output) == "" {
		return errors.New("usage: foxxycode miniapps bundle <miniapp.json> --output app.foxxyapp [--files bundle-root]")
	}
	portable, err := miniapps.LoadPortable(fs.Arg(0))
	if err != nil {
		return err
	}
	if strings.TrimSpace(*filesRoot) != "" {
		portable.Files, err = loadMiniAppBundleFiles(*filesRoot)
		if err != nil {
			return err
		}
	}
	return miniapps.WriteBundle(*output, portable)
}

func loadMiniAppBundleFiles(root string) (map[string][]byte, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("bundle files root must be a directory")
	}
	files := map[string][]byte{}
	var total int64
	err = filepath.WalkDir(absolute, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("bundle files may not contain symlinks: %s", path)
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(absolute, path)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		total += int64(len(raw))
		if total > 512<<20 {
			return errors.New("bundle files exceed the 512 MiB limit")
		}
		files[filepath.ToSlash(relative)] = raw
		return nil
	})
	return files, err
}

func miniAppBuildCLI(args []string) error {
	fs := flag.NewFlagSet("miniapps build", flag.ContinueOnError)
	output := fs.String("output", "", "output executable path")
	mode := fs.String("mode", "console", "console or ui")
	logScope := fs.String("log-scope", "", "override app run log scope: local or global")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 || strings.TrimSpace(*output) == "" {
		return errors.New("usage: foxxycode miniapps build <miniapp.json|bundle> --output app[.exe] --mode console|ui [--log-scope local|global]")
	}
	portable, err := miniapps.LoadPortable(fs.Arg(0))
	if err != nil {
		return err
	}
	if *logScope != "" {
		if *logScope != "local" && *logScope != "global" {
			return errors.New("log-scope must be local or global")
		}
		portable.App.Runtime.LogScope = *logScope
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	inputAbs, _ := filepath.Abs(executable)
	outputAbs, _ := filepath.Abs(*output)
	if strings.EqualFold(inputAbs, outputAbs) {
		return errors.New("output executable must differ from the running FoxxyCode interpreter")
	}
	return miniapps.BuildExecutable(executable, *output, portable, *mode)
}

func runEmbeddedMiniApp(args []string) (bool, error) {
	executable, err := os.Executable()
	if err != nil {
		return false, nil
	}
	portable, mode, embedded, err := miniapps.ReadEmbeddedExecutable(executable)
	if !embedded {
		return false, err
	}
	if err != nil {
		return true, err
	}
	fs := flag.NewFlagSet("embedded mini app", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	inputPath := fs.String("input", "", "JSON object with operator inputs; use - for stdin")
	confirmationPath := fs.String("confirm", "", "JSON object of explicit step confirmations")
	configPath := fs.String("config", "", "path to config.yaml")
	home := fs.String("home", "", "FoxxyCode home directory")
	console := fs.Bool("console", false, "override the embedded UI mode")
	if err := fs.Parse(args); err != nil {
		return true, err
	}
	uiMode := mode == "ui" && !*console
	return true, executePortableCLI(portable, *inputPath, *confirmationPath, uiMode, *configPath, *home)
}

func executePortableCLI(portable miniapps.Portable, inputPath, confirmationPath string, uiMode bool, configPath, home string) error {
	cfg, paths, err := loadMiniAppConfig(configPath, home)
	if err != nil {
		return err
	}
	store := miniapps.NewStoreWithRunRoot(filepath.Join(paths.Home, "miniapps"), filepath.Join(paths.Home, "apps"))
	runner := miniapps.NewRunner(store, miniapps.NewConfigModelExecutor(cfg, nil)).
		WithBundleFiles(portable.Files).
		WithLocalWorkspace(paths.CWD)
	if uiMode {
		return runMiniAppOperatorUI(portable.App, runner)
	}
	inputs, err := readMiniAppInputs(inputPath)
	if err != nil {
		return err
	}
	confirmations, err := readMiniAppConfirmations(confirmationPath)
	if err != nil {
		return err
	}
	run, err := runner.RunPortable(context.Background(), portable.App, inputs,
		&miniapps.OperatorDecisions{Confirmations: confirmations})
	if writeErr := writeIndentedJSON(os.Stdout, run); err == nil && writeErr != nil {
		err = writeErr
	}
	return err
}

func readMiniAppConfirmations(path string) (map[string]bool, error) {
	if strings.TrimSpace(path) == "" {
		return map[string]bool{}, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	var confirmations map[string]bool
	decoder := json.NewDecoder(io.LimitReader(file, 1<<20))
	if err := decoder.Decode(&confirmations); err != nil {
		return nil, err
	}
	return confirmations, nil
}

func loadMiniAppConfig(configPath, home string) (*config.Config, config.Paths, error) {
	cli := config.CLIPaths{Config: strings.TrimSpace(configPath), Home: strings.TrimSpace(home)}
	paths, err := config.Resolve(cli)
	if err != nil {
		return nil, config.Paths{}, err
	}
	if err := ensureFoxxyCodeHomeLayout(paths.Home); err != nil {
		return nil, config.Paths{}, err
	}
	cfg, err := config.LoadFromCLI(cli)
	if err != nil {
		return nil, config.Paths{}, fmt.Errorf("load config: %w", err)
	}
	return cfg, paths, nil
}

func readMiniAppInputs(path string) (map[string]any, error) {
	if strings.TrimSpace(path) == "" {
		return map[string]any{}, nil
	}
	var reader io.Reader
	if path == "-" {
		reader = os.Stdin
	} else {
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer func() { _ = file.Close() }()
		reader = file
	}
	var inputs map[string]any
	decoder := json.NewDecoder(io.LimitReader(reader, 16<<20))
	if err := decoder.Decode(&inputs); err != nil {
		return nil, err
	}
	return inputs, nil
}

func runMiniAppOperatorUI(app miniapps.MiniApp, runner *miniapps.Runner) error {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer func() { _ = listener.Close() }()
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = miniAppPage.Execute(w, app)
	})
	mux.HandleFunc("POST /run", func(w http.ResponseWriter, request *http.Request) {
		var payload struct {
			Inputs        map[string]any  `json:"inputs"`
			Confirmations map[string]bool `json:"confirmations"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, request.Body, 16<<20))
		if err := decoder.Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		run, runErr := runner.RunPortable(request.Context(), app, payload.Inputs,
			&miniapps.OperatorDecisions{Confirmations: payload.Confirmations})
		w.Header().Set("Content-Type", "application/json")
		if runErr != nil {
			w.WriteHeader(http.StatusUnprocessableEntity)
		}
		_ = json.NewEncoder(w).Encode(run)
	})
	server.Handler = mux
	serveError := make(chan error, 1)
	go func() { serveError <- server.Serve(listener) }()
	url := "http://" + listener.Addr().String() + "/"
	windowError := desktop.RunStandalone(app.Metadata.Name, url)
	if errors.Is(windowError, desktop.ErrStandaloneUnavailable) {
		if err := openMiniAppBrowser(url); err != nil {
			_ = server.Shutdown(context.Background())
			return err
		}
		<-serveError
		return nil
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
	return windowError
}

func openMiniAppBrowser(url string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		command = exec.Command("open", url)
	default:
		command = exec.Command("xdg-open", url)
	}
	return command.Start()
}

func writeIndentedJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func sortedKeys(values map[string][]byte) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

var miniAppPage = template.Must(template.New("miniapp").Funcs(template.FuncMap{
	"inputHTMLType": func(input miniapps.Input) string {
		switch input.Type {
		case "number", "integer":
			return "number"
		case "date":
			return "date"
		case "datetime":
			return "datetime-local"
		case "secret":
			return "password"
		default:
			return "text"
		}
	},
}).Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width">
<title>{{.Metadata.Name}}</title><style>
:root{font-family:Inter,Segoe UI,sans-serif;color:#26231f;background:#f6f3ed}
body{margin:0;display:grid;place-items:center;min-height:100vh}.card{width:min(760px,calc(100% - 40px));background:white;border:1px solid #ddd5c9;border-radius:18px;padding:28px;box-shadow:0 18px 70px #332b1d20}
h1{margin:0 0 8px}.description{color:#665f56;margin:0 0 24px}.field{display:grid;gap:7px;margin:15px 0}label{font-weight:650}
input,textarea,select{font:inherit;padding:11px 12px;border:1px solid #c9c0b3;border-radius:10px}textarea{min-height:100px}
button{font:inherit;font-weight:700;background:#dc6848;color:white;border:0;border-radius:10px;padding:11px 18px;cursor:pointer}
pre{white-space:pre-wrap;background:#f5f2ed;border-radius:12px;padding:16px;min-height:48px}
</style></head><body><main class="card"><h1>{{.Metadata.Name}}</h1><p class="description">{{.Metadata.Description}}</p>
<form id="form">{{range .Inputs}}<div class="field"><label for="{{.ID}}">{{.Title}}</label>
{{if eq .Type "boolean"}}<input id="{{.ID}}" data-id="{{.ID}}" data-type="{{.Type}}" type="checkbox">
{{else if .Validation.Enum}}<select id="{{.ID}}" data-id="{{.ID}}" data-type="{{.Type}}" {{if .Required}}required{{end}}>{{range .Validation.Enum}}<option>{{.}}</option>{{end}}</select>
{{else if eq .UI.Control "textarea"}}<textarea id="{{.ID}}" data-id="{{.ID}}" data-type="{{.Type}}" {{if .Required}}required{{end}}>{{.Default}}</textarea>
{{else}}<input id="{{.ID}}" data-id="{{.ID}}" data-type="{{.Type}}" type="{{inputHTMLType .}}" value="{{.Default}}" {{if .Required}}required{{end}}>{{end}}
{{if .Description}}<small>{{.Description}}</small>{{end}}</div>{{end}}
{{range .Workflow}}{{if eq .Kind "confirm"}}<div class="field"><label><input data-confirm="{{.ID}}" type="checkbox"> {{if .Message}}{{.Message}}{{else}}{{.Title}}{{end}}</label><small>Explicit operator approval is required.</small></div>{{end}}{{end}}
<button>Run</button></form>
<h2>Result</h2><pre id="result">Ready.</pre></main><script>
document.getElementById("form").addEventListener("submit",async(e)=>{e.preventDefault();const inputs={},confirmations={};
document.querySelectorAll("[data-id]").forEach(el=>{let v=el.type==="checkbox"?el.checked:el.value;if(el.dataset.type==="number"||el.dataset.type==="integer")v=Number(v);inputs[el.dataset.id]=v});
document.querySelectorAll("[data-confirm]").forEach(el=>{confirmations[el.dataset.confirm]=el.checked});
const out=document.getElementById("result");out.textContent="Running…";try{const r=await fetch("/run",{method:"POST",headers:{"content-type":"application/json"},body:JSON.stringify({inputs,confirmations})});const j=await r.json();out.textContent=j.status==="succeeded"?JSON.stringify(j.outputs,null,2):(j.error||JSON.stringify(j,null,2))}catch(err){out.textContent=String(err)}});
</script></body></html>`))
