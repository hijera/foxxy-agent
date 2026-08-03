//go:build windows

package platform

import (
	"fmt"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/sys/windows"
)

// DecodeOutput converts captured child-process output to UTF-8.
//
// Console programs on Windows write in the console output code page (866 on a
// Russian install, 437 on a US one), so their bytes are not UTF-8. Commands run
// through wrapPowerShellCommand switch to UTF-8 themselves, but a PowerShell
// script that fails to parse never reaches its own prologue, and its parser
// error still arrives in the legacy code page - that is the output the model has
// to read to recover. Output that is already valid UTF-8 is returned untouched,
// which keeps the wrapped case a no-op.
func DecodeOutput(b []byte) string {
	if len(b) == 0 || utf8.Valid(b) {
		return string(b)
	}
	codePage := childOutputCodePage()
	if codePage == 0 {
		return string(b)
	}
	decoded, err := decodeCodePage(codePage, b)
	if err != nil {
		return string(b)
	}
	return decoded
}

// procGetOEMCP is resolved lazily; x/sys/windows exports GetACP but not GetOEMCP.
var procGetOEMCP = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetOEMCP")

// childOutputCodePage returns the code page console children write in, or 0 when
// it cannot be determined.
//
// GetConsoleOutputCP only answers for a process that owns a console. The desktop
// launcher (foxxycode desktop / WebView2) and a detached `foxxycode http` have
// none, so it fails there - and those are exactly the runs where unreadable
// output is most visible, since there is no terminal to inspect. A console child
// spawned from such a process gets a fresh console at the system OEM code page
// (866 on a Russian install), which is what GetOEMCP reports, so that is the
// fallback. GetACP would be wrong here: on the same install it is 1251.
func childOutputCodePage() uint32 {
	if cp, err := windows.GetConsoleOutputCP(); err == nil && cp != 0 {
		return cp
	}
	if err := procGetOEMCP.Find(); err != nil {
		return 0
	}
	cp, _, _ := procGetOEMCP.Call()
	return uint32(cp)
}

// DecodeANSI converts bytes in the system ANSI code page (1251 on a Russian
// install, 1252 on a Western one) to a Go string, reporting the code page it
// used. It is the last-resort reading for a file a local editor saved without a
// byte-order mark; ok is false when there is no usable code page.
//
// This is the ANSI code page, not the console one DecodeOutput uses: a file
// written by Notepad and the output of a console child use different code pages
// on the same machine (1251 versus 866 on a Russian install).
func DecodeANSI(b []byte) (text string, codePage uint32, ok bool) {
	if len(b) == 0 {
		return "", 0, false
	}
	cp := windows.GetACP()
	if cp == 0 {
		return "", 0, false
	}
	decoded, err := decodeCodePage(cp, b)
	if err != nil {
		return "", 0, false
	}
	return decoded, cp, true
}

// decodeCodePage converts bytes in the given Windows code page to a Go string.
// dwFlags is 0 rather than MB_ERR_INVALID_CHARS so that a stray invalid byte
// yields a replacement character instead of discarding the whole message.
func decodeCodePage(codePage uint32, b []byte) (string, error) {
	size, err := windows.MultiByteToWideChar(codePage, 0, &b[0], int32(len(b)), nil, 0)
	if err != nil {
		return "", err
	}
	if size <= 0 {
		return "", fmt.Errorf("MultiByteToWideChar(%d) reported size %d", codePage, size)
	}
	buf := make([]uint16, size)
	if _, err := windows.MultiByteToWideChar(codePage, 0, &b[0], int32(len(b)), &buf[0], size); err != nil {
		return "", err
	}
	// utf16.Decode rather than windows.UTF16ToString: the latter stops at the
	// first NUL, and command output is arbitrary bytes.
	return string(utf16.Decode(buf)), nil
}
