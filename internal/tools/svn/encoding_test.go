package svn_test

// The tools sit on top of svnws, which decodes the client's output. This checks
// the tool layer does not undo that: report() has to hand the model text it can
// read, not the raw bytes svn.exe wrote.

import (
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hijera/foxxycode-agent/internal/svnws/svntest"
)

func TestToolOutputIsDecoded(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("only a Windows client writes a legacy code page")
	}
	codePage := svntest.ANSICodePage()
	if codePage == 0 || codePage == 65001 {
		t.Skip("no legacy ANSI code page on this machine")
	}
	sample, ok := svntest.NonASCIISample()
	if !ok {
		t.Skipf("ANSI code page %d is not covered by this test", codePage)
	}

	h := newHarness(t)
	state := svntest.NewState(repoRoot, h.wc)
	state.OutputCodePage = codePage
	state.Status = "M       " + sample + ".go"
	state.Diff = "Index: " + sample + ".go\n@@ -1,2 +1,3 @@\n+// " + sample
	state.Log = "r12 | dev | 2026-07-01 10:00:00 +0300 | 1 line\n\n" + sample
	if err := h.fake.WriteState(state); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		tool string
		args map[string]interface{}
	}{
		{"svn_status", map[string]interface{}{}},
		{"svn_diff", map[string]interface{}{}},
		{"svn_log", map[string]interface{}{"limit": 5}},
	} {
		out := h.run(t, tc.tool, tc.args)
		if !utf8.ValidString(out) {
			t.Errorf("%s output is not valid UTF-8: %q", tc.tool, out)
		}
		if !strings.Contains(out, sample) {
			t.Errorf("%s = %q, want it to contain %q", tc.tool, out, sample)
		}
	}
}
