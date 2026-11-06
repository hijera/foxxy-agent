// Package textutil holds the small UTF-8-safe string helpers every byte cap in
// the tree needs. They live here rather than beside any one caller because the
// callers sit on different leaves (internal/ideenv and internal/ideterm are
// independent state stores, internal/idehost renders both) and none of them may
// grow a dependency on another for the sake of a slice.
package textutil

// TailBytes returns the last maxBytes bytes of s once s is longer than the cap,
// dropped back to a rune boundary so the result never opens on a partial rune.
// Only the front of the kept tail can be mid-character: the end of s is a
// boundary already.
func TailBytes(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	tail := s[len(s)-maxBytes:]
	for i := 0; i < len(tail) && i < 4; i++ {
		if tail[i]&0xC0 != 0x80 {
			return tail[i:]
		}
	}
	return tail
}
