//go:build miniapps

package miniapps

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A declared host must not be able to redirect the run onto an undeclared one.
func TestAPIStepRefusesRedirectToUndeclaredHost(t *testing.T) {
	t.Parallel()

	undeclared := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"leaked":true}`))
	}))
	defer undeclared.Close()

	declared := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, undeclared.URL+"/secret", http.StatusFound)
	}))
	defer declared.Close()

	app := greetingApp()
	app.Inputs = nil
	app.Permissions = Permissions{Network: []NetworkPermission{
		{Host: strings.TrimPrefix(declared.URL, "http://"), Methods: []string{"GET"}},
	}}
	app.Workflow = []Step{{
		ID: "fetch", Kind: "api", Title: "Fetch",
		Request: &APIRequest{Method: "GET", URL: declared.URL + "/start"},
	}}
	app.Success = SuccessSpec{Mode: "all", Checks: []SuccessCheck{{
		Kind: "step", Step: "fetch", Status: "succeeded",
	}}}
	app.Outputs = nil

	runner := NewRunner(NewStore(t.TempDir()), nil)
	run, err := runner.RunPortable(context.Background(), app, nil, nil)
	if err == nil {
		t.Fatalf("redirect to an undeclared host was followed: %+v", run.Steps)
	}
	if !strings.Contains(err.Error(), "not declared in permissions") {
		t.Fatalf("err = %v, want a permission error", err)
	}
}

// Runtime provisioning is pinned by size and SHA-256 rather than by the app's
// network permissions, so its downloads must keep following redirects.
func TestRuntimeDownloadRedirectIsNotGatedByAppPermissions(t *testing.T) {
	t.Parallel()

	runner := NewRunner(NewStore(t.TempDir()), nil)
	request, err := http.NewRequest(http.MethodGet, "https://cdn.example.com/runtime.zip", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.httpClient.CheckRedirect(request, nil); err != nil {
		t.Fatalf("redirect without an attached policy was refused: %v", err)
	}
}

// A redirect that stays on a declared host is still followed.
func TestAPIStepFollowsRedirectWithinDeclaredHost(t *testing.T) {
	t.Parallel()

	var declared *httptest.Server
	declared = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, declared.URL+"/final", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer declared.Close()

	app := greetingApp()
	app.Inputs = nil
	app.Permissions = Permissions{Network: []NetworkPermission{
		{Host: strings.TrimPrefix(declared.URL, "http://"), Methods: []string{"GET"}},
	}}
	app.Workflow = []Step{{
		ID: "fetch", Kind: "api", Title: "Fetch",
		Request: &APIRequest{Method: "GET", URL: declared.URL + "/start"},
	}}
	app.Success = SuccessSpec{Mode: "all", Checks: []SuccessCheck{{
		Kind: "step", Step: "fetch", Status: "succeeded",
	}}}
	app.Outputs = nil

	runner := NewRunner(NewStore(t.TempDir()), nil)
	if _, err := runner.RunPortable(context.Background(), app, nil, nil); err != nil {
		t.Fatalf("same-host redirect was refused: %v", err)
	}
}
