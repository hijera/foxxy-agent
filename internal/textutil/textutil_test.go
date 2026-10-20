package textutil_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hijera/foxxycode-agent/internal/textutil"
)

func TestTailBytes(t *testing.T) {
	cases := []struct {
		name     string
		s        string
		maxBytes int
		want     string
	}{
		{"empty", "", 4, ""},
		{"under the cap keeps everything", "abc", 3, "abc"},
		{"at the cap keeps everything", "abc", 4, "abc"},
		{"ascii takes the tail exactly", "abcdefgh", 3, "fgh"},
		{"whole string when the cap is zero-length", "abc", 0, ""},
		{
			// The raw slice drops 52 of 2100 bytes: 17 whole runes plus the lead
			// byte of the next one, so the tail opens on two continuation bytes.
			"three-byte runes snap back to a boundary",
			strings.Repeat("好", 700), 2048,
			strings.Repeat("好", 682),
		},
		{
			// The raw slice drops 5 of 16 bytes, which is inside a two-byte rune.
			"two-byte runes snap back to a boundary",
			strings.Repeat("ж", 8), 11,
			strings.Repeat("ж", 5),
		},
		{
			"mixed tail keeps its ending",
			strings.Repeat("好", 700) + "\nTAIL_MARKER", 2048,
			strings.Repeat("好", 678) + "\nTAIL_MARKER",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := textutil.TailBytes(tc.s, tc.maxBytes)
			if got != tc.want {
				t.Fatalf("TailBytes(%d runes, %d) = %d runes, want %d runes",
					utf8.RuneCountInString(tc.s), tc.maxBytes,
					utf8.RuneCountInString(got), utf8.RuneCountInString(tc.want))
			}
			if !utf8.ValidString(got) {
				t.Fatalf("result is not valid UTF-8: %q", got)
			}
			if tc.s != "" && !strings.HasSuffix(tc.s, got) {
				t.Fatalf("result is not a suffix of the input: %q", got)
			}
			if len(got) > tc.maxBytes {
				t.Fatalf("result is %d bytes, over the %d byte cap", len(got), tc.maxBytes)
			}
		})
	}
}
