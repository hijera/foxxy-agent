# Upstream 1.2.36–1.2.45 port

## Scope

Port the complete net range `419c6930450a0e3107021ad7ad0b858542d0245b..bd28a329b9bbc53874556403bb2fd00f3f01c483` (77 commits), from upstream 1.2.35 through 1.2.45. Source: coddy-project/coddy-agent. Fork baseline: `11693d2039796728a609d4254b72bed40c17d491`.

- Typed JSON-frame truncation across OpenAI-compatible, Anthropic and Codex streams; incomplete calls never execute. Existing fork retry, progress and continuation policies remain.
- Shared MCP connection pool with workspace isolation, trust-gated leases and probes, eager global startup, idle release, process-tree cleanup and live reload. All surfaces retain asynchronous startup; turn execution waits for the active workspace.
- Feature worktree creation and per-call shell cwd, safe default-branch refresh, main-project workspace labels and History grouping.
- Context estimates account for tool schemas, reasoning, arguments and images and keep provider token anchors. Manual compaction and the independent `compaction.auto_enable` switch work with both canonical `coddy` and `opencode` engines; Summary remains separate.
- Remote tokens, reconnect diagnostics, environment-scoped state, relay CORS and settings, shared config backend, graph navigation and server information/version.
- Agent-initiated settings notices remain in the transcript. User commands and controls update their snapshots without a duplicate notice; old notices are filtered on reads.
- Bundled crossreview skill, reviewer definition, CLI detection and trusted `spawns` allowlists extending depth by one generation.
- Complete AppSec package: pinned Trivy and Semgrep runners, local make targets, reports and PR/main/weekly CI.

## Owner decisions

The owner approved the whole range, upstream crossreview depth exception, the complete AppSec package, configured remote tokens being returned to browsers, and restoration of the full hero footer (GitHub, API docs, server version). The remaining worktree and eager-global MCP questions were followed by “continue”; the upstream behavior was retained after stating that assumption.

## Fork adaptations

Preserve the divergence registry and its guard tests, 10 silent retries, the outer chunk-level stall guard, separate continuation budgets (including background agents), queue output-limit behavior, cross-process session refresh, stop notices, shared file decoding, export formats, SVN, localized labels, configured send shortcut, vision support and Chromium 104. The remote probe uses AbortController timers on older JCEF. Scope and trust checks are retained when a live session moves into its worktree.

Upstream tests that assume synchronous session startup now wait for MCP readiness. Tool announcements remain retractable hints, so an interrupted argument preview without text may use the fork's existing retry policy. Disabling retries alone does not disable continuation; that regression explicitly disables `llm_continue`.

## Visual evidence

Screenshots under `docs/assets/` were captured from the actual built SPA against isolated local agent and relay processes with a scripted provider. They cover the context popover, worktree History, settings notice, MCP failures, environment menu, swarm graph above a node and relay settings. Additional previous-source/current-build, narrow and light images are in the local ignored `dist/port-visual/` folder. No upstream binary screenshot replacement was copied over the fork's existing media.

## Validation

- `GOFLAGS='-count=1 -p=4 -timeout=20m' make test` passed on Windows: every shipped-tag Go package, TypeScript and 3,168 SPA tests. The UI BDD package completed in 647 seconds. Earlier concurrent runs had isolated phone-test subprocess and SVN linker failures; both passed on targeted reruns and in this final complete run.
- `make lint`, `make check-windows`, `make lint-windows`, `make test-cache`, generated documentation and schema synchronization passed. All divergence guard tests passed, including two run separately because the verification helper mapped them to the wrong packages.
- Real Chromium checks passed for layout, phone overflow and chevron alignment; folder-dialog scrolling passed in Chromium and WebKit. The rebuilt embedded assets pass the Chromium 104 compatibility check.
- The complete Docker-backed `make security` scan passed its upstream gate: Trivy reported zero critical, three high and 13 medium dependency findings; Semgrep reported zero errors and 42 warnings. Misconfiguration findings (one high, one low) remain report-only. Reports are local under `dist/security-linux/`; this port does not silently change the agreed severity gate or upgrade unrelated dependencies.
- Linux race verification used an isolated source snapshot, native Linux Node/npm and Go 1.25, matching CI. Every package except `internal/agent` and `internal/session` passed, including HTTP, LLM and real npm MCP integration. Those two packages failed filesystem I/O operations when the host C: volume filled; no data race was reported. Removing this task's generated caches recovered about 5.6 GiB, but WSL still could not start processes because of filesystem I/O errors. The complete race verdict is therefore left to PR CI; restarting a shared WSL distribution was not attempted. An earlier Go 1.27 run failed JSON diagnostic assumptions; Go 1.27 compatibility is outside this port's verified toolchain.
- The IntelliJ Windows x64 ZIP builds successfully, and all 112 compiled Kotlin unit tests pass through JUnit using a manifest classpath (Gradle worker argfiles cannot resolve this machine's Cyrillic paths). A smoke test extracts the actual bundled executable and verifies plugin/server versions, startup, the embedded SPA and crossreview catalog on a separate localhost port.

The IntelliJ test distribution and focused manual checks are described in [the test plan](upstream-1.2.45-test-plan.md). Test versions use a `0.0.0-dev-` identifier; release numbering remains the merge workflow's responsibility.
