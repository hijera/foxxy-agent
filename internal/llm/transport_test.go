package llm

import (
	"net/http"
	"testing"
)

func underlying(t *testing.T, c *http.Client) *http.Transport {
	t.Helper()
	if _, wrapped := c.Transport.(debugTransport); !wrapped {
		t.Fatalf("the provider client's transport is %T, not the debug wrapper", c.Transport)
	}
	tr, ok := UnwrapTransport(c.Transport).(*http.Transport)
	if !ok {
		t.Fatalf("the wrapped transport is %T, want *http.Transport", UnwrapTransport(c.Transport))
	}
	return tr
}

// fork(transport-debug-wrap) guard: the providers built for every turn share
// one transport per proxy setting, as upstream 1.1.47 does, but it is the
// fork's: behind the debug wrapper, with the response-header timeout and the
// CONNECT logging of proxy_http_client.go. Upstream's transport.go builds from
// http.DefaultTransport and type-asserts *http.Transport, which the wrapper
// would fail.
func TestProviderTransportKeepsHeaderTimeoutAndDebugWrap(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "")
	a, err := providerHTTPClient("", 0)
	if err != nil {
		t.Fatal(err)
	}
	b, err := providerHTTPClient("", 0)
	if err != nil {
		t.Fatal(err)
	}
	ta, tb := underlying(t, a), underlying(t, b)
	if ta != tb {
		t.Fatal("two providers for the same proxy setting got transports of their own")
	}
	if ta.ResponseHeaderTimeout != llmResponseHeaderTimeout {
		t.Errorf("ResponseHeaderTimeout = %v, want %v", ta.ResponseHeaderTimeout, llmResponseHeaderTimeout)
	}
	if ta.OnProxyConnectResponse == nil {
		t.Error("the CONNECT logging of proxy_http_client.go is missing")
	}
	if ta.HTTP2 == nil || ta.HTTP2.SendPingTimeout != llmHTTP2SendPingTimeout || ta.HTTP2.PingTimeout != llmHTTP2PingTimeout {
		t.Errorf("the shared transport lost the HTTP/2 health check of cloneLLMTransport: %+v", ta.HTTP2)
	}
}

// A different proxy - configured, or read from the environment - is a
// different transport; the same one is shared.
func TestProviderTransportFollowsTheProxySetting(t *testing.T) {
	p1, err := providerHTTPClient("http://proxy-one.test:3128", 0)
	if err != nil {
		t.Fatal(err)
	}
	p1again, err := providerHTTPClient("http://proxy-one.test:3128", 0)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := providerHTTPClient("http://proxy-two.test:3128", 0)
	if err != nil {
		t.Fatal(err)
	}
	if underlying(t, p1) != underlying(t, p1again) {
		t.Error("the same configured proxy got two transports")
	}
	if underlying(t, p1) == underlying(t, p2) {
		t.Error("two configured proxies share a transport")
	}

	t.Setenv("HTTPS_PROXY", "http://env-one.test:3128")
	e1, err := providerHTTPClient("", 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HTTPS_PROXY", "http://env-two.test:3128")
	e2, err := providerHTTPClient("", 0)
	if err != nil {
		t.Fatal(err)
	}
	if underlying(t, e1) == underlying(t, e2) {
		t.Error("a changed HTTPS_PROXY kept the transport built for the old one")
	}
}

// The request timeout of providers[].timeout_ms rides on the client, not on
// the shared transport.
func TestProviderClientCarriesTheRequestTimeout(t *testing.T) {
	c, err := providerHTTPClient("", 1500_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if c.Timeout != 1500_000_000 {
		t.Fatalf("Timeout = %v", c.Timeout)
	}
}
