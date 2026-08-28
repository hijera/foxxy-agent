//go:build browser

package browser

import (
	"context"
	"strings"
	"testing"
)

// longToken stands in for a credential an app parks in localStorage. It exists to
// prove the report truncates: seeing that the key is set is the point, pasting a
// whole JWT into the model's context on every look is not.
const longToken = "eyJhbGciOiJIUzI1NiJ9.aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.bbbbbbbb"

// TestInspectStorageReportsEveryStore covers the case the tool exists for: an app
// behaving oddly because of what is in its storage, which no screenshot can show.
func TestInspectStorageReportsEveryStore(t *testing.T) {
	m := newTestManager(t)
	env := testEnv(t, "inspect-storage")
	defer m.closeSession("inspect-storage")
	srv := diagnosticServer(t)
	navigateTo(t, m, env, srv.URL)

	out, err := m.executeInspect(context.Background(), `{"what":"storage"}`, env)
	if err != nil {
		t.Fatalf("inspect storage: %v", err)
	}
	for _, want := range []string{"auth_token", "theme", "dark", "draft", "unsent message", "visitor"} {
		if !strings.Contains(out, want) {
			t.Errorf("storage report is missing %q; got:\n%s", want, out)
		}
	}
	// A long credential must be truncated: the point is to see that it is set,
	// not to paste a whole JWT into the model's context on every look.
	if strings.Contains(out, longToken) {
		t.Errorf("a long value was reported in full; it must be truncated:\n%s", out)
	}
	if strings.Contains(out, "screenshot") {
		t.Errorf("inspect captured a screenshot; it must not:\n%s", out)
	}
}

// TestInspectTimingReportsLoadPhases is the "why is this slow" answer.
func TestInspectTimingReportsLoadPhases(t *testing.T) {
	m := newTestManager(t)
	env := testEnv(t, "inspect-timing")
	defer m.closeSession("inspect-timing")
	srv := diagnosticServer(t)
	navigateTo(t, m, env, srv.URL)

	out, err := m.executeInspect(context.Background(), `{"what":"timing"}`, env)
	if err != nil {
		t.Fatalf("inspect timing: %v", err)
	}
	for _, want := range []string{"ttfb", "dom_content_loaded", "load"} {
		if !strings.Contains(out, want) {
			t.Errorf("timing report is missing %q; got:\n%s", want, out)
		}
	}
}

// TestInspectMemoryDegradesHonestly: performance.memory is Chrome-only and absent
// in some configurations, so the tool must say so rather than report zeroes that
// read like a measurement.
func TestInspectMemoryReportsHeapOrSaysWhyNot(t *testing.T) {
	m := newTestManager(t)
	env := testEnv(t, "inspect-memory")
	defer m.closeSession("inspect-memory")
	srv := diagnosticServer(t)
	navigateTo(t, m, env, srv.URL)

	out, err := m.executeInspect(context.Background(), `{"what":"memory"}`, env)
	if err != nil {
		t.Fatalf("inspect memory: %v", err)
	}
	// DOM node count is always available and is the more actionable number.
	if !strings.Contains(out, "dom_nodes") {
		t.Errorf("memory report is missing the DOM node count; got:\n%s", out)
	}
	if !strings.Contains(out, "js_heap") && !strings.Contains(out, "unavailable") {
		t.Errorf("memory report neither reports the heap nor says it is unavailable; got:\n%s", out)
	}
}

// TestInspectRejectsUnknownSubject keeps the enum honest instead of silently
// reporting the wrong thing.
func TestInspectRejectsUnknownSubject(t *testing.T) {
	m := newTestManager(t)
	env := testEnv(t, "inspect-bad")
	defer m.closeSession("inspect-bad")

	out, err := m.executeInspect(context.Background(), `{"what":"cpu"}`, env)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if !strings.HasPrefix(out, "error:") {
		t.Errorf("unknown subject was accepted; got: %s", out)
	}
	for _, want := range []string{"storage", "timing", "memory"} {
		if !strings.Contains(out, want) {
			t.Errorf("the error does not name the valid subjects; got: %s", out)
		}
	}
}
