//go:build browser

package browser

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/tooling"
)

const testPageHTML = `<html><body>
<input id="name">
<button id="go" onclick="document.getElementById('out').textContent='clicked'">Go</button>
<div id="out"></div>
<div id="far" style="margin-top:3000px">far away</div>
</body></html>`

func testEnv(t *testing.T, id string) *tooling.Env {
	t.Helper()
	return &tooling.Env{
		SessionID:    id,
		SessionDir:   t.TempDir(),
		AddToolImage: func(_, _, _ string) {},
	}
}

func mustNotError(t *testing.T, label, res string) {
	t.Helper()
	if strings.HasPrefix(res, "error:") {
		t.Fatalf("%s: %s", label, res)
	}
}

func TestBrowserActionsRoundTrip(t *testing.T) {
	m := newTestManager(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(testPageHTML))
	}))
	defer srv.Close()

	env := testEnv(t, "actions")
	defer m.closeSession("actions")
	ctx := context.Background()

	nav, _ := json.Marshal(navigateArgs{URL: srv.URL})
	res, err := m.executeNavigate(ctx, string(nav), env)
	if err != nil {
		t.Fatal(err)
	}
	mustNotError(t, "navigate", res)

	// fill then read the value back via evaluate
	fillJSON, _ := json.Marshal(fillArgs{Selector: "#name", Text: "abc"})
	res, _ = m.executeFill(ctx, string(fillJSON), env)
	mustNotError(t, "fill", res)

	evalJSON, _ := json.Marshal(evaluateArgs{Expression: "document.getElementById('name').value"})
	res, _ = m.executeEvaluate(ctx, string(evalJSON), env)
	mustNotError(t, "evaluate", res)
	if !strings.Contains(res, `"abc"`) {
		t.Errorf("evaluate value = %q, want to contain \"abc\"", res)
	}

	// click the button and confirm the DOM changed
	clickJSON, _ := json.Marshal(selectorArgs{Selector: "#go"})
	res, _ = m.executeClick(ctx, string(clickJSON), env)
	mustNotError(t, "click", res)

	evalOut, _ := json.Marshal(evaluateArgs{Expression: "document.getElementById('out').textContent"})
	res, _ = m.executeEvaluate(ctx, string(evalOut), env)
	mustNotError(t, "evaluate out", res)
	if !strings.Contains(res, `"clicked"`) {
		t.Errorf("after click, out = %q, want \"clicked\"", res)
	}

	// scroll to a far element
	scrollJSON, _ := json.Marshal(scrollArgs{Selector: "#far"})
	res, _ = m.executeScroll(ctx, string(scrollJSON), env)
	mustNotError(t, "scroll", res)

	// hover over the button
	hoverJSON, _ := json.Marshal(selectorArgs{Selector: "#go"})
	res, _ = m.executeHover(ctx, string(hoverJSON), env)
	mustNotError(t, "hover", res)

	// explicit screenshot
	res, _ = m.executeScreenshot(ctx, "{}", env)
	mustNotError(t, "screenshot", res)

	// close
	res, _ = m.executeClose(ctx, "{}", env)
	if !strings.Contains(res, "closed") {
		t.Errorf("close result = %q", res)
	}
}

func TestActionsReturnErrorForMissingSelector(t *testing.T) {
	m := NewManager(nil)
	ctx := context.Background()
	env := &tooling.Env{SessionID: "x"}

	empty, _ := json.Marshal(selectorArgs{Selector: "  "})
	if res, _ := m.executeClick(ctx, string(empty), env); !strings.HasPrefix(res, "error:") {
		t.Errorf("click empty selector = %q, want error", res)
	}
	fillEmpty, _ := json.Marshal(fillArgs{Selector: "", Text: "x"})
	if res, _ := m.executeFill(ctx, string(fillEmpty), env); !strings.HasPrefix(res, "error:") {
		t.Errorf("fill empty selector = %q, want error", res)
	}
}
