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
