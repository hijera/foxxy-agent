package netx

import (
	"strings"
	"testing"
)

// The characters a proxy password can carry and what becomes of each. A raw @ is
// fine (the authority is split at the last one), and so is a colon after the
// first; / ? # cut the authority short, and must be percent-encoded.
func TestParseProxyURLPasswords(t *testing.T) {
	ok := []struct {
		raw, user, password, host string
	}{
		{"http://vlasov:p@ss@vm-squid3.example.com:3128", "vlasov", "p@ss", "vm-squid3.example.com:3128"},
		{"http://vlasov:p%40ss@vm-squid3.example.com:3128", "vlasov", "p@ss", "vm-squid3.example.com:3128"},
		{"http://vlasov:a:b@proxy:3128", "vlasov", "a:b", "proxy:3128"},
		{"http://vlasov:%2F%3F%23%25@proxy:3128", "vlasov", "/?#%", "proxy:3128"},
		{"socks5h://u:%D0%BF%D0%B0%D1%80%D0%BE%D0%BB%D1%8C@127.0.0.1:1080", "u", "пароль", "127.0.0.1:1080"},
		{"http://proxy.local:3128/", "", "", "proxy.local:3128"},
		{"HTTPS://proxy.local:8443", "", "", "proxy.local:8443"},
	}
	for _, c := range ok {
		u, err := ParseProxyURL(c.raw)
		if err != nil {
			t.Errorf("ParseProxyURL(%q): %v", c.raw, err)
			continue
		}
		pw, _ := u.User.Password()
		if u.User.Username() != c.user || pw != c.password || u.Host != c.host {
			t.Errorf("ParseProxyURL(%q) = user %q password %q host %q, want %q %q %q",
				c.raw, u.User.Username(), pw, u.Host, c.user, c.password, c.host)
		}
	}
}

// A broken proxy URL is refused with a hint, and the error never carries the
// password: it ends up in the settings dialog, the log and the dry run report.
func TestParseProxyURLRefusesWithoutLeakingThePassword(t *testing.T) {
	const secret = "S3cr"
	bad := []struct {
		name, raw, want string
	}{
		{"slash cuts the authority", "http://vlasov:" + secret + "/x@proxy:3128", "percent-encoded"},
		{"question mark", "http://vlasov:" + secret + "?x@proxy:3128", "percent-encoded"},
		{"hash", "http://vlasov:" + secret + "#x@proxy:3128", "percent-encoded"},
		// Digits right before the slash parse as a port: the request would go to
		// host "vlasov:3128" with no credentials at all.
		{"silent misroute", "http://vlasov:3128/" + secret + "@proxy:3128", "percent-encoded"},
		{"empty port then hash", "http://vlasov:#" + secret + "@proxy:3128", "percent-encoded"},
		{"space", "http://vlasov:" + secret + " x@proxy:3128", "percent-encoded"},
		{"cyrillic", "http://vlasov:" + secret + "ж@proxy:3128", "percent-encoded"},
		{"bad escape", "http://vlasov:" + secret + "%zz@proxy:3128", "percent-encoded"},
		{"scheme", "ftp://vlasov:" + secret + "@proxy:21", "unsupported proxy scheme"},
		{"no scheme", "vlasov:" + secret + "@proxy:3128", "unsupported proxy scheme"},
		{"no host", "http://", "no host"},
	}
	for _, c := range bad {
		_, err := ParseProxyURL(c.raw)
		if err == nil {
			t.Errorf("%s: ParseProxyURL(%q) accepted it", c.name, c.raw)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %q does not say %q", c.name, err, c.want)
		}
		if strings.Contains(err.Error(), secret) {
			t.Errorf("%s: error leaks the password: %q", c.name, err)
		}
	}
}

func TestRedactProxyURL(t *testing.T) {
	cases := map[string]string{
		"":                                  "",
		"http://proxy:3128":                 "http://proxy:3128",
		"http://alice:hunter2@proxy:3128":   "http://redacted@proxy:3128",
		"http://alice:p@ss/x#y@proxy:3128/": "http://redacted@proxy:3128/",
		"socks5h://token@10.0.0.1:1080":     "socks5h://redacted@10.0.0.1:1080",
		"alice:hunter2@proxy:3128":          "redacted@proxy:3128",
	}
	for raw, want := range cases {
		if got := RedactProxyURL(raw); got != want {
			t.Errorf("RedactProxyURL(%q) = %q, want %q", raw, got, want)
		}
	}
}

// Every caller of a proxy URL in netx goes through the same parser, so a dial
// setting with a broken password fails the same way and leaks nothing.
func TestDialFuncErrorDoesNotLeakTheProxyPassword(t *testing.T) {
	_, err := (Options{Proxy: "http://vlasov:S3cr/x@proxy:3128"}).DialFunc()
	if err == nil {
		t.Fatal("expected the broken proxy url to be refused")
	}
	if strings.Contains(err.Error(), "S3cr") {
		t.Fatalf("error leaks the password: %v", err)
	}
}
