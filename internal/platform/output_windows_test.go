//go:build windows

package platform

import (
	"testing"
	"unicode/utf8"
)

// cp866Russian is "Русский" in code page 866, the encoding a Russian Windows
// console uses. These are the kind of bytes a PowerShell parser error arrives as
// (the script never runs, so it cannot switch its own output encoding first).
var cp866Russian = []byte{0x90, 0xE3, 0xE1, 0xE1, 0xAA, 0xA8, 0xA9}

func TestDecodeCodePage866(t *testing.T) {
	got, err := decodeCodePage(866, cp866Russian)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Русский" {
		t.Fatalf("decodeCodePage(866, % x) = %q, want %q", cp866Russian, got, "Русский")
	}
}

// Whatever the console code page of the machine running the tests, output that
// is not UTF-8 must not be handed on as mojibake.
func TestDecodeOutputRepairsNonUTF8(t *testing.T) {
	got := DecodeOutput(cp866Russian)
	if !utf8.ValidString(got) {
		t.Fatalf("DecodeOutput(% x) = %q, which is still not valid UTF-8", cp866Russian, got)
	}
}

// The desktop launcher and a detached http server own no console, so
// GetConsoleOutputCP fails for them. The OEM fallback must still name a code
// page - otherwise decoding silently gives up exactly where there is no
// terminal to read the raw bytes in.
func TestChildOutputCodePageHasOEMFallback(t *testing.T) {
	if cp := childOutputCodePage(); cp == 0 {
		t.Fatal("childOutputCodePage() = 0, want a usable code page")
	}
	if err := procGetOEMCP.Find(); err != nil {
		t.Fatalf("GetOEMCP must resolve in kernel32: %v", err)
	}
	oem, _, _ := procGetOEMCP.Call()
	if oem == 0 {
		t.Fatal("GetOEMCP() = 0, want the system OEM code page")
	}
	if _, err := decodeCodePage(uint32(oem), cp866Russian); err != nil {
		t.Fatalf("decodeCodePage(GetOEMCP()=%d): %v", oem, err)
	}
}

// ansiSample returns bytes in the machine's own ANSI code page together with the
// text they stand for. The bytes are spelled out rather than produced by a
// codec, so the test asserts a real conversion instead of round-tripping through
// the same table the implementation uses. 1251 is the page a Russian install
// reports, 1252 the Western default a CI runner reports.
func ansiSample(t *testing.T) (raw []byte, text string) {
	t.Helper()
	_, cp, ok := DecodeANSI([]byte{0xC0})
	if !ok {
		t.Skip("no usable ANSI code page")
	}
	switch cp {
	case 1251:
		return []byte{0xCF, 0xF0, 0xE8, 0xE2, 0xE5, 0xF2}, "Привет"
	case 1252:
		return []byte{0x47, 0x72, 0xFC, 0xDF, 0x65}, "Grüße"
	}
	t.Skipf("ANSI code page %d is not covered by this test", cp)
	return nil, ""
}

// The svn case. Its output is converted through the APR locale charset, which is
// the ANSI code page - not the console page DecodeOutput resolves, so the two
// helpers are not interchangeable here.
func TestDecodeANSIOutputDecodesTheSystemCodePage(t *testing.T) {
	raw, want := ansiSample(t)
	if got := DecodeANSIOutput(raw); got != want {
		t.Fatalf("DecodeANSIOutput(% x) = %q, want %q", raw, got, want)
	}
}

// svn diff converts its own headers but copies file content through untouched,
// so one buffer carries both encodings. This is the case that forces the
// per-line split: decoding the buffer as a whole would turn the UTF-8 body into
// mojibake while repairing the header.
func TestDecodeANSIOutputHandlesAMixedBuffer(t *testing.T) {
	raw, sample := ansiSample(t)
	const body = "+// Привет ✅\n"
	var buf []byte
	buf = append(buf, "Index: "...)
	buf = append(buf, raw...)
	buf = append(buf, ".go\n"...)
	buf = append(buf, body...)

	want := "Index: " + sample + ".go\n" + body
	if got := DecodeANSIOutput(buf); got != want {
		t.Fatalf("DecodeANSIOutput(% x) = %q, want %q", buf, got, want)
	}
}
