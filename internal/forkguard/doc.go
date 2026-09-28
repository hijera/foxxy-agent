// Package forkguard holds the test that keeps the registry of upstream
// divergences honest (.claude/rules/upstream-divergences.md): the decisions
// where FoxxyCode keeps its own behaviour over upstream coddy-agent, the
// `// fork(<id>)` markers at their code sites, their guard tests, the Cursor
// mirror of the rule and the copy of the table in UPSTREAM_SYNC.md. A port that
// takes the upstream side of a marked site, or drops a guard test, fails here
// instead of shipping.
//
// The package has no code of its own; it exists for its test.
package forkguard
