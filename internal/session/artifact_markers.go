package session

import (
	"regexp"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/llm"
)

var foxxycodeFileMarker = regexp.MustCompile(`(?is)<foxxycode_file\s+id\s*=\s*(?:"[^"]*"|'[^']*')\s*/\s*>`)
var safeArtifactMarkerID = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// StripArtifactMarkers removes UI-only placement markers before history leaves
// the server. The UI associates markers only with the artifacts persisted on
// the same assistant message; markers are never model instructions.
func StripArtifactMarkers(content string) string {
	if !foxxycodeFileMarker.MatchString(content) {
		return content
	}
	return strings.TrimSpace(WithoutArtifactMarkers(content))
}

// WithoutArtifactMarkers removes UI placements without normalizing surrounding
// whitespace, which may carry Markdown indentation in a document export.
func WithoutArtifactMarkers(content string) string {
	return foxxycodeFileMarker.ReplaceAllString(content, "")
}

// ArtifactMarkers renders placement markers for already-verified artifact
// associations. Callers must never pass client or model-supplied ids here.
func ArtifactMarkers(artifacts []llm.Artifact) string {
	markers := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		if safeArtifactMarkerID.MatchString(artifact.ID) {
			markers = append(markers, `<foxxycode_file id="`+artifact.ID+`"/>`)
		}
	}
	return strings.Join(markers, "\n")
}

func placePendingArtifacts(msgs []llm.Message, assistant *llm.Message) {
	if assistant.Role != llm.RoleAssistant {
		return
	}
	assistant.Content = StripArtifactMarkers(assistant.Content)
	if assistant.Content == "" {
		return
	}
	placed := make(map[string]struct{})
	for _, msg := range msgs {
		if msg.Role != llm.RoleAssistant {
			continue
		}
		for _, artifact := range msg.Artifacts {
			if artifact.ID != "" {
				placed[artifact.ID] = struct{}{}
			}
		}
	}
	for _, artifact := range assistant.Artifacts {
		if artifact.ID != "" {
			placed[artifact.ID] = struct{}{}
		}
	}
	var pending []llm.Artifact
	for _, msg := range msgs {
		if msg.Role != llm.RoleTool {
			continue
		}
		for _, artifact := range msg.Artifacts {
			if artifact.ID == "" {
				continue
			}
			if _, ok := placed[artifact.ID]; ok {
				continue
			}
			placed[artifact.ID] = struct{}{}
			pending = append(pending, artifact)
		}
	}
	if len(pending) == 0 {
		return
	}
	assistant.Artifacts = append(assistant.Artifacts, pending...)
	assistant.Content = strings.TrimSpace(assistant.Content) + "\n\n" + ArtifactMarkers(pending)
}
