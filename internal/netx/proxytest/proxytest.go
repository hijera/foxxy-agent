// Package proxytest runs the proxies the outbound tests go through: an HTTP proxy
// (absolute-form requests and CONNECT tunnels) and a SOCKS5 server, either of them
// optionally requiring a login and password. Every destination is reached on
// loopback whatever host it names, so a test can use a name that is not loopback
// (llm.test) and keep the client from bypassing the proxy.
package proxytest

import (
	"bufio"
	"encoding/base64"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// Credentials is what a proxy was offered, decoded.
type Credentials struct {
	User, Password string
}

// Proxy is a running test proxy.
type Proxy struct {
	// Addr is host:port of the proxy.
	Addr string

	mu      sync.Mutex
	offered []Credentials
	used    int
}

// Offered returns every set of credentials a client presented, in order.
func (p *Proxy) Offered() []Credentials {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]Credentials(nil), p.offered...)
}

// Used reports how many requests or tunnels the proxy carried through.
func (p *Proxy) Used() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.used
}

func (p *Proxy) offer(c Credentials) {
	p.mu.Lock()
	p.offered = append(p.offered, c)
	p.mu.Unlock()
}

func (p *Proxy) carried() {
	p.mu.Lock()
	p.used++
	p.mu.Unlock()
}

// loopback maps a destination to the same port on 127.0.0.1.
func loopback(hostPort string) string {
	_, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		port = "80"
	}
	return net.JoinHostPort("127.0.0.1", port)
}

// HTTP starts an HTTP proxy. With a non-empty user it answers 407 unless the
// request carries Basic credentials equal to user and password.
func HTTP(t *testing.T, user, password string) *Proxy {
	t.Helper()
	p := &Proxy{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, hasAuth := basicProxyAuth(r.Header.Get("Proxy-Authorization"))
		if hasAuth {
			p.offer(got)
		}
		if user != "" && (!hasAuth || got.User != user || got.Password != password) {
			w.Header().Set("Proxy-Authenticate", `Basic realm="proxytest"`)
			http.Error(w, "proxy authentication required", http.StatusProxyAuthRequired)
			return
		}
		if r.Method == http.MethodConnect {
			tunnel(w, r.Host, p)
			return
		}
		forward(w, r, p)
	}))
	t.Cleanup(srv.Close)
	p.Addr = srv.Listener.Addr().String()
	return p
}

func basicProxyAuth(h string) (Credentials, bool) {
	enc, ok := strings.CutPrefix(h, "Basic ")
	if !ok {
		return Credentials{}, false
	}
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return Credentials{}, false
	}
	u, pw, _ := strings.Cut(string(raw), ":")
	return Credentials{User: u, Password: pw}, true
}

func forward(w http.ResponseWriter, r *http.Request, p *Proxy) {
	out, err := http.NewRequest(r.Method, "http://"+loopback(r.URL.Host)+r.URL.RequestURI(), r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	out.Header = r.Header.Clone()
	out.Header.Del("Proxy-Authorization")
	resp, err := http.DefaultTransport.RoundTrip(out)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	p.carried()
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func tunnel(w http.ResponseWriter, target string, p *Proxy) {
	upstream, err := net.Dial("tcp", loopback(target))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	conn, rw, err := w.(http.Hijacker).Hijack()
	if err != nil {
		_ = upstream.Close()
		return
	}
	p.carried()
	_, _ = rw.WriteString("HTTP/1.1 200 Connection established\r\n\r\n")
	_ = rw.Flush()
	go func() { _, _ = io.Copy(upstream, rw); _ = upstream.Close() }()
	_, _ = io.Copy(conn, upstream)
	_ = conn.Close()
}

// SOCKS5 starts a SOCKS5 server (CONNECT only). With a non-empty user it requires
// username/password authentication (RFC 1929) and refuses other credentials.
func SOCKS5(t *testing.T, user, password string) *Proxy {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	p := &Proxy{Addr: ln.Addr().String()}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go p.serveSOCKS(conn, user, password)
		}
	}()
	return p
}

func (p *Proxy) serveSOCKS(conn net.Conn, user, password string) {
	defer func() { _ = conn.Close() }()
	r := bufio.NewReader(conn)
	hdr := make([]byte, 2)
	if _, err := io.ReadFull(r, hdr); err != nil {
		return
	}
	methods := make([]byte, hdr[1])
	if _, err := io.ReadFull(r, methods); err != nil {
		return
	}
	if user == "" {
		_, _ = conn.Write([]byte{5, 0})
	} else {
		if !strings.ContainsRune(string(methods), 2) {
			_, _ = conn.Write([]byte{5, 0xff})
			return
		}
		_, _ = conn.Write([]byte{5, 2})
		got, ok := readRFC1929(r)
		if !ok {
			return
		}
		p.offer(got)
		if got.User != user || got.Password != password {
			_, _ = conn.Write([]byte{1, 1})
			return
		}
		_, _ = conn.Write([]byte{1, 0})
	}
	req := make([]byte, 4)
	if _, err := io.ReadFull(r, req); err != nil {
		return
	}
	switch req[3] {
	case 1:
		_, _ = io.ReadFull(r, make([]byte, 4))
	case 3:
		n, _ := r.ReadByte()
		_, _ = io.ReadFull(r, make([]byte, n))
	case 4:
		_, _ = io.ReadFull(r, make([]byte, 16))
	}
	pb := make([]byte, 2)
	if _, err := io.ReadFull(r, pb); err != nil {
		return
	}
	upstream, err := net.Dial("tcp", "127.0.0.1:"+strconv.Itoa(int(binary.BigEndian.Uint16(pb))))
	if err != nil {
		_, _ = conn.Write([]byte{5, 5, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	defer func() { _ = upstream.Close() }()
	p.carried()
	_, _ = conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0})
	go func() { _, _ = io.Copy(upstream, r) }()
	_, _ = io.Copy(conn, upstream)
}

func readRFC1929(r *bufio.Reader) (Credentials, bool) {
	ver, err := r.ReadByte()
	if err != nil || ver != 1 {
		return Credentials{}, false
	}
	read := func() (string, bool) {
		n, err := r.ReadByte()
		if err != nil {
			return "", false
		}
		b := make([]byte, n)
		if _, err := io.ReadFull(r, b); err != nil {
			return "", false
		}
		return string(b), true
	}
	u, ok := read()
	if !ok {
		return Credentials{}, false
	}
	pw, ok := read()
	if !ok {
		return Credentials{}, false
	}
	return Credentials{User: u, Password: pw}, true
}
