package llm

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
)

type mockRT struct {
	gotReqBody string
	respBody   string
}

func (m *mockRT) RoundTrip(req *http.Request) (*http.Response, error) {
	b, _ := io.ReadAll(req.Body)
	m.gotReqBody = string(b)
	return &http.Response{
		StatusCode:    200,
		Status:        "200 OK",
		Body:          io.NopCloser(strings.NewReader(m.respBody)),
		Header:        http.Header{},
		ContentLength: int64(len(m.respBody)),
	}, nil
}

func newCaptureLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestDebugTransportOffIsPassthrough(t *testing.T) {
	SetDebugCapture(false)
	defer SetDebugCapture(false)

	var buf bytes.Buffer
	SetDebugLogger(newCaptureLogger(&buf))

	mr := &mockRT{respBody: "RESP"}
	dt := debugTransportFor(mr)
	client := &http.Client{Transport: dt}

	req, _ := http.NewRequest(http.MethodPost, "https://example.com/v1/chat", strings.NewReader("REQ"))
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	if string(got) != "RESP" {
		t.Errorf("response body = %q, want RESP", got)
	}
	if mr.gotReqBody != "REQ" {
		t.Errorf("upstream received request body %q, want REQ", mr.gotReqBody)
	}
	if buf.Len() != 0 {
		t.Errorf("capture off should not log, got: %q", buf.String())
	}
}

func TestDebugTransportOnLogsAndPreservesBodies(t *testing.T) {
	SetDebugCapture(true)
	defer SetDebugCapture(false)

	var buf bytes.Buffer
	SetDebugLogger(newCaptureLogger(&buf))

	mr := &mockRT{respBody: "RESPONSE-BODY"}
	dt := debugTransportFor(mr)
	client := &http.Client{Transport: dt}

	req, _ := http.NewRequest(http.MethodPost, "https://example.com/v1/chat", strings.NewReader("REQUEST-BODY"))
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	// Bodies delivered intact to both sides.
	if mr.gotReqBody != "REQUEST-BODY" {
		t.Errorf("upstream received %q, want REQUEST-BODY", mr.gotReqBody)
	}
	if string(got) != "RESPONSE-BODY" {
		t.Errorf("client received %q, want RESPONSE-BODY", got)
	}

	// Both bodies appear in the log (request on RoundTrip, response on Close).
	logs := buf.String()
	for _, want := range []string{"REQUEST-BODY", "RESPONSE-BODY", "llm http request", "llm http response"} {
		if !strings.Contains(logs, want) {
			t.Errorf("log missing %q; got: %s", want, logs)
		}
	}
}

func TestDebugTransportTruncatesLargeBodiesButDeliversFull(t *testing.T) {
	SetDebugCapture(true)
	defer SetDebugCapture(false)

	var buf bytes.Buffer
	SetDebugLogger(newCaptureLogger(&buf))

	// Build bodies well past the cap on both sides.
	bigReq := strings.Repeat("R", debugBodyCap+4096)
	bigResp := strings.Repeat("S", debugBodyCap+4096)
	mr := &mockRT{respBody: bigResp}
	dt := debugTransportFor(mr)
	client := &http.Client{Transport: dt}

	req, _ := http.NewRequest(http.MethodPost, "https://example.com/v1/chat", strings.NewReader(bigReq))
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	// Full delivery is preserved even though the log excerpt is capped.
	if mr.gotReqBody != bigReq {
		t.Errorf("upstream received truncated request: got len %d, want %d", len(mr.gotReqBody), len(bigReq))
	}
	if string(got) != bigResp {
		t.Errorf("client received truncated response: got len %d, want %d", len(got), len(bigResp))
	}

	// The log marks both sides truncated and does not contain the full bodies.
	logs := buf.String()
	if !strings.Contains(logs, "[truncated]") {
		t.Errorf("log should mark truncated excerpts: %s", logs)
	}
	if strings.Count(logs, strings.Repeat("R", debugBodyCap+1)) > 0 || strings.Count(logs, strings.Repeat("S", debugBodyCap+1)) > 0 {
		t.Errorf("log must not contain a full untruncated body")
	}
}
