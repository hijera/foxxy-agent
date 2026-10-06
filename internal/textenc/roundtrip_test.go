package textenc_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/encoding/unicode/utf32"

	"github.com/hijera/foxxycode-agent/internal/textenc"
)

// The round-trip API is fork-specific: upstream only decodes attachments, while
// FoxxyCode also routes the file tools (read / write / edit / apply_patch)
// through this package, and those must put a file back exactly as they found it.

// TestDecodeEncodeRoundTripsBytes is the contract the file tools depend on:
// whatever Decode reports, Encode of the unchanged text reproduces the original
// bytes, byte-order mark included.
func TestDecodeEncodeRoundTripsBytes(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{name: "ascii", data: []byte("plain ascii text\n")},
		{name: "utf8", data: []byte(russian)},
		{name: "utf8 with bom", data: append([]byte{0xEF, 0xBB, 0xBF}, russian...)},
		{name: "windows-1251", data: mustEncode(t, charmap.Windows1251, russian)},
		{name: "koi8-r", data: mustEncode(t, charmap.KOI8R, russian)},
		{name: "utf16le with bom", data: mustEncode(t, unicode.UTF16(unicode.LittleEndian, unicode.UseBOM), russian)},
		{name: "utf16be with bom", data: mustEncode(t, unicode.UTF16(unicode.BigEndian, unicode.UseBOM), russian)},
		{name: "utf32le with bom", data: mustEncode(t, utf32.UTF32(utf32.LittleEndian, utf32.UseBOM), russian)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text, enc, err := textenc.Decode(tc.data)
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			got, err := enc.Encode(text)
			if err != nil {
				t.Fatalf("Encode(%+v): %v", enc, err)
			}
			if !bytes.Equal(got, tc.data) {
				t.Fatalf("round trip changed the bytes\n got %x\nwant %x\nencoding %+v", got, tc.data, enc)
			}
		})
	}
}

// TestDecodeReportsBOM pins that the mark is reported separately rather than
// folded into the charset name, since UTF-8 with and without a mark are the same
// charset but not the same file.
func TestDecodeReportsBOM(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
		want bool
	}{
		{name: "without bom", data: []byte(russian), want: false},
		{name: "with bom", data: append([]byte{0xEF, 0xBB, 0xBF}, russian...), want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text, enc, err := textenc.Decode(tc.data)
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if enc.BOM != tc.want {
				t.Fatalf("BOM = %v, want %v", enc.BOM, tc.want)
			}
			if text != russian {
				t.Fatalf("the mark leaked into the text: %q", text)
			}
		})
	}
}

// TestEncodeRejectsUnrepresentableRune keeps a silent substitution out of the
// write path: turning an inserted em dash into "?" would corrupt the file
// quietly, so the caller has to hear about it.
func TestEncodeRejectsUnrepresentableRune(t *testing.T) {
	enc := textenc.Encoding{Charset: "windows-1251"}
	if _, err := enc.Encode("японский текст: 日本語"); err == nil {
		t.Fatal("Encode accepted a rune Windows-1251 cannot represent")
	}
	// Everything Windows-1251 does cover must still go through untouched,
	// including the punctuation its high range carries.
	got, err := enc.Encode("тест — «кавычки», № 5, ёж")
	if err != nil {
		t.Fatalf("Encode rejected representable text: %v", err)
	}
	back, err := textenc.DecodeAs(got, enc)
	if err != nil {
		t.Fatalf("DecodeAs: %v", err)
	}
	if back != "тест — «кавычки», № 5, ёж" {
		t.Fatalf("round trip changed the text: %q", back)
	}
}

// TestEncodeUnsupportedCharset covers a charset name x/text does not know, which
// a Windows ANSI code page without an IANA name can produce.
func TestEncodeUnsupportedCharset(t *testing.T) {
	enc := textenc.Encoding{Charset: "windows-99999"}
	_, err := enc.Encode("text")
	if err == nil || !strings.Contains(err.Error(), "unsupported text encoding") {
		t.Fatalf("err = %v, want an unsupported-encoding error", err)
	}
}

// TestEncodeEmptyCharsetIsUTF8 pins the zero value as plain UTF-8, so a caller
// that never ran Decode still writes something sane.
func TestEncodeEmptyCharsetIsUTF8(t *testing.T) {
	got, err := textenc.Encoding{}.Encode(russian)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if string(got) != russian {
		t.Fatalf("encoded %q, want %q", got, russian)
	}
}

// TestLooksBinarySeparatesBinaryFromUnnamedText is what lets the file tools keep
// a legacy fallback without applying it to a PNG: ErrUndecodable alone does not
// say which of the two happened.
func TestLooksBinarySeparatesBinaryFromUnnamedText(t *testing.T) {
	pngHeader := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D, 'I', 'H', 'D', 'R'}
	if !textenc.LooksBinary(pngHeader) {
		t.Fatal("a PNG header did not read as binary")
	}
	// A short legacy fragment: too little evidence for detection, no NUL bytes.
	short := mustEncode(t, charmap.Windows1251, "да\n")
	if textenc.LooksBinary(short) {
		t.Fatal("short legacy text read as binary")
	}
	if _, _, err := textenc.DecodeToUTF8(pngHeader); !errors.Is(err, textenc.ErrUndecodable) {
		t.Fatalf("err = %v, want ErrUndecodable", err)
	}
	// Text with a byte-order mark is text whatever the mark is followed by.
	if textenc.LooksBinary(mustEncode(t, unicode.UTF16(unicode.LittleEndian, unicode.UseBOM), russian)) {
		t.Fatal("BOM-marked UTF-16 read as binary")
	}
}

// TestDecodeAsSkipsDetection is the escape hatch the fs layer uses for its own
// last-resort charset.
func TestDecodeAsSkipsDetection(t *testing.T) {
	data := mustEncode(t, charmap.Windows1251, russian)
	got, err := textenc.DecodeAs(data, textenc.Encoding{Charset: "windows-1251"})
	if err != nil {
		t.Fatalf("DecodeAs: %v", err)
	}
	if got != russian {
		t.Fatalf("decoded %q, want %q", got, russian)
	}
	if _, err := textenc.DecodeAs(data, textenc.Encoding{Charset: "windows-99999"}); err == nil {
		t.Fatal("DecodeAs accepted an unknown charset")
	}
}
