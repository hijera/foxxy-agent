//go:build browser

package browser

import (
	"context"
	"strings"
	"testing"
)

func TestNetworkJournalListsRequestsAndServesBodies(t *testing.T) {
	m := newTestManager(t)
	env := testEnv(t, "network")
	defer m.closeSession("network")
	srv := interactionsServer(t)
	navigateTo(t, m, env, srv.URL)

	list := runTool(t, "network requests", m.executeNetwork, `{"what":"requests"}`, env)
	if !strings.Contains(list, "GET 200 "+srv.URL) {
		t.Fatalf("request journal is missing the page load:\n%s", list)
	}

	id := requestIDFor(t, list, srv.URL+"/")
	body := runTool(t, "network body", m.executeNetwork, `{"what":"body","id":"`+id+`"}`, env)
	if !strings.Contains(body, "Second page") && !strings.Contains(body, "id=\"send\"") {
		t.Errorf("response body does not look like the served page:\n%s", firstN(body, 300))
	}

	res, _ := m.executeNetwork(context.Background(), `{"what":"body","id":"no-such-id"}`, env)
	if !strings.HasPrefix(res, "error:") {
		t.Errorf("body of an unknown id = %q, want error", res)
	}
}

// requestIDFor pulls the id= token from the journal line naming url.
func requestIDFor(t *testing.T, list, url string) string {
	t.Helper()
	for _, line := range strings.Split(list, "\n") {
		if !strings.Contains(line, url) {
			continue
		}
		_, id, ok := strings.Cut(line, "id=")
		if !ok {
			continue
		}
		return strings.TrimSpace(id)
	}
	t.Fatalf("no journal line names %q:\n%s", url, list)
	return ""
}

func TestRouteMockServesACannedResponse(t *testing.T) {
	m := newTestManager(t)
	env := testEnv(t, "route")
	defer m.closeSession("route")
	srv := interactionsServer(t)
	navigateTo(t, m, env, srv.URL)

	// The real server 500s under /api/. The mock must win.
	runTool(t, "mock", m.executeRoute,
		`{"action":"mock","url_substring":"/api/data","status":200,"content_type":"application/json","body":"{\"answer\":42}"}`, env)
	list := runTool(t, "route list", m.executeRoute, `{"action":"list"}`, env)
	if !strings.Contains(list, "/api/data") {
		t.Errorf("route list does not name the mock:\n%s", list)
	}

	navigateTo(t, m, env, srv.URL+"/api/data")
	if got := evalString(t, m, env, `document.body.innerText`); !strings.Contains(got, "42") {
		t.Errorf("mocked response did not reach the page; body = %q", got)
	}

	runTool(t, "unroute", m.executeRoute, `{"action":"unroute"}`, env)
	navigateTo(t, m, env, srv.URL+"/api/data")
	// With the mock gone the real 500 is back; the page log reports it.
	logRes, _ := m.executePageLog(context.Background(), "{}", env)
	if !strings.Contains(logRes, "500") {
		t.Errorf("after unroute the real 500 was not observed:\n%s", logRes)
	}
}

func TestRouteOfflineTogglesTheNetwork(t *testing.T) {
	m := newTestManager(t)
	env := testEnv(t, "offline")
	defer m.closeSession("offline")
	srv := interactionsServer(t)
	navigateTo(t, m, env, srv.URL)

	runTool(t, "offline", m.executeRoute, `{"action":"offline"}`, env)
	if got := evalString(t, m, env, `String(navigator.onLine)`); got != "false" {
		t.Errorf("navigator.onLine while offline = %q, want false", got)
	}
	runTool(t, "online", m.executeRoute, `{"action":"online"}`, env)
	if got := evalString(t, m, env, `String(navigator.onLine)`); got != "true" {
		t.Errorf("navigator.onLine after restore = %q, want true", got)
	}
}

func TestStorageSetWritesEveryScope(t *testing.T) {
	m := newTestManager(t)
	env := testEnv(t, "storageset")
	defer m.closeSession("storageset")
	srv := interactionsServer(t)
	navigateTo(t, m, env, srv.URL)

	runTool(t, "set local", m.executeStorageSet, `{"scope":"local","action":"set","key":"theme","value":"dark"}`, env)
	runTool(t, "set session", m.executeStorageSet, `{"scope":"session","action":"set","key":"draft","value":"wip"}`, env)
	runTool(t, "set cookie", m.executeStorageSet, `{"scope":"cookie","action":"set","key":"flavor","value":"vanilla"}`, env)

	report := runTool(t, "inspect storage", m.executeInspect, `{"what":"storage"}`, env)
	for _, want := range []string{"theme = dark", "draft = wip", "flavor = vanilla"} {
		if !strings.Contains(report, want) {
			t.Errorf("storage report is missing %q:\n%s", want, report)
		}
	}

	runTool(t, "delete local", m.executeStorageSet, `{"scope":"local","action":"delete","key":"theme"}`, env)
	runTool(t, "clear session", m.executeStorageSet, `{"scope":"session","action":"clear"}`, env)
	report = runTool(t, "inspect storage after delete", m.executeInspect, `{"what":"storage"}`, env)
	if strings.Contains(report, "theme = dark") {
		t.Errorf("deleted key still reported:\n%s", report)
	}
	if !strings.Contains(report, "sessionStorage: empty") {
		t.Errorf("cleared sessionStorage still has entries:\n%s", report)
	}

	res, _ := m.executeStorageSet(context.Background(), `{"scope":"local","action":"set"}`, env)
	if !strings.HasPrefix(res, "error:") {
		t.Errorf("set without a key = %q, want error", res)
	}
}
