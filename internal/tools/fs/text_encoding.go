package fs

import (
	"bytes"
	"fmt"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

type textEncoding uint8

const (
	textEncodingUTF8 textEncoding = iota
	textEncodingUTF8BOM
	textEncodingWindows1251
)

var utf8BOM = []byte{0xef, 0xbb, 0xbf}

// decodeText detects the supported on-disk encoding and returns UTF-8 text for tools.
// Valid UTF-8 wins because arbitrary legacy single-byte encodings are ambiguous.
func decodeText(data []byte) (string, textEncoding, error) {
	if bytes.HasPrefix(data, utf8BOM) {
		body := data[len(utf8BOM):]
		if !utf8.Valid(body) {
			return "", textEncodingUTF8BOM, fmt.Errorf("invalid UTF-8 after BOM")
		}
		return string(body), textEncodingUTF8BOM, nil
	}
	if utf8.Valid(data) {
		return string(data), textEncodingUTF8, nil
	}
	decoded, err := charmap.Windows1251.NewDecoder().Bytes(data)
	if err != nil {
		return "", textEncodingWindows1251, fmt.Errorf("decode Windows-1251: %w", err)
	}
	return string(decoded), textEncodingWindows1251, nil
}

func encodeText(content string, encoding textEncoding) ([]byte, error) {
	switch encoding {
	case textEncodingUTF8:
		return []byte(content), nil
	case textEncodingUTF8BOM:
		return append(append([]byte(nil), utf8BOM...), []byte(content)...), nil
	case textEncodingWindows1251:
		encoded, err := charmap.Windows1251.NewEncoder().Bytes([]byte(content))
		if err != nil {
			return nil, fmt.Errorf("encode Windows-1251: %w", err)
		}
		return encoded, nil
	default:
		return nil, fmt.Errorf("unsupported text encoding")
	}
}

func existingTextEncoding(data []byte) (textEncoding, error) {
	_, encoding, err := decodeText(data)
	return encoding, err
}
