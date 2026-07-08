package main

import (
	"os"
	"testing"
)

// TestWindowsIconResourcePresent guards the committed Windows icon resource so it
// cannot be silently dropped. The .syso embeds foxxycode.ico as RT_GROUP_ICON id 1
// and is auto-linked into every windows/amd64 build (desktop shell and CLI) as the
// .exe file icon; internal/desktop wires the same id into the WebView2 window.
// Regenerate with `make icon` when foxxycode2-Photoroom.png changes.
func TestWindowsIconResourcePresent(t *testing.T) {
	const syso = "rsrc_windows_amd64.syso"
	info, err := os.Stat(syso)
	if err != nil {
		t.Fatalf("missing Windows icon resource %s (run `make icon`): %v", syso, err)
	}
	if info.Size() < 1024 {
		t.Fatalf("%s is suspiciously small (%d bytes); regenerate with `make icon`", syso, info.Size())
	}
}
