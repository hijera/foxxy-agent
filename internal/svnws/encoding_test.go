package svnws_test

// Encoding coverage for the svn client's output. On Windows svn converts its
// messages to the system ANSI code page on the way out, so the bytes arriving on
// a pipe are not UTF-8 on an install whose ANSI page is a legacy one. See the
// comment on svnws.run for why that page, and not the console one.

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hijera/foxxycode-agent/internal/svnws"
	"github.com/hijera/foxxycode-agent/internal/svnws/svntest"
)

// newFakeState builds the fake client for a working copy at wc and installs the
// default repository model after mutate has adjusted it.
func newFakeState(t *testing.T, wc string, mutate func(*svntest.State)) (svntest.Fake, svnws.Options) {
	t.Helper()
	fake, err := svntest.Build(t.TempDir())
	if err != nil {
		t.Fatalf("build fake svn: %v", err)
	}
	fake.Setenv(t.Setenv)
	if err := os.MkdirAll(wc, 0o755); err != nil {
		t.Fatalf("mkdir wc: %v", err)
	}
	state := svntest.NewState(fakeRepoRoot, wc)
	mutate(&state)
	if err := fake.WriteState(state); err != nil {
		t.Fatalf("write fake state: %v", err)
	}
	return fake, svnws.Options{Binary: fake.Binary, TimeoutSeconds: 30}
}

// withSample fills status, diff and log with text built from sample and tells
// the fake client which code page to write it in. Zero leaves it UTF-8.
func withSample(codePage int, sample string) func(*svntest.State) {
	return func(s *svntest.State) {
		s.OutputCodePage = codePage
		s.Status = "M       " + sample + ".go"
		s.Diff = "Index: " + sample + ".go\n@@ -1,2 +1,3 @@\n+// " + sample
		s.Log = "r12 | dev | 2026-07-01 10:00:00 +0300 | 1 line\n\n" + sample
	}
}

// nonASCIISample returns text this machine's client can round-trip, skipping
// where svntest knows of none.
func nonASCIISample(t *testing.T) string {
	t.Helper()
	sample, ok := svntest.NonASCIISample()
	if !ok {
		t.Skipf("ANSI code page %d is not covered by this test", svntest.ANSICodePage())
	}
	return sample
}

// ansiFixture is nonASCIISample for the tests that need the client to write a
// legacy page, and skips where there is none to write.
func ansiFixture(t *testing.T) (codePage int, sample string) {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("only a Windows client writes a legacy code page")
	}
	cp := svntest.ANSICodePage()
	if cp == 0 || cp == 65001 {
		t.Skip("no legacy ANSI code page on this machine")
	}
	return cp, nonASCIISample(t)
}

// assertSampleSurvives runs the read-only operations and checks the sample text
// arrives intact and as valid UTF-8.
func assertSampleSurvives(t *testing.T, wc string, opts svnws.Options, sample string) {
	t.Helper()
	ctx := context.Background()
	for _, tc := range []struct {
		name string
		run  func() (string, error)
	}{
		{"status", func() (string, error) { return svnws.Status(ctx, wc, opts, nil) }},
		{"diff", func() (string, error) { return svnws.Diff(ctx, wc, opts, nil, "") }},
		{"log", func() (string, error) { return svnws.Log(ctx, wc, opts, "", 0) }},
	} {
		out, err := tc.run()
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if !utf8.ValidString(out) {
			t.Errorf("%s output is not valid UTF-8: %q", tc.name, out)
		}
		if !strings.Contains(out, sample) {
			t.Errorf("%s output = %q, want it to contain %q", tc.name, out, sample)
		}
	}
}

// A client that already writes UTF-8 - svn on Linux and macOS, and a Windows
// install with a UTF-8 ANSI code page - must round-trip byte for byte.
func TestUTF8ClientOutputIsRelayedVerbatim(t *testing.T) {
	const sample = "Привет-Мир"
	wc := filepath.Join(t.TempDir(), "wc")
	_, opts := newFakeState(t, wc, withSample(0, sample))
	assertSampleSurvives(t, wc, opts, sample)
}

// The regression: output in the system ANSI code page reached the model as
// mojibake because the raw bytes were handed on as a Go string.
func TestANSIClientOutputIsDecoded(t *testing.T) {
	codePage, sample := ansiFixture(t)
	wc := filepath.Join(t.TempDir(), "wc")
	_, opts := newFakeState(t, wc, withSample(codePage, sample))
	assertSampleSurvives(t, wc, opts, sample)
}

// An error message is the output the model has to read to recover, so it needs
// decoding as much as a successful result does.
func TestANSIErrorDetailIsDecoded(t *testing.T) {
	codePage, sample := ansiFixture(t)
	wc := filepath.Join(t.TempDir(), "wc")
	detail := "svn: E155015: conflict in " + sample + ".go"
	_, opts := newFakeState(t, wc, func(s *svntest.State) {
		s.OutputCodePage = codePage
		s.Fail = map[string]string{"commit": detail}
	})

	_, err := svnws.Commit(context.Background(), wc, opts, "msg", []string{"a.go"})
	if err == nil {
		t.Fatalf("commit should have failed")
	}
	if !strings.Contains(err.Error(), sample) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), sample)
	}
}
