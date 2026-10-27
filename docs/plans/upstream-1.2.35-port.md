# Port through coddy-agent 1.2.35

The consolidated range is `24bf9b367767bb0a76193c4dac57ab03b0203365`
(1.2.27, exclusive) through `419c6930450a0e3107021ad7ad0b858542d0245b`
(1.2.35, inclusive): 66 commits, including six overlapping integration merges.

## Scope and owner decisions

- Keep asynchronous configured MCP startup on every surface. Port concurrent
  dialing, console progress, the per-server timeout and one retry of an
  initialization timeout. Console session restore starts eagerly; other
  read-only session loads defer startup until the first turn.
- Keep FoxxyCode's continuation accounting outside `max_turns`. Add
  `agent.llm_continue_background_max` (5) and
  `agent.llm_continue_background_delays_ms`
  (`[5000, 20000, 80000, 120000, 120000]`) for delegated and scheduled runs.
  Memory keeps the ordinary budget; `llm_continue` disables both policies.
- Port finished-child resume, HTTP default headers, Android/Termux runtime and
  release packaging, optional tmux recommendations, plugin archive sources and
  named marketplaces, multimodal file reads and tool pictures, and the Codex
  catalog/version/context-window changes.
- Preserve the fork's trust gates, shared workspace text decoder, permission
  refusal UI, transient browser screenshots, localization, paginated command
  catalog, both compaction implementations and session-export renderers.
- Recapture only changed surfaces. Keep the existing documentation media;
  omit the upstream bulk screenshot/video replacement. The Android tutorial
  uses instructions without upstream device recordings. Installation points to
  FoxxyCode release archives, because the upstream installer installs coddy-agent.

## Integration fixes

MCP waits follow a replacement connection generation after a workspace switch.
Old configured clients are replaced, and a late result is rejected when its
declaration changed, even if its name stayed the same. Project trust remains
bound to the current declaration.

Tool pictures follow their saved copies when a session bundle moves. The
readable export projection retains structured attachments, so HTML, PDF and
DOCX receive the pictures too. Windows can replace an existing read-only
thumbnail while keeping the committed asset read-only.

## Validation

- The full Go suite passed with
  `http,ui,scheduler,memory,cli,browser,gateway,swarm` on Windows.
- All 2,976 SPA unit tests passed; TypeScript and Chromium 104 compatibility
  passed. Embedded SPA assets were rebuilt.
- `make lint`, `make lint-windows`, `make check-windows`, `make test-cache`,
  documentation generation and `make docs-check` passed.
- The final session/config/export/forkguard regression and the background
  continuation integration tests passed. The latter exercise five recoveries
  with `max_turns=1`, the memory policy and the global disable switch.
- Real-browser layout/menu checks passed at narrow and wide widths. The
  transcript overflow checks passed from 360 to 1280 pixels in English and
  Russian; the WebKit scroll checks passed.
- Documentation captures show tool-image previews, HTTP default-header maps
  and the console's pending MCP startup. Before/after UI evidence was captured
  from the base revision and the running port.
- Structural branding checks found no errors.

Android NDK cross-builds were unavailable locally: no NDK or C compiler is
installed. The race detector is likewise unavailable with this host's
`CGO_ENABLED=0`. The Android build and race jobs remain in CI; no device or live
provider validation is claimed by this port.

## Acceptance follow-up (2026-10-27)

Subsequent tests used IntelliJ IDEA Community 2022.2.1 with its bundled JCEF,
an isolated Windows home, and an Android 14 x86_64 emulator running the native
backend under Termux's UID. A deterministic local model supplied the responses.
This follow-up supersedes the initial Android tooling limitation above; it
does not claim validation against a live model provider.

The acceptance run found and fixed four defects:

- Slash/CLI HTML, PDF and DOCX export now resolves tool pictures against the
  persisted session assets. Each of the six Windows/Android exports was
  inspected and contains its picture.
- Remote health probes use a bounded AbortController on older JCEF versions
  without AbortSignal.timeout. The same IDE now shows the Android server as
  available, without the false remote-down banner.
- Marketplace entries sharing a repository root honor their declared skill
  paths. Selecting claude-api installs one skill; updating preserves its
  disabled state and does not install neighboring entries. Directory swaps
  also retry transient Windows reader locks for at most 450 ms per rename.
- The IntelliJ theme test helper queues look-and-feel changes in a write-safe
  IDE activity. Seven real IDE tests pass, including the regression for new
  TransactionGuard errors; the earlier red IDE notification came from this
  helper.

The updated SPA passes 2,979 tests and TypeScript checks; all 112 Kotlin tests
pass through direct JUnit. Windows build/vet, Windows lint, regular lint,
documentation checks and the Android x86_64 native build/vet pass. The IntelliJ
test ZIP was rebuilt with all five desktop backends and its archive contents
were checked. Before/after captures and focused results live on the orphan
screenshots branch under upstream-1.2.35/acceptance-fixes. The PR records the
final full-suite result. The race detector still requires CI's host compiler.
