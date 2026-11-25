# Upstream 1.2.74–1.2.79 port

## Scope

Port the complete net range
`d81d6cca0c12716035a0539130cab8ac66fe2a27..cd42d7df06fb3c64c1b482e7c4463f3fc239ebb4` from
coddy-project/coddy-agent through tag `1.2.79`, onto PR #167 head
`4959cccf21767fe95c7f08dc50c20f153b3f460e`. The range contains 43 commits:
29 constituent changes and 14 integration/release merges. No feature is
intentionally deferred.

- Add `httpserver.cors.allow_loopback` and `swarm.cors.allow_loopback`, with
  strict serialized-origin matching shared by Go and the SPA, settings and
  relay persistence, security warnings, dry-run diagnostics and localized
  environment guidance.
- Return provider usage in non-streaming OpenAI-compatible completions, include
  cached input in direct streams, and account for Anthropic cache reads/writes
  and final input-usage overrides.
- Keep History filter submenus inside phone screens, folding them inline when
  neither side has enough room.
- Portal shared-file action menus out of clipped cards, preserve focus and
  keyboard navigation, follow visible triggers, close covered/offscreen menus,
  and keep downloads available after unrelated action failures.
- Skip Codex keep-alive/empty SSE frames through the lenient shared scanner and
  retain idle-stall protection for Codex responses without Content-Type.
- Apply uppercase-first proxy environment resolution, Vite/Vitest security
  updates, Go 1.26 toolchain guidance and govulncheck as the third AppSec
  scanner.

## Fork adaptations

Preserve every registered divergence. In particular, the idle guard remains at
the provider/chunk layer rather than upstream's response-body transport layer;
whole tool calls still survive truncated answers; transport debug/proxy wrappers
and system-proxy behavior remain; and token updates remain cumulative for the
turn. The HTTP usage collectors therefore consume the latest cumulative update
instead of summing cumulative snapshots a second time.

Keep FoxxyCode's POST reveal action and independent copy/reveal error notices.
Keep the Chromium 104 baseline: upstream `:has()` selectors become the React
marker classes `inline-artifact-card--image` and
`inline-artifacts--before-foot`, and the menu has a `vh` fallback before `dvh`.
The existing intentionally non-Prettier-clean SPA files remain excluded from
the new staged-file format gate; all other touched SPA files are formatted
individually.

FoxxyCode already carries newer Go module and toolchain security versions than
the 1.2.75 floor (Go 1.26.10 and x/crypto 0.59.0), so those versions are not
downgraded. Upstream-branded screenshots are not copied: the three documentation
captures come from real FoxxyCode components on an isolated local stand.

## Validation

- Focused config, netx, LLM, HTTP, Swarm and CLI tests pass, including the new
  CORS, usage, Anthropic, Codex keep-alive and no-Content-Type scenarios.
- TypeScript checking and 146 focused SPA tests pass; the final three changed
  component suites pass 54/54 after adaptation.
- `npm run check:artifacts` passes every Chromium case: English/Russian,
  desktop/phone, touch/non-touch, portrait/landscape and a short scrolling
  window.
- `make build TAGS="http ui"`, Chromium 104 compatibility, `make docs-fast`,
  `make site-schema-check` and source diff checks pass. Embedded content-hashed
  renderer chunks are included.
- `GOFLAGS='-count=1 -p=2 -timeout=20m' make test` passes all 377 SPA files /
  3,776 tests and the complete shipped-tag Go tree. `make lint`, full
  `make docs-check`, `npm run test:layout`, the complete English/Russian phone
  overflow grid, both editor changelog suites and all 185 VS Code tests pass.

CI's tag matrix and race detector remain PR checks. Work is isolated in the
`H:/kimi/foxxy-agent-upstream-1.2.79` worktree on `codex/upstream-1.2.79`;
nothing is pushed by the local port.
