package mcp

import (
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/config"
)

// redactMinLength is the shortest individual word RedactValues takes out of a
// message. A whole declared value is redacted at any length; the threshold only
// keeps an ordinary word of a longer value from mangling unrelated text.
const redactMinLength = 8

// RedactValues takes out of a message about srv - the probe or connect error a
// surface shows next to the server - what its declaration resolves to and the
// list never shows. The URL as contacted becomes the URL as written, since a
// transport error prints the request URL and a ${NAME} in its query is a
// token. Every env and header value, as written and as resolved, and every
// variable the declaration reads (ReadsEnvironment) becomes
// config.RedactedValue: a server's own refusal may echo the credential it
// refused.
func RedactValues(srv config.MCPServerConfig, cwd, msg string) string {
	if msg == "" {
		return msg
	}
	if srv.URL != "" {
		if contacted := config.ExpandMCPValue(srv.URL, cwd); contacted != srv.URL && contacted != "" {
			msg = strings.ReplaceAll(msg, contacted, srv.URL)
			if u, err := url.Parse(contacted); err == nil {
				msg = strings.ReplaceAll(msg, u.String(), srv.URL)
			}
		}
	}
	var values []string
	// A value as written is a secret only when it names no variable: a
	// ${NAME} reference is what the list shows, and taking it out would also
	// cut it from the URL restored above. Its resolved form is added anyway.
	written := func(v string) {
		if !strings.Contains(v, "${") {
			values = append(values, v)
		}
		values = append(values, config.ExpandMCPValue(v, cwd))
	}
	for _, e := range srv.Env {
		written(e.Value)
	}
	for _, h := range srv.Headers {
		written(h.Value)
	}
	for _, name := range ReadsEnvironment(srv) {
		values = append(values, os.Getenv(name))
	}
	seen := map[string]bool{}
	var secrets []string
	add := func(s string) {
		if s != "" && s != config.RedactedValue && !seen[s] {
			seen[s] = true
			secrets = append(secrets, s)
		}
	}
	for _, v := range values {
		// The whole value is sensitive at any length. Individual words use a
		// floor because a short word of "Bearer <token>" may be ordinary text.
		for _, s := range []string{v, url.QueryEscape(v), url.PathEscape(v)} {
			add(s)
		}
		for _, s := range strings.Fields(v) {
			if len(s) < redactMinLength {
				continue
			}
			add(s)
			add(url.QueryEscape(s))
			add(url.PathEscape(s))
		}
	}
	// Longest first, so a value is never cut by a shorter one inside it.
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	for _, s := range secrets {
		if len(s) >= redactMinLength {
			msg = strings.ReplaceAll(msg, s, config.RedactedValue)
			continue
		}
		// A short value is replaced only as a token: DEBUG=1 must not turn
		// HTTP 401 into 40<redacted>, but an echoed one-character value still leaks.
		msg = replaceShortSecret(msg, s)
	}
	return msg
}

func replaceShortSecret(msg, secret string) string {
	var b strings.Builder
	for i := 0; i < len(msg); {
		end := i + len(secret)
		if end <= len(msg) && strings.HasPrefix(msg[i:], secret) &&
			(i == 0 || !secretTokenByte(msg[i-1])) &&
			(end == len(msg) || !secretTokenByte(msg[end])) {
			b.WriteString(config.RedactedValue)
			i = end
			continue
		}
		b.WriteByte(msg[i])
		i++
	}
	return b.String()
}

func secretTokenByte(b byte) bool {
	return b == '_' || b >= '0' && b <= '9' || b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z'
}
