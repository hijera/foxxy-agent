# Mini Apps Implementation Plan

## Status

This document defines the implementation plan for Mini Apps in FoxxyCode. It
is based on:

- the portable Mini Apps prototype in pull request #28;
- the Mini Workflow implementation in the neighboring ValeDesk project; and
- the current FoxxyCode session, agent, tool, permission, HTTP, and UI
  architecture.

The plan deliberately separates the first usable FoxxyCode-hosted workflow
from later portability work. The first milestone must prove that FoxxyCode can
turn a real successful session into a replayable workflow without requiring an
author to replace the generated workflow by hand.

## Product goal

A Mini App is a versioned, operator-facing workflow distilled from a successful
FoxxyCode session. An author reviews and verifies the workflow once. An operator
can then run a released version from a generated form without repeating the
original planning conversation.

The primary flow is:

1. Complete a task in a normal FoxxyCode web or desktop session.
2. Select **Create Mini App**.
3. Confirm the scenario that FoxxyCode identified in the session.
4. Review inferred inputs, steps, permissions, success criteria, and outputs.
5. Replay the exact draft revision against sanitized source fixtures.
6. Repair discrepancies through bounded, reviewable draft patches.
7. Review sanitization findings and release an immutable version.
8. Run the released version with new operator inputs.

## Scope decisions

### Included in the first release

- optional compilation behind the `miniapps` build tag;
- explicit distillation of one selected session and one confirmed scenario;
- normalized extraction of prompts, tool calls, tool results, artifacts, and
  relevant permission grants;
- generated and editable inputs;
- sequential typed workflow steps;
- deterministic tool execution through the existing FoxxyCode tool registry;
- bounded LLM and ReAct agent steps;
- explicit confirmation and restricted branching;
- exact-version composition of another released Mini App;
- inferred and enforced permissions;
- same-data replay in an isolated workspace;
- deterministic and model-assisted success checks;
- bounded verification and repair cycles;
- mutable draft revisions and immutable semantic releases;
- global catalog and run history;
- asynchronous distillation and runs with progress and cancellation;
- web and desktop authoring and operator surfaces;
- import and export of the canonical JSON document only if it does not delay
  the core flow.

### Deferred

- `foxxy-vm/1` and a general-purpose bytecode runtime;
- arbitrary inline Python or JavaScript;
- automatic portable runtime, package, or local-model provisioning;
- app-specific standalone executable generation;
- bundled MCP servers and executable skills;
- project-local Mini Apps;
- scheduled runs;
- semantic discovery and automatic routing between Mini Apps and ordinary
  agent sessions;
- organizational catalogs, signing, and publisher trust;
- parallel workflow branches and distributed execution.

Project-local Mini Apps are executable project-owned content. When introduced,
their discovery and execution must pass through the workspace trust mechanism
in `internal/mcp`, using the same trust-store principles as project-local
`.foxxycode/mcp.json` declarations.

## Existing work to reuse

### Pull request #28

Reuse or adapt:

- the optional `miniapps` build boundary;
- the canonical JSON concepts;
- draft revisions and immutable releases;
- storage locking and atomic JSON writes;
- validation, reference resolution, and sanitization foundations;
- model binding and exact provider/model matching;
- permission declarations;
- run records and release gates;
- HTTP/OpenAPI naming where it matches this plan;
- UI interaction and layout ideas, split into maintainable components.

Replace before merge:

- the current distiller, which derives a draft from only the first user message
  and last assistant response;
- the current `agent` step, which performs a single completion without the
  FoxxyCode ReAct/tool loop;
- synchronous test and release-run handlers;
- authoring edits that mutate and save a draft without an explicit patch review;
- the BDD scenario that manually replaces the generated draft before testing;
- step kinds advertised by the schema but rejected by the runtime.

Keep deferred code out of the first merge rather than shipping dormant or
partially supported contracts.

### ValeDesk

Reuse as design input:

- phased LLM distillation: result identification, input extraction, workflow
  synthesis, and determinization;
- `not_suitable` and clarification outcomes;
- progress reporting and cancellation;
- replay through the existing agent implementation;
- isolated verification workspaces;
- model-assisted comparison with the accepted result;
- bounded refinement after discrepancies.

Do not carry over:

- storage of Mini Apps as ordinary skills;
- source session identifiers and transcripts inside the portable workflow;
- arbitrary generated scripts executed directly by a subprocess;
- secret inputs copied into process environment variables;
- coarse permission categories without per-operation enforcement;
- orchestration concentrated in a single IPC handler.

## Canonical data model

The portable document contains only reviewed runtime behavior:

```text
MiniApp
  schema_version
  kind
  id
  state
  version
  revision
  metadata
  requirements
  permissions
  inputs[]
  workflow[]
  success
  outputs[]
  display
  runtime
```

Source evidence is separate authoring data and is never exported or copied into
a release:

```text
AuthoringEvidence
  source_session_id
  scenario_candidates[]
  confirmed_scenario
  sanitized_trace
  accepted_result
  source_fixture
  metrics
```

Secrets are runtime handles. Secret values must not be stored in drafts,
releases, fixtures, logs, authoring conversations, or run records.

### Initial step types

`tool`

: Executes one exact registered FoxxyCode tool with templated JSON arguments.
  The tool must exist at validation and preflight time. Existing permission
  checks remain authoritative.

`llm`

: Performs one model call without tools. It is suitable for classification,
  rewriting, extraction, or another bounded semantic transformation.

`agent`

: Runs a FoxxyCode ReAct loop with an explicit model binding, allowlisted tools,
  maximum turns, timeout, and output contract.

`confirm`

: Pauses execution until the operator approves or rejects a declared action.

`branch`

: Selects one of two step lists using the restricted condition language.

`miniapp`

: Runs an exact immutable release of another Mini App. Nested execution depth
  is bounded.

Filesystem operations and commands use existing FoxxyCode tools in the first
release. They do not require separate workflow step kinds.

## Package design

The optional domain remains below HTTP and UI integration:

```text
external/miniapps/
  types.go          canonical JSON types
  validate.go       schema, references, and capability validation
  permissions.go    permission inference and enforcement
  store.go          drafts, revisions, releases, jobs, and runs
  trace.go          normalized source trace types
  distill.go        scenario and workflow synthesis
  verify.go         replay comparison and repair
  runner.go         step orchestration
  authoring.go      bounded patch proposals
  service.go        job lifecycle, progress, events, and cancellation
```

Execution dependencies are interfaces owned by the Mini Apps module:

```go
type ToolExecutor interface { /* execute a registered tool */ }
type AgentExecutor interface { /* run one bounded agent step */ }
type ModelExecutor interface { /* run one tool-free model step */ }
type PermissionGate interface { /* preflight and runtime checks */ }
type EventSink interface { /* persist and publish safe events */ }
```

Adapters may depend on `internal/agent`, `internal/tooling`, session persistence,
MCP, and configured model providers. Core validation and storage must not depend
on HTTP, React, desktop, or ACP packages.

The existing `internal/bgtask` pool is command-oriented. Mini App jobs should
not be forced into it. The Mini Apps service owns contexts, cancellation,
persisted state, and event subscriptions for distillation and workflow runs.

## Distillation pipeline

### 1. Eligibility

- the session exists and is readable;
- no turn is currently running;
- at least one completed user task and assistant outcome exists;
- the selected surface is web or desktop;
- tool-centric or hybrid sessions are accepted;
- conversation-only sessions return `not_suitable` and may suggest a prompt
  preset in a later feature.

### 2. Trace extraction

Build a deterministic normalized trace from:

- persisted `llm.Message` values;
- assistant `ToolCalls` and matching tool-result messages;
- `internal/session/toolcalls_store.go` metadata, arguments, and results;
- persisted UI events when needed for status and timing;
- workspace diffs and known output artifacts;
- effective model and relevant permission grants.

Each observed action records its source turn, name, arguments, result, status,
duration when available, and produced artifacts. Failed attempts remain
evidence but are not automatically compiled into the successful path.

### 3. Scenario selection

The model receives the sanitized normalized trace, not an unrestricted raw
transcript. It returns structured candidates with task, accepted outcome,
boundaries, and confidence.

The job enters `waiting_for_scenario` when confirmation or correction is
needed. Multiple independent tasks must never be silently combined.

### 4. Input classification

Classify observed values as:

- fixed constants;
- operator inputs;
- secret handles;
- prior-step outputs;
- environment requirements;
- source-specific data that must be discarded.

Source values are retained only in the private test fixture. Detected secrets
become unresolved secret handles.

### 5. Workflow synthesis

- convert stable observed tool calls into `tool` steps;
- preserve argument structure and replace variable values with references;
- use `llm` for bounded semantic transformations;
- use `agent` only where dynamic tool selection or iterative work is required;
- infer the smallest tool allowlist and permission set;
- define explicit outputs and success checks;
- validate the generated document before saving a draft.

### 6. Verification and repair

1. Create an isolated workspace.
2. Copy only declared source fixtures.
3. Run the exact draft revision with effective source inputs.
4. Evaluate deterministic success checks.
5. Compare outputs and artifacts with the accepted source result.
6. Use a verifier model only when deterministic checks are insufficient.
7. Produce a structured discrepancy report.
8. Propose a patch against the exact draft revision.
9. Validate and rerun after the patch is accepted.
10. Stop after the configured limit, initially three attempts.

A failed verification leaves an editable draft. It never releases
automatically.

## Runtime behavior

- validate the exact draft revision or release before execution;
- preflight tools, models, inputs, permissions, and dependencies;
- create a dedicated run workspace and persisted run record;
- execute steps sequentially;
- propagate cancellation through tools, agents, model calls, nested apps, and
  child processes;
- persist step attempts and outputs incrementally;
- expose only sanitized operator events;
- keep raw diagnostic details in bounded local logs;
- redact secrets from inputs, outputs, errors, and events;
- evaluate success checks before marking a run successful;
- record duration and token usage when providers expose them.

Run statuses are:

```text
pending
running
waiting_for_input
waiting_for_confirmation
succeeded
failed
cancelled
interrupted
```

## Persistence layout

Initial global layout:

```text
<home>/miniapps/<app-id>/
  catalog.json
  draft/
    miniapp.json
    authoring/
      evidence.json
      fixture/
      patches/
    passing-test.json
  releases/<version>/
    miniapp.json

<home>/apps/<app-id>/runs/<run-id>/
  run.json
  events.jsonl
  execution.log
  artifacts/
```

Draft saves increment an opaque revision. Test reports include that revision.
Release requires a successful test for the current revision, a clean blocking
sanitization report, and explicit human confirmation. Released versions are
immutable.

## HTTP contract

Long-running operations return `202 Accepted` and persist their state.

### Distillation

```text
POST /foxxycode/sessions/{id}/miniapps/distill
GET  /foxxycode/miniapp-distillations/{job_id}
GET  /foxxycode/miniapp-distillations/{job_id}/events
POST /foxxycode/miniapp-distillations/{job_id}/scenario
POST /foxxycode/miniapp-distillations/{job_id}/cancel
```

### Catalog and authoring

```text
GET/POST       /foxxycode/miniapps
GET/PATCH      /foxxycode/miniapps/{id}
GET/PUT        /foxxycode/miniapps/{id}/draft
POST           /foxxycode/miniapps/{id}/authoring/patches
POST           /foxxycode/miniapps/{id}/authoring/patches/{patch_id}/accept
POST           /foxxycode/miniapps/{id}/validate
POST           /foxxycode/miniapps/{id}/sanitize
POST           /foxxycode/miniapps/{id}/release
```

### Runs

```text
POST /foxxycode/miniapps/{id}/test-runs
POST /foxxycode/miniapps/{id}/versions/{version}/runs
GET  /foxxycode/miniapp-runs/{run_id}
GET  /foxxycode/miniapp-runs/{run_id}/events
POST /foxxycode/miniapp-runs/{run_id}/confirmation
POST /foxxycode/miniapp-runs/{run_id}/cancel
```

Every implemented route must be added to `external/httpserver/openapi.go` and
documented in `docs/http-api.md` in the same change.

## UI delivery

Split the prototype workspace into focused components:

```text
miniapps/
  MiniAppsCatalog
  DistillationWorkspace
  ScenarioReview
  MiniAppEditor
  InputsEditor
  StepsEditor
  PermissionsReview
  VerificationReport
  ReleaseReview
  MiniAppRunner
  RunHistory
```

The first UI increment includes:

- **Create Mini App** on an eligible session;
- distillation progress and cancellation;
- scenario confirmation or correction;
- editable metadata, inputs, steps, permissions, success criteria, and raw JSON;
- verification results and proposed patch review;
- sanitization and release review;
- searchable catalog;
- generated run form, confirmation prompts, progress, result, and rerun;
- run history with sanitized diagnostics.

The UI is absent from IDE surfaces and hidden when the backend capability is
false. Every UI pull request includes screenshots required by the repository
workflow rules.

## Delivery sequence

Implementation proceeds as reviewable vertical increments from a fresh branch
based on `main`. Pull request #28 remains a reference and source of selected
code; it is not merged and repaired as one large change.

### PR 1: Contract, validation, and store

- reduce and finalize the v1 schema;
- add tagged step validation;
- implement draft revisions, releases, and atomic persistence;
- implement baseline sanitization and release gates;
- add unit tests for schema, references, revisions, conflicts, and traversal.

Exit criterion: a hand-authored Mini App can be validated, saved, tested by a
fake executor, and released only under the correct revision.

### PR 2: Trace extraction and scenario selection

- normalize persisted session and tool-call data;
- sanitize the trace;
- implement eligibility and scenario outcomes;
- add asynchronous jobs, progress, scenario confirmation, and cancellation;
- add fixture-based tests for successful, ambiguous, and unsuitable sessions.

Exit criterion: a real persisted session produces a confirmed normalized
scenario without generating a workflow yet.

### PR 3: Workflow synthesis

- implement structured input classification;
- compile observed tool calls into `tool` steps;
- synthesize bounded `llm` and `agent` steps;
- infer permissions, success checks, and outputs;
- save a validated draft and private evidence;
- replace the existing BDD draft-substitution step.

Exit criterion: distillation creates an executable draft from the recorded
session without manual JSON replacement.

### PR 4: Runtime and verification

- adapt `tooling.Registry.Execute` for deterministic steps;
- add bounded runtime session state for `agent.Agent.Run`;
- implement step events, cancellation, and exact revision execution;
- add isolated source replay and deterministic success checks;
- add verifier and bounded repair patch generation;
- test cancellation, permissions, secret redaction, and nested depth.

Exit criterion: the unchanged generated draft reproduces the accepted source
result and a failing draft produces a reviewable discrepancy report.

### PR 5: HTTP and OpenAPI

- expose jobs, drafts, patches, releases, and runs;
- implement SSE event streams and cancellation endpoints;
- add HTTP unit tests and BDD steps;
- update OpenAPI and HTTP documentation.

Exit criterion: the entire lifecycle can be driven headlessly over HTTP.

### PR 6: Web and desktop UI

- add the session action and capability gating;
- implement scenario, editor, verification, release, catalog, and runner views;
- split the existing prototype into focused components;
- add responsive interaction tests and required screenshots;
- rebuild embedded UI assets.

Exit criterion: a web/desktop user completes the lifecycle without editing raw
files or calling the API manually.

### PR 7: Hardening and operational readiness

- add quotas and retention policies;
- recover interrupted jobs and runs;
- verify process and network cancellation;
- expand sanitizer coverage to all authoring and run artifacts;
- run host and Windows build/lint matrices;
- document configuration, operations, and troubleshooting.

Exit criterion: security and failure-mode tests pass across supported build-tag
combinations and platforms.

### Later portability track

Only after the FoxxyCode-hosted lifecycle is stable:

1. portable bundle format and conformance fixtures;
2. restricted deterministic program representation, if real workflows require
   more than registered tools;
3. dependency locks and private provisioning;
4. headless interpreter;
5. console executable builder;
6. Windows desktop executable builder;
7. signing and publisher trust.

## Test strategy

### Required BDD happy path

```gherkin
Feature: Distill and run a reusable Mini App
  Scenario: A successful tool session becomes a released Mini App
    Given a completed FoxxyCode session with successful tool calls
    When I distill the session and confirm the selected scenario
    Then the generated draft contains inferred inputs and executable steps
    When I test the unchanged generated draft with its source inputs
    Then it reproduces the accepted source result
    When I release the tested draft
    And I run the release with different inputs
    Then it produces the corresponding new result
```

The feature must not contain a step that replaces the generated workflow with a
hand-authored fixture.

### Unit and integration coverage

- trace pairing and failed-attempt filtering;
- ambiguous and conversation-only sessions;
- input classification and secret handles;
- structured model response validation;
- tool allowlists and permission mismatches;
- reference resolution and condition evaluation;
- cancellation propagation;
- stale revision and release conflicts;
- same-data comparison and repair limits;
- path traversal and unsafe artifacts;
- redaction in every persisted and streamed surface;
- nested Mini App recursion bounds;
- HTTP status and OpenAPI parity;
- capability absence without `miniapps`;
- responsive UI behavior and operator interaction.

Each behavior change follows repository BDD/TDD order: failing test, narrow
green test, full `make test`, documentation synchronization, and `make lint`.

## MVP acceptance criteria

The first release is complete when:

1. A real persisted session with successful tool calls creates a valid draft.
2. The author confirms one scenario before workflow generation.
3. Inputs, constants, secrets, prior outputs, and environment requirements are
   classified separately.
4. The generated draft runs without manual workflow replacement.
5. Deterministic steps use existing FoxxyCode tools and permissions.
6. Agent steps use the real bounded FoxxyCode ReAct loop.
7. Same-data replay reproduces the accepted source result in isolation.
8. Failed verification cannot release and produces reviewable discrepancies.
9. Accepted repairs create a new revision and require another passing test.
10. Releases are sanitized, explicitly approved, versioned, and immutable.
11. Released runs support progress, confirmation, cancellation, results, and
    sanitized history.
12. Secrets and source provenance do not appear in releases or run logs.
13. A build without `miniapps` has no Mini Apps routes, commands, or UI.
14. Three representative scenarios pass end to end:
    - deterministic file transformation;
    - hybrid research or report generation;
    - unsuitable conversation-only session.
15. `make test` and `make lint` pass, plus Windows checks when shared or
    Windows-specific signatures are touched.

