package platform

import (
	"strings"
	"testing"
)

// DecodeOutput must be a no-op for anything already valid UTF-8, on every
// platform: that is the path all wrapped PowerShell output and all POSIX output
// takes.
func TestDecodeOutputPassesThroughUTF8(t *testing.T) {
	for _, want := range []string{
		"",
		"plain ascii output\n",
		"Русский текст — em dash ✅ 中文\r\n",
		"Expand `agent.qwen.md` to **7 blocks** (21 items)",
	} {
		if got := DecodeOutput([]byte(want)); got != want {
			t.Fatalf("DecodeOutput(%q) = %q, want it unchanged", want, got)
		}
	}
}

// DecodeANSIOutput takes the same no-op path for anything already valid UTF-8,
// so svn on Linux and macOS - and a Windows install with a UTF-8 ANSI code page
// - round-trips verbatim.
func TestDecodeANSIOutputPassesThroughUTF8(t *testing.T) {
	for _, want := range []string{
		"",
		"M       src/main.go\n",
		"Русский текст — em dash ✅ 中文\r\n",
		"Committed revision 12.\n",
	} {
		if got := DecodeANSIOutput([]byte(want)); got != want {
			t.Fatalf("DecodeANSIOutput(%q) = %q, want it unchanged", want, got)
		}
	}
}

// The per-line contract, assertable without a code page: whatever happens to the
// undecodable line, the UTF-8 lines around it come back byte for byte. A whole
// buffer decode would rewrite them too, which is how a UTF-8 source file inside
// an svn diff gets corrupted.
func TestDecodeANSIOutputKeepsTheUTF8LinesOfAMixedBuffer(t *testing.T) {
	const body = "+// Привет ✅\n"
	var buf []byte
	buf = append(buf, body...)
	// 0xCF is not a valid UTF-8 lead byte, so this line forces the slow path.
	buf = append(buf, []byte{0x49, 0x6e, 0x64, 0x65, 0x78, 0x3a, 0x20, 0xCF, 0x0a}...)
	buf = append(buf, body...)

	lines := strings.SplitAfter(DecodeANSIOutput(buf), "\n")
	if len(lines) < 3 {
		t.Fatalf("DecodeANSIOutput(% x) = %q, want three lines", buf, lines)
	}
	if lines[0] != body || lines[2] != body {
		t.Errorf("UTF-8 lines were rewritten: %q and %q, want %q", lines[0], lines[2], body)
	}
}
