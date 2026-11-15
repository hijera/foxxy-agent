//go:build http && !miniapps

package httpserver

import "testing"

func TestMiniAppsOpenAPIStubAdvertisesCapabilityOnly(t *testing.T) {
	doc := openAPISpec()
	paths, _ := doc["paths"].(map[string]interface{})
	if _, ok := paths["/foxxycode/capabilities"]; !ok {
		t.Fatal("capability endpoint missing from lean OpenAPI")
	}
	for path := range paths {
		if path == "/foxxycode/capabilities" {
			continue
		}
		if len(path) >= len("/foxxycode/miniapps") && path[:len("/foxxycode/miniapps")] == "/foxxycode/miniapps" ||
			len(path) >= len("/foxxycode/miniapp-") && path[:len("/foxxycode/miniapp-")] == "/foxxycode/miniapp-" {
			t.Fatalf("Mini Apps lifecycle path leaked into lean OpenAPI: %s", path)
		}
	}
	components, _ := doc["components"].(map[string]interface{})
	schemas, _ := components["schemas"].(map[string]interface{})
	if _, ok := schemas["Capabilities"]; !ok {
		t.Fatal("capability schema missing from lean OpenAPI")
	}
}
