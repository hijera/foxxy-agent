//go:build cli

package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/textenc"
)

func TestPromptFileUsesWorkspaceEncodingButStdinStaysUTF(t *testing.T) {
	const want = "Прочитай изменения в проекте и проверь русский текст.\r\n\r\n"
	data, err := (textenc.Encoding{Charset: "windows-1251"}).Encode(want)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "brief.md")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	env := printInputEnv{Stdin: stdinDevNull(t), Notices: io.Discard, IsTerminal: func(*os.File) bool { return false }, Limit: MaxPromptInputBytes}
	in, err := readPrintInput(parsedRequest(t, "-i", path), env)
	if err != nil || in.Prompt != want {
		t.Fatalf("Windows-1251 file: prompt=%q error=%v", in.Prompt, err)
	}
	env.Stdin = stdinPipe(t, string(data))
	if _, err := readPrintInput(parsedRequest(t, "-p", "-"), env); err == nil || !strings.Contains(err.Error(), "not UTF-8") {
		t.Fatalf("Windows-1251 stdin must be refused: %v", err)
	}
}
