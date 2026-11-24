package intellij

import (
	"os"
	"strings"
	"testing"
)

// This platform boundary cannot run in plain JUnit: preparing a preview loads
// VirtualFile and Document through the IDE application. Guard its exception
// routing here, alongside the editor-context threading boundary guard.
func TestHookPreviewPreservesEncodingFallback(t *testing.T) {
	data, err := os.ReadFile("src/main/kotlin/dev/foxxycode/intellij/diff/FoxxyCodeToolExecutor.kt")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	start := strings.Index(src, "ev = prepareHookPreview(ev)")
	if start < 0 {
		t.Fatal("hook preview boundary missing")
	}
	boundary := strings.SplitN(src[start:], "val path = ev.path.trim()", 2)[0]
	encoding := strings.Index(boundary, "catch (e: EncodingNotSupported)")
	failure := strings.Index(boundary, "catch (e: Throwable)")
	if encoding < 0 || failure < 0 || encoding > failure || !strings.Contains(boundary[encoding:failure], "throw e") {
		t.Fatal("unsupported encoding must reach the outer not_handled handler, not become a handled error")
	}
}
