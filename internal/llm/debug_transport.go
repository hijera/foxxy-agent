package llm

import (
	"bytes"
	"io"
	"net/http"

	"github.com/hijera/foxxycode-agent/internal/version"
)

// debugBodyCap bounds how many bytes of each LLM request/response body are logged.
// LLM payloads can be large (full conversation + tool schemas, long streamed replies),
// so the excerpt is capped; a truncation marker is appended when the body is longer.
const debugBodyCap = 16 * 1024

// debugTransport wraps an http.RoundTripper and, when DebugCaptureEnabled() is on,
// logs a capped excerpt of each LLM HTTP request and response at debug level. The
// response body is teed as it is read so streaming (SSE) responses stay streaming —
// nothing is buffered in full.
type debugTransport struct{ base http.RoundTripper }

// debugTransportFor wraps an existing transport with the debug layer. When debug
// capture is off the wrapper is still cheap: RoundTrip reads the atomic once and
// delegates directly.
func debugTransportFor(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return debugTransport{base: base}
}

// UnwrapTransport returns the underlying transport behind the debug wrapper (or the
// transport itself when it is not wrapped). Tests and internals use it to inspect the
// proxy configuration set by HTTPClientForOptionalProxy without knowing about the layer.
func UnwrapTransport(rt http.RoundTripper) http.RoundTripper {
	if dt, ok := rt.(debugTransport); ok {
		return dt.base
	}
	return rt
}

func (d debugTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !DebugCaptureEnabled() {
		return d.base.RoundTrip(req)
	}

	// Request bodies are built in memory by the providers, so reading them fully is
	// safe and does not disturb the request.
	reqExcerpt := readExcerpt(req.Body)
	if req.Body != nil {
		req.Body = io.NopCloser(bytes.NewReader(reqExcerpt.full))
	}

	resp, err := d.base.RoundTrip(req)
	if err != nil {
		debugLogger().Debug("llm http request",
			"method", req.Method,
			"url", req.URL.String(),
			"req_body", reqExcerpt.string(),
			"error", err.Error(),
			"version", version.Get(),
		)
		return nil, err
	}

	// The response may be a long-lived SSE stream; tee it as it is read instead of
	// buffering it, and log the capped excerpt once the body is closed.
	tb := newTeeBody(resp.Body)
	resp.Body = tb
	debugLogger().Debug("llm http request",
		"method", req.Method,
		"url", req.URL.String(),
		"req_body", reqExcerpt.string(),
		"resp_status", resp.Status,
		"version", version.Get(),
	)
	return resp, nil
}

// teeBody passes reads through to src while accumulating up to debugBodyCap bytes;
// on Close it logs the capped response excerpt. It never blocks streaming: Read
// returns as soon as src yields.
type teeBody struct {
	src   io.ReadCloser
	buf   bytes.Buffer
	capped bool
}

func newTeeBody(src io.ReadCloser) *teeBody { return &teeBody{src: src} }

func (t *teeBody) Read(p []byte) (int, error) {
	n, err := t.src.Read(p)
	if n > 0 && !t.capped {
		room := debugBodyCap - t.buf.Len()
		if room > 0 {
			if n < room {
				t.buf.Write(p[:n])
			} else {
				t.buf.Write(p[:room])
				t.capped = true
			}
		}
	}
	return n, err
}

func (t *teeBody) Close() error {
	err := t.src.Close()
	body := t.buf.Bytes()
	excerpt := string(body)
	truncated := ""
	if t.capped || len(body) > debugBodyCap {
		truncated = "…[truncated]"
		if len(excerpt) > debugBodyCap {
			excerpt = excerpt[:debugBodyCap]
		}
	}
	debugLogger().Debug("llm http response",
		"resp_body", excerpt+truncated,
		"version", version.Get(),
	)
	return err
}

// excerpt holds a capped view of a body plus whether it was truncated.
type excerpt struct {
	full      []byte
	truncated bool
}

func (e excerpt) string() string {
	if len(e.full) > debugBodyCap {
		return string(e.full[:debugBodyCap]) + "…[truncated]"
	}
	if e.truncated {
		return string(e.full) + "…[truncated]"
	}
	return string(e.full)
}

// readExcerpt reads up to debugBodyCap+1 bytes from r to detect truncation, then
// returns the full bytes read (callers restore the body from .full). nil r yields
// an empty excerpt.
func readExcerpt(r io.Reader) excerpt {
	if r == nil {
		return excerpt{}
	}
	buf := make([]byte, debugBodyCap+1)
	n, err := io.ReadFull(r, buf)
	if err == io.ErrUnexpectedEOF || (err == nil && n <= debugBodyCap) {
		return excerpt{full: buf[:n]}
	}
	// n == debugBodyCap+1 means the body is longer; drain the rest so the restored
	// body is complete for the provider, then mark truncated.
	rest, _ := io.ReadAll(r)
	full := append(buf[:n], rest...)
	return excerpt{full: full, truncated: len(rest) > 0}
}
