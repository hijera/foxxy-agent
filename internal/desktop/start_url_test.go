package desktop

import "testing"

func TestDesktopStartURL(t *testing.T) {
	tests := []struct {
		addr   string
		locale string
		want   string
	}{
		{"127.0.0.1:12345", "", "http://127.0.0.1:12345/?desktop=1#/chat"},
		{"127.0.0.1:12345", "en", "http://127.0.0.1:12345/?lang=en&desktop=1#/chat"},
		{"127.0.0.1:12345", "ru", "http://127.0.0.1:12345/?lang=ru&desktop=1#/chat"},
		{"127.0.0.1:12345", "  ru  ", "http://127.0.0.1:12345/?lang=ru&desktop=1#/chat"},
		{"127.0.0.1:12345", "de", "http://127.0.0.1:12345/?desktop=1#/chat"},
	}
	for _, tc := range tests {
		if got := DesktopStartURL(tc.addr, tc.locale); got != tc.want {
			t.Fatalf("DesktopStartURL(%q, %q) = %q, want %q", tc.addr, tc.locale, got, tc.want)
		}
	}
}
