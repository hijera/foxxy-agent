package platform

import (
	"bytes"
	"strings"
	"unicode/utf8"
)

// DecodeANSIOutput converts captured child-process output written in the system
// ANSI code page to UTF-8, one line at a time.
//
// It is the sibling of DecodeOutput for a child that does not write in the
// console code page. Subversion is the case in hand: svn converts its messages
// through the APR locale charset, which on Windows is the ANSI code page, and it
// does so whether or not it owns a console - the bytes that reach a pipe are
// 1251 on a Russian install, never the 866 DecodeOutput assumes. Handing them to
// DecodeOutput would swap one kind of mojibake for another.
//
// The decision is per line because a single invocation can mix encodings: svn
// diff converts its own headers but copies file content through untouched, so a
// UTF-8 source file under a legacy-encoded path arrives as UTF-8 body lines
// around an ANSI header line. Deciding once for the whole buffer would corrupt
// whichever half lost.
//
// A line that is already valid UTF-8 is returned untouched, so a client that
// writes UTF-8 - svn on Linux and macOS, or a Windows install with a UTF-8 ANSI
// code page - round-trips verbatim.
func DecodeANSIOutput(b []byte) string {
	if len(b) == 0 || utf8.Valid(b) {
		return string(b)
	}
	var out strings.Builder
	out.Grow(len(b))
	// SplitAfter keeps the line terminators, so the result is the input with only
	// the undecodable lines rewritten.
	for _, line := range bytes.SplitAfter(b, []byte("\n")) {
		if utf8.Valid(line) {
			out.Write(line)
			continue
		}
		if text, _, ok := DecodeANSI(line); ok {
			out.WriteString(text)
			continue
		}
		out.Write(line)
	}
	return out.String()
}
