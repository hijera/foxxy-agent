package docsgen

import (
	"bytes"
	"os"
)

// readFile reads a documentation source with its line endings normalised to
// LF. This repository is developed on Windows, where a checkout under
// core.autocrlf carries CRLF: the fence and heading patterns anchor on "$",
// and a generated block compared against a CRLF file would read as stale on
// every run. Git stores LF, so what is generated and checked is the committed
// form, the same on a Windows working tree and on the Linux runner.
func readFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n")), nil
}
