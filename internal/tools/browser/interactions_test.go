//go:build browser

package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// interactionsHTML exercises every interaction the new tools drive: a form that
// submits on Enter, a dropdown with a change listener, a confirm() dialog, a
// file input, late-arriving content, a coordinates-only button, and a
// mouse-event drag handle.
const interactionsHTML = `<html><body>
<form id="f"><input id="q"><button id="send" type="submit">Send</button></form>
<div id="submitted"></div>
<script>document.getElementById('f').addEventListener('submit', function(e){
  e.preventDefault();
  document.getElementById('submitted').textContent = 'sent:' + document.getElementById('q').value;
});</script>
<select id="pet"><option value="">--</option><option value="cat">Cat</option><option value="dog">Dog</option></select>
<div id="petout"></div>
<script>document.getElementById('pet').addEventListener('change', function(e){
  document.getElementById('petout').textContent = e.target.value;
});</script>
<button id="ask">Ask</button><div id="verdict"></div>
<script>document.getElementById('ask').addEventListener('click', function(){
  document.getElementById('verdict').textContent = confirm('Proceed?') ? 'yes' : 'no';
});</script>
<input type="file" id="up"><div id="upname"></div>
<script>document.getElementById('up').addEventListener('change', function(e){
  document.getElementById('upname').textContent = e.target.files.length ? e.target.files[0].name : '';
});</script>
<div id="late"></div>
<script>setTimeout(function(){ document.getElementById('late').textContent = 'late content arrived'; }, 400);</script>
<input id="first"><textarea id="notes"></textarea><input type="checkbox" id="agree">
<button id="xy">XY</button><div id="xyout"></div>
<script>document.getElementById('xy').addEventListener('click', function(){
  document.getElementById('xyout').textContent = 'xy-clicked';
});</script>
<div id="dragout"></div>
<div id="handle" style="position:absolute;left:20px;top:400px;width:30px;height:30px;background:#c00"></div>
<div id="dropzone" style="position:absolute;left:200px;top:400px;width:60px;height:60px;background:#0c0"></div>
<script>
var dragging = false;
document.getElementById('handle').addEventListener('mousedown', function(){ dragging = true; });
document.addEventListener('mouseup', function(e){
  if (!dragging) return;
  dragging = false;
  var z = document.getElementById('dropzone').getBoundingClientRect();
  if (e.clientX >= z.left && e.clientX <= z.right && e.clientY >= z.top && e.clientY <= z.bottom) {
    document.getElementById('dragout').textContent = 'dropped';
  }
});
</script>
<a id="tolink" href="/second">Second page</a>
</body></html>`

const secondPageHTML = `<html><body><h1>Second page</h1></body></html>`

// interactionsServer serves the interaction page, a second page, and a broken
// API prefix that only a route mock can make succeed.
func interactionsServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/"):
			w.WriteHeader(http.StatusInternalServerError)
		case r.URL.Path == "/second":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(secondPageHTML))
		default:
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(interactionsHTML))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// evalString runs an expression and returns its string value, unwrapping the
// "result: " prefix and the JSON encoding of the evaluate tool.
func evalString(t *testing.T, m *Manager, env *tooling.Env, expr string) string {
	t.Helper()
	args, _ := json.Marshal(evaluateArgs{Expression: expr})
	res, err := m.executeEvaluate(context.Background(), string(args), env)
	if err != nil {
		t.Fatalf("evaluate %q: %v", expr, err)
	}
	raw, ok := strings.CutPrefix(res, "result: ")
	if !ok {
		t.Fatalf("evaluate %q returned %q", expr, res)
	}
	var s string
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		// Not a JSON string (a number or bool); return the raw text.
		return raw
	}
	return s
}

func runTool(t *testing.T, label string, exec func(context.Context, string, *tooling.Env) (string, error), argsJSON string, env *tooling.Env) string {
	t.Helper()
	res, err := exec(context.Background(), argsJSON, env)
	if err != nil {
		t.Fatalf("%s: %v", label, err)
	}
	mustNotError(t, label, res)
	return res
}

func TestPressKeySubmitsAndWaitForSeesLateContent(t *testing.T) {
	m := newTestManager(t)
	env := testEnv(t, "keys")
	defer m.closeSession("keys")
	srv := interactionsServer(t)
	navigateTo(t, m, env, srv.URL)

	runTool(t, "fill", m.executeFill, `{"selector":"#q","text":"abc"}`, env)
	runTool(t, "press enter", m.executePressKey, `{"key":"Enter","selector":"#q"}`, env)
	if got := evalString(t, m, env, `document.getElementById('submitted').textContent`); got != "sent:abc" {
		t.Errorf("after Enter, submitted = %q, want \"sent:abc\"", got)
	}

	res := runTool(t, "wait_for", m.executeWaitFor, `{"text":"late content arrived","seconds":10}`, env)
	if !strings.Contains(res, "condition met") {
		t.Errorf("wait_for did not report the condition: %s", res)
	}

	// A wrong expectation must time out with an error, not hang.
	timedOut, err := m.executeWaitFor(context.Background(), `{"text":"never appears","seconds":1}`, env)
	if err != nil {
		t.Fatalf("wait_for timeout case: %v", err)
	}
	if !strings.HasPrefix(timedOut, "error:") || !strings.Contains(timedOut, "not met") {
		t.Errorf("wait_for on absent text = %q, want a timeout error", timedOut)
	}
}

func TestParseKeySpec(t *testing.T) {
	cases := []struct {
		in      string
		wantErr bool
	}{
		{"Enter", false},
		{"escape", false},
		{"a", false},
		{"Control+a", false},
		{"Shift+Tab", false},
		{"+", false},
		{"NoSuchKey", true},
		{"Hyper+a", true},
	}
	for _, c := range cases {
		_, _, err := parseKeySpec(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("parseKeySpec(%q) err=%v wantErr=%v", c.in, err, c.wantErr)
		}
	}
}

func TestSelectOptionListsAndPicks(t *testing.T) {
	m := newTestManager(t)
	env := testEnv(t, "select")
	defer m.closeSession("select")
	srv := interactionsServer(t)
	navigateTo(t, m, env, srv.URL)

	list := runTool(t, "list options", m.executeSelectOption, `{"selector":"#pet"}`, env)
	for _, want := range []string{`label="Cat"`, `label="Dog"`} {
		if !strings.Contains(list, want) {
			t.Errorf("options listing is missing %s:\n%s", want, list)
		}
	}

	runTool(t, "pick by label", m.executeSelectOption, `{"selector":"#pet","label":"Dog"}`, env)
	if got := evalString(t, m, env, `document.getElementById('petout').textContent`); got != "dog" {
		t.Errorf("after picking Dog, change handler saw %q, want \"dog\"", got)
	}

	res, _ := m.executeSelectOption(context.Background(), `{"selector":"#pet","value":"fish"}`, env)
	if !strings.Contains(res, "no option matches") {
		t.Errorf("picking a missing option = %q, want a no-match report", res)
	}
}

func TestFillFormFillsEverythingInOneCall(t *testing.T) {
	m := newTestManager(t)
	env := testEnv(t, "fillform")
	defer m.closeSession("fillform")
	srv := interactionsServer(t)
	navigateTo(t, m, env, srv.URL)

	args := `{"fields":[
		{"selector":"#first","value":"hello"},
		{"selector":"#notes","value":"world"},
		{"selector":"#agree","value":"true"},
		{"selector":"#pet","value":"cat"}
	]}`
	res := runTool(t, "fill_form", m.executeFillForm, args, env)
	if !strings.Contains(res, "filled 4 of 4") {
		t.Errorf("fill_form did not fill everything: %s", res)
	}
	if got := evalString(t, m, env, `document.getElementById('first').value`); got != "hello" {
		t.Errorf("#first = %q", got)
	}
	if got := evalString(t, m, env, `String(document.getElementById('agree').checked)`); got != "true" {
		t.Errorf("#agree checked = %q", got)
	}
	if got := evalString(t, m, env, `document.getElementById('petout').textContent`); got != "cat" {
		t.Errorf("select inside fill_form did not fire change: petout = %q", got)
	}

	// A missing selector is reported per-field, not swallowed.
	res = runTool(t, "fill_form partial", m.executeFillForm, `{"fields":[{"selector":"#nope","value":"x"}]}`, env)
	if !strings.Contains(res, "filled 0 of 1") || !strings.Contains(res, "no element matches") {
		t.Errorf("fill_form did not report the missing field: %s", res)
	}
}

func TestDialogsAutoDismissAndArmedAccept(t *testing.T) {
	m := newTestManager(t)
	env := testEnv(t, "dialogs")
	defer m.closeSession("dialogs")
	srv := interactionsServer(t)
	navigateTo(t, m, env, srv.URL)

	// Unarmed: the confirm is dismissed so the click cannot hang, and the page
	// log says so.
	res := runTool(t, "click ask (unarmed)", m.executeClick, `{"selector":"#ask"}`, env)
	if got := evalString(t, m, env, `document.getElementById('verdict').textContent`); got != "no" {
		t.Errorf("unarmed confirm verdict = %q, want \"no\"", got)
	}
	logRes, _ := m.executePageLog(context.Background(), "{}", env)
	if !strings.Contains(res+logRes, "[dialog]") {
		t.Errorf("no [dialog] line reported anywhere:\naction: %s\nlog: %s", res, logRes)
	}

	// Armed: the next dialog is accepted.
	runTool(t, "arm dialog", m.executeHandleDialog, `{"accept":true}`, env)
	runTool(t, "click ask (armed)", m.executeClick, `{"selector":"#ask"}`, env)
	if got := evalString(t, m, env, `document.getElementById('verdict').textContent`); got != "yes" {
		t.Errorf("armed confirm verdict = %q, want \"yes\"", got)
	}
}

func TestNavigateBackReturnsToThePreviousPage(t *testing.T) {
	m := newTestManager(t)
	env := testEnv(t, "back")
	defer m.closeSession("back")
	srv := interactionsServer(t)
	navigateTo(t, m, env, srv.URL)
	navigateTo(t, m, env, srv.URL+"/second")

	res := runTool(t, "navigate back", m.executeNavigateBack, `{}`, env)
	if !strings.Contains(res, "navigated back") {
		t.Errorf("navigate_back result: %s", res)
	}
	if got := evalString(t, m, env, `location.pathname`); got != "/" {
		t.Errorf("after back, pathname = %q, want \"/\"", got)
	}
	runTool(t, "navigate forward", m.executeNavigateBack, `{"direction":"forward"}`, env)
	if got := evalString(t, m, env, `location.pathname`); got != "/second" {
		t.Errorf("after forward, pathname = %q, want \"/second\"", got)
	}
}

func TestResizeChangesTheViewport(t *testing.T) {
	m := newTestManager(t)
	env := testEnv(t, "resize")
	defer m.closeSession("resize")
	srv := interactionsServer(t)
	navigateTo(t, m, env, srv.URL)

	runTool(t, "resize", m.executeResize, `{"width":500,"height":400}`, env)
	if got := evalString(t, m, env, `String(window.innerWidth)`); got != "500" {
		t.Errorf("innerWidth after resize = %q, want 500", got)
	}
	runTool(t, "reset viewport", m.executeResize, `{"width":0,"height":0}`, env)

	res, _ := m.executeResize(context.Background(), `{"width":50,"height":400}`, env)
	if !strings.HasPrefix(res, "error:") {
		t.Errorf("resize below the minimum = %q, want error", res)
	}
}

func TestUploadFileReachesTheInput(t *testing.T) {
	m := newTestManager(t)
	env := testEnv(t, "upload")
	defer m.closeSession("upload")
	srv := interactionsServer(t)
	navigateTo(t, m, env, srv.URL)

	dir := t.TempDir()
	path := filepath.Join(dir, "report.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(uploadFileArgs{Selector: "#up", Path: path})
	runTool(t, "upload", m.executeUploadFile, string(args), env)
	if got := evalString(t, m, env, `document.getElementById('upname').textContent`); got != "report.txt" {
		t.Errorf("uploaded file name seen by the page = %q, want \"report.txt\"", got)
	}

	res, _ := m.executeUploadFile(context.Background(), `{"selector":"#up","path":"Z:\\no\\such\\file.bin"}`, env)
	if !strings.HasPrefix(res, "error:") {
		t.Errorf("upload of a missing file = %q, want error", res)
	}
}

func TestClickByCoordinates(t *testing.T) {
	m := newTestManager(t)
	env := testEnv(t, "clickxy")
	defer m.closeSession("clickxy")
	srv := interactionsServer(t)
	navigateTo(t, m, env, srv.URL)

	x := evalString(t, m, env, `String(Math.round(document.getElementById('xy').getBoundingClientRect().left + 5))`)
	y := evalString(t, m, env, `String(Math.round(document.getElementById('xy').getBoundingClientRect().top + 5))`)
	runTool(t, "click xy", m.executeClick, fmt.Sprintf(`{"x":%s,"y":%s}`, x, y), env)
	if got := evalString(t, m, env, `document.getElementById('xyout').textContent`); got != "xy-clicked" {
		t.Errorf("coordinate click missed: xyout = %q", got)
	}

	res, _ := m.executeClick(context.Background(), `{}`, env)
	if !strings.HasPrefix(res, "error:") {
		t.Errorf("click with neither selector nor coordinates = %q, want error", res)
	}
}

func TestDragMovesFromHandleToDropzone(t *testing.T) {
	m := newTestManager(t)
	env := testEnv(t, "drag")
	defer m.closeSession("drag")
	srv := interactionsServer(t)
	navigateTo(t, m, env, srv.URL)

	runTool(t, "drag", m.executeDrag, `{"from_selector":"#handle","to_selector":"#dropzone"}`, env)
	if got := evalString(t, m, env, `document.getElementById('dragout').textContent`); got != "dropped" {
		t.Errorf("drag did not land in the dropzone: dragout = %q", got)
	}

	res, _ := m.executeDrag(context.Background(), `{"from_selector":"#nope","to_selector":"#dropzone"}`, env)
	if !strings.HasPrefix(res, "error:") {
		t.Errorf("drag from a missing element = %q, want error", res)
	}
}

func TestFindReportsMatchingControls(t *testing.T) {
	m := newTestManager(t)
	env := testEnv(t, "find")
	defer m.closeSession("find")
	srv := interactionsServer(t)
	navigateTo(t, m, env, srv.URL)

	res := runTool(t, "find", m.executeFind, `{"query":"Send"}`, env)
	if !strings.Contains(res, "selector=#send") {
		t.Errorf("find did not report the button's selector:\n%s", res)
	}
	if strings.Contains(res, "screenshot") {
		t.Errorf("find captured a screenshot; it must not:\n%s", res)
	}

	miss := runTool(t, "find miss", m.executeFind, `{"query":"nonexistent-control-xyz"}`, env)
	if !strings.Contains(miss, "no elements match") {
		t.Errorf("find with no matches = %q", miss)
	}
}

func TestTabsOpenListSwitchClose(t *testing.T) {
	m := newTestManager(t)
	env := testEnv(t, "tabs")
	defer m.closeSession("tabs")
	srv := interactionsServer(t)
	navigateTo(t, m, env, srv.URL)

	runTool(t, "new tab", m.executeTabs, `{"action":"new","url":"`+srv.URL+`/second"}`, env)
	if got := evalString(t, m, env, `location.pathname`); got != "/second" {
		t.Errorf("after new tab, current pathname = %q, want \"/second\"", got)
	}

	list := runTool(t, "list tabs", m.executeTabs, `{"action":"list"}`, env)
	if !strings.Contains(list, "2 tab(s)") {
		t.Errorf("tab list does not show 2 tabs:\n%s", list)
	}

	// Work out which listed number is the first page and switch back to it.
	firstNum := tabNumberFor(t, list, srv.URL+"/")
	runTool(t, "select tab", m.executeTabs, fmt.Sprintf(`{"action":"select","tab":%d}`, firstNum), env)
	if got := evalString(t, m, env, `location.pathname`); got != "/" {
		t.Errorf("after select, pathname = %q, want \"/\"", got)
	}

	secondNum := tabNumberFor(t, list, srv.URL+"/second")
	runTool(t, "close tab", m.executeTabs, fmt.Sprintf(`{"action":"close","tab":%d}`, secondNum), env)
	list = runTool(t, "list after close", m.executeTabs, `{"action":"list"}`, env)
	if !strings.Contains(list, "1 tab(s)") {
		t.Errorf("tab list after close:\n%s", list)
	}

	res, _ := m.executeTabs(context.Background(), `{"action":"close","tab":1}`, env)
	if !strings.Contains(res, "refusing to close the last tab") {
		t.Errorf("closing the last tab = %q, want refusal", res)
	}
}

// tabNumberFor extracts the 1-based number of the tab whose URL is exactly url.
// A substring match would confuse "/" with "/second", which shares the prefix.
func tabNumberFor(t *testing.T, list, url string) int {
	t.Helper()
	for _, line := range strings.Split(list, "\n") {
		var num int
		if _, err := fmt.Sscanf(strings.TrimLeft(line, "* "), "tab %d:", &num); err != nil {
			continue
		}
		_, rest, ok := strings.Cut(line, ": ")
		if !ok {
			continue
		}
		lineURL, _, _ := strings.Cut(rest, " — ")
		if strings.TrimSpace(lineURL) == url {
			return num
		}
	}
	t.Fatalf("no tab line has URL %q:\n%s", url, list)
	return 0
}
