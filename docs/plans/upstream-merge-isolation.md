# Plan: isolate fork features to simplify upstream ports

Date: 2026-10-17. Status: proposed; no refactoring has been implemented by
this plan. Audit baseline: FoxxyCode `3e9e4c6f8`, synchronized through coddy
`1.2.10` (`5ed775c47789a5c9ec49cfb66c0d575730599cf8`).

## Objective and scope

Reduce the number of shared files that must be understood and edited when
porting coddy changes. A feature should own its domain rules, UI components
and styles, with a small explicit connection to the common runtime.

Success means that a later SVN or IDE enhancement can usually be implemented
inside its feature packages without adding conditions to `react.go`,
`App.tsx`, shared settings forms or shared tool message components.

This is a plan for moving existing functionality while preserving behavior.
It does not authorize a new configuration format, HTTP contract, permission
policy or retirement of a recorded upstream divergence. Implementation is
separate work; unchecked steps below are pending.

## Evidence: upstream 1.2.11

The audit compared upstream `1.2.10` with `1.2.11`
(`2b47c1f7e2163e8edd4b5910562fb43efffb0e30`) against the fork baseline.
The release changes provider model fetching, context-window display, settings
layout and skill version metadata.

A per-file trial used `git merge-file -p --diff3`, with upstream `1.2.10` as
the explicit base, FoxxyCode as ours and upstream `1.2.11` as theirs. Inputs
normalized branding, module names and CRLF; the bundled configuration skill
path was mapped from `configure-coddy` to `configure-foxxycode`.

| Trial result | Files |
| --- | ---: |
| Existing text files merged without conflict | 33 |
| Existing text files with conflicts | 37 |
| New text files absent from the fork | 12 |
| Upstream text files absent at the corresponding fork path | 2 |
| Binary assets, excluded from text merging | 10 |
| Total changed upstream paths | 94 |

The 37 conflicting files contain 159 textual conflict blocks. The SPA accounts
for 25 files and 133 blocks. Representative overlaps are `SettingsSection.tsx`
(13 blocks), `SchemaForm.tsx` (14), `styles.css` (16) and settings ordering.
The two missing text paths are `settings/schemaI18n.ts` and its test; the fork
uses `ui/i18n/schemaStrings.ts`, so these require semantic adaptation.

These are results of a normalized text merge, including formatting and nearby
edits. They are not measurements of developer effort, semantic defects or a
full merge of Git histories. No refactored counterfactual was built, so the
plan makes no percentage claim about future conflict reduction.

Upstream 1.2.11 does not change `react.go`, the IDE route registration file or
the HTTP bridge. Moving those IDE seams would not directly resolve this
release's main conflicts. Shared settings extensions have the strongest
observed overlap; SVN is a smaller pilot for establishing a useful boundary.

## Priorities and delivery order

Benefit and delivery order are different: settings have the strongest evidence
of benefit, while a limited SVN policy extraction is a bounded first step.
Relative effort is an estimate of scope, not a time commitment.

| Order | Work package | Expected benefit | Relative effort | Dependency |
| --- | --- | --- | --- | --- |
| 0 | Record a repeatable port baseline | Makes the outcome assessable | Small | None |
| 1 | SVN tool-policy pilot | Medium; removes domain rules from the agent | Small to medium | Baseline |
| 2 | Settings extensions | High; directly addresses 1.2.11 overlaps | Medium to large | Baseline; independent of pilot implementation |
| 3 | Additional tool rendering and previews | High for later chat/tool UI ports | Medium | Existing SVN cards as first consumer |
| 4 | Complete SVN workspace/UI isolation | Medium to high within SVN | Medium | SVN policy and rendering boundaries |
| 5 | IDE host integration | Medium, dependent on the upstream release | Medium | Existing context stores and event contracts |
| 6 | Project/environment selection | Medium; reduces additions to `App.tsx` | Medium | Workspace boundaries from order 4 where reusable |
| 7 | Autocomplete service | Medium architectural benefit; limited immediate port benefit | Medium | Explicit provider and editor-context inputs |
| 8 | Diagnostics adapters | Low to medium | Medium | Evidence from a relevant future port |

Do not make a large feature framework a prerequisite for these steps. Each
work package should be independently reviewable, retain existing public
behavior and leave the tree buildable.

## 1. SVN tool-policy pilot

Keep `internal/svnws` for the client and working-copy operations, and
`internal/tools/svn` for tool construction. Both are already separate
packages and absent from upstream 1.2.11. Moving their directories alone
offers little benefit.

The remaining agent coupling is concrete:

| Current site | Domain knowledge to move | Resulting connection |
| --- | --- | --- |
| `internal/agent/react.go`: `toolKind`, `filesystemWriteTool` | Hardcoded `svn_*` classifications | Small tool-policy lookup with the existing core fallback |
| `internal/agent/result_eviction.go`: `svnWriteTargets` | SVN arguments and content invalidation | SVN-owned affected-path policy; eviction stays in the runtime |
| `internal/agent/toolsets.go` | Read-only SVN names for allowed modes | Explicit mode contributions subject to core restrictions |
| `internal/tools/export.go` | SVN registration | Retain one registration call |

Place shared policy contracts below the agent, for example alongside the
existing `internal/tooling` contracts if needed. SVN must not import the agent
to implement them. Resolve how policies are available during history replay,
including when a tool is disabled or the local SVN client is absent; do not
make old transcript interpretation depend only on the active tool registry.

Preserve the distinction between approval and content invalidation.
`svn_add` and `svn_commit` change SVN state but do not invalidate earlier
file reads. Update, revert and resolve invalidate their target paths;
switch, merge and checkout can affect a whole subtree. Preserve the existing
conservative scope when the arguments do not identify narrower targets.

Acceptance: SVN classifications and invalidation rules have one domain owner;
core execution still owns permission decisions and eviction. Mode and child
restrictions continue to narrow access. Marking a tool read-only must not
automatically admit it to every restricted mode.

## 2. Settings extensions

Keep the common forms close to coddy. Extract fork additions currently spread
across `Settings.tsx`, `SettingsSection.tsx`, `settingsSections.ts`,
`SchemaForm.tsx`, translations and `styles.css`:

- Additional sections, including General and autocomplete, with explicit
  ordering and visibility contributions.
- Provider row actions such as import/export, and custom field overrides.
- Fork-specific appearance/onboarding additions and host compatibility styles.
- A narrow translation adapter for the common form, preserving the existing
  schema translation behavior instead of introducing a third translation path.

Prefer a small typed set of section definitions and component slots to a
dynamic plugin loader. Keep public config keys, schemas and DTOs centrally
defined. Retain existing disabled/unavailable-feature presentation.

Provider model fetching remains a common provider capability. Reconcile the
fork's `/foxxycode/providers/models-probe` contract with the upstream model
listing changes explicitly; moving it under IDE would not resolve that work.

Acceptance: adding a fork-only settings section or provider row action does
not require a new branch in the generic schema form. Replaying the settings
portion of 1.2.11 needs fewer domain-specific adaptations, with the same saved
config, translations, navigation and supported browser behavior.

## 3. Additional tool rendering and previews

Domain cards such as `SvnAction.tsx` already exist. Their selection, labels,
error summaries and previews still affect `ToolCallMessage.tsx`,
`PermissionPromptPreview.tsx`, `permissionToolPreview.ts` and `liveStatus.ts`.

Introduce a small explicit adapter for added tool renderers, permission
previews and status labels, starting with SVN. Preserve the generic fallback
for unknown tools and old transcripts. Display adapters must not determine
whether a tool is authorized. Move feature styles into a feature stylesheet
while preserving cascade order and supported browser behavior.

Acceptance: a new SVN operation uses the SVN adapter without adding matching
conditions to each common chat component; pending, approved, failed,
cancelled and replayed states remain correct.

## 4. Complete SVN workspace/UI isolation

Move SVN-specific discovery and branch operations out of the common parts of
`external/httpserver/workspace_context.go`. Keep session locks, admission
checks, workspace changes and catalog/MCP reload orchestration in the common
handler. An adapter may return the selected directory or operation result;
it must not bypass those checks.

Extract a `SvnWorkspaceChip` and an SVN workspace action hook from
`WorkspaceChips.tsx` and `App.tsx`. Keep the existing payload shape, branch
folder preference and independent Git/SVN detection. A directory can contain
both repositories. Share branch-label resolution for exports and the CLI
through a lower-layer helper instead of making command surfaces depend on
the tool implementation merely to describe a workspace.

Keep typed config under `internal/config`: importing the tool package from
config would create the wrong dependency direction. OpenAPI and schema
entries remain deliberate integration points, even after implementation
details move.

Acceptance: changing SVN branch behavior or its menu is localized, while
Git behavior, session workspace locking and workspace reloads remain intact.

## 5–8. Later extractions

| Area | Boundary | Acceptance |
| --- | --- | --- |
| IDE integration | Editor/terminal prompt context outside `react.go`; separate route registration, event delivery and embed styles; reuse `ideenv`, `ideterm`, `idecopy` | New editor state does not grow the agent loop; event/auth and context limits remain intact |
| Project/environment selection | Action/state hooks and workspace widgets; reuse `internal/project` | Additions no longer expand the central `App.tsx` state machine; session/remote selection remains consistent |
| Autocomplete | Completion service with explicit config, provider and related-file inputs; thin HTTP adapter | Preserve cancellation, timeouts, FIM/chat fallback, output cleaning and editor-context bounds |
| Diagnostics | Event sink and transport wrappers; retain necessary runtime event calls | Capture stays opt-in; tracing preserves event ordering and transport behavior |

## Keep existing boundaries

Export, desktop and branding already have standalone packages. Tighten a
specific adapter only when a port exposes a problem. Encoding, built-in docs
and preview server also have shared packages; use those contracts rather
than adding another implementation or moving them again.

Retry, continuation, compaction and cross-process session refresh change core
runtime semantics. They are unsuitable first extractions. Pure decision
helpers may help later, but preserve their existing guards and consult the
[upstream divergence registry](../../.cursor/rules/upstream-divergences.mdc)
before changing any marked site.

Build tags are a later packaging decision. A tag does not affect Git's text
merge and does not automatically exclude TypeScript from the embedded SPA.
Separate Go modules or repositories are outside this plan. Avoid broad
upstream file moves: an abstraction that creates more structural divergence
than it removes fails the objective.

## Verification and evidence for each work package

1. Record the fork SHA, upstream base/target SHAs and exact normalization used
   in the trial. Use the same upstream range before and after the extraction.
2. List common files that lose feature rules, the remaining integration calls
   and the feature files that now own those rules.
3. Compare textual conflicts and semantic adaptations separately. Moved files
   can change conflict counts without reducing reasoning effort; inspect the
   conflicting blocks and the resulting behavior as well.
4. Preserve or extend meaningful regression coverage for the moved behavior.
   Use the existing happy-path Gherkin coverage and focused edge-case tests;
   do not add tests that only assert the new file layout.
5. Run the repository's required checks for implementation: `make test`,
   `make lint` and documentation checks. Rebuild embedded assets with
   `make build TAGS="http ui"` for SPA changes, and capture the actual changed
   surfaces before/after, including narrow and wide layouts and affected
   themes. Verify the supported IDE browser for compatibility changes.
6. Update OpenAPI, config schema/DTOs and user documentation if a contract or
   documented behavior changes. Run Windows checks for Windows-specific
   changes and targeted combinations when a build-tag boundary changes.

Configuration, tool names, permission modes, persisted sessions, API shapes
and the rendered UI should remain compatible during extraction. Behavioral
changes belong in separate, explicit changes.

## Delivery checklist

- [ ] Reproduce and retain the baseline comparison.
- [ ] Complete the limited SVN policy pilot and verify replay/permission rules.
- [ ] Separate settings additions and assess the 1.2.11 replay.
- [ ] Extract additional tool rendering/previews with SVN as first consumer.
- [ ] Complete SVN workspace adapters, chip and styles.
- [ ] Extract IDE host integration when its porting benefit is demonstrated.
- [ ] Separate project/environment actions where the common shell is affected.
- [ ] Extract the autocomplete service when needed by its next change.
- [ ] Reassess diagnostics against a concrete port before starting it.

This document records the proposed order at the stated baseline. Record actual
outcomes and revised decisions in implementation changes; do not treat the
unchecked plan as a statement that the architecture has already moved.

Related: [upstream sync tracker](../../UPSTREAM_SYNC.md),
[architecture](../contributing/architecture.md),
[documentation contract](../contributing/documentation.md),
[web UI](../surfaces/web-ui.md),
[session export](../features/session-export.md).
