//go:build http && miniapps

package httpserver

import (
	"testing"
)

func TestMiniAppsOpenAPIListsRegisteredSurface(t *testing.T) {
	doc := openAPISpec()
	paths, ok := doc["paths"].(map[string]interface{})
	if !ok {
		t.Fatal("OpenAPI paths missing")
	}
	want := map[string]string{
		"/foxxycode/capabilities":                            "get",
		"/foxxycode/sessions/{id}/miniapps/distill":          "post",
		"/foxxycode/miniapp-distillations/{job_id}":          "get",
		"/foxxycode/miniapp-distillations/{job_id}/events":   "get",
		"/foxxycode/miniapp-distillations/{job_id}/scenario": "post",
		"/foxxycode/miniapps":                                "get",
		"/foxxycode/miniapps/{id}":                           "patch",
		"/foxxycode/miniapps/{id}/versions/{version}":        "get",
		"/foxxycode/miniapps/{id}/draft":                     "put",
		"/foxxycode/miniapps/{id}/authoring/source":          "get",
		"/foxxycode/miniapps/{id}/validate":                  "post",
		"/foxxycode/miniapps/{id}/sanitize":                  "post",
		"/foxxycode/miniapps/{id}/release":                   "post",
		"/foxxycode/miniapps/{id}/runs":                      "get",
		"/foxxycode/miniapp-runs/{run_id}":                   "get",
		"/foxxycode/miniapp-runs/{run_id}/events":            "get",
	}
	for path, method := range want {
		item, ok := paths[path].(map[string]interface{})
		if !ok {
			t.Errorf("OpenAPI path %s missing", path)
			continue
		}
		if _, ok := item[method]; !ok {
			t.Errorf("OpenAPI path %s missing %s", path, method)
		}
	}
	components, _ := doc["components"].(map[string]interface{})
	schemas, _ := components["schemas"].(map[string]interface{})
	for _, name := range []string{"MiniApp", "MiniAppJob", "MiniAppRun", "MiniAppValidationReport"} {
		if _, ok := schemas[name]; !ok {
			t.Errorf("OpenAPI schema %s missing", name)
		}
	}
}
