package session

import "testing"

// The fork's listing reads session.json alone, so a row carries no message
// count. SortMessages gets it from a reader the caller supplies, and that reader
// opens a transcript: it must run only for that column, and once per session.
func TestSortMessagesCountsOnlyWhenAskedAndOncePerSession(t *testing.T) {
	counts := map[string]int{"sess_a": 2, "sess_b": 9, "sess_c": 5}
	reads := map[string]int{}
	messagesOf := func(id string) int {
		reads[id]++
		return counts[id]
	}
	rows := func() []SessionListEntry {
		return []SessionListEntry{
			{SessionID: "sess_a", UpdatedAt: "2026-09-01T10:00:00Z"},
			{SessionID: "sess_b", UpdatedAt: "2026-09-02T10:00:00Z"},
			{SessionID: "sess_c", UpdatedAt: "2026-09-03T10:00:00Z"},
		}
	}

	byMessages := rows()
	SortSessionListWith(byMessages, SortMessages, SortDesc, SortCounters{MessagesOf: messagesOf})
	if got := []string{byMessages[0].SessionID, byMessages[1].SessionID, byMessages[2].SessionID}; got[0] != "sess_b" || got[1] != "sess_c" || got[2] != "sess_a" {
		t.Fatalf("order by messages desc = %v, want sess_b, sess_c, sess_a", got)
	}
	for id, n := range reads {
		if n != 1 {
			t.Errorf("transcript of %s was counted %d times, want once", id, n)
		}
	}

	reads = map[string]int{}
	byUpdated := rows()
	SortSessionListWith(byUpdated, SortUpdated, SortDesc, SortCounters{MessagesOf: messagesOf})
	if len(reads) != 0 {
		t.Fatalf("a listing that is not sorted by messages opened %d transcript(s)", len(reads))
	}
	if byUpdated[0].SessionID != "sess_c" {
		t.Fatalf("default order broke: %+v", byUpdated)
	}

	// No reader at all (a caller that cannot count) keeps the listing stable.
	plain := rows()
	SortSessionListWith(plain, SortMessages, SortDesc, SortCounters{})
	if plain[0].SessionID != "sess_a" {
		t.Fatalf("without a reader the rows fall back to the id tiebreak: %+v", plain)
	}
}
