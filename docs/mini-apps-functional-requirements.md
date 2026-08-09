# FoxxyCode Mini Apps Functional Requirements

Status: clean-room functional requirements for the MVP.

Russian version: [mini-apps-functional-requirements.ru.md](mini-apps-functional-requirements.ru.md)

Related architecture and JSON language specification:
[mini-apps-spec.md](mini-apps-spec.md).

Behavioral research reference:
[NeuralDeskApp PR #115](https://github.com/vakovalskii/NeuralDeskApp/pull/115)
and [issue #106](https://github.com/vakovalskii/NeuralDeskApp/issues/106).

This document describes independently formulated FoxxyCode behavior. It does not
authorize copying NeuralDeskApp source code or specification text.

## 1. Objective

FoxxyCode must let a non-developer turn a successfully completed session into an
editable, tested, portable mini app. The mini app must expose a generated input
form, execute a versioned JSON workflow, verify the result, and present declared
outputs without depending on the original session.

The MVP must prioritize reliable replay over fast draft generation. A long
distillation is acceptable when it produces a safer and more reproducible
program.

## 2. Actors

| Actor | Responsibility |
|-------|----------------|
| Author | Creates a mini app from a session, edits it, tests it, and releases it. |
| Operator | Supplies inputs, grants declared permissions, handles checkpoints, and consumes results. |
| Distiller | Analyzes a source session and produces or repairs a draft. |
| Interpreter | Validates and executes the JSON program. |
| Verifier | Compares replay results with declared success criteria and expected outcomes. |
| Administrator | Future actor controlling organizational collection and shared catalogs; not an MVP UI role. |

The author and operator may be the same person.

## 3. Requirement language

`MUST`, `MUST NOT`, `SHOULD`, and `MAY` have their usual requirements meaning.
Requirement ids are stable references for implementation, tests, and future
design documents.

## 4. Product surfaces

### FR-SURFACE-001 — Supported surfaces

Creation, editing, testing, release, catalog management, and interactive runs
MUST be available in the FoxxyCode web and desktop surfaces.

### FR-SURFACE-002 — IDE exclusion

Mini-app controls MUST NOT be rendered in IntelliJ, VS Code, ACP, or other
IDE-embedded surfaces in the MVP.

### FR-SURFACE-003 — Session action

A completed session MUST expose a **Create mini app** action. The action MUST be
disabled while the session has an active turn.
After an accepted session result, the same operation MAY additionally be exposed
as **Save as mini app** beside the result summary.

### FR-SURFACE-004 — Catalog entry point

The main web/desktop navigation MUST contain a **Mini Apps** section with list,
search, import, run, edit, version, archive/restore, export, and history actions.

Archiving is catalog visibility state and MUST NOT change the mini app's
`draft`/`released` lifecycle state.

### FR-SURFACE-005 — `miniapps` build tag

Mini-app support MUST be compiled only when the Go build tag `miniapps` is
enabled. A tagged build MUST include the interpreter, executable builder,
mini-app HTTP registrations, and a true backend `miniapps` capability. An
untagged build MUST NOT register those commands or routes and MUST advertise the
capability as false.

Web/desktop UI controls and routes MUST require the backend capability. IDE
surface exclusion remains an additional condition and MUST NOT be bypassed by
the tag.

### FR-SURFACE-006 — Session quick catalog

An open web/desktop session MUST be able to open and close a compact Mini Apps
catalog drawer without leaving the session. The drawer and full catalog MUST use
the same records, filters, release resolution, and archive visibility state.

## 5. Distillation eligibility

### FR-ELIG-001 — Basic eligibility

FoxxyCode MUST reject distillation before any model call when:

- the session is empty;
- a turn is still active;
- the session cannot be read;
- no user task can be identified;
- no observable outcome or target result can be identified.

### FR-ELIG-002 — Scenario classification

The distiller MUST classify the selected scenario as one of:

- `deterministic`: reproducible with scripts, API, MCP/skill, and file steps;
- `hybrid`: deterministic steps plus bounded LLM or agent steps;
- `agent_heavy`: primarily requires dynamic reasoning but can still be bounded
  by inputs, tools, outputs, and success checks;
- `not_distillable`: no stable task/result contract can be produced.

The classification and explanation MUST be shown before draft generation.

### FR-ELIG-003 — Whole-session analysis, single scenario

The complete session MUST be available to scenario analysis, but one
distillation job MUST produce exactly one scenario. If several independent tasks
are detected, the author MUST choose one.

### FR-ELIG-004 — Scenario confirmation

Before workflow synthesis, the author MUST be able to confirm or edit:

- task name;
- task description;
- expected result;
- included session interval or logical task;
- known side effects that may be repeated during testing.

## 6. Distillation pipeline

### FR-DISTILL-001 — Asynchronous job

Distillation MUST run as a cancellable asynchronous job with a stable job id,
persisted phase, progress events, elapsed time, and terminal result.

### FR-DISTILL-002 — Phased processing

The distiller MUST use separate bounded phases rather than a single unconstrained
prompt:

1. sanitize and normalize the source-session snapshot;
2. identify the task, result, artifacts, and successful path;
3. separate constants, operator inputs, secrets, and produced values;
4. build the typed workflow chain;
5. replace unnecessary model reasoning with deterministic scripts or calls;
6. infer requirements, permissions, outputs, display settings, and success
   criteria;
7. validate the JSON program and bundle;
8. replay with the source fixture;
9. verify the replay;
10. refine and repeat when verification fails.

Each phase MUST persist structured output so a failed job can report where and
why it stopped.

### FR-DISTILL-003 — Context minimization

Only the phases that require the full sanitized session SHOULD receive it.
Later phases SHOULD receive the task contract, prior structured phase results,
and only relevant artifacts. This limits token use and accidental data leakage.

### FR-DISTILL-004 — Successful-path extraction

The distiller MUST distinguish successful actions from exploration, failed
attempts, duplicate retries, and abandoned branches. Failed actions MAY be
retained only as explicit fallback knowledge.

### FR-DISTILL-005 — Determinization

When a session contains scripts, exact commands, API calls, or stable tool
sequences that produced the accepted result, the draft SHOULD preserve those
operations as deterministic steps instead of asking an agent to rediscover
them.

### FR-DISTILL-006 — Explicit agent boundaries

Every remaining LLM or agent operation MUST become an explicit bounded step with
a prompt, model capability, allowed tools, maximum turns, expected output
schema, timeout, and success behavior.

### FR-DISTILL-007 — Editable draft

A successful synthesis MUST create a mutable draft and open it in the editor.
It MUST NOT publish or release automatically.

### FR-DISTILL-008 — Cancellation

Cancellation MUST stop active model calls, agent cycles, scripts, HTTP calls,
MCP calls, and test replay. Temporary distillation sessions and test workspaces
MUST be deleted unless the author explicitly keeps a failed workspace for
debugging.

### FR-DISTILL-009 — Failed job recovery

A failed job MUST preserve its last valid phase output and diagnostic report.
The author MUST be able to retry from a safe phase or restart from the source
session.

### FR-DISTILL-010 — Source artifact classification

Every candidate artifact observed in the source session MUST be classified as
one of: operator input, bundled asset, test fixture, expected-output example, or
discarded evidence. The author MUST be able to inspect and change the proposed
classification before release.

### FR-DISTILL-011 — Source benchmark evidence

When available, the distillation job SHOULD record total source-session
duration, model/API duration, and model input/output token counts as private
authoring evidence. These values MUST NOT be copied into the portable program as
source-session provenance. A released app's estimates MUST be derived from its
own test/run history.

## 7. Input discovery and generated form

### FR-INPUT-001 — Value classification

Every relevant source-session value MUST be classified as:

- fixed constant;
- operator input;
- runtime secret binding;
- prior-step output;
- environment/dependency requirement;
- source-specific data to remove.

### FR-INPUT-002 — Supported input types

The generated form MUST support:

- single-line string;
- multiline text;
- integer and number;
- boolean;
- enum;
- date and datetime;
- file;
- multiple files;
- directory;
- secret.

### FR-INPUT-003 — Generated controls

The distiller MUST select an initial compatible control: text field, textarea,
number field, checkbox, select, radio group, date/datetime picker, file picker,
multi-file picker, directory picker, or secret field.

### FR-INPUT-004 — Validation

Inputs MUST support required state, default, enum, range, length, pattern, file
kind, extension/media type, file-count/size limits, and path existence.

### FR-INPUT-005 — Input dependencies

Inputs MUST support `visible_when`, `enabled_when`, and `required_when`.
Dependencies MUST be validated as an acyclic graph.

### FR-INPUT-006 — Author editing

The author MUST be able to edit field id, title, description, type, control,
default, validation, order, visibility, and workflow binding through both the
visual editor and raw JSON.

### FR-INPUT-007 — Dynamic operator input

The workflow MUST support an explicit operator-input step whose options may be
derived from prior step outputs.

## 8. Workflow requirements

### FR-WORKFLOW-001 — Canonical JSON

The versioned JSON program MUST be the only canonical execution definition.
Visual editor state MUST be derived from and written back to that JSON.

### FR-WORKFLOW-002 — Sequential semantics

Steps MUST execute sequentially unless an explicit `branch`, `fallback`, or
nested exact-version mini-app call changes the path.

### FR-WORKFLOW-003 — Step types

The MVP interpreter MUST support:

- operator input;
- deterministic inline or bundled script;
- general-purpose `foxxy-vm/1` JSON program;
- explicitly declared external command;
- bounded agent cycle;
- HTTP API call;
- bundled or declared MCP tool call;
- bundled skill call;
- file operation;
- operator confirmation;
- condition/branch;
- exact-version mini-app call.

### FR-WORKFLOW-004 — Common controls

Every executable step MUST support id, title, condition, timeout, retry policy,
error policy, declared inputs, declared outputs, logging/redaction policy, and
terminal status.

### FR-WORKFLOW-005 — Conditions

Conditions MUST use a restricted data expression language. Arbitrary JavaScript,
shell, Go, Python, or template evaluation MUST NOT be accepted as a condition.

### FR-WORKFLOW-006 — Retry and fallback

Retry MUST have a finite attempt count and bounded backoff. Fallback MUST be an
explicit validated sequence. Every attempt MUST be visible in the run log.

### FR-WORKFLOW-007 — Output propagation

Steps MUST expose named typed outputs. References to missing, future, or
incompatible outputs MUST fail validation before execution.

### FR-WORKFLOW-008 — Script protocols

Scripts and commands MUST receive arguments without implicit shell
concatenation. Structured results SHOULD use JSON and MUST be validated against
the declared output schema.

### FR-WORKFLOW-009 — General-purpose JSON VM

The MVP MUST provide the versioned, JSON-native, stack-based language
`foxxy-vm/1` in a `program` step. It MUST support functions, local values,
bounded loops and jumps, exceptions, JSON-compatible values, arithmetic,
comparison, string/array/object operations, schema validation, and typed
returns.

The validator MUST reject unknown opcodes, unresolved functions or labels,
invalid control targets, and invalid declared imports before side effects begin.
Execution MUST enforce positive engine-policy limits for instruction count,
wall time, heap, stack depth, call depth, and cancellation.

### FR-WORKFLOW-010 — JSON VM host boundary

The VM MUST be pure and deterministic by default. Filesystem, process, network,
time, random, model, MCP, skill, and operator effects MUST be available only
through typed import ids declared by the step. Imports MUST use the same
permission, secret, timeout, redaction, and logging enforcement as first-class
workflow steps.

The VM MUST NOT provide `eval`, dynamic opcode loading, or implicit access to
the host environment. The restricted condition language MUST remain separate
from the VM and MUST NOT call host imports.

## 9. Dependencies and portability

### FR-PORT-001 — Dependency declaration

The bundle MUST declare interpreter version, OS/architecture constraints,
executables, language runtimes, portable packages, models, secret bindings,
network access, skills, and MCP components.

### FR-PORT-002 — Bundle contents

Inline scripts, bundled script files, skill files, redistributable MCP
components, display assets, and validation fixtures MAY be included in the
portable bundle.

### FR-PORT-003 — Non-redistributable components

A component that cannot legally or technically be bundled MUST be represented
as an external host requirement with an exact identity and compatibility rule.

### FR-PORT-004 — Portable provisioning

The interpreter MUST support `silent_private` provisioning without a run-time
installation prompt when:

- the requirement declares a trusted HTTPS source;
- an exact checksum, artifact identity, and size or size ceiling are present;
- the declarative install action or bundled install script was included in
  release/import review;
- network and process permissions are declared;
- the exact release's authority has already been accepted.

Silent provisioning MUST write only to an app-specific cache and
same-filesystem staging directory. It MUST NOT elevate privileges, invoke a
system package manager, install a service, change global `PATH`/registry state,
or write outside that cache. Download, verification, and installation MUST be
transactional and concurrency-safe. Failure MUST leave the previous cache
usable and MUST fail preflight instead of attempting a system-wide fallback.

The same policy MUST cover embedded-runtime extraction and declared local-model
downloads. The operator surface MAY show a generic runtime-preparation status;
detailed redacted diagnostics MUST be written to the scoped app run log.

### FR-PORT-005 — Shared interpreter semantics

The FoxxyCode embedded runtime and standalone executable MUST use the same schema
validator, reference resolver, condition evaluator, step semantics, permission
model, and result contract.

### FR-PORT-006 — Import and export

The author MUST be able to export a draft or exact release as one portable
bundle and import it on another compatible installation. Import MUST validate
archive safety, integrity, schema compatibility, and id/version conflicts before
writing files.

### FR-PORT-007 — FoxxyCode interpreter mode

A FoxxyCode binary built with `miniapps` MUST validate, inspect, and execute
either a `miniapp.json` path, a `.foxxyapp` bundle, or a JSON program supplied
through stdin. Execution MUST work without `http`, `ui`, or `desktop` tags.

Headless execution MUST accept complete JSON inputs, emit machine-readable
status/events separately from the result JSON, and fail preflight instead of
opening undeclared interaction.

### FR-PORT-008 — Single-executable build

The tagged FoxxyCode builder MUST create one app-specific executable containing
the version-matched interpreter, canonical `miniapp.json`, complete reviewed
bundle, integrity manifest, and UI assets when selected.

The v1 Go implementation MUST build a version-matched runner source/template
whose payload is included using `//go:embed`. The builder MUST use an approved
compatible Go toolchain, MAY provision a checksum-locked portable toolchain
using `silent_private`, MUST NOT depend on a developer checkout, and MUST inspect
the produced executable and verify its embedded digest.
UI runner templates MUST contain prebuilt version-matched SPA assets; an
app-specific build MUST NOT require Node.js or npm.

Components requiring filesystem paths MAY be verified and extracted from the
executable into an app-specific runtime cache on first run. Non-redistributable
or host-only dependencies remain preflight requirements.

### FR-PORT-009 — Console and UI build modes

The builder MUST offer `console` and `ui` modes. Console mode MUST retain the
target console subsystem and support TTY prompts plus non-interactive JSON
input. UI mode MUST open an app-only desktop window whose form and result view
come from the embedded JSON and which does not expose the general FoxxyCode chat
UI.

The first required UI target is `windows/amd64`, using the existing WebView2
desktop shell and Go linker option `-H=windowsgui`. The WebView2 Runtime MUST be
detected as an explicit platform dependency; one application executable does
not imply that WebView2 is statically linked into it.

### FR-PORT-010 — Model binding modes

A model requirement MUST have a stable binding id and MUST select either:

- `fixed`: one exact provider identity and exact provider API model id; or
- `capability`: an author-approved capability-based selection.

An agent/model step MUST reference the binding id. A fixed binding MUST NOT
silently select a similar, newer, cheaper, or locally available model.
Alternative models MUST be represented as an explicit ordered fallback of
binding ids.

### FR-PORT-011 — Provider identity and reuse

The interpreter MUST match a bundled provider to local configuration by its
canonical effective `base_url`, not by the provider's local alias. URL
canonicalization MUST lowercase scheme and host, convert the host to ASCII,
remove a default port, normalize dot segments, and remove one trailing slash.
URL path, including `/v1`, MUST remain significant. User information, query, and
fragment components MUST be invalid.

Matching MUST be exact and MUST NOT use prefix, substring, DNS, or
`localhost`/loopback-alias equivalence. For OpenAI and Anthropic providers, the
declared protocol type MUST also match. A declared provider adapter MUST also
match. The interpreter MUST reuse only the matched local provider's credential
and proxy bindings; secret values MUST NOT be copied into the app or bundle.

### FR-PORT-012 — Exact-model resolution

After provider matching, the interpreter MUST require the exact API model id. It
MUST check matching local model configuration and then MAY call the provider's
documented model-list endpoint. It MUST verify declared capabilities before the
first model step. Missing credentials, provider, exact model, or capabilities
MUST fail preflight with a redacted diagnostic.

The interpreter MUST NOT guess by display name. Provider/model fallback MUST
occur only when the program declares an ordered fallback binding.

### FR-PORT-013 — Local-provider bootstrap

`scope: local` MUST be accepted only for a loopback URL or supported local
socket; private-LAN addresses MUST NOT acquire local-bootstrap authority.

For a local binding the interpreter MUST probe the endpoint, and MAY start an
adapter, download an exact missing model, or load it only when each operation is
explicitly declared. Model downloads MUST follow `silent_private` provisioning,
including checksum/digest or provider identity lock, storage ceiling, network
permission, private cache, redacted logging, and no privilege elevation.
A provider-exposed model digest MUST be locked. A matching provider that is
already running MAY be reused immediately when the exact model exists, but its
shared model store MUST NOT be mutated by silent-private provisioning.

The MVP MUST support:

- Ollama enumeration through `GET /api/tags`, optional pull through
  `POST /api/pull`, and calls through the declared OpenAI-compatible `/v1`
  endpoint;
- LM Studio enumeration through `GET /api/v1/models`, optional load through
  `POST /api/v1/models/load`, and download only when declared;
- a generic compatible adapter that can probe/list but cannot start, pull, or
  load without a separately declared adapter recipe.

An adapter MUST derive native management endpoints only through a versioned
built-in same-origin mapping. An author-supplied management path MUST NOT be
concatenated to the provider URL. A model pull under `silent_private` MUST use
`storage_scope: app_cache` and an app-managed provider store.

After bootstrap, the interpreter MUST re-probe the endpoint and verify the exact
model id and capabilities. If this cannot be done before the timeout, the run
MUST fail before a model request.

## 10. Permissions and secret safety

### FR-SEC-001 — Permission inference

The distiller MUST infer proposed filesystem, process, network, model,
MCP/skill, secret, and operator-interaction permissions from every step and
bundled script.

### FR-SEC-002 — Permission mismatch

The validator MUST fail when step behavior requires authority that is not
declared. Declaring a broad permission MUST NOT hide a more specific executable
or network-host requirement from release review.

### FR-SEC-003 — Permission review

The author MUST see detected permission badges and details in the editor. The
operator MUST see effective permissions before the first run and when a newer
release expands authority.

### FR-SEC-004 — Secret isolation

Secret values MUST be provided through runtime bindings. They MUST NOT be copied
from the source session into the draft, fixture, bundle, export, run history, or
display configuration.

### FR-SEC-005 — Prompt isolation

A secret MUST NOT enter an LLM/agent prompt unless the JSON program explicitly
binds that secret to the step and release review identifies the disclosure.
The default MUST be to pass secrets through process environment, stdin, or HTTP
secret headers without including them in prompts.

### FR-SEC-006 — Redaction

Redaction MUST apply to:

- source snapshots;
- phase outputs;
- generated JSON;
- scripts and assets;
- process arguments and environment previews;
- model/agent messages;
- logs and errors;
- HTTP headers and bodies;
- MCP arguments/results;
- run history and displayed results.

### FR-SEC-007 — Release sanitization

Release MUST be blocked by unresolved secrets, private keys, session ids,
transcript fragments, user-specific absolute paths, undeclared external files,
or undeclared authority.

## 11. Same-data replay

### FR-TEST-001 — Source fixture

The draft MUST include a local, non-exported test fixture that maps inferred
inputs to the same effective data used by the successful source scenario.
Secrets MUST remain references rather than fixture values.

### FR-TEST-002 — Isolated workspace

Test replay MUST execute in a newly created isolated workspace. Required input
files MUST be copied or mounted read-only according to the declared permission
plan.

### FR-TEST-003 — Side-effect policy

Network calls, writes outside the isolated workspace, external commands, and
other side effects MUST be previewed and approved before the first test. The
author MUST be able to replace a side-effecting step with a fixture or mock for
verification.

### FR-TEST-004 — Exact draft revision

A test result MUST identify the exact draft revision, effective non-secret
inputs, resolved requirements, interpreter version, step attempts, and produced
artifacts.

### FR-TEST-005 — Draft execution

A draft MAY run only through the test flow. The normal operator catalog MUST
offer **Run** only for released versions.

## 12. Verification and refinement

### FR-VERIFY-001 — Verification order

The verifier MUST run deterministic checks before any model-assisted judge:

1. required step statuses;
2. output schemas and predicates;
3. artifact presence and integrity;
4. expected file/content properties;
5. optional model-assisted semantic comparison;
6. optional author acceptance.

### FR-VERIFY-002 — Expected result contract

The draft MUST contain a human-editable description of what a successful result
looks like. It MAY include structured schemas, artifact rules, predicates, and a
judge prompt.

### FR-VERIFY-003 — Discrepancy report

Failed verification MUST produce structured discrepancies containing:

- check id;
- expected result;
- actual result;
- relevant step and artifact;
- severity;
- suggested workflow area to revise.

### FR-VERIFY-004 — Automated refine loop

The distiller MUST support a configurable replay/verify/refine loop:

- default maximum: 3 cycles;
- allowed range: 1–10 cycles;
- early exit immediately after all required checks pass;
- every cycle creates a new draft revision;
- no cycle may silently expand permissions.

### FR-VERIFY-005 — Refinement context

The refine phase SHOULD receive the current workflow, failed checks, relevant
step logs, and selected artifacts. It SHOULD NOT receive the full source session
again unless the author explicitly restarts scenario analysis.

### FR-VERIFY-006 — Manual refinement

The author MUST be able to activate **Fix discrepancies**, edit the draft
manually, or accept a non-blocking discrepancy. Blocking discrepancies cannot be
accepted for release.

### FR-VERIFY-007 — Debug artifacts

The editor MUST provide safe links or preview actions for test artifacts and
logs. Opening them MUST respect the web/desktop path and permission rules.

## 13. Draft editor

### FR-EDIT-001 — Editor sections

The editor MUST expose:

- overview;
- inputs and live form preview;
- workflow outline and step editor;
- requirements;
- permissions and secrets;
- success checks;
- outputs and display;
- bundle files;
- raw JSON;
- test runs;
- release review.

### FR-EDIT-002 — JSON validation

Invalid raw JSON MUST NOT replace the last valid canonical draft. Errors MUST
identify a JSON location and, when possible, the related visual control.

### FR-EDIT-003 — Autosave and revision

Valid draft edits SHOULD autosave after a short debounce and increment
`draft_revision`. In-progress invalid edits MAY remain local in the editor until
corrected.

### FR-EDIT-004 — Distillation progress

While distillation is running, the UI MUST show current phase, elapsed time,
cycle number, token usage when available, cancel, and recent diagnostic events.
The progress UI SHOULD remain compact until an editable result exists.

### FR-EDIT-005 — Distillation workspace regions

On a wide screen the editor SHOULD provide three functional regions:

1. ordered workflow navigation and source evidence;
2. the structured draft editor;
3. an authoring refinement assistant.

On a narrow screen the same functions MUST remain available through tabs or
drawers. Exact dimensions and visual styling are not part of the contract.

### FR-EDIT-006 — Step navigator and source evidence

The workflow navigator MUST show every step in execution order with its number,
type badge, title, validation state, and an expand/collapse control. Authoring or
system context MUST be labelled separately and MUST NOT become an implicit
runtime step.

During distillation only, the author MUST be able to inspect a read-only accepted
session-result summary, detected requirements, candidate artifacts, and a
sanitized collapsible source-session context. This evidence MUST NOT enter the
canonical JSON, export, released bundle, or operator run history.

### FR-EDIT-007 — Direct overview editing

The structured overview MUST allow direct editing of name, description, goal,
input id/label/type/required state and validation, acceptance criteria, and
permission summaries. Permission badges MUST cover at least network,
filesystem, process/Git, model, MCP, and skill authority and MUST open the
complete permission detail rather than replacing it.

### FR-EDIT-008 — Refinement assistant

The authoring assistant MUST accept natural-language requests to add, remove,
reorder, or modify steps, inputs, prompts, dependencies, permissions, success
checks, and result display. Its current authoring provider/model identity SHOULD
be visible.

Every assistant response that changes the app MUST produce a reviewable patch
against an exact `draft_revision`. The author MUST accept or reject that patch.
Acceptance MUST create a new revision and MUST NOT publish, release, expand
permissions without review, or bypass validation and test gates. The refinement
conversation and its source context MUST NOT be embedded in the app.

### FR-EDIT-009 — Draft, release, and close actions

The workspace MUST expose explicit **Save draft**, **Release**, and close
actions. Release MUST use the normal release gates. Closing with unsaved
locally-invalid edits MUST warn the author; closing an autosaved valid draft
MUST preserve it.

### FR-EDIT-010 — Direct session conversion

An idle non-empty web or desktop chat MUST expose a direct action that starts
distillation for that exact selected session, opens the mini-app workspace, and
opens the generated draft. The action MUST be absent in IDE embeds and builds
without the `miniapps` tag.

### FR-EDIT-011 — Expected-result generation

The structured editor MUST accept plain-language author expectations and expose
an explicit LLM generation action. It MUST save a reusable expected result,
acceptance criterion, fixed model binding, and executable prompt success check
in canonical JSON. Runtime verification MUST return a structured verdict and
MUST NOT expose or persist model reasoning.

### FR-EDIT-012 — Logical-model selection

The editor MUST show the configured logical model ids above every editable
draft. Selecting a model MUST resolve and store its exact fixed provider/model
binding as `primary`. Every agent step, model-assisted success check,
expected-result generation, and authoring-assistant request MUST use that
binding. The runtime MUST NOT silently substitute another logical model.

### FR-EDIT-013 — Manual input and step CRUD

The structured editor MUST provide explicit add and remove actions for
`inputs[]` and top-level `workflow[]` steps. A selected step MUST expose its
id, title, kind, and complete editable step JSON. Removing the final workflow
step MUST be prevented.

### FR-EDIT-014 — Bounded authoring tools

The authoring assistant MUST mutate an in-memory draft only through declared
mini-app tools for reading the document, changing metadata, adding/replacing or
removing inputs and steps, and replacing the editable document. Each request
MUST have finite model-round and operation limits. The complete result MUST
validate before it is atomically saved. Provider reasoning and raw tool payloads
MUST NOT be returned to the authoring UI or embedded into the mini app.

### FR-EDIT-015 — Full-surface workspace

In browser and desktop modes the mini-app workspace MUST use the full available
shell surface rather than a fixed-width sheet. The three authoring regions MAY
reflow below 1200px but MUST remain accessible without horizontal page
overflow.

## 14. Release and versioning

### FR-RELEASE-001 — Lifecycle

The content lifecycle has only `draft` and `released` states in the MVP.
`testing`, `failed`, `running`, and `interrupted` are job/run states rather than
mini-app lifecycle states.

### FR-RELEASE-002 — Release gates

A release MUST require:

- schema and reference validation;
- resolved or explicitly host-provided requirements;
- no permission mismatch;
- no blocking sanitization finding;
- a passing same-data test for the current draft revision;
- successful required checks;
- explicit human confirmation.

### FR-RELEASE-003 — Immutable release

A released version MUST be immutable. Editing it MUST create or update a draft.

### FR-RELEASE-004 — Version increase

The first release SHOULD default to `1.0.0`. A later release MUST have a higher
semantic version. The editor SHOULD propose patch/minor/major based on contract
compatibility.

### FR-RELEASE-005 — Release review

Release review MUST show:

- version and compatibility diff;
- input/output contract diff;
- workflow change summary;
- permission diff;
- dependencies and install scripts;
- bundled skills/MCP components;
- sanitization report;
- latest test and verification results;
- file integrity manifest.

## 15. Operator runner

### FR-RUN-001 — Released version selection

Every normal run MUST target an exact released version. The catalog MAY default
to the latest release but MUST record the resolved version.

### FR-RUN-002 — Preflight

Before execution, the runner MUST validate inputs, requirements, integrity,
permissions, secret bindings, and required interaction policy.

### FR-RUN-003 — Live execution

The runner MUST display ordered sanitized step progress, duration,
retry/fallback attempts, produced artifacts, and waiting operator interactions.
Execution logs MUST be written to the selected app run directory and MUST NOT be
streamed inline as agent transcript.

### FR-RUN-004 — Interaction

Only declared operator-input and confirmation steps may pause a run. A headless
run MUST fail preflight when a required interaction has no supplied answer
policy.

### FR-RUN-005 — Cancellation

Cancellation MUST propagate to model calls, agent cycles, child processes, HTTP,
MCP, skills, file operations where possible, and nested mini apps.

### FR-RUN-006 — Result view

After execution, the runner MUST evaluate success and render the configured text,
Markdown, JSON, table, file, directory, archive, and generated-media outputs.

### FR-RUN-007 — Run again

The operator MUST be able to repeat a run with previous non-secret inputs loaded
into an editable form. Secret fields MUST be empty or rebound.

### FR-RUN-008 — Hidden agent internals

Console, UI, HTTP, and SSE operator streams MUST hide raw model reasoning,
chain-of-thought, assistant scratch messages, raw tool calls, and raw tool
arguments/results. They MAY expose only declared questions and confirmations,
sanitized lifecycle/step status, explicit typed agent-step results, final
results, and artifact metadata.

Raw agent reasoning MUST NOT be persisted. Diagnostic tool events MAY persist
only according to the reviewed policy `none`, `metadata`, or `sanitized`, with
redaction performed before writing.

### FR-RUN-009 — Local or global run root

Every app or executable build profile MUST select `local` or `global` log scope:

- global:
  `$FOXXYCODE_HOME/apps/<app-slug>--<short-id>/runs/<run-id>/`;
- local:
  `<run-workspace>/.foxxycode/apps/<app-slug>--<short-id>/runs/<run-id>/`.

A local run MUST have an explicit workspace. A UI executable without one MUST
ask for it or apply its reviewed fallback to global. Each run directory MUST
contain `run.json`, `events.jsonl`, `execution.log`, `artifacts/`, and a private
`runtime/` extraction/cache directory where required.

### FR-RUN-010 — Safe execution metrics

When available, the runner result MUST display total duration, cumulative
model/API duration, input/output token counts, and resolved non-secret
provider/model binding ids. Missing provider metrics MUST be shown as
unavailable, not estimated from unrelated source-session values.

## 16. Run history and diagnostics

### FR-HISTORY-001 — Persisted history

Each run MUST persist mini-app id/version, timestamps, duration, non-secret
inputs, resolved requirements, step attempts, approvals, success checks,
outputs, artifact metadata, and terminal status.

### FR-HISTORY-002 — Statuses

Run status MUST include `queued`, `preflight`, `running`, `waiting`,
`succeeded`, `failed`, `cancelled`, and `interrupted`.

### FR-HISTORY-003 — Logs

Logs MUST be structured and bounded by size and retention policy. The runtime
MUST preserve enough information to diagnose the failed step without storing
secrets or unrestricted model context.

### FR-HISTORY-004 — Interrupted run

After a process restart, an unfinished run MUST become `interrupted`. Resume MAY
be offered only when prior outputs are durable and every remaining step supports
safe continuation.

### FR-HISTORY-005 — Diagnostic boundaries

Run files MUST NOT contain chain-of-thought, raw reasoning, secrets, or
unredacted tool I/O. Tool diagnostics MUST be bounded, attributed to a workflow
step, and include only the permitted metadata or sanitized/truncated payload.

### FR-HISTORY-006 — Usage metrics

Run history SHOULD persist per-run and, when available, per-step total duration,
model/API duration, input/output tokens, and resolved provider/model binding ids.
These metrics MUST remain non-secret, distinguish measured from unavailable
values, and MUST NOT expose prompts, responses, credentials, or raw model
internals.

## 17. Catalog and reuse paths

### FR-CATALOG-001 — Search and filters

The catalog MUST support text search and filters for lifecycle state, archived
visibility, author, tags, and compatibility with the current platform.

### FR-CATALOG-002 — Duration

The catalog SHOULD display last and median successful duration when enough run
history exists.

### FR-CATALOG-003 — Three user-controlled paths

The UI MUST let the user:

1. run a suitable release;
2. create a new version from an existing mini app;
3. ignore mini apps and continue with an ordinary agent session.

The MVP MUST NOT claim to automatically estimate or select among the three
paths.

### FR-CATALOG-004 — Version modification

Modification of a released mini app MUST create a draft associated with a future
higher release. Temporary mutation of a released version is forbidden.

### FR-CATALOG-005 — Compact cards and safe Run action

The quick drawer MUST provide name/tag search and an explicit **Show archived**
control. A compact released-app card MUST show name, short description, exact
release version, declared input count, compatibility/availability, and measured
duration when known. It SHOULD show an icon or generated media-type mark.

Each card MUST provide a primary **Run** action and an overflow menu for
edit/new-version, history, export, and archive/restore. **Run** MUST open the
generated form for the resolved exact version and MUST NOT begin side effects
before input validation and preflight.

## 18. HTTP and standalone interfaces

### FR-API-001 — Distillation API

The HTTP surface MUST support starting, reading, streaming, confirming the
scenario for, retrying, and cancelling a distillation job.

### FR-API-002 — Catalog API

The HTTP surface MUST support list/search, draft CRUD, bundle-file CRUD,
validation, sanitization, release, import, export, archive, and restore.

### FR-API-003 — Run API

The HTTP surface MUST support starting test/released runs, reading and streaming
run state, answering input/confirmation steps, cancellation, and artifact
download.

### FR-API-004 — Standalone CLI

With `miniapps`, FoxxyCode MUST expose `miniapps validate`, `miniapps inspect`,
`miniapps requirements`, `miniapps run`, and `miniapps build`. `run` MUST accept
a JSON file, bundle, or stdin program plus JSON inputs. `build` MUST accept
`--mode console|ui`, target, output, and local/global log-scope options.

Headless execution MUST emit machine-readable events and result JSON on
separate output channels. An untagged binary MUST treat the command as absent,
not as a disabled runtime feature.

### FR-API-005 — API documentation

Implemented HTTP behavior MUST be represented in the served OpenAPI document and
the repository HTTP API documentation.

### FR-API-006 — UI capability

The HTTP-enabled build MUST expose a backend capability response that
unambiguously states whether `miniapps` was compiled in. The SPA MUST use that
value to register or hide mini-app navigation and actions.

## 19. Clean-room adaptation decisions

The following product decisions intentionally adapt observed behavior from the
research reference to FoxxyCode requirements:

| Observed concept | FoxxyCode requirement |
|------------------|-----------------------|
| Multi-stage distillation | Adopt as bounded structured phases. |
| Replay/verify/refine loop | Adopt with 1–10 cycles, early exit, and immutable test reports. |
| Agent-assisted replay | Replace implicit agent behavior with explicit JSON agent steps. |
| Workflow saved as local app/skill | Replace with a portable JSON bundle and standalone interpreter. |
| Project and global copies | Use an explicitly selected local or global app/run root; never duplicate or merge scopes implicitly. |
| Draft/testing/published/archived workflow states | Keep only draft/released content states; testing is a run state and archive is catalog visibility. |
| Full session in distillation prompts | Restrict full context to early sanitized phases. |
| Runtime/global secret maps | Replace with declared runtime secret bindings and end-to-end redaction. |
| Session-side workflow panel | Provide a quick drawer backed by the same data as the full FoxxyCode Mini Apps catalog. |
| Three-region distillation editor | Separate step/source navigation, structured editing, and authoring refinement; use responsive tabs/drawers when needed. |
| Visible source transcript and session result | Keep as temporary sanitized authoring evidence; never make it portable app state. |
| Chat-based refinement | Produce reviewable revision-bound patches; never mutate a release or bypass gates. |
| Result duration/API/token chips | Show measured authoring/run metrics while keeping source metrics out of released provenance. |
| Python and LLM replay | Generalize to typed script, JSON VM, command, agent, API, MCP/skill, file, and operator steps. |

## 20. MVP scenario acceptance

### AC-MINIAPP-001 — Successful distillation

Given a completed web/desktop session with an accepted output, when the author
creates a mini app, then FoxxyCode confirms one scenario, runs the phased
distillation, and opens a valid editable draft.

### AC-MINIAPP-002 — Not-distillable session

Given a session without an identifiable result, when creation is requested, then
FoxxyCode stops before synthesis and explains what result contract is missing.

### AC-MINIAPP-003 — Same-data verification

Given a valid draft and source fixture, when test is activated, then FoxxyCode
runs in isolation, performs deterministic checks followed by any declared judge,
and records the exact draft revision and discrepancies.

### AC-MINIAPP-004 — Automatic refinement

Given a failed verification and remaining cycles, when automatic refinement is
enabled, then FoxxyCode creates a revised draft and repeats replay until checks
pass or the cycle limit is reached.

### AC-MINIAPP-005 — Secret protection

Given source-session and operator secrets, when distillation, testing, export,
and normal execution complete, then no secret value appears in the bundle,
fixture, prompt without explicit binding, log, history, or display.

### AC-MINIAPP-006 — Release gate

Given a draft whose current revision has no passing test, when release is
attempted, then release is refused with the missing gate identified.

### AC-MINIAPP-007 — Portability

Given a released exported bundle, when it is imported on a compatible host and
run with equivalent inputs and bindings, then the embedded and standalone
interpreters produce equivalent step/output semantics.

### AC-MINIAPP-008 — IDE absence

Given an IDE-embedded FoxxyCode UI, when a session is opened, then no mini-app
creation, catalog, editor, or runner control is visible.

### AC-MINIAPP-009 — Build-tag gating

Given otherwise equivalent binaries built with and without `miniapps`, when
their CLI, HTTP routes, and web/desktop UI are inspected, then only the tagged
binary exposes interpreter/builder commands, routes, capability, creation
button, and Mini Apps section.

### AC-MINIAPP-010 — Headless JSON interpretation

Given a valid JSON program and complete JSON inputs, when a tagged FoxxyCode
binary runs it without HTTP/UI, then the workflow completes, emits only
machine-readable safe operator events and result JSON, and persists the run in
the selected app root.

### AC-MINIAPP-011 — Console and UI executables

Given one released bundle, when the builder produces `console` and `ui`
executables, then each contains the same canonical program and bundle; the
console build supports TTY/headless input and the Windows UI build opens a
desktop form generated from the JSON.

### AC-MINIAPP-012 — Hidden internals and scoped logs

Given an app containing agent and tool steps, when it runs in console and UI
modes, then neither surface displays reasoning or raw tool calls, no run file
contains chain-of-thought or unredacted tool I/O, and redacted diagnostics are
written only below the selected local or global
`.foxxycode/apps/<app>/runs/<run-id>` root.

### AC-MINIAPP-013 — Universal JSON program

Given a `foxxy-vm/1` step containing functions and a bounded loop, when it is
run by embedded and built interpreters, then both produce the same typed result;
an unknown opcode, undeclared host import, or exhausted resource limit fails in
the declared step without gaining host authority.

### AC-MINIAPP-014 — Silent private provisioning

Given a released app with a missing locked portable dependency, when its exact
reviewed release runs, then the interpreter verifies and installs the dependency
transactionally inside the app cache without an installation prompt. It does not
elevate privileges or mutate system state, and a failed install leaves no active
partial version.

### AC-MINIAPP-015 — Fixed provider and local model

Given a fixed binding whose normalized base URL, protocol type, and exact model
id match local FoxxyCode configuration, when the app starts, then the
interpreter reuses the local credential/proxy binding and calls that exact model.
Given a declared loopback provider with a missing model, it probes, optionally
starts, pulls/loads, and verifies that exact model under `silent_private`, or
fails preflight without substituting another model.

### AC-MINIAPP-016 — Distillation workspace

Given an editable distilled draft, when the author opens it, then the workspace
shows the ordered typed steps, accepted session result, sanitized source
context, detected requirements/artifacts, editable goal/inputs/acceptance
criteria, permission summaries, and refinement assistant. Exporting the draft
or release contains none of the source transcript, source metrics, or refinement
conversation.

### AC-MINIAPP-017 — Revision-bound refinement

Given a request to change a step, input, or prompt, when the refinement assistant
responds, then it presents a patch against the current exact revision. Rejecting
it changes nothing; accepting it creates a new draft revision and still requires
normal validation, testing, sanitization, and human release.

### AC-MINIAPP-018 — Session quick catalog

Given an open web/desktop session, when the user opens the Mini Apps drawer,
then they can search by name/tag, include or exclude archived apps, inspect exact
version/input count/description/availability, and open overflow actions. Pressing
**Run** opens the generated form and causes no workflow side effect before
preflight.

### AC-MINIAPP-019 — Measured usage metrics

Given provider metric data, when source evidence, a test, or a released run is
viewed, then total duration, model/API duration, and input/output tokens are
labelled as measured. Missing values are unavailable rather than guessed, and
source-session metrics are absent from the released portable program.

## 21. Deferred functional work

- autonomous opt-in session harvesting;
- organizational approval and shared catalogs;
- semantic/vector search;
- automatic three-path time estimation and routing;
- parallel workflow branches;
- signed bundles and publisher trust;
- remote execution workers;
- scheduled mini-app runs;
- collaborative draft editing.
