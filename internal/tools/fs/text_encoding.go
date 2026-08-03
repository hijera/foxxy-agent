package fs

import (
	"errors"
	"fmt"

	"github.com/hijera/foxxycode-agent/internal/textenc"
)

// textEncoding is how a workspace file was read, carried between decode and
// encode so a tool that rewrites a file puts it back in the encoding it found.
type textEncoding = textenc.Encoding

// fallbackEncoding is the legacy charset assumed for bytes that are not UTF-8
// and that neither statistical detection nor the system ANSI code page could
// name.
//
// FoxxyCode is developed and mostly run on Russian Windows, where an unmarked
// legacy text file is Windows-1251 in overwhelming practice. Reading one as such
// is what this layer always did unconditionally, and keeping it as the last rung
// makes the file tools behave identically on a Linux CI box, where DecodeANSI
// has no code page to offer.
var fallbackEncoding = textEncoding{Charset: "windows-1251"}

// errNotText is returned for content that has no text reading at all. It is a
// sentinel so callers can tell it from a decoder that merely failed.
var errNotText = errors.New("file is not text: it is binary or its encoding could not be detected")

// decodeText returns the file as UTF-8 text plus the encoding to write it back
// in. Detection is shared with prompt attachments (internal/textenc): byte-order
// marks, a UTF-8 fast path, a binary guard, charset detection, and the system
// ANSI code page. Only the last rung is local - see fallbackEncoding.
func decodeText(data []byte) (string, textEncoding, error) {
	text, enc, err := textenc.Decode(data)
	if err == nil {
		return text, enc, nil
	}
	if !errors.Is(err, textenc.ErrUndecodable) {
		return "", textEncoding{}, err
	}
	// Binary content has no text reading. Saying so beats handing the model a
	// page of noise, and it matches what search has always done with such files.
	if textenc.LooksBinary(data) {
		return "", textEncoding{}, errNotText
	}
	decoded, ferr := textenc.DecodeAs(data, fallbackEncoding)
	if ferr != nil {
		return "", textEncoding{}, fmt.Errorf("decode %s: %w", fallbackEncoding.Charset, ferr)
	}
	return decoded, fallbackEncoding, nil
}

func encodeText(content string, encoding textEncoding) ([]byte, error) {
	return encoding.Encode(content)
}

func existingTextEncoding(data []byte) (textEncoding, error) {
	_, encoding, err := decodeText(data)
	return encoding, err
}
