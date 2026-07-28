//go:build http && !miniapps

package httpserver

import "testing"

func TestMiniAppsOpenAPIIsAbsentWithoutTag(t *testing.T) {
	if miniAppsCapability() {
		t.Fatal("miniapps capability must be false without tag")
	}
	paths := openAPISpec()["paths"].(map[string]interface{})
	if _, ok := paths["/foxxycode/capabilities"]; !ok {
		t.Fatal("capability discovery path is missing")
	}
	if _, ok := paths["/foxxycode/miniapps"]; ok {
		t.Fatal("mini-app paths must not be linked without the miniapps tag")
	}
}
