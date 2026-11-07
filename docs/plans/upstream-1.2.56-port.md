# Upstream 1.2.55–1.2.56 port

## Scope

Port the complete net range
`cc012f2b890a5a271b40dd15566a43b922669ce3..1e1cd320a1a0408d4a52155c8d76e31d4550e83e`
from coddy-project/coddy-agent through tag `1.2.56`, onto fork baseline
`f664fecc4`. The range contains 16 commits: 14 changes and two integration
merges without additional merge resolutions. Branding is normalized; conflicts
are reconciled against the fork divergence registry.

- Scope skill and subagent catalogs, mentions, completions, installation and
  deletion to the selected conversation's workspace, including a folder picked
  before a new conversation has its first session id. Discard stale responses
  after changing the workspace or session.
- Fit the folder browser to desktop and phone screens, inherit the application
  font, identify links, and keep the create-folder row outside its scrollport.
- Keep Tasks open when a desktop History backdrop closes; retain the stacked
  shell's full-screen closing behavior.
- Always read the four standard skill and subagent directories; configured
  directories append rather than replace them.
- Move legacy MCP servers and skill marketplaces from YAML into the global
  `mcp.json` and `marketplaces.json`, preserving existing declarations and making
  a backup before removing the legacy keys. Project declarations keep their
  trust gates. List MCP environment/header names without disclosing values.
- Add marketplace scope, declaration and digest-bound project trust controls
  to the CLI, HTTP API and Settings.
- Carry mandatory home/workspace/nested `AGENTS.md` and `DESIGN.md` layers
  independently of rules settings and prompt templates; `instructions.files`
  appends. Preserve prompt caching and provider-specific template variants.
- Use 165 ReAct steps by default, two repeated calls across responses and one
  loop warning; explicit `max_turns: 0` remains unlimited. Repeated calls within
  one response do not consume the repeat budget.
- Filter inline-suppressed Semgrep results out of SARIF.

## Owner decisions and fork adaptations

The owner approved all new defaults, automatic migration and Settings changes,
the additive standard folders, project marketplace trust, and mandatory document
layers. The owner explicitly chose upstream's subagent discovery: `.claude/agents`
is read only when listed in `subagents.dirs`.

Keep retry limit 10, useful-step accounting, continuation budgets, cycle and
quarantine policies, both compaction engines, Summary, provider templates and IDE
context. Preserve asynchronous MCP acquisition, TLS overrides, project gates,
cross-process state, shared encoding, SVN, localization, exports and Chromium 104.
UTF-16 legacy migration uses the existing decoder and backs up the original bytes;
the new JSON declaration preserves `insecureSkipVerify`.
Migration rechecks the YAML under the write lock so an intervening Settings save
is loaded rather than overwritten by the older snapshot.
Concurrent backup replacements are serialized: on Windows, a watcher reload
and a Settings save must not race to rename over `config.yaml.bak`. The parallel
backup regression and the live reload scenario each pass ten repetitions.
The retry-accounting diagnostic uses a five-second first-token allowance rather
than the fixture's 500 ms: it tests HTTP attempt counts, not deadline handling.
The silent-provider tests keep their short deadlines; the diagnostic passes
twenty repetitions under the full validation load.

History is adapted through the existing App rather than replaced. Preserve
filters and persisted status, project scope, collapsed groups, pinned rows, tags,
pagination, message counts, running/error state, IDE last-session restoration and
the fork's centered empty-chat composer. The desktop Tasks change is confined to
the backdrop behavior. No session storage format is replaced by this port.

## Validation and evidence

- Linux full shipped-tag `GOFLAGS='-count=1 -p=2 -timeout=20m' make test`:
  all Go packages and 3,314 SPA tests pass, including
  workspace races, marketplace trust, migration, mandatory documents, loop guards
  and existing History behavior.
- The subsequent MCP trust-label localization is covered by an English/Russian
  regression, UI type checking and all 3,315 SPA tests. The embedded bundle and
  actual before/after screenshots are rebuilt for that follow-up.
- `make lint`: every configured Go pass and UI type checking pass.
- The VS Code changelog's 18 tests and the IntelliJ plugin checks pass.
- `make test-cache`: mandatory documents, rule generations and provider-bound
  prompt projections pass.
- Full non-UI shipped-tag `make test-race` passes on Linux with Go 1.26.10 and
  native Node/npx, without races. The subsequent stale-migration guard also
  passes the entire configuration package with `go test -race -count=1`.
- Documentation generation/checking, published-schema synchronization and
  `internal/forkguard` pass. The wording checks introduce no prohibited Russian
  terms; existing historical records and rule quotations are retained.
- A real embedded `http,ui` build passes Chromium 104 compatibility checking.
  Real Chromium checks cover History with Tasks, Settings catalogs at 1280/390,
  Russian/English overflow and chevrons. Chromium and WebKit folder scrolling,
  short viewports and the create-folder row pass; `npm run test:layout` passes.
- The feature pages embed captures of the real running surface under
  `docs/assets/skills/`, `subagents/`, `mcp/`, `ui-folder-picker/` and
  `session-history/`. Additional local browser evidence remains under ignored
  `dist/visual-1.2.56/captures/`.

The Linux stand uses Go 1.26.10, native Node/npx and headless Chromium. Its
snapshot preserves Git executable bits and all seven asset symlinks; a plain
copy from a Windows checkout does not preserve the latter. Browser dependencies
are extracted into an isolated test directory rather than installed system-wide.

The native Windows Go UI/browser suite passes. A highly concurrent full Windows
run exceeded the 50 ms header-list budget, the 1.5 s History-list budget and
short MCP startup deadlines. Each affected test passes ten isolated repetitions
with its original limits. These timing failures remain a limitation of the
loaded Windows run; no session-list threshold or MCP deadline was relaxed.

CI's tag matrix is evaluated by the pull-request workflows.
