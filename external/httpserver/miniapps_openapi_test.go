//go:build http && miniapps

package httpserver

import "testing"

func TestMiniAppsOpenAPIIsLinkedWithTag(t *testing.T) {
	if !miniAppsCapability() {
		t.Fatal("miniapps capability must be true with tag")
	}
	paths := openAPISpec()["paths"].(map[string]interface{})
	for _, path := range []string{
		"/foxxycode/capabilities",
		"/foxxycode/sessions/{id}/miniapps/distill",
		"/foxxycode/miniapps",
		"/foxxycode/miniapps/{id}/draft",
		"/foxxycode/miniapps/{id}/expected-result",
		"/foxxycode/miniapps/{id}/versions/{version}/runs",
	} {
		if _, ok := paths[path]; !ok {
			t.Errorf("OpenAPI path %q is missing", path)
		}
	}
}
