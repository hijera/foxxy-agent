//go:build http

package httpserver

import (
	"testing"

	"github.com/hijera/foxxycode-agent/internal/llm"
)

func TestLlmMsgsToFoxxyCodeOpenAIIncludesCreatedAt(t *testing.T) {
	out := llmMsgsToFoxxyCodeOpenAI([]llm.Message{
		{Role: llm.RoleUser, Content: "u", CreatedAt: "2026-05-01T12:00:00Z"},
		{Role: llm.RoleAssistant, Content: "a", CreatedAt: "2026-05-01T12:00:01Z"},
	})
	if len(out) != 2 {
		t.Fatalf("len=%d", len(out))
	}
	if got, _ := out[0]["created_at"].(string); got != "2026-05-01T12:00:00Z" {
		t.Fatalf("user created_at: %#v", out[0])
	}
	if got, _ := out[1]["created_at"].(string); got != "2026-05-01T12:00:01Z" {
		t.Fatalf("assistant created_at: %#v", out[1])
	}
}

func TestLlmMsgsToFoxxyCodeOpenAIOmitsEmptyCreatedAt(t *testing.T) {
	out := llmMsgsToFoxxyCodeOpenAI([]llm.Message{
		{Role: llm.RoleUser, Content: "u"},
	})
	if _, ok := out[0]["created_at"]; ok {
		t.Fatalf("expected no created_at, got %#v", out[0])
	}
}

func TestLlmMsgsToFoxxyCodeOpenAIForSessionIncludesPersistedFilePreview(t *testing.T) {
	out := llmMsgsToFoxxyCodeOpenAIForSession("sess_files", []llm.Message{
		{
			Role:    llm.RoleUser,
			Content: "look",
			ImageParts: []llm.ImagePart{{
				DataURL:       "data:image/png;base64,abc",
				Name:          "photo.png",
				FilePath:      `/tmp/sessions/sess_files/assets/photo.png`,
				ThumbnailPath: `/tmp/sessions/sess_files/assets/thumbnails/photo.png.png`,
			}},
		},
	})
	files, ok := out[0]["files"].([]map[string]interface{})
	if !ok || len(files) != 1 {
		t.Fatalf("files: %#v", out[0]["files"])
	}
	if got := files[0]["name"]; got != "photo.png" {
		t.Fatalf("name = %#v", got)
	}
	if got := files[0]["mime_type"]; got != "image/png" {
		t.Fatalf("mime_type = %#v", got)
	}
	if got := files[0]["preview_url"]; got != "/foxxycode/sessions/sess_files/assets/photo.png/thumbnail" {
		t.Fatalf("preview_url = %#v", got)
	}
}
