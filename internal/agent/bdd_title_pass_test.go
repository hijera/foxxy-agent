package agent

import "github.com/hijera/foxxycode-agent/internal/config"

// disableTitlePass turns the session-title pass off for scenarios that do not
// assert on titles.
//
// startTitleGeneration deliberately detaches that pass: it runs on its own
// goroutine with its own context so it can outlive the turn, including a turn the
// user stopped. A BDD scenario hands the agent a stub provider that records every
// request it was given, so the detached pass keeps writing to that stub after the
// step which started the turn has returned — the race detector reports it, and the
// recorded "last request" stops meaning the agent's turn.
//
// Scenarios that do exercise titles (bdd_session_title_test.go) leave it on.
func disableTitlePass(cfg *config.Config) {
	off := false
	cfg.Title.Enabled = &off
}
