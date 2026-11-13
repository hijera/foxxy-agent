//go:build miniapps

package miniapps

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestUploadedInputIsCopiedAndDeclaredResultSurvivesReload(t *testing.T) {
	store := NewStore(t.TempDir())
	app := coreValidApp()
	app.Inputs[0].Validation = Validation{Extensions: []string{".txt"}}
	app.Outputs = []Output{{ID: "download", Type: "file", Value: "result.txt", Renderer: "download"}}
	if err := store.CreateDraft(app, &SourceEvidence{}); err != nil {
		t.Fatal(err)
	}
	app, _ = store.GetDraft(app.ID)
	upload, err := store.PutUpload(app, "source", "hello world.txt", bytes.NewBufferString("uploaded contents"))
	if err != nil {
		t.Fatal(err)
	}
	runner := NewRunner(store, Executors{Tool: &runnerTestTool{fn: func(_ context.Context, req ToolRequest) (any, error) {
		args := req.Arguments.(map[string]any)
		data, err := os.ReadFile(filepath.Join(req.Workspace, args["path"].(string)))
		if err != nil {
			return nil, err
		}
		return "done", os.WriteFile(filepath.Join(req.Workspace, "result.txt"), data, 0o600)
	}}})
	run, err := runner.RunPortable(context.Background(), app, map[string]any{"source": map[string]any{"upload_id": upload.ID}}, nil)
	if err != nil || run.Status != RunSucceeded {
		t.Fatalf("run=%+v err=%v", run, err)
	}
	if len(run.Files) != 1 || run.Files[0].Name != "result.txt" {
		t.Fatalf("files=%+v", run.Files)
	}
	reloaded := NewStore(store.Root())
	file, _, err := reloaded.OpenRunFile(run.ID, run.Files[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(file)
	if err != nil || string(data) != "uploaded contents" {
		t.Fatalf("data=%q err=%v", data, err)
	}
}

func TestUploadRejectsWrongInputAndChangedRevision(t *testing.T) {
	store := NewStore(t.TempDir())
	app := coreValidApp()
	if _, err := store.PutUpload(app, "unknown", "a.txt", bytes.NewBufferString("a")); err == nil {
		t.Fatal("unknown input accepted")
	}
	app.Revision = "revision-one"
	upload, err := store.PutUpload(app, "source", "a.txt", bytes.NewBufferString("a"))
	if err != nil {
		t.Fatal(err)
	}
	app.Revision = "revision-two"
	if _, err := store.UploadForInput(app, app.Inputs[0], upload.ID); err == nil {
		t.Fatal("stale upload accepted")
	}
	if _, err := store.PutUpload(app, "source", "../a.txt", bytes.NewBufferString("a")); err == nil {
		t.Fatal("unsafe filename accepted")
	}
}

func TestUploadEnforcesContentAndSizeAndResultRejectsTampering(t *testing.T) {
	store := NewStore(t.TempDir())
	app := coreValidApp()
	maximum := int64(3)
	app.Inputs[0].Validation.MaxTotalBytes = &maximum
	if _, err := store.PutUpload(app, "source", "data.txt", bytes.NewBufferString("1234")); !errors.Is(err, ErrUploadTooLarge) {
		t.Fatalf("size err=%v", err)
	}
	app.Inputs[0].Validation = Validation{MediaTypes: []string{"image/png"}}
	if _, err := store.PutUpload(app, "source", "fake.png", bytes.NewBufferString("not an image")); err == nil {
		t.Fatal("declared image accepted non-image content")
	}
	app.Inputs[0].Validation = Validation{}
	app.Outputs = []Output{{ID: "download", Type: "file", Value: "result.txt"}}
	runner := NewRunner(store, Executors{Tool: &runnerTestTool{fn: func(_ context.Context, req ToolRequest) (any, error) {
		return "ok", os.WriteFile(filepath.Join(req.Workspace, "result.txt"), []byte("original"), 0o600)
	}}})
	run, err := runner.RunPortable(context.Background(), app, map[string]any{"source": "unused.txt"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(filepath.Dir(run.LogPath), "workspace", "result.txt")
	if err := os.WriteFile(path, []byte("modified"), 0o600); err != nil {
		t.Fatal(err)
	}
	if file, _, err := store.OpenRunFile(run.ID, run.Files[0].ID); err == nil {
		_ = file.Close()
		t.Fatal("changed result accepted")
	}
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if file, _, err := store.OpenRunFile(run.ID, run.Files[0].ID); err == nil {
		_ = file.Close()
		t.Fatal("linked result accepted")
	}
}

func TestVerificationUploadSurvivesStagingRemoval(t *testing.T) {
	store := NewStore(t.TempDir())
	app := coreValidApp()
	if err := store.CreateDraft(app, &SourceEvidence{}); err != nil {
		t.Fatal(err)
	}
	app, _ = store.GetDraft(app.ID)
	upload, err := store.PutUpload(app, "source", "source.txt", bytes.NewBufferString("private source fixture"))
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string]any{"source": map[string]any{"upload_id": upload.ID}}
	if err := store.RecordFileInputs(app, inputs); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteUpload(app.ID, upload.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(store.Root()).UploadForInput(app, app.Inputs[0], upload.ID); err != nil {
		t.Fatalf("pinned fixture lost: %v", err)
	}
}
