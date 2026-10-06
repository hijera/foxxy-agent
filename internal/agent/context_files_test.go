package agent

import (
	"testing"

	"github.com/hijera/foxxycode-agent/internal/acp"
)

// The two surfaces spell a mention differently, and a path-scoped rule or skill
// is matched against whatever comes out of here. Reading only the file:// form
// left every such rule inactive everywhere except an editor speaking ACP: the
// catalog listed them and nothing ever activated them.
func TestExtractContextFilesReadsBothURIShapes(t *testing.T) {
	for _, tc := range []struct {
		name string
		uri  string
		want string
	}{
		{"acp sends a file URI", "file:///C:/proj/src/sample.go", "C:/proj/src/sample.go"},
		{"http sends the workspace-relative path", "src/sample.go", "src/sample.go"},
		{"a windows path is a path, not a scheme", `C:\proj\src\sample.go`, `C:\proj\src\sample.go`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := extractContextFiles([]acp.ContentBlock{{
				Type:     "resource",
				Resource: &acp.Resource{URI: tc.uri},
			}})
			if len(got) != 1 || got[0] != tc.want {
				t.Fatalf("extractContextFiles(%q) = %q, want [%q]", tc.uri, got, tc.want)
			}
		})
	}
}

// A resource that is not a file on disk names no context file: matching a rule
// against "https://example.com/x.go" would activate it on a page nobody opened.
func TestExtractContextFilesDropsNonFileURIs(t *testing.T) {
	for _, uri := range []string{
		"https://example.com/src/sample.go",
		"data:text/plain;base64,aGk=",
		"   ",
	} {
		got := extractContextFiles([]acp.ContentBlock{{
			Type:     "resource",
			Resource: &acp.Resource{URI: uri},
		}})
		if len(got) != 0 {
			t.Fatalf("extractContextFiles(%q) = %q, want none", uri, got)
		}
	}
}
