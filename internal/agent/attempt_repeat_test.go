package agent

// Unit coverage for the cross-attempt repeat detector. The end-to-end behaviour
// (which nudge a restarted turn receives, and when the tools come off) lives in
// react_stall_test.go and features/llm_stall_retry.feature.

import (
	"strings"
	"testing"

	"github.com/hijera/foxxycode-agent/internal/llm"
)

func TestAttemptFingerprintIgnoresCosmeticDifferences(t *testing.T) {
	a := attemptFingerprint("Let me fix the compile errors.", "I will start with the constants.", nil)
	b := attemptFingerprint("Let   me fix the  compile errors.", "I WILL start with\tthe constants.", nil)
	if a != b {
		t.Errorf("whitespace and case changed the fingerprint:\n%q\n%q", a, b)
	}
}

func TestAttemptFingerprintSeparatesDifferentAttempts(t *testing.T) {
	a := attemptFingerprint("", "I will read the repository first.", nil)
	b := attemptFingerprint("", "I will write the patch now.", nil)
	if a == b {
		t.Error("two different openings produced the same fingerprint")
	}
}

func TestAttemptFingerprintCoversRequestedCalls(t *testing.T) {
	same := "Reading the file."
	a := attemptFingerprint("", same, []llm.ToolCall{{Name: "read", InputJSON: `{"path":"a.go"}`}})
	b := attemptFingerprint("", same, []llm.ToolCall{{Name: "read", InputJSON: `{"path":"b.go"}`}})
	if a == b {
		t.Error("the same opening with a different tool call must not look identical")
	}
	// Key order in the arguments is cosmetic, the same rule canonicalToolCallKey follows.
	c := attemptFingerprint("", same, []llm.ToolCall{{Name: "read", InputJSON: `{"path":"a.go","limit":10}`}})
	d := attemptFingerprint("", same, []llm.ToolCall{{Name: "read", InputJSON: `{"limit":10,"path":"a.go"}`}})
	if c != d {
		t.Error("argument key order changed the fingerprint")
	}
}

func TestAttemptFingerprintOfNothingIsEmpty(t *testing.T) {
	if fp := attemptFingerprint("  ", "", nil); fp != "" {
		t.Errorf("fingerprint of an attempt that produced nothing = %q, want empty", fp)
	}
}

func TestAttemptRepeatDetectorTripsOnTheSecondCopy(t *testing.T) {
	d := newAttemptRepeatDetector()
	if seen, repeated := d.Observe("one"); seen != 1 || repeated {
		t.Errorf("first attempt: seen=%d repeated=%v, want 1 false", seen, repeated)
	}
	if seen, repeated := d.Observe("two"); seen != 1 || repeated {
		t.Errorf("a different attempt: seen=%d repeated=%v, want 1 false", seen, repeated)
	}
	if seen, repeated := d.Observe("one"); seen != 2 || !repeated {
		t.Errorf("repeat: seen=%d repeated=%v, want 2 true", seen, repeated)
	}
	if seen, repeated := d.Observe("one"); seen != 3 || !repeated {
		t.Errorf("third copy: seen=%d repeated=%v, want 3 true", seen, repeated)
	}
}

func TestAttemptRepeatDetectorIgnoresEmptyAttempts(t *testing.T) {
	d := newAttemptRepeatDetector()
	_, _ = d.Observe("")
	if seen, repeated := d.Observe(""); seen != 0 || repeated {
		t.Errorf("an attempt that produced nothing must never count: seen=%d repeated=%v", seen, repeated)
	}
}

func TestAlreadyRanStepsListsExecutedCallsOfThisTurn(t *testing.T) {
	history := []llm.Message{
		{Role: llm.RoleUser, Content: "an older request"},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "old", Name: "read", InputJSON: `{"path":"old.java"}`}}},
		{Role: llm.RoleTool, ToolCallID: "old", Content: "..."},
		{Role: llm.RoleUser, Content: "fix the compile errors"},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			{ID: "c1", Name: "read", InputJSON: `{"path":"Person.java"}`},
			{ID: "c2", Name: "grep", InputJSON: `{"pattern":"PARTICIPANT_CODE"}`},
		}},
		{Role: llm.RoleTool, ToolCallID: "c1", Content: "..."},
		{Role: llm.RoleTool, ToolCallID: "c2", Content: "..."},
		// Announced but never answered: the stream was cut mid-call, so it never ran.
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "c3", Name: "edit", InputJSON: `{"path":"Person.java"}`}}},
	}

	got := alreadyRanSteps(history, 8)
	joined := strings.Join(got, ", ")
	if !strings.Contains(joined, "read(Person.java)") || !strings.Contains(joined, "grep(PARTICIPANT_CODE)") {
		t.Errorf("steps = %q, want the executed calls of this turn", joined)
	}
	if strings.Contains(joined, "old.java") {
		t.Errorf("steps = %q, must not reach past the last user message", joined)
	}
	if strings.Contains(joined, "edit(") {
		t.Errorf("steps = %q, must not list a call that never got a result", joined)
	}
}

func TestAlreadyRanStepsDedupesAndCaps(t *testing.T) {
	history := []llm.Message{{Role: llm.RoleUser, Content: "go"}}
	for i := 0; i < 6; i++ {
		history = append(history,
			llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "same", Name: "read", InputJSON: `{"path":"a.java"}`}}},
			llm.Message{Role: llm.RoleTool, ToolCallID: "same", Content: "..."},
		)
	}
	if got := alreadyRanSteps(history, 3); len(got) != 1 {
		t.Errorf("steps = %v, want the repeated call listed once", got)
	}

	history = []llm.Message{{Role: llm.RoleUser, Content: "go"}}
	for _, p := range []string{"a", "b", "c", "d", "e"} {
		history = append(history,
			llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: p, Name: "read", InputJSON: `{"path":"` + p + `.java"}`}}},
			llm.Message{Role: llm.RoleTool, ToolCallID: p, Content: "..."},
		)
	}
	got := alreadyRanSteps(history, 3)
	if len(got) != 3 {
		t.Fatalf("steps = %v, want the cap to hold at 3", got)
	}
	if !strings.Contains(got[len(got)-1], "e.java") {
		t.Errorf("steps = %v, want the newest kept", got)
	}
}

func TestRepeatedAttemptNudgeNamesTheRepeatAndTheDoneWork(t *testing.T) {
	msg := repeatedAttemptNudge([]string{"read(Person.java)", "grep(PARTICIPANT_CODE)"}, 3)
	for _, want := range []string{"read(Person.java)", "grep(PARTICIPANT_CODE)", "already"} {
		if !strings.Contains(msg, want) {
			t.Errorf("nudge = %q, want it to mention %q", msg, want)
		}
	}
	if !strings.Contains(msg, "3") {
		t.Errorf("nudge = %q, want it to name the attempt number", msg)
	}
	// With nothing executed yet the nudge must still read as a sentence.
	bare := repeatedAttemptNudge(nil, 2)
	if strings.Contains(bare, "already run in this turn:") {
		t.Errorf("nudge with no steps = %q, want the list omitted", bare)
	}
}

func TestAttemptRepeatDetectorMatchesAnAttemptCutShorter(t *testing.T) {
	// The connection gives up at a different point every time, so the second try at
	// the same answer arrives truncated.
	full := attemptFingerprint("", "I will fix the compile errors. First the constants, then the repository, then the message fields.", nil)
	cut := attemptFingerprint("", "I will fix the compile errors. First the constants, then the repo", nil)

	d := newAttemptRepeatDetector()
	if _, repeated := d.Observe(full); repeated {
		t.Fatal("the first attempt cannot be a repeat")
	}
	if seen, repeated := d.Observe(cut); seen != 2 || !repeated {
		t.Errorf("a shorter copy of the same answer: seen=%d repeated=%v, want 2 true", seen, repeated)
	}
	// And the other way round: the short one first.
	d = newAttemptRepeatDetector()
	_, _ = d.Observe(cut)
	if seen, repeated := d.Observe(full); seen != 2 || !repeated {
		t.Errorf("a longer copy of the same answer: seen=%d repeated=%v, want 2 true", seen, repeated)
	}
}

func TestAttemptRepeatDetectorIgnoresAShortSharedOpening(t *testing.T) {
	short := attemptFingerprint("", "Okay.", nil)
	long := attemptFingerprint("", "Okay. Now for something entirely different: the repository layer needs a new query.", nil)

	d := newAttemptRepeatDetector()
	_, _ = d.Observe(short)
	if _, repeated := d.Observe(long); repeated {
		t.Error("a fragment too short to identify anything must not match a different answer")
	}
}

func TestAttemptRepeatDetectorMatchesAnAttemptCutBeforeItsToolCall(t *testing.T) {
	opening := "Let me look at the entity before changing anything, because the field names must line up."
	withCall := attemptFingerprint("", opening, []llm.ToolCall{{Name: "read", InputJSON: `{"path":"Person.java"}`}})
	withoutCall := attemptFingerprint("", opening, nil)

	d := newAttemptRepeatDetector()
	_, _ = d.Observe(withCall)
	if seen, repeated := d.Observe(withoutCall); seen != 2 || !repeated {
		t.Errorf("an attempt cut before its call: seen=%d repeated=%v, want 2 true", seen, repeated)
	}
}

// The shape the reported turn actually had: the same answer begun again, but not
// reproduced word for word - the second attempt got the constant names right.
// Neither attempt contains the other, so a prefix test misses exactly the case
// this guard exists for.
func TestAttemptRepeatDetectorMatchesARewrittenOpening(t *testing.T) {
	d := newAttemptRepeatDetector()
	first := attemptFingerprint("", "Understood. I will fix the compilation errors. The main problems: "+
		"1. Wrong constant names (PARTICIPANT_CODE_FIELD, PERSON_ID_FIELD)", nil)
	second := attemptFingerprint("", "Understood. I will fix the compilation errors. The main problems: "+
		"1. Wrong constant names (ATS_PARTICIPANT_CODE_FIELD, ATS_PERSON_ID_FIELD)", nil)

	if _, repeated := d.Observe(first); repeated {
		t.Fatal("the first attempt must not read as a repeat")
	}
	seen, repeated := d.Observe(second)
	if !repeated || seen != 2 {
		t.Fatalf("seen = %d repeated = %v, want the rewritten opening recognised", seen, repeated)
	}
}

// A model that opens every step with the same stock sentence is not repeating an
// answer, and must not be told that it is.
func TestAttemptRepeatDetectorIgnoresAStockPreamble(t *testing.T) {
	preamble := "Let me look at the code to understand the structure before making any changes. "
	d := newAttemptRepeatDetector()
	d.Observe(attemptFingerprint("", preamble+"First I will fix the constant names in the participant mapper, "+
		"which is where the compiler points, and then run the build to see what is left over.", nil))
	seen, repeated := d.Observe(attemptFingerprint("", preamble+"Now the repository return type: the projection "+
		"has to become Person, and the callers of findAll need updating to match that signature.", nil))
	if repeated {
		t.Errorf("a shared preamble was taken for a repeated answer (seen = %d)", seen)
	}
}
