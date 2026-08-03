package fs

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"

	"github.com/hijera/foxxycode-agent/internal/tooling"
)

// This matrix is the anti-regression net for routing the file tools through
// internal/textenc. Detection now happens in one shared place, so a change there
// moves read, grep, write, edit, apply_patch and the diff previews at once, and
// only a table that exercises every tool against every encoding catches it.
//
// Windows-1251 comes first on purpose: it is the everyday case for this fork,
// and the one where a silent charset swap would corrupt real files.

// matrixBody is the fixture every case is built from. It mixes Cyrillic letters
// with the punctuation that lives in the high range of Windows-1251 (guillemets,
// em dash, numero sign, yo) - exactly the characters a naive charset swap
// mangles first - plus an ASCII bulk, so detection has to work on a file whose
// majority is plain ASCII.
const matrixBody = "package main\n" +
	"\n" +
	"// Точка входа в программу.\n" +
	"func main() {\n" +
	"\tprintln(\"hello\")\n" +
	"\t// Здесь считаем сумму значений «от» и «до» — строка № 5, ёж.\n" +
	"\ttotal := 0\n" +
	"\tfor i := 0; i < 10; i++ {\n" +
	"\t\ttotal += i\n" +
	"\t}\n" +
	"\tprintln(total)\n" +
	"}\n"

// plainCyrillicBody drops the punctuation that only Windows-1251 and Unicode
// carry, for charsets such as KOI8-R whose high range is letters only.
const plainCyrillicBody = "package main\n" +
	"\n" +
	"// Точка входа в программу.\n" +
	"func main() {\n" +
	"\tprintln(\"hello\")\n" +
	"\t// Здесь считаем сумму значений от и до, строка пятая.\n" +
	"\ttotal := 0\n" +
	"\tfor i := 0; i < 10; i++ {\n" +
	"\t\ttotal += i\n" +
	"\t}\n" +
	"\tprintln(total)\n" +
	"}\n"

// latinBody is the Western counterpart, for the charsets that cannot carry
// Cyrillic at all.
const latinBody = "#!/bin/sh\n" +
	"# Copyright (C) 2003 Guido Günther <agx@example.org>\n" +
	"# Gruesse aus Muenchen, sagte er.\n" +
	"# Die Straße war größtenteils gesperrt und blieb es lange.\n" +
	"set -e\n" +
	"echo \"starting daemon\"\n" +
	"sleep 1\n" +
	"exit 0\n"

// asciiBody has no high bytes at all, so every tier of detection has to leave
// it alone.
const asciiBody = "package main\n" +
	"\n" +
	"// entry point of the program\n" +
	"func main() {\n" +
	"\tprintln(\"hello\")\n" +
	"\ttotal := 0\n" +
	"\tprintln(total)\n" +
	"}\n"

type encodingCase struct {
	name string
	body string
	// encode turns the body into the on-disk bytes for this case.
	encode func(t *testing.T, body string) []byte
	// insert is text the edit and patch steps add. It must be representable in
	// the case's charset, or writing back would legitimately fail.
	insert string
	// asciiBytesIntact says the ASCII characters of the body are stored as the
	// same single bytes on disk. It holds for UTF-8 and every single-byte legacy
	// charset, and is false for UTF-16, which interleaves NUL bytes. Only then
	// can system ripgrep - which matches raw bytes - answer an ASCII pattern.
	asciiBytesIntact bool
}

func encodeWith(enc encoding.Encoding) func(*testing.T, string) []byte {
	return func(t *testing.T, body string) []byte {
		t.Helper()
		out, err := enc.NewEncoder().Bytes([]byte(body))
		if err != nil {
			t.Fatalf("encode fixture: %v", err)
		}
		return out
	}
}

func encodingCases() []encodingCase {
	raw := func(_ *testing.T, body string) []byte { return []byte(body) }
	withBOM := func(_ *testing.T, body string) []byte {
		return append([]byte{0xEF, 0xBB, 0xBF}, body...)
	}
	return []encodingCase{
		{name: "windows-1251", body: matrixBody, encode: encodeWith(charmap.Windows1251), insert: "добавлено «тут» — №7", asciiBytesIntact: true},
		{name: "utf-8", body: matrixBody, encode: raw, insert: "добавлено 日本語", asciiBytesIntact: true},
		{name: "utf-8-bom", body: matrixBody, encode: withBOM, insert: "добавлено «тут»", asciiBytesIntact: true},
		{name: "utf-16le-bom", body: matrixBody, encode: encodeWith(unicode.UTF16(unicode.LittleEndian, unicode.UseBOM)), insert: "добавлено 日本語", asciiBytesIntact: false},
		{name: "utf-16be-bom", body: matrixBody, encode: encodeWith(unicode.UTF16(unicode.BigEndian, unicode.UseBOM)), insert: "добавлено 日本語", asciiBytesIntact: false},
		{name: "koi8-r", body: plainCyrillicBody, encode: encodeWith(charmap.KOI8R), insert: "добавлено тут", asciiBytesIntact: true},
		{name: "iso-8859-1", body: latinBody, encode: encodeWith(charmap.ISO8859_1), insert: "hinzugefügt hier", asciiBytesIntact: true},
		{name: "ascii", body: asciiBody, encode: raw, insert: "appended here", asciiBytesIntact: true},
	}
}

// TestEncodingMatrixReadReturnsTheSameText is the first column: whatever the
// on-disk encoding, read hands the model the original text, not mojibake.
func TestEncodingMatrixReadReturnsTheSameText(t *testing.T) {
	for _, tc := range encodingCases() {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeFixture(t, root, "source.go", tc.encode(t, tc.body))

			out, err := executeRead(context.Background(), `{"path":"source.go"}`, &tooling.Env{CWD: root})
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if out != tc.body {
				t.Fatalf("read returned\n%q\nwant\n%q", out, tc.body)
			}
		})
	}
}

// TestEncodingMatrixReadPaginates covers the paged form, which slices the
// decoded text rather than the raw bytes - a place a byte/rune mix-up would
// only show up on a non-UTF-8 file.
func TestEncodingMatrixReadPaginates(t *testing.T) {
	for _, tc := range encodingCases() {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeFixture(t, root, "source.go", tc.encode(t, tc.body))

			out, err := executeRead(context.Background(), `{"path":"source.go","offset":2,"limit":2}`, &tooling.Env{CWD: root})
			if err != nil {
				t.Fatalf("read paged: %v", err)
			}
			lines := strings.Split(strings.TrimRight(tc.body, "\n"), "\n")
			for _, want := range lines[1:min(3, len(lines))] {
				if want != "" && !strings.Contains(out, want) {
					t.Fatalf("paged read missing %q, got\n%s", want, out)
				}
			}
		})
	}
}

// TestEncodingMatrixGrepFindsBothPatternKinds exercises both grep branches: an
// ASCII pattern may go to the system ripgrep, a non-ASCII one is routed to the
// built-in engine precisely because ripgrep searches raw bytes.
func TestEncodingMatrixGrepFindsBothPatternKinds(t *testing.T) {
	for _, tc := range encodingCases() {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeFixture(t, root, "source.go", tc.encode(t, tc.body))

			asciiPattern, nonASCIIPattern := patternsFor(tc.body)

			// An ASCII pattern may be answered by system ripgrep, which matches
			// raw bytes. That only reaches files whose ASCII characters are
			// stored as single bytes; UTF-16 has to go through the built-in
			// engine, which decodes each file first.
			out, err := runGrep(t, root, asciiPattern, tc.asciiBytesIntact)
			if err != nil {
				t.Fatalf("grep ascii: %v", err)
			}
			if !strings.Contains(out, asciiPattern) {
				t.Fatalf("grep %q found nothing in %s:\n%s", asciiPattern, tc.name, out)
			}

			if nonASCIIPattern == "" {
				return
			}
			// A non-ASCII pattern always uses the built-in engine, whatever is
			// on PATH; this is the branch legacy encodings depend on.
			out, err = runGrep(t, root, nonASCIIPattern, true)
			if err != nil {
				t.Fatalf("grep non-ascii: %v", err)
			}
			if !strings.Contains(out, nonASCIIPattern) {
				t.Fatalf("grep %q found nothing in %s:\n%s", nonASCIIPattern, tc.name, out)
			}
		})
	}
}

// TestEncodingMatrixWritePreservesEncoding pins the round trip that matters
// most: overwriting a file must not convert it, and the bytes must come back
// exactly - byte-order mark included.
func TestEncodingMatrixWritePreservesEncoding(t *testing.T) {
	for _, tc := range encodingCases() {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			original := tc.encode(t, tc.body)
			path := writeFixture(t, root, "source.go", original)

			// Rewriting the identical content must reproduce the identical bytes.
			args := mustJSON(t, map[string]string{"path": "source.go", "content": tc.body})
			if _, err := executeWrite(context.Background(), args, &tooling.Env{CWD: root}); err != nil {
				t.Fatalf("write: %v", err)
			}
			assertFileBytes(t, path, original, "rewriting identical content changed the file")

			// A real change keeps the encoding and stays readable through read.
			changed := tc.body + tc.insert + "\n"
			args = mustJSON(t, map[string]string{"path": "source.go", "content": changed})
			if _, err := executeWrite(context.Background(), args, &tooling.Env{CWD: root}); err != nil {
				t.Fatalf("write changed: %v", err)
			}
			assertFileBytes(t, path, tc.encode(t, changed), "write did not keep the file's encoding")
		})
	}
}

// TestEncodingMatrixWriteCreatesUTF8 pins the other half of the write rule: a
// file that did not exist is created as UTF-8, whatever its neighbours are.
func TestEncodingMatrixWriteCreatesUTF8(t *testing.T) {
	root := t.TempDir()
	args := mustJSON(t, map[string]string{"path": "fresh.txt", "content": "Новый файл\n"})
	if _, err := executeWrite(context.Background(), args, &tooling.Env{CWD: root}); err != nil {
		t.Fatalf("write: %v", err)
	}
	assertFileBytes(t, filepath.Join(root, "fresh.txt"), []byte("Новый файл\n"), "a new file was not created as UTF-8")
}

// TestEncodingMatrixEditPreservesEncoding checks the edit path, including that
// the parts of the file nobody touched come back byte for byte.
func TestEncodingMatrixEditPreservesEncoding(t *testing.T) {
	for _, tc := range encodingCases() {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := writeFixture(t, root, "source.go", tc.encode(t, tc.body))

			oldLine, newLine := editPairFor(tc.body, tc.insert)
			args := mustJSON(t, map[string]string{"path": "source.go", "oldString": oldLine, "newString": newLine})
			if _, err := executeEdit(context.Background(), args, &tooling.Env{CWD: root}); err != nil {
				t.Fatalf("edit: %v", err)
			}

			want := tc.encode(t, strings.Replace(tc.body, oldLine, newLine, 1))
			assertFileBytes(t, path, want, "edit did not keep the file's encoding")
		})
	}
}

// TestEncodingMatrixApplyPatchPreservesEncoding does the same for unified diffs,
// whose hunks are matched against the decoded text.
func TestEncodingMatrixApplyPatchPreservesEncoding(t *testing.T) {
	for _, tc := range encodingCases() {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := writeFixture(t, root, "source.go", tc.encode(t, tc.body))

			lines := strings.Split(strings.TrimRight(tc.body, "\n"), "\n")
			patch := "@@\n" +
				" " + lines[0] + "\n" +
				"+" + tc.insert + "\n"
			args := mustJSON(t, map[string]string{"path": "source.go", "patch": patch})
			if _, err := executeApplyPatch(context.Background(), args, &tooling.Env{CWD: root}); err != nil {
				t.Fatalf("apply_patch: %v", err)
			}

			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			text, enc, err := decodeText(got)
			if err != nil {
				t.Fatalf("re-decode after patch: %v", err)
			}
			if !strings.Contains(text, tc.insert) {
				t.Fatalf("patch did not land: %q", text)
			}
			// The file must still be the same encoding it started as, which is
			// what re-encoding the patched text and comparing bytes proves.
			roundTripped, err := encodeText(text, enc)
			if err != nil {
				t.Fatalf("re-encode: %v", err)
			}
			if !bytes.Equal(roundTripped, got) {
				t.Fatalf("patched file is no longer self-consistent in %+v", enc)
			}
			if tc.name != "utf-8" && tc.name != "ascii" && bytes.Equal(got, []byte(text)) {
				t.Fatalf("patch silently converted %s to UTF-8", tc.name)
			}
		})
	}
}

// TestEncodingMatrixPreviewMatchesWrite pins that the diff shown in a permission
// prompt is the byte stream that would actually be written.
func TestEncodingMatrixPreviewMatchesWrite(t *testing.T) {
	for _, tc := range encodingCases() {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			original := tc.encode(t, tc.body)
			writeFixture(t, root, "source.go", original)

			changed := tc.body + tc.insert + "\n"
			args := mustJSON(t, map[string]string{"path": "source.go", "content": changed})
			_, before, after, ok, err := EditPreview("write", args, root)
			if err != nil || !ok {
				t.Fatalf("EditPreview: ok=%v err=%v", ok, err)
			}
			if !bytes.Equal(before, original) {
				t.Fatal("preview reported different bytes than the file holds")
			}
			if !bytes.Equal(after, tc.encode(t, changed)) {
				t.Fatalf("preview would write the file in a different encoding (%s)", tc.name)
			}
		})
	}
}

// TestEncodingMatrixBinaryFileIsRefused is the negative row. Binary content has
// no text reading, and handing the model a page of noise is worse than an error;
// grep and glob skip such files rather than reporting matches in noise.
func TestEncodingMatrixBinaryFileIsRefused(t *testing.T) {
	root := t.TempDir()
	png := append([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, bytes.Repeat([]byte{0x00, 0x01, 0x02, 0xFF}, 64)...)
	path := writeFixture(t, root, "image.png", png)

	if _, err := executeRead(context.Background(), `{"path":"image.png"}`, &tooling.Env{CWD: root}); err == nil {
		t.Fatal("read accepted a binary file")
	}

	out, err := runGrep(t, root, "IHDR", false)
	if err != nil {
		t.Fatalf("grep over a binary file: %v", err)
	}
	if strings.Contains(out, "image.png") {
		t.Fatalf("grep reported a match inside binary content:\n%s", out)
	}

	// Nothing above may have rewritten the file.
	assertFileBytes(t, path, png, "a binary file was modified by a read-only tool")
}

func writeFixture(t *testing.T, root, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertFileBytes(t *testing.T, path string, want []byte, msg string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s\n got %x\nwant %x", msg, got, want)
	}
}

func mustJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// runGrep searches root for pattern. allowSystemRG false pins the built-in
// engine regardless of what is installed, by handing grep a runner that has no
// ripgrep to find.
func runGrep(t *testing.T, root, pattern string, allowSystemRG bool) (string, error) {
	t.Helper()
	args := mustJSON(t, map[string]interface{}{"pattern": pattern, "path": ".", "case_sensitive": true})
	if allowSystemRG {
		return executeGrep(context.Background(), args, &tooling.Env{CWD: root})
	}
	return executeGrepWithRunner(context.Background(), args, &tooling.Env{CWD: root}, grepRunner{})
}

// patternsFor picks one ASCII and one non-ASCII substring out of the fixture, so
// both grep branches are exercised with text that is really in the file.
func patternsFor(body string) (ascii, nonASCII string) {
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		hasNonASCII := false
		for _, r := range trimmed {
			if r > 127 {
				hasNonASCII = true
				break
			}
		}
		if hasNonASCII && nonASCII == "" {
			nonASCII = firstWordOver(trimmed, 3, true)
		}
		if !hasNonASCII && ascii == "" {
			ascii = firstWordOver(trimmed, 3, false)
		}
	}
	return ascii, nonASCII
}

// firstWordOver returns the first word of at least minLen runes whose
// non-ASCII-ness matches want, and which carries no regex metacharacters.
func firstWordOver(line string, minLen int, wantNonASCII bool) string {
	for _, word := range strings.FieldsFunc(line, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '(' || r == ')' || r == '"' || r == '.' || r == ',' ||
			r == '{' || r == '}' || r == ';' || r == ':' || r == '<' || r == '>' || r == '+' ||
			r == '=' || r == '/' || r == '«' || r == '»' || r == '№' || r == '—' || r == '#' || r == '!'
	}) {
		if len([]rune(word)) < minLen {
			continue
		}
		hasNonASCII := false
		for _, r := range word {
			if r > 127 {
				hasNonASCII = true
				break
			}
		}
		if hasNonASCII == wantNonASCII {
			return word
		}
	}
	return ""
}

// editPairFor returns a line present in the fixture and its replacement.
func editPairFor(body, insert string) (oldLine, newLine string) {
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	oldLine = lines[len(lines)-1]
	return oldLine, oldLine + " " + insert
}
