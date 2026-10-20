package config

// What an editor leaves in a config file besides the configuration.
//
// A config.yaml comes off disk in whatever shape the editor that wrote it chose. On
// Windows that means every line ends with a carriage return and a line feed, and
// Notepad and its relatives may write a byte order mark in front of the first one.
// Neither is visible to the operator and neither says anything about the settings,
// but both travel through the file: the mark hides the "# yaml-language-server:"
// modeline from the check that looks for it, and a carriage return stays inside every
// comment the parser hands back, so the comment-preserving save wrote it out again
// and pushed a fresh blank line under each comment on every save.
//
// So the bytes are normalized on the way in - mark dropped, every line ending turned
// into a line feed, the line count untouched so a finding still points at the line the
// editor shows - and the file's own ending is put back on the way out, so a config
// written on Windows stays a Windows file.
//
// UTF-16 - Notepad's "Unicode", and what a Windows PowerShell 5.1 redirect writes - is
// decoded on the way in as well. Upstream refuses such a file instead; this fork reads
// it, because the YAML parser always decoded one, so a UTF-16 config has been starting
// FoxxyCode and refusing it would break a working setup on upgrade. It is still named:
// the check warns that a save writes the file back as UTF-8.

import (
	"bytes"

	"github.com/hijera/foxxycode-agent/internal/textenc"
)

const (
	utf8BOM   = "\xef\xbb\xbf"
	lineFeed  = "\n"
	crLineEnd = "\r\n"
)

// normalizeConfigSource makes config bytes read the same whatever editor wrote them:
// UTF-16 text is decoded, a UTF-8 byte order mark is dropped and every line ending
// becomes a line feed. The number of lines never changes, so positions reported
// against the result are the positions an editor shows.
func normalizeConfigSource(data []byte) []byte {
	return toLineFeeds(bytes.TrimPrefix(decodeUTF16Config(data), []byte(utf8BOM)))
}

// decodeUTF16Config returns the UTF-8 text of a config file saved as UTF-16, and any
// other file as it is. Only the byte order mark identifies UTF-16 here, the way
// utf16Encoding does; a file that does not decode is handed on unchanged, so the
// parser reports what is wrong with it.
func decodeUTF16Config(data []byte) []byte {
	var charset string
	switch utf16Encoding(data) {
	case "UTF-16 LE":
		charset = textenc.CharsetUTF16LE
	case "UTF-16 BE":
		charset = textenc.CharsetUTF16BE
	default:
		return data
	}
	text, err := textenc.DecodeAs(data, textenc.Encoding{Charset: charset, BOM: true})
	if err != nil {
		return data
	}
	return []byte(text)
}

// toLineFeeds turns Windows (CR LF) and classic Mac (CR) line endings into line feeds.
func toLineFeeds(data []byte) []byte {
	if !bytes.ContainsRune(data, '\r') {
		return data
	}
	out := bytes.ReplaceAll(data, []byte(crLineEnd), []byte(lineFeed))
	return bytes.ReplaceAll(out, []byte("\r"), []byte(lineFeed))
}

// configLineEnding reports the line ending a config file uses, so a save can write the
// file back the way its operator's editor expects it. A file whose lines mostly end the
// Windows way is a Windows file; everything else, an empty file included, is a line feed.
// A UTF-16 file is judged by its text, not by its bytes.
func configLineEnding(data []byte) string {
	data = decodeUTF16Config(data)
	total := bytes.Count(data, []byte(lineFeed))
	if total > 0 && bytes.Count(data, []byte(crLineEnd))*2 > total {
		return crLineEnd
	}
	return lineFeed
}

// applyLineEnding rewrites rendered config bytes with the given line ending.
func applyLineEnding(data []byte, ending string) []byte {
	data = toLineFeeds(data)
	if ending == crLineEnd {
		return bytes.ReplaceAll(data, []byte(lineFeed), []byte(crLineEnd))
	}
	return data
}

// utf16Encoding names the UTF-16 flavour a file was saved in, or the empty string when
// it is not one. Only the byte order mark is read: a file that starts with one is not
// UTF-8 whatever follows.
func utf16Encoding(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte{0xFF, 0xFE}):
		return "UTF-16 LE"
	case bytes.HasPrefix(data, []byte{0xFE, 0xFF}):
		return "UTF-16 BE"
	default:
		return ""
	}
}

// utf16Fix is how to turn such a file into UTF-8, in the words of the editor that wrote it.
const utf16Fix = `save the file as UTF-8 ("UTF-8" in the encoding list of Notepad's Save As dialog, "UTF-8 without BOM" in most editors)`
