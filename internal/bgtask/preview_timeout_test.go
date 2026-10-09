package bgtask

import "testing"

// The preview lifetime does not change the fork's advisory command estimates
// or let a command bypass its configured ceiling with NoTimeout.
func TestPreviewServerLifetimeKeepsCommandTimeoutPolicy(t *testing.T) {
	cfg := Config{DefaultTimeoutSeconds: 900, MaxTimeoutSeconds: 3600}.normalised()
	for _, tc := range []struct {
		name string
		spec Spec
		want int
	}{
		{"command estimate stays advisory", Spec{Kind: KindCommand, ExpectedSeconds: 4}, 900},
		{"explicit command timeout stays capped", Spec{Kind: KindCommand, NoTimeout: true, TimeoutSeconds: 100000}, 3600},
		{"server without a limit", Spec{Kind: KindServer, NoTimeout: true}, 0},
		{"server lifetime ignores command ceiling", Spec{Kind: KindServer, NoTimeout: true, TimeoutSeconds: 100000}, 100000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveTimeoutSeconds(tc.spec, cfg); got != tc.want {
				t.Fatalf("timeout = %d, want %d", got, tc.want)
			}
		})
	}
}
