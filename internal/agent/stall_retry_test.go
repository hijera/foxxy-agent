package agent

import (
	"context"
	"testing"
	"time"

	"github.com/hijera/foxxycode-agent/internal/config"
)

// TestStallRetryLadder pins the schedule the default config produces: one
// minute, then three, then five - and five for every attempt after that.
func TestStallRetryLadder(t *testing.T) {
	s := newStallRetry(&config.Agent{})
	want := []time.Duration{
		time.Minute, 3 * time.Minute, 5 * time.Minute,
		5 * time.Minute, 5 * time.Minute, 5 * time.Minute,
	}
	for i, w := range want {
		got, ok := s.next()
		if !ok {
			t.Fatalf("attempt %d refused inside the one-hour budget", i+1)
		}
		if got != w {
			t.Errorf("attempt %d delay = %v, want %v", i+1, got, w)
		}
	}
}

// TestStallRetryBudgetStops covers the default one-hour window: retries continue
// while the next pause still fits, then stop rather than overrunning it.
func TestStallRetryBudgetStops(t *testing.T) {
	s := newStallRetry(&config.Agent{})
	total := time.Duration(0)
	n := 0
	for {
		d, ok := s.next()
		if !ok {
			break
		}
		total += d
		n++
		if n > 100 {
			t.Fatal("budget never ran out; the ladder is not bounded")
		}
	}
	if total > time.Hour {
		t.Errorf("total wait %v exceeds the one-hour budget", total)
	}
	// 1 + 3 + 5 then five-minute steps: the last that fits lands on 59m.
	if n != 13 || total != 59*time.Minute {
		t.Errorf("ladder ran %d attempts totalling %v, want 13 totalling 59m", n, total)
	}
	if s.attempts() != n || s.spent() != total {
		t.Errorf("attempts/spent = %d/%v, want %d/%v", s.attempts(), s.spent(), n, total)
	}
}

// TestStallRetryUnboundedBudget checks that an explicit 0 keeps going far past
// the point the default window would have stopped.
func TestStallRetryUnboundedBudget(t *testing.T) {
	zero := 0
	s := newStallRetry(&config.Agent{LLMStallRetryMaxWaitMS: &zero})
	for i := 0; i < 500; i++ {
		if _, ok := s.next(); !ok {
			t.Fatalf("unbounded retries stopped at attempt %d", i+1)
		}
	}
	if s.spent() <= time.Hour {
		t.Errorf("500 attempts spent only %v; the ladder is not advancing", s.spent())
	}
}

// TestStallRetryDisabled is the settings switch: off means the caller never
// retries and the turn keeps today's immediate error.
func TestStallRetryDisabled(t *testing.T) {
	off := false
	s := newStallRetry(&config.Agent{LLMStallRetry: &off})
	if _, ok := s.next(); ok {
		t.Error("llm_stall_retry: false must refuse every retry")
	}
}

// TestStallRetryWaitHonoursCancel is the Stop path: a cancelled turn must not
// sit out a five-minute pause.
func TestStallRetryWaitHonoursCancel(t *testing.T) {
	s := newStallRetry(&config.Agent{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if err := s.wait(ctx, time.Hour); err == nil {
		t.Fatal("wait on a cancelled context must return an error")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("wait took %v on a cancelled context; it must return at once", elapsed)
	}
}

// TestStallRetryCustomLadder covers the configured form, which is also how the
// end-to-end tests keep their runtime in milliseconds.
func TestStallRetryCustomLadder(t *testing.T) {
	max := 1000
	s := newStallRetry(&config.Agent{
		LLMStallRetryDelaysMS:  []int{10, 20},
		LLMStallRetryMaxWaitMS: &max,
	})
	want := []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 20 * time.Millisecond}
	for i, w := range want {
		got, ok := s.next()
		if !ok || got != w {
			t.Errorf("attempt %d = %v/%v, want %v/true", i+1, got, ok, w)
		}
	}
}

// TestTrimToResumeBoundary pins the seam fix. The inputs are the shapes a real
// stall produces; the qwen3.6 case is the one measured live against
// api.neuraldeep.ru, where the model resumed by restarting the line it was cut
// in and duplicated the fragment.
func TestTrimToResumeBoundary(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "drops the unfinished list item",
			in:   "1. Use the active voice.\n2. Name the failing operation.\n3. Include the",
			want: "1. Use the active voice.\n2. Name the failing operation.\n",
		},
		{
			name: "prose falls back to the last sentence end",
			in:   "A mutex guards shared state. It also prev",
			want: "A mutex guards shared state.",
		},
		{
			name: "a single unfinished sentence is kept rather than discarded",
			in:   "The quick brown fox jum",
			want: "The quick brown fox jum",
		},
		{
			name: "text already ending on a boundary is untouched",
			in:   "1. First rule.\n2. Second rule.\n",
			want: "1. First rule.\n2. Second rule.\n",
		},
		{
			name: "empty stays empty",
			in:   "",
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := trimToResumeBoundary(tc.in); got != tc.want {
				t.Errorf("trimToResumeBoundary(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
