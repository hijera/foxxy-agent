package intellij

// FoxxyCodeEditorContextService snapshots the open editors and the text selection
// for POST /foxxycode/ide/editor-state. FileEditorManager.getSelectedTextEditor
// asserts the dispatch thread - a read action on a pooled thread is not enough -
// so the debounced report must run on the Swing thread and only the HTTP call may
// leave it. Shipped once the other way round (0.2.46) and threw
// "Access is allowed from event dispatch thread only" on every selection change.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditorContextSnapshotRunsOnTheDispatchThread(t *testing.T) {
	path := filepath.Join("src", "main", "kotlin", "dev", "foxxycode", "intellij", "editor",
		"FoxxyCodeEditorContextService.kt")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	src := string(data)
	if !strings.Contains(src, "Alarm(Alarm.ThreadToUse.SWING_THREAD") {
		t.Fatalf("%s: the report alarm must use Alarm.ThreadToUse.SWING_THREAD; "+
			"getSelectedTextEditor asserts the EDT", path)
	}
	if strings.Contains(src, "runReadAction") {
		t.Fatalf("%s: the snapshot must not be taken under a pooled read action; "+
			"FileEditorManager.getSelectedTextEditor asserts the dispatch thread", path)
	}
	if !strings.Contains(src, "executeOnPooledThread { post(") {
		t.Fatalf("%s: the HTTP POST must leave the EDT via executeOnPooledThread", path)
	}
}
