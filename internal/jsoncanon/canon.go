// Package jsoncanon writes a JSON document in one canonical form: no
// insignificant whitespace and the keys of every object sorted, so the same
// document written with the keys in two orders compares equal. Objects and
// arrays are read into typed containers of raw members and canonicalized
// member by member; a scalar is compacted as it was written, so nothing is
// decoded into an arbitrary interface value - which also means two spellings
// of one string or number ("A" and "\u0041", 1.0 and 1e0) stay different,
// close enough for the fingerprints this package feeds.
package jsoncanon

import (
	"bytes"
	"encoding/json"
	"errors"
	"sort"
)

// Canonical returns raw in its canonical form, or an error when raw is not
// one JSON value.
func Canonical(raw []byte) ([]byte, error) {
	var b bytes.Buffer
	if err := write(&b, bytes.TrimSpace(raw)); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func write(b *bytes.Buffer, raw []byte) error {
	if len(raw) == 0 {
		return errors.New("jsoncanon: empty document")
	}
	switch raw[0] {
	case '{':
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			return err
		}
		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			name, err := json.Marshal(k)
			if err != nil {
				return err
			}
			b.Write(name)
			b.WriteByte(':')
			if err := write(b, bytes.TrimSpace(obj[k])); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	case '[':
		var arr []json.RawMessage
		if err := json.Unmarshal(raw, &arr); err != nil {
			return err
		}
		b.WriteByte('[')
		for i, el := range arr {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := write(b, bytes.TrimSpace(el)); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	default:
		if !json.Valid(raw) {
			return errors.New("jsoncanon: not a JSON value")
		}
		if err := json.Compact(b, raw); err != nil {
			return err
		}
	}
	return nil
}
