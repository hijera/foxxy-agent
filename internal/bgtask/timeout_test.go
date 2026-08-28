package bgtask

import "testing"

// TestEstimateNeverShortensTheHardTimeout pins the contract both the Spec doc
// comment and the run_command schema state: expected_seconds is advisory, it
// drives the overdue ticker and nothing else.
//
// It used to buy the hard timeout instead (estimate x3), which killed exactly the
// work an honest estimate cannot describe: `yarn serve` estimated at 30s got a
// 90-second execution and was terminated mid-session, leaving the agent to
// navigate to a dead port. A low estimate must never be more dangerous than no
// estimate at all.
func TestEstimateNeverShortensTheHardTimeout(t *testing.T) {
	cfg := Config{DefaultTimeoutSeconds: 900, MaxTimeoutSeconds: 3600}.normalised()

	cases := []struct {
		name string
		spec Spec
		want int
	}{
		{"no estimate falls back to the default", Spec{}, 900},
		{"a tiny estimate still gets the full default", Spec{ExpectedSeconds: 30}, 900},
		{"a large estimate does not extend it either", Spec{ExpectedSeconds: 5000}, 900},
		{"an explicit timeout is what actually bounds the work", Spec{TimeoutSeconds: 5, ExpectedSeconds: 600}, 5},
		{"an explicit timeout is capped by the ceiling", Spec{TimeoutSeconds: 100000}, 3600},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveTimeoutSeconds(tc.spec, cfg); got != tc.want {
				t.Fatalf("resolveTimeoutSeconds() = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestNoTimeoutTasksAreNeverTerminatedByTheClock covers work that has no natural
// end — a dev server, a file watcher, a daemon. Any finite limit is wrong for it:
// the process is supposed to outlive the turn and be ended by background_stop.
func TestNoTimeoutTasksAreNeverTerminatedByTheClock(t *testing.T) {
	cfg := Config{DefaultTimeoutSeconds: 900, MaxTimeoutSeconds: 3600}.normalised()

	if got := resolveTimeoutSeconds(Spec{NoTimeout: true}, cfg); got > 0 {
		t.Errorf("NoTimeout spec resolved to %d, want a non-positive value meaning no limit", got)
	}
	// An explicit timeout is a deliberate instruction and still wins: asking for
	// no timeout is a default, not an override of what the caller spelled out.
	if got := resolveTimeoutSeconds(Spec{NoTimeout: true, TimeoutSeconds: 30}, cfg); got != 30 {
		t.Errorf("explicit timeout with NoTimeout = %d, want 30", got)
	}
}
