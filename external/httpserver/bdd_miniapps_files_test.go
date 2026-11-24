//go:build http && miniapps

package httpserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/hijera/foxxycode-agent/external/miniapps"
)

type miniAppFilesFeature struct {
	miniAppsFeatureState
	uploadID, revision, fileID, runID string
}

func (s *miniAppFilesFeature) create() error {
	if err := s.reset(); err != nil {
		return err
	}
	s.appID = "uploaded-transform"
	app := miniapps.MiniApp{SchemaVersion: miniapps.SchemaVersion, Kind: miniapps.KindMiniApp, ID: s.appID, State: miniapps.StateDraft, Metadata: miniapps.Metadata{Name: "File transformer", Goal: "Read an uploaded file"}, Inputs: []miniapps.Input{{ID: "source", Type: "file", Title: "Source file", Required: true, UI: miniapps.InputUI{Control: "file"}}}, Permissions: miniapps.Permissions{Tools: []string{"read", "write"}}, Workflow: []miniapps.Step{{ID: "read-source", Kind: "tool", Title: "Read", Tool: "read", Arguments: map[string]any{"path": miniapps.Ref{Ref: "inputs.source"}}}, {ID: "write-result", Kind: "tool", Title: "Write", Tool: "write", Arguments: map[string]any{"path": "result.txt", "content": miniapps.Ref{Ref: "steps.read-source.outputs.result"}}}}, Outputs: []miniapps.Output{{ID: "download", Type: "file", Value: "result.txt", Renderer: "download"}}, Success: miniapps.SuccessSpec{Mode: "all", Checks: []miniapps.SuccessCheck{{Kind: "step", Step: "write-result", Status: "succeeded"}}}, Runtime: miniapps.RuntimePolicy{LogScope: "local", OperatorEventLevel: "status", DiagnosticToolEvents: "sanitized"}}
	response, err := s.post("/foxxycode/miniapps", app)
	if err != nil {
		return err
	}
	if response.StatusCode != 201 {
		return fmt.Errorf("create: %d %s", response.StatusCode, response.Body)
	}
	var saved miniapps.MiniApp
	if err := json.Unmarshal([]byte(response.Body), &saved); err != nil {
		return err
	}
	s.revision = saved.Revision
	return nil
}
func (s *miniAppFilesFeature) upload() error {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "source.txt")
	if err != nil {
		return err
	}
	if _, err := io.WriteString(part, "a real uploaded source"); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	response, err := http.Post(s.ts.URL+"/foxxycode/miniapps/"+s.appID+"/inputs/source/uploads?revision="+s.revision, writer.FormDataContentType(), &body)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != 201 {
		data, _ := io.ReadAll(response.Body)
		return fmt.Errorf("upload: %d %s", response.StatusCode, data)
	}
	var upload struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&upload); err != nil {
		return err
	}
	s.uploadID = upload.ID
	return nil
}
func (s *miniAppFilesFeature) run() error {
	response, err := s.post("/foxxycode/miniapps/"+s.appID+"/test-runs", map[string]any{"inputs": map[string]any{"source": map[string]any{"upload_id": s.uploadID}}})
	if err != nil {
		return err
	}
	if response.StatusCode != 202 {
		return fmt.Errorf("run: %d %s", response.StatusCode, response.Body)
	}
	var job miniapps.AsyncJob
	if err := json.Unmarshal([]byte(response.Body), &job); err != nil {
		return err
	}
	result, err := s.waitRunJob(job.ID, "succeeded")
	if err != nil {
		return err
	}
	s.runID, _ = result["run_id"].(string)
	return nil
}
func (s *miniAppFilesFeature) download() error {
	response, err := http.Get(s.ts.URL + "/foxxycode/miniapp-runs/" + s.runID)
	if err != nil {
		return err
	}
	var result struct {
		Files []struct {
			ID string `json:"id"`
		} `json:"files"`
	}
	err = json.NewDecoder(response.Body).Decode(&result)
	_ = response.Body.Close()
	if err != nil {
		return err
	}
	if len(result.Files) != 1 {
		return fmt.Errorf("files=%v", result.Files)
	}
	s.fileID = result.Files[0].ID
	return s.readFile()
}
func (s *miniAppFilesFeature) readFile() error {
	response, err := http.Get(s.ts.URL + "/foxxycode/miniapp-runs/" + s.runID + "/files/" + s.fileID)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode != 200 || !strings.Contains(string(data), "a real uploaded source") {
		return fmt.Errorf("download: %d %s", response.StatusCode, data)
	}
	return nil
}
func TestMiniAppFilesFeature(t *testing.T) {
	state := &miniAppFilesFeature{}
	defer state.close()
	suite := godog.TestSuite{Name: "miniapps-files", ScenarioInitializer: func(sc *godog.ScenarioContext) {
		sc.Step(`^a Mini App that reads an uploaded text file and writes a downloadable result$`, state.create)
		sc.Step(`^I upload a source file for its declared input$`, state.upload)
		sc.Step(`^I test the Mini App with that upload$`, state.run)
		sc.Step(`^the successful run exposes a downloadable file containing the source text$`, state.download)
		sc.Step(`^I can verify again without uploading the source a second time$`, func() error {
			response, err := state.post("/foxxycode/miniapps/"+state.appID+"/test-runs", map[string]any{"inputs": map[string]any{}})
			if err != nil {
				return err
			}
			if response.StatusCode != 202 {
				return fmt.Errorf("retest: %d %s", response.StatusCode, response.Body)
			}
			var job miniapps.AsyncJob
			if err := json.Unmarshal([]byte(response.Body), &job); err != nil {
				return err
			}
			_, err = state.waitRunJob(job.ID, "succeeded")
			return err
		})
		sc.Step(`^the file remains downloadable after the Mini Apps service restarts$`, func() error {
			state.ts.Close()
			state.srv.Drain()
			if err := state.boot(); err != nil {
				return err
			}
			return state.readFile()
		})
	}, Options: &godog.Options{Format: "pretty", Paths: []string{"../../features/miniapps_files.feature"}, TestingT: t}}
	if suite.Run() != 0 {
		t.Fatal("file workflow scenarios failed")
	}
}
