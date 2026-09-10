package agent

// Cross-attempt repeat protection: what to do when a turn the agent restarted -
// because the provider dropped the stream mid-answer - comes back with the same
// answer all over again.
//
// The detectors in loopguard.go cannot see this shape. They watch one streamed
// response, or the tool calls inside one turn; here every attempt is a separate,
// perfectly well-formed response, and the repetition exists only when attempt N
// is compared with attempt N-1. A hub that abandons roughly half of its long
// generations turns that into the loop the operator actually watches: the same
// opening paragraph, every few minutes, for as long as the continuation budget
// lasts.
//
// The cure is to stop pretending the previous attempt never happened.
// streamStallNudge asks the model to carry on from a partial it often cannot even
// see; once the same opening arrives a second time, it is told plainly that it is
// repeating itself, and which steps have already run.
//
// Pure and allocation-light, like loopguard.go: the ReAct loop owns the policy.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/hijera/foxxycode-agent/internal/llm"
)

const (
	// attemptFingerprintPrefix is how much of an attempt's opening identifies it.
	// Long enough that two genuinely different plans differ inside it, short enough
	// that an attempt cut at a different point still matches the one before.
	attemptFingerprintPrefix = 400
	// attemptRepeatLimit is how many copies of the same opening it takes to call it
	// a repeat. Two: the first copy is the answer, the second is the loop.
	attemptRepeatLimit = 2
	// attemptRepeatMinHead is how much text two attempts must share before the
	// shorter one counts as the same answer restarted. A connection cut lands at a
	// different place every time, so the shorter attempt is only ever a prefix of
	// the longer one - but "Okay." is a prefix of half the answers a model could
	// write, and must not be one.
	attemptRepeatMinHead = 48
	// attemptRestartsBeforeAnswer is how many repeats may pass before the tools come
	// off for one request and the model has to answer from what it gathered. The
	// same escalation maxBlockedRoundsBeforeAnswer applies to a quarantined loop.
	attemptRestartsBeforeAnswer = 2
	// alreadyRanStepsMax bounds the "you already did this" list so a long turn does
	// not push the nudge itself out of the model's attention.
	alreadyRanStepsMax = 8
	// toolCallLabelMax bounds one entry of that list.
	toolCallLabelMax = 60
)

// attemptFingerprint identifies what one attempt at an assistant turn produced:
// the opening of its reasoning and answer text, plus the calls it requested.
//
// Normalization is shared with the stream loop guard (normalizeForRepeat), so
// cosmetic differences between two runs of the same answer - a collapsed space, a
// capital letter - do not hide the repeat. Arguments go through
// canonicalToolCallKey for the same reason.
//
// An attempt that produced nothing at all fingerprints as the empty string: that
// is the silent-provider case, which is replayed verbatim rather than nudged, and
// counting it here would blame the model for the hub's outage.
func attemptFingerprint(reasoning, content string, calls []llm.ToolCall) string {
	var raw strings.Builder
	raw.WriteString(reasoning)
	if reasoning != "" && content != "" {
		raw.WriteByte('\n')
	}
	raw.WriteString(content)

	norm, _ := normalizeForRepeat(raw.String())
	head := strings.TrimSpace(string(norm))
	if len(head) > attemptFingerprintPrefix {
		head = head[:attemptFingerprintPrefix]
	}

	keys := make([]string, 0, len(calls))
	for _, tc := range calls {
		keys = append(keys, canonicalToolCallKey(tc.Name, tc.InputJSON))
	}
	sort.Strings(keys)

	if head == "" && len(keys) == 0 {
		return ""
	}
	// Head first, calls after it: the separator is what lets attemptHead isolate the
	// text again, and it keeps a call key from ever reading as answer text.
	return head + "\x00" + strings.Join(keys, "\x00")
}

// attemptHead is the text half of a fingerprint, without the calls appended to
// it. canonicalToolCallKey embeds NUL bytes of its own, so only the first one
// separates the two halves.
func attemptHead(fingerprint string) string {
	if i := strings.IndexByte(fingerprint, 0); i >= 0 {
		return fingerprint[:i]
	}
	return fingerprint
}

// sameAttempt reports whether two fingerprints are the same answer being written
// again.
//
// Equality alone is too strict, and so is asking one attempt to be a prefix of the
// other. A model rewriting its answer does not reproduce it verbatim: measured on
// the reported turn, one attempt opened "...wrong constant names
// (PARTICIPANT_CODE_FIELD)" and the next "...wrong constant names
// (ATS_PARTICIPANT_CODE_FIELD, ATS_PERSON_ID_FIELD)". Neither contains the other,
// yet they are plainly the same answer begun twice. What they do share is a long
// common opening, so that is what is measured.
//
// Two floors keep a stock preamble from reading as a repeat. The shared opening
// must be at least attemptRepeatMinHead - a fragment too short identifies nothing -
// and it must be at least half of the shorter attempt, so "most of what it wrote
// is the same" counts while "it opened with its usual sentence" does not.
//
// The requested calls take part only through the equality above. Once two
// attempts share that much of an opening, the answer is being rewritten whether
// or not the truncated one reached the call it was heading for - and an attempt
// with no text at all has nothing but its calls to be recognised by.
func sameAttempt(a, b string) bool {
	if a == b {
		return true
	}
	short, long := attemptHead(a), attemptHead(b)
	if len(short) > len(long) {
		short, long = long, short
	}
	if len(short) < attemptRepeatMinHead {
		return false
	}
	common := commonPrefixLen(short, long)
	return common >= attemptRepeatMinHead && common*2 >= len(short)
}

// commonPrefixLen is how many leading bytes two strings share. Bytes, not runes:
// the result is only ever compared against a length, and both inputs come out of
// the same normalizer.
func commonPrefixLen(a, b string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

// attemptRepeatDetector remembers the openings one turn has already produced.
// A slice, not a map: attempts are matched by prefix (see sameAttempt), there are
// at most a handful of them in a turn, and the scan is cheaper than the map the
// exact-match version needed.
type attemptRepeatDetector struct {
	attempts []*attemptRecord
}

type attemptRecord struct {
	fingerprint string
	count       int
}

func newAttemptRepeatDetector() *attemptRepeatDetector {
	return &attemptRepeatDetector{}
}

// Observe records one attempt and reports how many times this opening has arrived
// and whether that is enough to call it a repeat.
func (d *attemptRepeatDetector) Observe(fingerprint string) (seen int, repeated bool) {
	if d == nil || fingerprint == "" {
		return 0, false
	}
	for _, rec := range d.attempts {
		if !sameAttempt(rec.fingerprint, fingerprint) {
			continue
		}
		rec.count++
		// Keep the longest copy seen: it is the fullest view of the answer the model
		// keeps trying to write, and the one a later, shorter attempt must match.
		if len(fingerprint) > len(rec.fingerprint) {
			rec.fingerprint = fingerprint
		}
		return rec.count, rec.count >= attemptRepeatLimit
	}
	d.attempts = append(d.attempts, &attemptRecord{fingerprint: fingerprint, count: 1})
	return 1, false
}

// alreadyRanSteps names the tool calls the current turn has already executed, so
// the nudge can hand the model the list instead of a vague "you have been here
// before".
//
// Scoped to the turn: everything after the last persisted user message. Nudges are
// LLM-facing only and never stored, so the last stored user message is this turn's
// prompt. Only answered calls are listed - a call whose stream was cut mid-arguments
// never ran, and telling the model it did would send it looking for a result that
// is not there.
func alreadyRanSteps(history []llm.Message, limit int) []string {
	start := 0
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == llm.RoleUser {
			start = i
			break
		}
	}
	return executedSteps(history[start:], limit)
}

// executedSteps names the calls one stretch of transcript actually ran, newest
// last. Split out of alreadyRanSteps so the resume path can pass the previous
// turn rather than the current one; the rule is the same for both.
func executedSteps(turn []llm.Message, limit int) []string {
	answered := make(map[string]struct{}, 8)
	for _, m := range turn {
		if m.Role == llm.RoleTool {
			if id := strings.TrimSpace(m.ToolCallID); id != "" {
				answered[id] = struct{}{}
			}
		}
	}

	var out []string
	seen := make(map[string]struct{}, 8)
	for _, m := range turn {
		for _, tc := range m.ToolCalls {
			if _, ok := answered[strings.TrimSpace(tc.ID)]; !ok {
				continue
			}
			label := toolCallLabel(tc)
			if label == "" {
				continue
			}
			if _, dup := seen[label]; dup {
				continue
			}
			seen[label] = struct{}{}
			out = append(out, label)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

// toolCallLabel renders one executed call as "name(main argument)". The argument
// is picked the way loopPinKey picks one: whichever of the well-known fields the
// call carries, which covers every tool a stalled turn tends to be circling.
func toolCallLabel(tc llm.ToolCall) string {
	name := strings.TrimSpace(tc.Name)
	if name == "" {
		return ""
	}
	var args struct {
		Path    string `json:"path"`
		Pattern string `json:"pattern"`
		Command string `json:"command"`
		Query   string `json:"query"`
		URL     string `json:"url"`
	}
	_ = json.Unmarshal([]byte(tc.InputJSON), &args)
	for _, v := range []string{args.Path, args.Pattern, args.Command, args.Query, args.URL} {
		if s := strings.TrimSpace(v); s != "" {
			if len(s) > toolCallLabelMax {
				s = s[:toolCallLabelMax] + "..."
			}
			return name + "(" + s + ")"
		}
	}
	return name
}

// repeatedAttemptNudge tells the model that the answer the connection just ate is
// the one it already wrote, and what it can stop redoing. LLM-facing only, never
// persisted - the same contract as streamStallNudge and the loop-guard nudges.
func repeatedAttemptNudge(done []string, attempt int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "The connection dropped your answer again, and this is attempt %d that begins the same way: you are starting the whole response over instead of moving forward. ", attempt)
	if len(done) > 0 {
		b.WriteString("These steps have already run in this turn and their results are above - do not run them again: ")
		b.WriteString(strings.Join(done, ", "))
		b.WriteString(". ")
	}
	b.WriteString("Do not write that opening a third time and do not re-plan what you have already planned. Take the single next concrete step instead - one different tool call, or the reply itself - and keep it short enough to survive the connection.")
	return b.String()
}
