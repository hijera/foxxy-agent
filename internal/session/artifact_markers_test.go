package session

import (
	"testing"

	"github.com/hijera/foxxycode-agent/internal/llm"
)

func TestArtifactMarkersSkipsUnsafeIDs(t *testing.T) {
	markers := ArtifactMarkers([]llm.Artifact{
		{ID: "safe_123-abc"},
		{ID: `unsafe"/>`},
	})
	if markers != `<foxxycode_file id="safe_123-abc"/>` {
		t.Fatalf("markers = %q", markers)
	}
}
