package textenc

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
)

func TestReaderStreamsPastSampleAndPreservesSplitRune(t *testing.T) {
	want := strings.Repeat("a", 8191) + "Привет\n" + strings.Repeat("tail\n", 200000)
	src := strings.NewReader(want)
	r, charset, err := NewReader(src)
	if err != nil {
		t.Fatal(err)
	}
	if src.Len() != len(want)-8193 {
		t.Fatalf("eagerly read %d bytes", len(want)-src.Len())
	}
	got, err := io.ReadAll(r)
	if err != nil || string(got) != want || charset != CharsetUTF8 {
		t.Fatalf("stream changed: charset=%s length=%d err=%v", charset, len(got), err)
	}
}

func TestReaderUsesExistingEncodingPolicy(t *testing.T) {
	want := strings.Repeat("Привет, мир!\r\n", 1000)
	utf16, err := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewEncoder().Bytes([]byte(want))
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := charmap.Windows1251.NewEncoder().Bytes([]byte(want))
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"utf16": utf16, "legacy": legacy, "bom": append([]byte{0xef, 0xbb, 0xbf}, []byte(want)...)} {
		t.Run(name, func(t *testing.T) {
			r, _, err := NewReader(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			got, err := io.ReadAll(r)
			if err != nil || string(got) != want {
				t.Fatalf("wrong decoding: %.40q %v", got, err)
			}
		})
	}
}

func TestReaderPreservesLegacyBytesAtSampleEnd(t *testing.T) {
	for _, size := range []int{242, 8192, 8193} {
		for _, tail := range [][]byte{{0xcf}, {0xcf, 0xf0}, {0xff}} {
			if size > 8192 && len(tail) == 1 {
				continue // Entirely ASCII samples cannot predict later legacy text.
			}
			prefix := bytes.Repeat([]byte("name = value\n"), size/13+1)
			data := append(prefix[:size-len(tail):size-len(tail)], tail...)
			r, gotCharset, err := NewReader(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			got, err := io.ReadAll(r)
			// Short legacy samples can tie between single-byte charsets. They
			// must never lose their non-ASCII bytes to UTF-8 replacement runes.
			if err != nil || gotCharset == CharsetUTF8 || bytes.ContainsRune(got, utf8.RuneError) || utf8.RuneCount(got) != len(data) {
				t.Errorf("size=%d tail=%x: charset=%s runes=%d err=%v", size, tail, gotCharset, utf8.RuneCount(got), err)
			}
		}
	}
}
