package web

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/config"
	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// capture is one request as a test server received it.
type capture struct {
	method, uri, host string
	header            http.Header
	body              []byte
}

// recordingServer answers with handler after recording the request.
func recordingServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *[]capture) {
	t.Helper()
	var seen []capture
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		seen = append(seen, capture{method: r.Method, uri: r.RequestURI, host: r.Host, header: r.Header.Clone(), body: body})
		if handler != nil {
			handler(w, r)
			return
		}
		_, _ = io.WriteString(w, "ok")
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

func runHTTPRequest(t *testing.T, cwd, args string) (string, error) {
	t.Helper()
	return HTTPRequestTool().Execute(context.Background(), args, &tooling.Env{CWD: cwd})
}

func lastCapture(t *testing.T, seen *[]capture) capture {
	t.Helper()
	if len(*seen) == 0 {
		t.Fatal("the server received no request")
	}
	return (*seen)[len(*seen)-1]
}

func TestHTTPRequestDefaultsMethodToGetAndToPostWithAPayload(t *testing.T) {
	srv, seen := recordingServer(t, nil)
	if _, err := runHTTPRequest(t, t.TempDir(), `{"url":"`+srv.URL+`/a"}`); err != nil {
		t.Fatal(err)
	}
	if got := lastCapture(t, seen).method; got != http.MethodGet {
		t.Fatalf("method without payload = %s, want GET", got)
	}
	if _, err := runHTTPRequest(t, t.TempDir(), `{"url":"`+srv.URL+`/a","body":"x"}`); err != nil {
		t.Fatal(err)
	}
	if got := lastCapture(t, seen).method; got != http.MethodPost {
		t.Fatalf("method with payload = %s, want POST", got)
	}
}

func TestHTTPRequestUppercasesTheMethodAndRefusesAnInvalidOne(t *testing.T) {
	srv, seen := recordingServer(t, nil)
	if _, err := runHTTPRequest(t, t.TempDir(), `{"url":"`+srv.URL+`","method":"propfind"}`); err != nil {
		t.Fatal(err)
	}
	if got := lastCapture(t, seen).method; got != "PROPFIND" {
		t.Fatalf("method = %s, want PROPFIND", got)
	}
	_, err := runHTTPRequest(t, t.TempDir(), `{"url":"`+srv.URL+`","method":"GET /x"}`)
	if err == nil || !strings.Contains(err.Error(), "method") {
		t.Fatalf("invalid method error = %v", err)
	}
}

func TestHTTPRequestRefusesTwoPayloadsAtOnce(t *testing.T) {
	_, err := ParseHTTPRequest(`{"url":"https://example.com","body":"a","json":{"b":1}}`, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "body") || !strings.Contains(err.Error(), "json") {
		t.Fatalf("error = %v, want one naming both payloads", err)
	}
}

func TestHTTPRequestRefusesANonHTTPAddress(t *testing.T) {
	for _, raw := range []string{"ftp://example.com/x", "example.com/x", "file:///etc/passwd", ""} {
		if _, err := ParseHTTPRequest(`{"url":"`+raw+`"}`, t.TempDir()); err == nil {
			t.Errorf("url %q was accepted", raw)
		}
	}
}

func TestHTTPRequestEmptyHeaderValueRemovesWhatTheToolWouldSend(t *testing.T) {
	srv, seen := recordingServer(t, nil)
	args := `{"url":"` + srv.URL + `","json":{"a":1},"headers":{"User-Agent":"","Content-Type":""}}`
	if _, err := runHTTPRequest(t, t.TempDir(), args); err != nil {
		t.Fatal(err)
	}
	got := lastCapture(t, seen)
	if _, ok := got.header["User-Agent"]; ok {
		t.Errorf("User-Agent was sent as %q", got.header.Get("User-Agent"))
	}
	if _, ok := got.header["Content-Type"]; ok {
		t.Errorf("Content-Type was sent as %q", got.header.Get("Content-Type"))
	}
}

func TestHTTPRequestHeadersOverrideTheDefaultsAndTheHost(t *testing.T) {
	srv, seen := recordingServer(t, nil)
	args := `{"url":"` + srv.URL + `","json":{"a":1},"headers":{"user-agent":"probe/2","content-type":"application/vnd.api+json","Host":"api.internal","Accept-Encoding":"identity"}}`
	if _, err := runHTTPRequest(t, t.TempDir(), args); err != nil {
		t.Fatal(err)
	}
	got := lastCapture(t, seen)
	if ua := got.header.Get("User-Agent"); ua != "probe/2" {
		t.Errorf("User-Agent = %q", ua)
	}
	if ct := got.header.Get("Content-Type"); ct != "application/vnd.api+json" {
		t.Errorf("Content-Type = %q", ct)
	}
	if got.host != "api.internal" {
		t.Errorf("Host = %q", got.host)
	}
	if ae := got.header.Get("Accept-Encoding"); ae != "identity" {
		t.Errorf("Accept-Encoding = %q", ae)
	}
}

func TestHTTPRequestSendsNoAcceptEncodingOfItsOwn(t *testing.T) {
	srv, seen := recordingServer(t, nil)
	if _, err := runHTTPRequest(t, t.TempDir(), `{"url":"`+srv.URL+`"}`); err != nil {
		t.Fatal(err)
	}
	if ae, ok := lastCapture(t, seen).header["Accept-Encoding"]; ok {
		t.Fatalf("Accept-Encoding %v was added without being asked for", ae)
	}
}

func TestHTTPRequestContentLengthMustDescribeTheBody(t *testing.T) {
	srv, seen := recordingServer(t, nil)
	if _, err := runHTTPRequest(t, t.TempDir(), `{"url":"`+srv.URL+`","body":"abc","headers":{"Content-Length":"3"}}`); err != nil {
		t.Fatalf("matching Content-Length refused: %v", err)
	}
	if got := string(lastCapture(t, seen).body); got != "abc" {
		t.Fatalf("body = %q", got)
	}
	_, err := ParseHTTPRequest(`{"url":"`+srv.URL+`","body":"abc","headers":{"Content-Length":"10"}}`, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "Content-Length") {
		t.Fatalf("mismatched Content-Length error = %v", err)
	}
}

func TestHTTPRequestSendsABase64Body(t *testing.T) {
	srv, seen := recordingServer(t, nil)
	raw := []byte{0, 1, 2, 250, 255}
	args := `{"url":"` + srv.URL + `","method":"PUT","body_base64":"` + base64.StdEncoding.EncodeToString(raw) + `"}`
	if _, err := runHTTPRequest(t, t.TempDir(), args); err != nil {
		t.Fatal(err)
	}
	if got := lastCapture(t, seen).body; !bytes.Equal(got, raw) {
		t.Fatalf("body = %v, want %v", got, raw)
	}
	if _, err := ParseHTTPRequest(`{"url":"`+srv.URL+`","body_base64":"%%%"}`, t.TempDir()); err == nil {
		t.Fatal("invalid base64 accepted")
	}
}

func TestHTTPRequestSendsAFileAsTheBodyWithAGuessedType(t *testing.T) {
	srv, seen := recordingServer(t, nil)
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "data.json"), []byte(`{"k":"v"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runHTTPRequest(t, cwd, `{"url":"`+srv.URL+`","method":"PUT","body_file":"data.json"}`); err != nil {
		t.Fatal(err)
	}
	got := lastCapture(t, seen)
	if string(got.body) != `{"k":"v"}` {
		t.Errorf("body = %q", got.body)
	}
	if ct := got.header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if cl := got.header.Get("Content-Length"); cl != "" && cl != "9" {
		t.Errorf("Content-Length = %q", cl)
	}
}

func TestHTTPRequestMissingFileFailsBeforeAnythingIsSent(t *testing.T) {
	srv, seen := recordingServer(t, nil)
	for _, args := range []string{
		`{"url":"` + srv.URL + `","body_file":"absent.bin"}`,
		`{"url":"` + srv.URL + `","form_data":[{"name":"f","file":"absent.bin"}]}`,
	} {
		if _, err := runHTTPRequest(t, t.TempDir(), args); err == nil || !strings.Contains(err.Error(), "absent.bin") {
			t.Errorf("args %s: error = %v", args, err)
		}
	}
	if len(*seen) != 0 {
		t.Fatalf("the server received %d requests", len(*seen))
	}
}

func TestHTTPRequestJSONGivenAsATextIsSentAsTheDocument(t *testing.T) {
	srv, seen := recordingServer(t, nil)
	if _, err := runHTTPRequest(t, t.TempDir(), `{"url":"`+srv.URL+`","json":"{\"a\": [1, 2]}"}`); err != nil {
		t.Fatal(err)
	}
	if got := string(lastCapture(t, seen).body); got != `{"a":[1,2]}` {
		t.Fatalf("body = %q", got)
	}
	// A string that is not a document stays a JSON string.
	if _, err := runHTTPRequest(t, t.TempDir(), `{"url":"`+srv.URL+`","json":"hello"}`); err != nil {
		t.Fatal(err)
	}
	if got := string(lastCapture(t, seen).body); got != `"hello"` {
		t.Fatalf("body = %q", got)
	}
}

func TestHTTPRequestEncodesAFormAndMergesTheQuery(t *testing.T) {
	srv, seen := recordingServer(t, nil)
	args := `{"url":"` + srv.URL + `/s?q=1","query":{"page":2,"tag":["a","b c"],"raw":true},"form":{"name":"Иван","ids":[1,2]}}`
	if _, err := runHTTPRequest(t, t.TempDir(), args); err != nil {
		t.Fatal(err)
	}
	got := lastCapture(t, seen)
	u, _ := url.Parse(got.uri)
	q := u.Query()
	if q.Get("q") != "1" || q.Get("page") != "2" || q.Get("raw") != "true" || strings.Join(q["tag"], "|") != "a|b c" {
		t.Errorf("query = %v", q)
	}
	if ct := got.header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q", ct)
	}
	form, err := url.ParseQuery(string(got.body))
	if err != nil {
		t.Fatal(err)
	}
	if form.Get("name") != "Иван" || strings.Join(form["ids"], ",") != "1,2" {
		t.Errorf("form = %v", form)
	}
}

func TestHTTPRequestRefusesANestedQueryValue(t *testing.T) {
	_, err := ParseHTTPRequest(`{"url":"https://example.com","query":{"a":{"b":1}}}`, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "query") {
		t.Fatalf("error = %v", err)
	}
}

func TestHTTPRequestAnswersLikeCurlInclude(t *testing.T) {
	srv, _ := recordingServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Add("Set-Cookie", "a=1")
		w.Header().Add("Set-Cookie", "b=2")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":7}`)
	})
	out, err := runHTTPRequest(t, t.TempDir(), `{"url":"`+srv.URL+`","json":{}}`)
	if err != nil {
		t.Fatal(err)
	}
	head, body, ok := strings.Cut(out, "\n\n")
	if !ok {
		t.Fatalf("no blank line between headers and body:\n%s", out)
	}
	lines := strings.Split(head, "\n")
	if lines[0] != "HTTP/1.1 201 Created" {
		t.Errorf("status line = %q", lines[0])
	}
	if !strings.Contains(head, "Set-Cookie: a=1\nSet-Cookie: b=2") {
		t.Errorf("repeated header lost:\n%s", head)
	}
	if strings.TrimSpace(body) != `{"id":7}` {
		t.Errorf("body = %q", body)
	}
}

func TestHTTPRequestAnErrorStatusIsAnAnswerNotAFailure(t *testing.T) {
	srv, _ := recordingServer(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusTeapot)
	})
	out, err := runHTTPRequest(t, t.TempDir(), `{"url":"`+srv.URL+`"}`)
	if err != nil {
		t.Fatalf("a 418 failed the call: %v", err)
	}
	if !strings.HasPrefix(out, "HTTP/1.1 418") || !strings.Contains(out, "nope") {
		t.Fatalf("answer:\n%s", out)
	}
}

func TestHTTPRequestHeadCarriesNoBody(t *testing.T) {
	srv, seen := recordingServer(t, nil)
	out, err := runHTTPRequest(t, t.TempDir(), `{"url":"`+srv.URL+`","method":"HEAD"}`)
	if err != nil {
		t.Fatal(err)
	}
	if lastCapture(t, seen).method != http.MethodHead {
		t.Fatal("not a HEAD request")
	}
	if strings.Contains(out, "ok") {
		t.Fatalf("HEAD answer carries a body:\n%s", out)
	}
}

func TestHTTPRequestDoesNotFollowARedirectUnlessAsked(t *testing.T) {
	srv, seen := recordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/old" {
			http.Redirect(w, r, "/new", http.StatusFound)
			return
		}
		_, _ = io.WriteString(w, "arrived")
	})
	out, err := runHTTPRequest(t, t.TempDir(), `{"url":"`+srv.URL+`/old"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "HTTP/1.1 302") || !strings.Contains(out, "Location: /new") || len(*seen) != 1 {
		t.Fatalf("redirect was followed or lost (%d requests):\n%s", len(*seen), out)
	}
	out, err = runHTTPRequest(t, t.TempDir(), `{"url":"`+srv.URL+`/old","follow_redirects":true}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "HTTP/1.1 200") || !strings.Contains(out, "arrived") || !strings.Contains(out, srv.URL+"/new") {
		t.Fatalf("same-origin redirect not followed:\n%s", out)
	}
}

func TestHTTPRequestDoesNotFollowARedirectToAnotherOrigin(t *testing.T) {
	other, otherSeen := recordingServer(t, nil)
	srv, _ := recordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/elsewhere", http.StatusTemporaryRedirect)
	})
	out, err := runHTTPRequest(t, t.TempDir(), `{"url":"`+srv.URL+`","method":"POST","body":"secret","follow_redirects":true}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(*otherSeen) != 0 {
		t.Fatal("the request reached another origin without approval")
	}
	if !strings.HasPrefix(out, "HTTP/1.1 307") || !strings.Contains(out, "not followed") || !strings.Contains(out, other.URL+"/elsewhere") {
		t.Fatalf("answer does not explain the held redirect:\n%s", out)
	}
}

func TestHTTPRequestDescribesABinaryBodyInsteadOfDumpingIt(t *testing.T) {
	srv, _ := recordingServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte{0, 159, 146, 150, 0, 1})
	})
	out, err := runHTTPRequest(t, t.TempDir(), `{"url":"`+srv.URL+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "binary body") || !strings.Contains(out, "6 bytes") || !strings.Contains(out, "output_file") {
		t.Fatalf("answer:\n%q", out)
	}
	if strings.ContainsRune(out, 0) {
		t.Fatal("raw bytes leaked into the answer")
	}
}

func TestHTTPRequestSavesTheBodyToOutputFile(t *testing.T) {
	payload := bytes.Repeat([]byte{0xde, 0xad, 0xbe, 0xef}, 1000)
	srv, _ := recordingServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	})
	cwd := t.TempDir()
	out, err := runHTTPRequest(t, cwd, `{"url":"`+srv.URL+`","output_file":"dl/blob.bin"}`)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(cwd, "dl", "blob.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, payload) {
		t.Fatalf("saved %d bytes, want %d", len(data), len(payload))
	}
	if !strings.Contains(out, "saved 4000 bytes to "+filepath.Join(cwd, "dl", "blob.bin")) {
		t.Fatalf("answer:\n%s", out)
	}
}

func TestHTTPRequestDecodesAGzipAnswerForReading(t *testing.T) {
	srv, _ := recordingServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Encoding", "gzip")
		zw := gzip.NewWriter(w)
		_, _ = io.WriteString(zw, "compressed hello")
		_ = zw.Close()
	})
	out, err := runHTTPRequest(t, t.TempDir(), `{"url":"`+srv.URL+`","headers":{"Accept-Encoding":"gzip"}}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "compressed hello") {
		t.Fatalf("answer:\n%q", out)
	}
}

func TestHTTPRequestDecodesADeclaredCharset(t *testing.T) {
	srv, _ := recordingServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=windows-1251")
		_, _ = w.Write([]byte{0xcf, 0xf0, 0xe8, 0xe2, 0xe5, 0xf2}) // "Привет"
	})
	out, err := runHTTPRequest(t, t.TempDir(), `{"url":"`+srv.URL+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Привет") {
		t.Fatalf("answer:\n%q", out)
	}
}

func TestHTTPRequestCutsALongBodyAndSaysSo(t *testing.T) {
	srv, _ := recordingServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write(bytes.Repeat([]byte("a"), maxHTTPBodyDisplayBytes+100))
	})
	out, err := runHTTPRequest(t, t.TempDir(), `{"url":"`+srv.URL+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "truncated") || strings.Count(out, "a") > maxHTTPBodyDisplayBytes+10 {
		t.Fatalf("long body not cut (len %d)", len(out))
	}
}

func TestHTTPRequestTimeoutIsBoundedAndApplied(t *testing.T) {
	req, err := ParseHTTPRequest(`{"url":"https://example.com","timeout_seconds":100000}`, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if req.Timeout != maxHTTPTimeout {
		t.Fatalf("timeout = %s, want the ceiling %s", req.Timeout, maxHTTPTimeout)
	}
	release := make(chan struct{})
	srv, _ := recordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	defer close(release)
	start := time.Now()
	_, err = runHTTPRequest(t, t.TempDir(), `{"url":"`+srv.URL+`","timeout_seconds":1}`)
	if err == nil {
		t.Fatal("a stalled exchange did not time out")
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("timeout took %s", time.Since(start))
	}
}

func TestHTTPRequestUploadsMultipartPartsInOrder(t *testing.T) {
	srv, seen := recordingServer(t, nil)
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "a.csv"), []byte("x,y\n1,2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	args := `{"url":"` + srv.URL + `","form_data":[{"name":"meta","value":"m"},{"name":"sheet","file":"a.csv","filename":"renamed.csv"}]}`
	if _, err := runHTTPRequest(t, cwd, args); err != nil {
		t.Fatal(err)
	}
	got := lastCapture(t, seen)
	body := string(got.body)
	if !strings.HasPrefix(got.header.Get("Content-Type"), "multipart/form-data; boundary=") {
		t.Fatalf("Content-Type = %q", got.header.Get("Content-Type"))
	}
	meta, sheet := strings.Index(body, `name="meta"`), strings.Index(body, `filename="renamed.csv"`)
	if meta < 0 || sheet < 0 || meta > sheet || !strings.Contains(body, "x,y\n1,2\n") {
		t.Fatalf("multipart body:\n%s", body)
	}
	if cl := got.header.Get("Content-Length"); cl == "" {
		t.Error("multipart upload was sent without a Content-Length")
	}
	if _, err := ParseHTTPRequest(`{"url":"`+srv.URL+`","form_data":[{"name":"x"}]}`, cwd); err == nil {
		t.Error("a part with neither value nor file was accepted")
	}
}

func TestParseHTTPRequestNamesTheOriginAndTheAddress(t *testing.T) {
	cases := []struct{ raw, origin, address string }{
		{"https://API.Example.com:443/v1/items?x=1#frag", "https://api.example.com", "https://api.example.com/v1/items"},
		{"http://localhost:8080", "http://localhost:8080", "http://localhost:8080/"},
		{"http://[::1]:80/a", "http://[::1]", "http://[::1]/a"},
	}
	for _, c := range cases {
		req, err := ParseHTTPRequest(`{"url":"`+c.raw+`"}`, t.TempDir())
		if err != nil {
			t.Fatalf("%s: %v", c.raw, err)
		}
		if req.Origin() != c.origin || req.Address() != c.address {
			t.Errorf("%s: origin %q address %q, want %q %q", c.raw, req.Origin(), req.Address(), c.origin, c.address)
		}
	}
}

func TestHTTPRequestDescribeShowsWhatTheRequestCarries(t *testing.T) {
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "r.pdf"), []byte("%PDF-1"), 0o644); err != nil {
		t.Fatal(err)
	}
	req, err := ParseHTTPRequest(`{
		"url": "https://api.example.com/upload?v=2",
		"headers": {"Authorization": "Bearer abc"},
		"form_data": [{"name": "doc", "file": "r.pdf"}, {"name": "note", "value": "hi"}],
		"output_file": "out/receipt.json",
		"permission_rationale": "Upload the report"
	}`, cwd)
	if err != nil {
		t.Fatal(err)
	}
	text := req.Describe()
	for _, want := range []string{
		"Upload the report",
		"POST https://api.example.com/upload?v=2",
		"Authorization: Bearer abc",
		"Content-Type: multipart/form-data; boundary=<generated>",
		filepath.Join(cwd, "r.pdf") + " (6 bytes)",
		"note",
		filepath.Join(cwd, "out", "receipt.json"),
	} {
		if !strings.Contains(text, want) {
			t.Errorf("description does not show %q:\n%s", want, text)
		}
	}
	if len(req.Files) != 1 || req.Files[0].Path != filepath.Join(cwd, "r.pdf") {
		t.Errorf("files = %+v", req.Files)
	}
}

func TestWebFetchReadsAPageThroughTheSharedClient(t *testing.T) {
	srv, seen := recordingServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<html><head><title>Lamp</title></head><body><article><h1>Lamp</h1><p>A lamp gives light to the reading room every evening, and this paragraph is long enough to count as the article body.</p></article></body></html>`)
	})
	restore := fetchGuard
	fetchGuard = func(context.Context, *url.URL) error { return nil }
	t.Cleanup(func() { fetchGuard = restore })
	out, err := WebFetchTool().Execute(context.Background(), `{"url":"`+srv.URL+`/page"}`, &tooling.Env{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "reading room") {
		t.Fatalf("markdown:\n%s", out)
	}
	if ua := lastCapture(t, seen).header.Get("User-Agent"); !strings.HasPrefix(ua, "foxxycode-agent/") {
		t.Errorf("User-Agent = %q", ua)
	}
}

func TestWebFetchChecksEveryRedirectAgainstTheGuard(t *testing.T) {
	private, privateSeen := recordingServer(t, nil)
	public, _ := recordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, private.URL+"/metadata", http.StatusFound)
	})
	blocked, _ := url.Parse(private.URL)
	restore := fetchGuard
	fetchGuard = func(_ context.Context, u *url.URL) error {
		if u.Host == blocked.Host {
			return ErrDisallowedURL
		}
		return nil
	}
	t.Cleanup(func() { fetchGuard = restore })
	_, err := WebFetchTool().Execute(context.Background(), `{"url":"`+public.URL+`"}`, &tooling.Env{})
	if !errors.Is(err, ErrDisallowedURL) {
		t.Fatalf("error = %v, want the guard's refusal", err)
	}
	if len(*privateSeen) != 0 {
		t.Fatal("the redirect reached the refused address")
	}
}

func TestHTTPRequestGoesThroughTheProxyItNames(t *testing.T) {
	proxy, seen := recordingServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "from the proxy")
	})
	out, err := runHTTPRequest(t, t.TempDir(), `{"url":"http://api.example.invalid/items?id=3","proxy":"`+proxy.URL+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	got := lastCapture(t, seen)
	if got.uri != "http://api.example.invalid/items?id=3" {
		t.Fatalf("the proxy received %q, want the absolute target address", got.uri)
	}
	if !strings.Contains(out, "from the proxy") {
		t.Fatalf("answer:\n%s", out)
	}
}

func TestHTTPRequestRouteChoosesTheProxyAndTheCertificateCheck(t *testing.T) {
	direct, ok := transportFor(false, route{direct: true}).(*http.Transport)
	if !ok || direct.Proxy != nil {
		t.Fatal("proxy \"direct\" still consults the environment's proxies")
	}
	env, ok := transportFor(false, route{}).(*http.Transport)
	if !ok || env.Proxy == nil {
		t.Fatal("an unset proxy no longer consults the environment")
	}
	if env.TLSClientConfig != nil && env.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("the default route skips the certificate check")
	}
	req, err := ParseHTTPRequest(`{"url":"https://example.com","proxy":"DIRECT"}`, t.TempDir())
	if err != nil || !req.Direct || req.Proxy != nil {
		t.Fatalf("direct parsed as %+v, %v", req, err)
	}
	for _, bad := range []string{"ftp://proxy:21", "socks4://proxy:1080", "http://"} {
		if _, err := ParseHTTPRequest(`{"url":"https://example.com","proxy":"`+bad+`"}`, t.TempDir()); err == nil {
			t.Errorf("proxy %q was accepted", bad)
		}
	}
}

func TestHTTPRequestVerifiesTheCertificateUnlessToldNotTo(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "self-signed hello")
	}))
	t.Cleanup(srv.Close)
	if _, err := runHTTPRequest(t, t.TempDir(), `{"url":"`+srv.URL+`"}`); err == nil || !strings.Contains(err.Error(), "certificate") {
		t.Fatalf("a self-signed certificate was accepted by default: %v", err)
	}
	out, err := runHTTPRequest(t, t.TempDir(), `{"url":"`+srv.URL+`","verify_tls":false}`)
	if err != nil {
		t.Fatalf("verify_tls false still checked the certificate: %v", err)
	}
	if !strings.Contains(out, "self-signed hello") {
		t.Fatalf("answer:\n%s", out)
	}
	if _, err := runHTTPRequest(t, t.TempDir(), `{"url":"`+srv.URL+`","verify_tls":true}`); err == nil {
		t.Fatal("verify_tls true accepted a self-signed certificate")
	}
}

func TestHTTPRequestDescribeNamesTheProxyAndAnUncheckedCertificate(t *testing.T) {
	req, err := ParseHTTPRequest(`{"url":"https://api.example.com","proxy":"socks5h://user:secret@10.0.0.2:1080","verify_tls":false}`, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	text := req.Describe()
	if !strings.Contains(text, "Proxy: socks5h://user:xxxxx@10.0.0.2:1080") || strings.Contains(text, "secret") {
		t.Errorf("proxy line:\n%s", text)
	}
	if !strings.Contains(text, "TLS certificate: NOT verified") {
		t.Errorf("unchecked certificate not named:\n%s", text)
	}
	if req.ProxyOrigin() != "socks5h://10.0.0.2:1080" {
		t.Errorf("proxy origin = %q", req.ProxyOrigin())
	}
}

// browserUA is the User-Agent the default-header tests configure.
const browserUA = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

func runHTTPRequestWithDefaults(t *testing.T, cwd, args string, defaults map[string]string) (string, error) {
	t.Helper()
	return HTTPRequestTool().Execute(context.Background(), args, &tooling.Env{CWD: cwd, HTTPDefaultHeaders: defaults})
}

func TestHTTPRequestWithoutDefaultHeadersSendsWhatItAlwaysSent(t *testing.T) {
	cwd := t.TempDir()
	for _, args := range []string{
		`{"url":"https://example.com"}`,
		`{"url":"https://example.com","json":{"a":1}}`,
		`{"url":"https://example.com","headers":{"User-Agent":"","X-Trace":"t"}}`,
	} {
		plain, err := ParseHTTPRequest(args, cwd)
		if err != nil {
			t.Fatal(err)
		}
		for _, defaults := range []map[string]string{nil, {}} {
			got, err := ParseHTTPRequestInEnv(args, &tooling.Env{CWD: cwd, HTTPDefaultHeaders: defaults})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.Header, plain.Header) || got.Describe() != plain.Describe() {
				t.Errorf("%s with defaults %v: headers %v, want %v", args, defaults, got.Header, plain.Header)
			}
		}
	}
	srv, seen := recordingServer(t, nil)
	if _, err := runHTTPRequestWithDefaults(t, cwd, `{"url":"`+srv.URL+`"}`, nil); err != nil {
		t.Fatal(err)
	}
	if ua := lastCapture(t, seen).header.Get("User-Agent"); ua != userAgent {
		t.Fatalf("User-Agent = %q, want the tool's own %q", ua, userAgent)
	}
}

func TestHTTPRequestDefaultHeadersGoOutWithEveryCall(t *testing.T) {
	srv, seen := recordingServer(t, nil)
	defaults := map[string]string{"User-Agent": browserUA, "accept": "application/manifest+json"}
	for _, args := range []string{
		`{"url":"` + srv.URL + `/app.webmanifest"}`,
		`{"url":"` + srv.URL + `/items","method":"PATCH","json":{"name":"lamp"}}`,
	} {
		if _, err := runHTTPRequestWithDefaults(t, t.TempDir(), args, defaults); err != nil {
			t.Fatal(err)
		}
		got := lastCapture(t, seen)
		if ua := got.header.Get("User-Agent"); ua != browserUA {
			t.Errorf("%s: User-Agent = %q, want the configured one", args, ua)
		}
		if accept := got.header.Get("Accept"); accept != "application/manifest+json" {
			t.Errorf("%s: Accept = %q, want the configured one", args, accept)
		}
	}
	// A default header does not touch what the payload says about itself.
	if ct := lastCapture(t, seen).header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type of a JSON body = %q", ct)
	}
}

func TestHTTPRequestCallHeadersWinOverTheDefaults(t *testing.T) {
	srv, seen := recordingServer(t, nil)
	defaults := map[string]string{"User-Agent": browserUA, "X-Client": "foxxycode-lab", "Accept": "application/json"}
	args := `{"url":"` + srv.URL + `","headers":{"user-agent":"probe/2","X-CLIENT":"other"}}`
	if _, err := runHTTPRequestWithDefaults(t, t.TempDir(), args, defaults); err != nil {
		t.Fatal(err)
	}
	got := lastCapture(t, seen)
	if ua := got.header.Get("User-Agent"); ua != "probe/2" {
		t.Errorf("User-Agent = %q, want the call's", ua)
	}
	if client := got.header.Values("X-Client"); len(client) != 1 || client[0] != "other" {
		t.Errorf("X-Client = %q, want only the call's", client)
	}
	if accept := got.header.Get("Accept"); accept != "application/json" {
		t.Errorf("Accept = %q, want the configured one the call left alone", accept)
	}
}

func TestHTTPRequestEmptyCallValueRemovesADefault(t *testing.T) {
	srv, seen := recordingServer(t, nil)
	defaults := map[string]string{"User-Agent": browserUA, "X-Client": "foxxycode-lab"}
	args := `{"url":"` + srv.URL + `","headers":{"User-Agent":"","X-Client":""}}`
	if _, err := runHTTPRequestWithDefaults(t, t.TempDir(), args, defaults); err != nil {
		t.Fatal(err)
	}
	got := lastCapture(t, seen)
	// Not the configured one, not the tool's own and not Go's own either.
	if ua, ok := got.header["User-Agent"]; ok {
		t.Errorf("User-Agent was sent as %q", ua)
	}
	if client, ok := got.header["X-Client"]; ok {
		t.Errorf("X-Client was sent as %q", client)
	}
}

func TestHTTPRequestEmptyDefaultLeavesTheHeaderOut(t *testing.T) {
	srv, seen := recordingServer(t, nil)
	defaults := map[string]string{"User-Agent": ""}
	if _, err := runHTTPRequestWithDefaults(t, t.TempDir(), `{"url":"`+srv.URL+`"}`, defaults); err != nil {
		t.Fatal(err)
	}
	if ua, ok := lastCapture(t, seen).header["User-Agent"]; ok {
		t.Errorf("User-Agent was sent as %q", ua)
	}
	if _, err := runHTTPRequestWithDefaults(t, t.TempDir(), `{"url":"`+srv.URL+`","headers":{"User-Agent":"probe/2"}}`, defaults); err != nil {
		t.Fatal(err)
	}
	if ua := lastCapture(t, seen).header.Get("User-Agent"); ua != "probe/2" {
		t.Errorf("User-Agent = %q, want the call's", ua)
	}
}

func TestHTTPRequestRefusesADefaultHeaderNotMeantForEveryRequest(t *testing.T) {
	for name, value := range map[string]string{
		"Host":                "api.internal",
		"content-length":      "3",
		"Transfer-Encoding":   "chunked",
		"Content-Type":        "application/json",
		"Proxy-Authorization": "Basic dTpw",
		"Upgrade":             "h2c",
		"Connection":          "close",
		"Bad Name":            "x",
		"X-Split":             "a\r\nInjected: b",
	} {
		_, err := ParseHTTPRequestInEnv(`{"url":"https://example.com"}`, &tooling.Env{HTTPDefaultHeaders: map[string]string{name: value}})
		if err == nil {
			t.Errorf("default header %q = %q was accepted", name, value)
			continue
		}
		if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(strings.Fields(name)[0])) {
			t.Errorf("default header %q: error %q does not name it", name, err)
		}
	}
}

func TestHTTPRequestDescribeNamesTheConfiguredHeaders(t *testing.T) {
	defaults := map[string]string{"User-Agent": browserUA, "X-Client": "foxxycode-lab", "Accept": "application/json", "X-Off": ""}
	req, err := ParseHTTPRequestInEnv(`{"url":"https://api.example.com/items","headers":{"Accept":"text/html"}}`, &tooling.Env{CWD: t.TempDir(), HTTPDefaultHeaders: defaults})
	if err != nil {
		t.Fatal(err)
	}
	text := req.Describe()
	for _, want := range []string{
		"User-Agent: " + browserUA,
		"X-Client: <redacted>",
		"Accept: text/html",
		// X-Off is left out by the configuration: named, never sent.
		"Headers from tools.http_request.default_headers: User-Agent, X-Client, X-Off (not sent)\n",
	} {
		if !strings.Contains(text+"\n", want) {
			t.Errorf("description does not show %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "X-Off:") {
		t.Errorf("a header the configuration leaves out is shown as sent:\n%s", text)
	}
	plain, err := ParseHTTPRequest(`{"url":"https://api.example.com/items"}`, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plain.Describe(), "default_headers") {
		t.Errorf("a request without default headers names them:\n%s", plain.Describe())
	}
}

func TestWebFetchAndMentionsDoNotSendTheDefaultHeaders(t *testing.T) {
	srv, seen := recordingServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<html><head><title>Lamp</title></head><body><article><h1>Lamp</h1><p>A lamp gives light to the reading room every evening, and this paragraph is long enough to count as the article body.</p></article></body></html>`)
	})
	restore := fetchGuard
	fetchGuard = func(context.Context, *url.URL) error { return nil }
	t.Cleanup(func() { fetchGuard = restore })
	env := &tooling.Env{HTTPDefaultHeaders: map[string]string{"User-Agent": browserUA, "X-Client": "foxxycode-lab"}}
	if _, err := WebFetchTool().Execute(context.Background(), `{"url":"`+srv.URL+`/page"}`, env); err != nil {
		t.Fatal(err)
	}
	if _, err := FetchForMention(context.Background(), srv.URL+"/page"); err != nil {
		t.Fatal(err)
	}
	for i, got := range *seen {
		if ua := got.header.Get("User-Agent"); ua != userAgent {
			t.Errorf("request %d: User-Agent = %q, want the tool's own", i+1, ua)
		}
		if client, ok := got.header["X-Client"]; ok {
			t.Errorf("request %d: X-Client was sent as %q", i+1, client)
		}
	}
}

func TestHTTPRequestDescribeKeepsAConfiguredCredentialOutOfThePrompt(t *testing.T) {
	// A credential hides under any name: the prompt shows the configured value
	// of a header that describes the client and of no other.
	defaults := map[string]string{
		"Authorization": "Bearer t0p-secret", "X-Api-Key": "k3y", "X-Auth": "s3ss", "Authentication": "Bearer 0ther",
		"Sec-Token": "s3c", "Referer": "https://example.com/?sig=r3f", "User-Agent": browserUA,
		"Accept-Language": "en-US", "Sec-Ch-Ua-Platform": `"Linux"`,
	}
	req, err := ParseHTTPRequestInEnv(`{"url":"https://api.example.com/items"}`, &tooling.Env{CWD: t.TempDir(), HTTPDefaultHeaders: defaults})
	if err != nil {
		t.Fatal(err)
	}
	text := req.Describe()
	for _, want := range []string{
		"Authorization: <redacted>",
		"Authentication: <redacted>",
		"X-Api-Key: <redacted>",
		"X-Auth: <redacted>",
		"Sec-Token: <redacted>",
		"Referer: <redacted>",
		"User-Agent: " + browserUA,
		"Accept-Language: en-US",
		`Sec-Ch-Ua-Platform: "Linux"`,
		"Headers from tools.http_request.default_headers: Accept-Language, Authentication, Authorization, Referer, Sec-Ch-Ua-Platform, Sec-Token, User-Agent, X-Api-Key, X-Auth",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("description does not show %q:\n%s", want, text)
		}
	}
	for _, secret := range []string{"t0p-secret", "k3y", "s3ss", "0ther", "s3c", "r3f"} {
		if strings.Contains(text, secret) {
			t.Errorf("the prompt shows the configured credential %q:\n%s", secret, text)
		}
	}
	if got := req.Header.Get("Authorization"); got != "Bearer t0p-secret" {
		t.Errorf("the request sends Authorization %q, want the configured value", got)
	}
	// A credential the call wrote itself is shown as written: the model knows it,
	// and the transcript carries it already.
	own, err := ParseHTTPRequestInEnv(`{"url":"https://api.example.com/items","headers":{"Authorization":"Bearer abc"}}`, &tooling.Env{CWD: t.TempDir(), HTTPDefaultHeaders: defaults})
	if err != nil {
		t.Fatal(err)
	}
	if text := own.Describe(); !strings.Contains(text, "Authorization: Bearer abc") {
		t.Errorf("the call's own credential is hidden:\n%s", text)
	}
}

// The loader and the request builder keep one rule on which default headers no
// request can carry, each in its own package: whatever the loader accepts the
// tool sends, and whatever it refuses the tool refuses too.
func TestDefaultHeaderRulesMatchTheLoader(t *testing.T) {
	for _, headers := range []map[string]string{
		{"User-Agent": "v"}, {"Accept": "v"}, {"Accept-Language": "v"}, {"Authorization": "v"}, {"Cookie": "v"},
		{"X-Client": "v"}, {"Cache-Control": "v"}, {"Host": "v"}, {"content-type": "v"}, {"Content-Length": "v"},
		{"Transfer-Encoding": "v"}, {"Proxy-Authorization": "v"}, {"Connection": "v"}, {"Te": "v"},
		{"Keep-Alive": "v"}, {"Proxy-Connection": "v"}, {"Trailer": "v"}, {"Upgrade": "v"},
		{"123": "v"}, {"X.Trace": "v"}, {"_X": "v"}, {"User-Agent": "a", "user-agent": "b"},
	} {
		loaderErr := (&config.Tools{HTTPRequest: config.ToolHTTPRequest{DefaultHeaders: headers}}).Validate()
		_, toolErr := ParseHTTPRequestInEnv(`{"url":"https://example.com"}`, &tooling.Env{HTTPDefaultHeaders: headers})
		if (loaderErr == nil) != (toolErr == nil) {
			t.Errorf("%v: the loader says %v, the tool says %v", headers, loaderErr, toolErr)
		}
	}
}
