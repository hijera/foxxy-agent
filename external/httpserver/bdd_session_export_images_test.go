//go:build http

package httpserver

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/export"
	"github.com/hijera/foxxycode-agent/internal/llm"
	"github.com/hijera/foxxycode-agent/internal/session"
)

func (s *sessionExportFeatureState) seedPictures() error {
	st, err := s.liveState()
	if err != nil {
		return err
	}
	assets := session.AssetsPath(st.GetPersistedSessionDir())
	if err := os.MkdirAll(assets, 0o755); err != nil {
		return err
	}
	for i, name := range []string{"upload.png", "tool.png"} {
		var data bytes.Buffer
		if err := png.Encode(&data, image.NewGray(image.Rect(0, 0, 16+i, 12))); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(assets, name), data.Bytes(), 0o644); err != nil {
			return err
		}
	}
	st.AddMessage(llm.Message{Role: llm.RoleUser, Content: exportUserQuestion,
		ImageParts: []llm.ImagePart{{Name: "upload.png", FilePath: "upload.png"}}})
	st.AddMessage(llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{
		ID: "read_picture", Name: "read", InputJSON: `{"path":"private-tool-argument"}`,
	}}})
	st.AddMessage(llm.Message{Role: llm.RoleTool, ToolCallID: "read_picture", Content: "private-tool-output",
		ImageParts: []llm.ImagePart{{Name: "tool.png", FilePath: "tool.png"}}})
	st.AddMessage(llm.Message{Role: llm.RoleAssistant, Content: exportAssistantAnswer})
	return nil
}

func (s *sessionExportFeatureState) exportIllustrated(format, delivery string) error {
	if delivery == "download" {
		return s.exportChat(format)
	}
	if err := s.exportToFile(format); err != nil {
		return err
	}
	if s.respStatus != 200 {
		return fmt.Errorf("file export status %d: %s", s.respStatus, s.respBody)
	}
	body, err := os.ReadFile(s.exportPath)
	s.respBody = body
	f, _ := export.ParseExportFormat(format)
	s.respHeaders.Set("Content-Type", f.ContentType())
	return err
}

func (s *sessionExportFeatureState) embedsBothPictures() error {
	if s.respStatus != 200 {
		return fmt.Errorf("export status %d: %s", s.respStatus, s.respBody)
	}
	count := bytes.Count(s.respBody, []byte("data:image/png;base64,"))
	if bytes.HasPrefix(s.respBody, []byte("%PDF")) {
		count = bytes.Count(s.respBody, []byte("/Subtype /Image"))
	} else if bytes.HasPrefix(s.respBody, []byte("PK")) {
		z, err := zip.NewReader(bytes.NewReader(s.respBody), int64(len(s.respBody)))
		if err != nil {
			return err
		}
		for _, f := range z.File {
			if strings.HasPrefix(f.Name, "word/media/") {
				count++
			}
		}
	}
	if count != 2 {
		return fmt.Errorf("export embeds %d pictures, want both upload and tool picture", count)
	}
	return nil
}

func (s *sessionExportFeatureState) referencesBothPictures() error {
	var doc export.ExportDocument
	if err := json.Unmarshal(s.respBody, &doc); err != nil {
		return err
	}
	var names []string
	for _, e := range doc.Entries {
		for _, a := range e.Attachments {
			names = append(names, a.Name)
		}
	}
	if strings.Join(names, ",") != "upload.png,tool.png" {
		return fmt.Errorf("picture references %v, want upload.png and tool.png", names)
	}
	return nil
}

func (s *sessionExportFeatureState) hidesPictureToolDetails() error {
	text, err := s.exportedText()
	if err != nil {
		return err
	}
	for _, detail := range []string{"private-tool-argument", "private-tool-output"} {
		if strings.Contains(text, detail) {
			return fmt.Errorf("dialogue export leaked %s", detail)
		}
	}
	return nil
}
