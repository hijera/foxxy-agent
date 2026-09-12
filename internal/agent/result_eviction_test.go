package agent

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/llm"
)

// testCWD is a real absolute directory so read/grep/write paths resolve through
// the same absPath logic as production (filepath.Abs adds the OS drive/root).
var testCWD = func() string { d, _ := filepath.Abs("testproj"); return d }()

func bigBody(marker string) string {
	return marker + "\n" + strings.Repeat("padding line\n", 100)
}

func asstRead(id, path string, offset, limit int, keep bool) llm.Message {
	args := map[string]interface{}{"path": path}
	if offset > 0 {
		args["offset"] = offset
	}
	if limit > 0 {
		args["limit"] = limit
	}
	if keep {
		args["keep"] = true
	}
	b, _ := json.Marshal(args)
	return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: id, Name: "read", InputJSON: string(b)}}}
}

func asstGrep(id, pattern, path string, keep bool) llm.Message {
	args := map[string]interface{}{"pattern": pattern}
	if path != "" {
		args["path"] = path
	}
	if keep {
		args["keep"] = true
	}
	b, _ := json.Marshal(args)
	return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: id, Name: "grep", InputJSON: string(b)}}}
}

func asstWrite(id, tool, path string) llm.Message {
	b, _ := json.Marshal(map[string]interface{}{"path": path})
	return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: id, Name: tool, InputJSON: string(b)}}}
}

func asstMove(id, src, dst string) llm.Message {
	b, _ := json.Marshal(map[string]interface{}{"src": src, "dst": dst})
	return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: id, Name: "mv", InputJSON: string(b)}}}
}

func asstKeepResult(id string, args map[string]interface{}) llm.Message {
	b, _ := json.Marshal(args)
	return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: id, Name: "keep_result", InputJSON: string(b)}}}
}

func toolResult(id, content string) llm.Message {
	return llm.Message{Role: llm.RoleTool, ToolCallID: id, Content: content}
}

func grepBody(pattern string, files ...string) string {
	var b strings.Builder
	b.WriteString("results for " + pattern + "\n")
	for i, f := range files {
		// Emit the same absolute path the search would have produced.
		fmt.Fprintf(&b, "%s:%d:match %d\n", absPath(f, testCWD), i+10, i)
	}
	// Pad so it clears MinResultBytes.
	b.WriteString(strings.Repeat("x", 200))
	return b.String()
}

func defaultOpts() resultEvictionOptions {
	return resultEvictionOptions{Enabled: true, KeepRecent: 1, MinResultBytes: 10, CWD: testCWD}
}

func evicted(content string) bool { return strings.HasPrefix(content, "[evicted:") }

// contentByID returns the tool-result content for a tool_call_id.
func contentByID(msgs []llm.Message, id string) string {
	for _, m := range msgs {
		if m.Role == llm.RoleTool && m.ToolCallID == id {
			return m.Content
		}
	}
	return ""
}

func TestPruneUnmarkedKeepsOnlyWorkingWindow(t *testing.T) {
	in := []llm.Message{
		asstRead("a", "big.go", 1, 500, false),
		toolResult("a", bigBody("PAGE-A")),
		asstRead("b", "big.go", 501, 500, false),
		toolResult("b", bigBody("PAGE-B")),
	}
	out := pruneToolResults(in, defaultOpts())
	if !evicted(contentByID(out, "a")) {
		t.Fatalf("page A should be evicted: %q", contentByID(out, "a"))
	}
	if !strings.Contains(contentByID(out, "a"), "big.go") || !strings.Contains(contentByID(out, "a"), "lines 1-500") {
		t.Fatalf("page A placeholder missing range: %q", contentByID(out, "a"))
	}
	if !strings.Contains(contentByID(out, "b"), "PAGE-B") {
		t.Fatalf("page B (working window) must survive: %q", contentByID(out, "b"))
	}
}

func TestPruneKeepTrueSurvives(t *testing.T) {
	in := []llm.Message{
		asstRead("a", "big.go", 1, 500, true), // marked
		toolResult("a", bigBody("PAGE-A")),
		asstRead("b", "other.go", 1, 500, false),
		toolResult("b", bigBody("PAGE-B")),
	}
	out := pruneToolResults(in, defaultOpts())
	if !strings.Contains(contentByID(out, "a"), "PAGE-A") {
		t.Fatalf("keep:true page must survive: %q", contentByID(out, "a"))
	}
}

func TestPruneKeepResultPinOverlappingRange(t *testing.T) {
	in := []llm.Message{
		asstRead("a", "big.go", 1, 500, false),
		toolResult("a", bigBody("PAGE-A")),
		asstKeepResult("k", map[string]interface{}{"path": "big.go", "offset": 100, "limit": 50}),
		toolResult("k", "marked as useful"),
		asstRead("b", "other.go", 1, 500, false),
		toolResult("b", bigBody("PAGE-B")),
	}
	out := pruneToolResults(in, defaultOpts())
	if !strings.Contains(contentByID(out, "a"), "PAGE-A") {
		t.Fatalf("pinned overlapping page must survive: %q", contentByID(out, "a"))
	}
}

func TestPruneKeepResultDoesNotPinFutureReads(t *testing.T) {
	in := []llm.Message{
		asstRead("a", "big.go", 1, 500, false),
		toolResult("a", bigBody("PAGE-A")),
		asstKeepResult("k", map[string]interface{}{"path": "big.go", "offset": 1, "limit": 500}),
		toolResult("k", "marked as useful"),
		asstRead("b", "big.go", 1, 500, false),
		toolResult("b", bigBody("PAGE-B")),
		asstRead("c", "other.go", 1, 500, false),
		toolResult("c", bigBody("PAGE-C")),
	}
	out := pruneToolResults(in, defaultOpts())
	if !strings.Contains(contentByID(out, "a"), "PAGE-A") {
		t.Fatalf("the earlier explicitly pinned page must survive: %q", contentByID(out, "a"))
	}
	if !evicted(contentByID(out, "b")) {
		t.Fatalf("a pin must not retain a later matching read: %q", contentByID(out, "b"))
	}
}

func TestPruneTwoPinsOneFile(t *testing.T) {
	in := []llm.Message{
		asstRead("a", "big.go", 1, 100, false),
		toolResult("a", bigBody("PAGE-A")),
		asstRead("b", "big.go", 200, 100, false),
		toolResult("b", bigBody("PAGE-B")),
		asstKeepResult("k1", map[string]interface{}{"path": "big.go", "offset": 1, "limit": 100}),
		toolResult("k1", "ok"),
		asstKeepResult("k2", map[string]interface{}{"path": "big.go", "offset": 200, "limit": 100}),
		toolResult("k2", "ok"),
		asstRead("c", "other.go", 1, 10, false),
		toolResult("c", bigBody("PAGE-C")),
	}
	out := pruneToolResults(in, defaultOpts())
	if !strings.Contains(contentByID(out, "a"), "PAGE-A") || !strings.Contains(contentByID(out, "b"), "PAGE-B") {
		t.Fatalf("both pinned pages must survive: a=%q b=%q", contentByID(out, "a"), contentByID(out, "b"))
	}
}

func TestPruneWriteInvalidatesPinnedRead(t *testing.T) {
	for _, tool := range []string{"edit", "write", "apply_patch"} {
		in := []llm.Message{
			asstRead("a", "big.go", 1, 500, true), // pinned via keep:true
			toolResult("a", bigBody("PAGE-A")),
			asstWrite("w", tool, "big.go"),
			toolResult("w", "written"),
		}
		out := pruneToolResults(in, defaultOpts())
		got := contentByID(out, "a")
		if !evicted(got) || !strings.Contains(got, "modified after this read") {
			t.Fatalf("%s: stale read must be evicted even when pinned: %q", tool, got)
		}
	}
}

func TestPruneMoveInvalidatesRead(t *testing.T) {
	in := []llm.Message{
		asstRead("a", "big.go", 1, 500, true),
		toolResult("a", bigBody("PAGE-A")),
		asstMove("m", "big.go", "renamed.go"),
		toolResult("m", "moved"),
	}
	out := pruneToolResults(in, defaultOpts())
	if !evicted(contentByID(out, "a")) {
		t.Fatalf("read of moved file must be evicted: %q", contentByID(out, "a"))
	}
}

func TestPruneSVNMutationInvalidatesPinnedRead(t *testing.T) {
	in := []llm.Message{
		asstRead("r", "src/main.go", 1, 500, true),
		toolResult("r", bigBody("PINNED BEFORE SVN UPDATE")),
		{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{
				ID: "svn", Name: "svn_update", InputJSON: `{}`,
			}},
		},
		toolResult("svn", "Updated to revision 42."),
	}
	out := pruneToolResults(in, resultEvictionOptions{
		Enabled: true, KeepRecent: 10, MinResultBytes: 10, CWD: testCWD,
	})
	got := contentByID(out, "r")
	if !evicted(got) || !strings.Contains(got, "modified after this read") {
		t.Fatalf("successful svn mutation must invalidate a pinned read: %q", got)
	}
}

func TestPruneSVNMutationScope(t *testing.T) {
	// Only Subversion tools that rewrite working-copy content may invalidate an
	// earlier read, and a call that names paths must not reach beyond them.
	svnCall := func(name, argsJSON string) []llm.Message {
		return []llm.Message{
			{
				Role: llm.RoleAssistant,
				ToolCalls: []llm.ToolCall{{
					ID: "svn", Name: name, InputJSON: argsJSON,
				}},
			},
			toolResult("svn", "done, revision 42."),
		}
	}

	tests := []struct {
		name        string
		tool        string
		argsJSON    string
		wantEvicted bool
	}{
		{"add does not rewrite content", "svn_add", `{"paths":["src/main.go"]}`, false},
		{"commit only ships what is on disk", "svn_commit", `{"paths":["src/main.go"],"message":"m"}`, false},
		{"update of another path leaves the read alone", "svn_update", `{"paths":["docs"]}`, false},
		{"update of the read path invalidates it", "svn_update", `{"paths":["src/main.go"]}`, true},
		{"update of a parent invalidates the read", "svn_update", `{"paths":["src"]}`, true},
		{"revert without paths means the whole tree", "svn_revert", `{}`, true},
		{"switch can replace any subtree", "svn_switch", `{"branch":"branches/x"}`, true},
		{"merge can replace any subtree", "svn_merge", `{"source":"branches/x"}`, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := append([]llm.Message{
				asstRead("r", "src/main.go", 1, 500, true),
				toolResult("r", bigBody("PINNED BEFORE SVN")),
			}, svnCall(tc.tool, tc.argsJSON)...)
			out := pruneToolResults(in, resultEvictionOptions{
				Enabled: true, KeepRecent: 10, MinResultBytes: 10, CWD: testCWD,
			})
			got := contentByID(out, "r")
			if evicted(got) != tc.wantEvicted {
				t.Fatalf("%s %s: evicted = %v, want %v (%q)",
					tc.tool, tc.argsJSON, evicted(got), tc.wantEvicted, got)
			}
		})
	}
}

func TestPruneDirectoryMutationInvalidatesRelatedReads(t *testing.T) {
	tests := []struct {
		name  string
		read  llm.Message
		write llm.Message
	}{
		{
			name:  "child write invalidates directory listing",
			read:  asstRead("a", "pkg", 0, 0, true),
			write: asstWrite("w", "write", filepath.Join("pkg", "new.go")),
		},
		{
			name:  "directory move invalidates child read",
			read:  asstRead("a", filepath.Join("pkg", "file.go"), 1, 500, true),
			write: asstMove("w", "pkg", "renamed"),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opts := defaultOpts()
			opts.KeepRecent = 0
			in := []llm.Message{
				tc.read,
				toolResult("a", bigBody("PAGE-A")),
				tc.write,
				toolResult("w", "written"),
			}
			out := pruneToolResults(in, opts)
			if !evicted(contentByID(out, "a")) {
				t.Fatalf("related filesystem mutation must invalidate the read: %q", contentByID(out, "a"))
			}
		})
	}
}

func TestPruneWindowsPathCaseDoesNotHideWrite(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows path comparison")
	}
	opts := defaultOpts()
	opts.KeepRecent = 0
	in := []llm.Message{
		asstRead("a", filepath.Join("PKG", "File.go"), 1, 500, true),
		toolResult("a", bigBody("PAGE-A")),
		asstWrite("w", "write", filepath.Join("pkg", "file.go")),
		toolResult("w", "written"),
	}
	out := pruneToolResults(in, opts)
	if !evicted(contentByID(out, "a")) {
		t.Fatalf("case-only path differences must not hide a write: %q", contentByID(out, "a"))
	}
}

func TestPruneFailedWriteDoesNotInvalidateRead(t *testing.T) {
	opts := defaultOpts()
	opts.KeepRecent = 0
	for _, result := range []string{
		"error: edit: old_string not found",
		"permission denied by user",
		toolLoopNudge,
		toolLoopSkippedResult,
	} {
		in := []llm.Message{
			asstRead("a", "big.go", 1, 500, true),
			toolResult("a", bigBody("PAGE-A")),
			asstWrite("w", "edit", "big.go"),
			toolResult("w", result),
		}
		out := pruneToolResults(in, opts)
		if evicted(contentByID(out, "a")) {
			t.Fatalf("unsuccessful write result %q invalidated the read", result)
		}
	}
}

func TestPruneGrepUnmarkedEvictedKeepSurvives(t *testing.T) {
	in := []llm.Message{
		asstGrep("g1", "handleFoo", "", false),
		toolResult("g1", grepBody("handleFoo", "a.go", "b.go")),
		asstGrep("g2", "handleBar", "", true), // marked
		toolResult("g2", grepBody("handleBar", "c.go")),
		asstRead("r", "x.go", 1, 10, false),
		toolResult("r", bigBody("PAGE")),
	}
	out := pruneToolResults(in, defaultOpts())
	if !evicted(contentByID(out, "g1")) {
		t.Fatalf("unmarked grep must be evicted: %q", contentByID(out, "g1"))
	}
	if !strings.Contains(contentByID(out, "g1"), `grep "handleFoo"`) {
		t.Fatalf("grep placeholder missing pattern: %q", contentByID(out, "g1"))
	}
	if !strings.Contains(contentByID(out, "g2"), "handleBar") {
		t.Fatalf("marked grep must survive: %q", contentByID(out, "g2"))
	}
}

func TestPruneGrepStaleAfterWriteToMatchedFile(t *testing.T) {
	in := []llm.Message{
		asstGrep("g", "handleFoo", "", true), // even pinned
		toolResult("g", grepBody("handleFoo", "a.go", "b.go")),
		asstWrite("w", "edit", "b.go"),
		toolResult("w", "written"),
	}
	out := pruneToolResults(in, defaultOpts())
	got := contentByID(out, "g")
	if !evicted(got) || !strings.Contains(got, "stale after") {
		t.Fatalf("grep must be evicted stale after writing a matched file: %q", got)
	}
}

func TestPruneGrepStaleAfterWriteBelowSearchRoot(t *testing.T) {
	opts := defaultOpts()
	opts.KeepRecent = 0
	in := []llm.Message{
		asstGrep("g", "handleFoo", "pkg", true),
		toolResult("g", grepBody("handleFoo", filepath.Join("pkg", "existing.go"))),
		asstWrite("w", "write", filepath.Join("pkg", "new.go")),
		toolResult("w", "written"),
	}
	out := pruneToolResults(in, opts)
	got := contentByID(out, "g")
	if !evicted(got) || !strings.Contains(got, "stale after") {
		t.Fatalf("a write below the grep root can change its matches: %q", got)
	}
}

func TestPruneGrepNotStaleAfterUnrelatedWrite(t *testing.T) {
	// keep_recent 0 so the grep is not shielded by the working window; only
	// staleness or a mark could evict it, and neither applies here (unrelated
	// write + keep:true).
	opts := defaultOpts()
	opts.KeepRecent = 0
	in := []llm.Message{
		asstGrep("g", "handleFoo", "pkg", true),
		toolResult("g", grepBody("handleFoo", filepath.Join("pkg", "a.go"))),
		asstWrite("w", "edit", filepath.Join("other", "unrelated.go")),
		toolResult("w", "written"),
	}
	out := pruneToolResults(in, opts)
	if evicted(contentByID(out, "g")) {
		t.Fatalf("grep must not be invalidated by an unrelated write: %q", contentByID(out, "g"))
	}
}

func TestPruneSmallResultNeverEvictedAndNoBudget(t *testing.T) {
	opts := defaultOpts()
	opts.MinResultBytes = 1000 // the small read below is under this
	in := []llm.Message{
		asstRead("a", "big.go", 1, 500, false),
		toolResult("a", bigBody("PAGE-A")), // large candidate (old)
		asstRead("s", "tiny.go", 1, 5, false),
		toolResult("s", "small"), // under MinResultBytes -> not a candidate
		asstRead("b", "big2.go", 1, 500, false),
		toolResult("b", bigBody("PAGE-B")), // large candidate (new)
	}
	out := pruneToolResults(in, opts)
	if !evicted(contentByID(out, "a")) {
		t.Fatalf("old large read should be evicted with keep_recent=1: %q", contentByID(out, "a"))
	}
	if contentByID(out, "s") != "small" {
		t.Fatalf("small result must be untouched: %q", contentByID(out, "s"))
	}
	if !strings.Contains(contentByID(out, "b"), "PAGE-B") {
		t.Fatalf("newest large read must survive the working window: %q", contentByID(out, "b"))
	}
}

func TestPruneDisabledReturnsInputUnchanged(t *testing.T) {
	in := []llm.Message{
		asstRead("a", "big.go", 1, 500, false),
		toolResult("a", bigBody("PAGE-A")),
		asstRead("b", "big.go", 501, 500, false),
		toolResult("b", bigBody("PAGE-B")),
	}
	opts := defaultOpts()
	opts.Enabled = false
	out := pruneToolResults(in, opts)
	if evicted(contentByID(out, "a")) {
		t.Fatalf("disabled eviction must not modify content")
	}
}

func TestPruneSharedWindowAcrossReadAndGrep(t *testing.T) {
	in := []llm.Message{
		asstRead("a", "big.go", 1, 500, false),
		toolResult("a", bigBody("PAGE-A")),
		asstGrep("g", "handleFoo", "", false),
		toolResult("g", grepBody("handleFoo", "a.go")),
	}
	out := pruneToolResults(in, defaultOpts())
	if !evicted(contentByID(out, "a")) {
		t.Fatalf("older read must be evicted (grep took the single window slot): %q", contentByID(out, "a"))
	}
	if evicted(contentByID(out, "g")) {
		t.Fatalf("newest candidate (grep) must survive: %q", contentByID(out, "g"))
	}
}

func TestPrunePreservesRoleAndToolCallIDAndDoesNotMutateInput(t *testing.T) {
	in := []llm.Message{
		asstRead("a", "big.go", 1, 500, false),
		toolResult("a", bigBody("PAGE-A")),
		asstRead("b", "big.go", 501, 500, false),
		toolResult("b", bigBody("PAGE-B")),
	}
	origA := in[1].Content
	out := pruneToolResults(in, defaultOpts())
	// Evicted message keeps its structure.
	var evictedMsg *llm.Message
	for i := range out {
		if out[i].Role == llm.RoleTool && out[i].ToolCallID == "a" {
			evictedMsg = &out[i]
		}
	}
	if evictedMsg == nil || evictedMsg.Role != llm.RoleTool || evictedMsg.ToolCallID != "a" {
		t.Fatal("evicted message lost its role or tool_call_id")
	}
	// Input slice untouched (copy-on-write).
	if in[1].Content != origA {
		t.Fatalf("input slice was mutated: %q", in[1].Content)
	}
}

// TestPruneAlternatingReadGrepWindowSize documents the working-window trade-off
// on a workload that alternates between two large results. With keep_recent=1 the
// newest result evicts the other one, so a model that needs both is invited (by
// the placeholder) to re-fetch it; keep_recent=2 keeps the pair live.
func TestPruneAlternatingReadGrepWindowSize(t *testing.T) {
	in := []llm.Message{
		asstRead("r1", "big.go", 1, 400, false),
		toolResult("r1", bigBody("READ-PAGE")),
		asstGrep("g1", "filler", "", false),
		toolResult("g1", grepBody("filler", "big.go")),
	}

	t.Run("keep_recent=1 evicts the older half of the pair", func(t *testing.T) {
		opts := defaultOpts()
		opts.KeepRecent = 1
		out := pruneToolResults(in, opts)
		if !evicted(contentByID(out, "r1")) {
			t.Fatalf("read should be evicted when the grep takes the only window slot: %q", contentByID(out, "r1"))
		}
		if evicted(contentByID(out, "g1")) {
			t.Fatalf("newest result must stay live: %q", contentByID(out, "g1"))
		}
	})

	t.Run("keep_recent=2 keeps both live", func(t *testing.T) {
		opts := defaultOpts()
		opts.KeepRecent = 2
		out := pruneToolResults(in, opts)
		if evicted(contentByID(out, "r1")) || evicted(contentByID(out, "g1")) {
			t.Fatalf("a window of 2 must keep the read+grep pair live: read=%q grep=%q",
				contentByID(out, "r1"), contentByID(out, "g1"))
		}
	})
}

// asstReadsRound builds one assistant message that requests several reads at
// once, which is how a model gathers context before reasoning about it.
func asstReadsRound(paths ...string) llm.Message {
	m := llm.Message{Role: llm.RoleAssistant}
	for i, p := range paths {
		b, _ := json.Marshal(map[string]interface{}{"path": p})
		m.ToolCalls = append(m.ToolCalls, llm.ToolCall{ID: fmt.Sprintf("r%d", i), Name: "read", InputJSON: string(b)})
	}
	return m
}

func TestPruneKeepsUnseenRoundReads(t *testing.T) {
	in := []llm.Message{
		asstReadsRound("one.go", "two.go", "three.go"),
		toolResult("r0", bigBody("PAGE-ONE")),
		toolResult("r1", bigBody("PAGE-TWO")),
		toolResult("r2", bigBody("PAGE-THREE")),
	}
	out := pruneToolResults(in, defaultOpts())
	for _, id := range []string{"r0", "r1", "r2"} {
		if evicted(contentByID(out, id)) {
			t.Fatalf("%s belongs to the round the model has not read yet: %q", id, contentByID(out, id))
		}
	}
}

func TestPruneEvictsUnseenReadMadeStaleByWrite(t *testing.T) {
	readArgs, _ := json.Marshal(map[string]interface{}{"path": "big.go"})
	writeArgs, _ := json.Marshal(map[string]interface{}{"path": "big.go"})
	in := []llm.Message{
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			{ID: "a", Name: "read", InputJSON: string(readArgs)},
			{ID: "w", Name: "edit", InputJSON: string(writeArgs)},
		}},
		toolResult("a", bigBody("PAGE-A")),
		toolResult("w", "written"),
	}
	out := pruneToolResults(in, defaultOpts())
	if !evicted(contentByID(out, "a")) {
		t.Fatalf("a write in the same round makes the read stale, unseen or not: %q", contentByID(out, "a"))
	}
}

func TestPruneStillTruncatesEarlierRounds(t *testing.T) {
	in := []llm.Message{
		asstReadsRound("one.go", "two.go"),
		toolResult("r0", bigBody("PAGE-ONE")),
		toolResult("r1", bigBody("PAGE-TWO")),
		asstRead("b", "later.go", 0, 0, false),
		toolResult("b", bigBody("PAGE-LATER")),
	}
	out := pruneToolResults(in, defaultOpts())
	if !evicted(contentByID(out, "r0")) || !evicted(contentByID(out, "r1")) {
		t.Fatalf("a round the model has already reasoned over is still subject to KeepRecent: r0=%q r1=%q",
			contentByID(out, "r0"), contentByID(out, "r1"))
	}
	if evicted(contentByID(out, "b")) {
		t.Fatalf("the newest round must stay live: %q", contentByID(out, "b"))
	}
}

func asstCall(id, name, argsJSON string) llm.Message {
	return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: id, Name: name, InputJSON: argsJSON}}}
}

func TestPruneKeepsLoopPinnedRead(t *testing.T) {
	opts := defaultOpts()
	opts.KeepRecent = 0
	opts.Pinned = map[string]struct{}{loopPinReadPrefix + absPath("big.go", testCWD): {}}
	in := []llm.Message{
		asstRead("a", "big.go", 0, 0, false),
		toolResult("a", bigBody("PAGE-A")),
		asstRead("b", "other.go", 0, 0, false),
		toolResult("b", bigBody("PAGE-B")),
		{Role: llm.RoleAssistant, Content: "done"},
	}
	out := pruneToolResults(in, opts)
	if evicted(contentByID(out, "a")) {
		t.Fatalf("a read the loop guard rescued must never be collapsed again: %q", contentByID(out, "a"))
	}
	if !evicted(contentByID(out, "b")) {
		t.Fatalf("an unpinned read is still subject to the window: %q", contentByID(out, "b"))
	}
}

func TestPruneKeepsLoopPinnedGrep(t *testing.T) {
	opts := defaultOpts()
	opts.KeepRecent = 0
	opts.Pinned = map[string]struct{}{loopPinGrepPrefix + "handler": {}}
	in := []llm.Message{
		asstGrep("g", "handler", "", false),
		toolResult("g", grepBody("handler", "a.go")),
		{Role: llm.RoleAssistant, Content: "done"},
	}
	out := pruneToolResults(in, opts)
	if evicted(contentByID(out, "g")) {
		t.Fatalf("a rescued search must survive: %q", contentByID(out, "g"))
	}
}

func TestCollapseLoopDuplicatesKeepsTheNewestCopy(t *testing.T) {
	args := `{"pattern":"**/*.go"}`
	key := canonicalToolCallKey("glob", args)
	in := []llm.Message{
		asstCall("g1", "glob", args),
		toolResult("g1", bigBody("RUN-1")),
		asstCall("g2", "glob", args),
		toolResult("g2", bigBody("RUN-2")),
		asstCall("g3", "glob", args),
		toolResult("g3", bigBody("RUN-3")),
	}
	out := collapseLoopDuplicates(in, map[string]struct{}{key: {}}, 10)
	if contentByID(out, "g1") != loopDuplicatePlaceholder || contentByID(out, "g2") != loopDuplicatePlaceholder {
		t.Fatalf("earlier repeats must collapse: g1=%q g2=%q", contentByID(out, "g1"), contentByID(out, "g2"))
	}
	if !strings.Contains(contentByID(out, "g3"), "RUN-3") {
		t.Fatalf("the newest copy is the one the model works from: %q", contentByID(out, "g3"))
	}
	// The input must not be touched, and every call still needs its result.
	if !strings.Contains(in[1].Content, "RUN-1") {
		t.Fatal("collapseLoopDuplicates mutated its input")
	}
	for _, id := range []string{"g1", "g2", "g3"} {
		if contentByID(out, id) == "" {
			t.Fatalf("tool call %s lost its result", id)
		}
	}
}

func TestCollapseLoopDuplicatesLeavesUnquarantinedCallsAlone(t *testing.T) {
	// The same command before and after an edit returns different output, so the
	// second result is not a duplicate of the first.
	args := `{"command":"go test ./..."}`
	in := []llm.Message{
		asstCall("t1", "run_command", args),
		toolResult("t1", bigBody("FAIL")),
		asstWrite("w", "edit", "big.go"),
		toolResult("w", "written"),
		asstCall("t2", "run_command", args),
		toolResult("t2", bigBody("PASS")),
	}
	out := collapseLoopDuplicates(in, map[string]struct{}{canonicalToolCallKey("glob", `{}`): {}}, 10)
	if !strings.Contains(contentByID(out, "t1"), "FAIL") {
		t.Fatalf("a call outside the quarantine must be untouched: %q", contentByID(out, "t1"))
	}
}

func TestCollapseLoopDuplicatesSkipsShortSyntheticResults(t *testing.T) {
	args := `{"pattern":"**/*.go"}`
	key := canonicalToolCallKey("glob", args)
	in := []llm.Message{
		asstCall("g1", "glob", args),
		toolResult("g1", bigBody("RUN-1")),
		asstCall("g2", "glob", args),
		toolResult("g2", toolQuarantinedResult),
	}
	out := collapseLoopDuplicates(in, map[string]struct{}{key: {}}, 10)
	if !strings.Contains(contentByID(out, "g1"), "RUN-1") {
		t.Fatalf("the only substantial copy must survive: %q", contentByID(out, "g1"))
	}
	if contentByID(out, "g2") != toolQuarantinedResult {
		t.Fatalf("the guard's own short answer must be left alone: %q", contentByID(out, "g2"))
	}
}
