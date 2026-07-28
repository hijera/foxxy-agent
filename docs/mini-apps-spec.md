# FoxxyCode Mini Apps Specification

Status: product and architecture proposal for the first implementation.

Russian version: [mini-apps-spec.ru.md](mini-apps-spec.ru.md)

Testable product requirements:
[mini-apps-functional-requirements.md](mini-apps-functional-requirements.md).

## 1. Purpose

A FoxxyCode mini app is a portable, operator-facing program distilled from a
successful FoxxyCode session. It turns a task that was completed interactively
with an agent into a repeatable workflow that can later run without the original
session and without a general-purpose agent deciding every step again.

The primary experience is:

1. An employee completes a task in a normal session.
2. The employee activates **Create mini app** for that session.
3. FoxxyCode analyzes the session, extracts one concrete successful scenario,
   finds values that should become inputs, and creates an editable draft.
4. The generated input form, workflow, dependencies, permissions, success
   checks, and output presentation are reviewed in the mini-app editor.
5. FoxxyCode tests the draft with the same effective inputs as the source
   session and iteratively improves it until its declared success criteria pass.
6. A human reviews the sanitized bundle and releases it.
7. An operator can run the released mini app through FoxxyCode or through a
   compatible standalone interpreter.

Mini apps are intended to reduce repeated agent work, token use, execution time,
and the amount of prompt or programming knowledge required for routine tasks.

## 2. Product scope

### 2.1 MVP

The first implementation includes:

- an explicit **Create mini app** action on a completed session;
- a **Mini Apps** section in the web and desktop navigation;
- session distillation into an editable draft;
- a structured distillation workspace with an ordered step navigator, temporary
  source evidence, and an authoring-only refinement assistant;
- automatically inferred operator inputs and generated form controls;
- a versioned JSON workflow language with a general-purpose, stack-based
  `foxxy-vm/1` program step;
- sequential execution, conditions, retry, fallback, and composition through
  another mini app;
- deterministic scripts, external commands, agent cycles, HTTP API calls, MCP
  and bundled skill calls, file operations, operator input, and explicit
  operator confirmation;
- inline scripts and bundled files;
- declared host dependencies and silent private provisioning of locked portable
  runtimes, packages, and local models;
- capability-based or fixed provider/model bindings, including local-provider
  discovery and declared model bootstrap;
- a test run using the same effective source-session data;
- configurable deterministic and model-assisted success checks;
- draft sanitization and a human release review;
- `draft` and `released` lifecycle states;
- import and export of portable bundles;
- a compact searchable Mini Apps drawer available from an open web/desktop
  session in addition to the full catalog section;
- a FoxxyCode interpreter mode that runs `miniapp.json` or a bundle without the
  web UI;
- a builder that produces one application executable containing the
  interpreter, JSON program, UI assets when selected, and the complete bundle;
- operator-selected console or desktop-UI executable mode;
- run history, inputs, outputs, logs, duration, cancellation, and repeat;
- source/test/run duration and model-usage metrics when the provider exposes
  them;
- manual user choice between reusing a mini app, creating a new released
  version, or continuing with an ordinary agent session.

### 2.2 Explicitly excluded from the MVP

- automatic background distillation of all sessions;
- use inside IntelliJ, VS Code, ACP, or other IDE-integrated surfaces;
- automatic three-way routing or automatic comparison of “build from scratch”,
  “adapt an existing mini app”, and “run an ordinary agent”;
- automatic organizational harvesting without an explicit opt-in;
- parallel workflow branches;
- silent system-wide dependency installation, privilege elevation, or
  permission escalation.

The schema reserves compatibility points for future routing and discovery, but
the MVP does not automatically choose a path for the user.

## 3. Supported product surfaces

Mini-app creation, editing, release, catalog browsing, and interactive execution
are available only in:

- the bundled web UI served by `foxxycode http`; and
- the FoxxyCode desktop application.

IDE integrations must not show the **Create mini app** action or the **Mini
Apps** navigation section. The HTTP API and standalone interpreter may still be
used by non-IDE automation clients.

Mini-app support is a compile-time optional module selected by the Go build tag
`miniapps`:

- with `miniapps`, FoxxyCode includes the interpreter, executable builder,
  mini-app HTTP routes, and a `miniapps` runtime capability;
- without `miniapps`, those CLI commands and HTTP routes are not registered and
  the capability is false;
- the web/desktop UI renders the session action, catalog route, editor, and
  runner only when the backend advertises that capability;
- IDE surfaces keep the feature hidden even if their bundled backend was built
  with the tag.

The visibility decision must come from the backend capability, not from a
hard-coded frontend build assumption, so an embedded SPA cannot expose controls
that its FoxxyCode binary cannot serve.

## 4. Terms

| Term | Meaning |
|------|---------|
| Source session | The session analyzed transiently to produce a draft. |
| Distillation | Converting a session into a parameterized workflow and testing it. |
| Mini app | The logical application, identified by a stable `id`. |
| Draft | The mutable working copy of a mini app. |
| Release | An immutable, executable version of a mini app. |
| Bundle | The JSON program plus scripts, skills, MCP assets, and other files needed to run it. |
| Interpreter | The engine that validates and executes a bundle. |
| Operator | A person who fills inputs, approves checkpoints, and consumes results. |
| Author | The user responsible for a mini app; inferred by default and editable. |
| Run | One execution of a draft test or a released version. |

## 5. Core principles

1. **Session-derived, session-independent.** A session is input to distillation,
   not a runtime dependency.
2. **Portable program contract.** Behavior is defined by a versioned JSON file,
   not by UI state or an implicit FoxxyCode conversation.
3. **Generated but editable.** FoxxyCode proposes fields, steps, prompts,
   success criteria, and display settings; a human may edit all of them.
4. **Explicit nondeterminism.** Every LLM or agent use is a declared workflow
   step. The rest of the workflow remains deterministic.
5. **Explicit operator interaction.** Mid-run questions and approvals exist only
   as declared steps.
6. **Least authority.** Required filesystem, process, network, secret, MCP, and
   model capabilities are declared before execution.
7. **No hidden provenance in a release.** Released artifacts do not store the
   source session id, transcript, tool-call history, or source model.
8. **Human-controlled release.** Sanitization and release always require a human
   review in the MVP.
9. **Shared execution semantics.** FoxxyCode and the standalone executable use
   the same validation and interpreter core.
10. **Operator-safe execution surface.** Agent reasoning and raw tool traffic
    are implementation details. Operators see declared questions, approvals,
    sanitized step status, results, and artifacts, while diagnostics are written
    to the selected app run directory.

## 6. High-level architecture

The feature consists of six logical components.

### 6.1 Distiller

The distiller is an asynchronous FoxxyCode process that:

- reads an explicitly selected session;
- identifies the task, successful path, retries, failed experiments, effective
  inputs, produced outputs, and implicit environmental assumptions;
- converts stable values to constants and variable values to operator inputs;
- converts observed actions to typed workflow steps;
- copies or generates required scripts and portable assets;
- copies required skills and portable MCP components into the draft bundle;
- proposes dependencies, permissions, success criteria, and result renderers;
- classifies every source artifact as an operator input, bundled asset, test
  fixture, expected output example, or discarded evidence;
- records source duration, model/API time, and input/output token counts as
  private authoring evidence when available;
- sanitizes secrets and source-specific data;
- runs and repairs the draft against the source-session fixture.

The source session may be referenced by a private distillation job while the job
is active. That reference must be removed when the job finishes or is deleted
and must never be written into a released bundle.

The accepted session result, candidate artifacts, detected requirements, and a
sanitized source transcript may be shown read-only in the distillation
workspace. They are evidence for the author and refinement process, not workflow
steps or portable program state.

### 6.2 Catalog and storage

The catalog indexes local drafts and releases under the FoxxyCode configuration
directory. It provides listing, text search, tags, status, versions, and basic
measured run duration. Semantic/vector discovery may be added later.

The same catalog source powers both the full **Mini Apps** section and a compact
drawer that can be opened without leaving a session. The drawer is a quick
run/discovery surface, not a separate copy of catalog state.

### 6.3 Editor

The editor exposes:

- application metadata;
- an ordered, collapsible workflow navigator with step number, type badge,
  title, and validation state;
- a read-only source-result/evidence area available only during distillation;
- generated input form preview;
- visual workflow outline;
- per-step settings;
- raw JSON editing;
- bundled file management;
- dependency and permission review;
- success criteria;
- output presentation;
- test runs and logs;
- sanitization findings;
- release controls.

Visual edits and raw JSON edits operate on the same canonical draft.

On wide screens the distillation workspace may use three functional regions:
workflow/source evidence, the structured editor, and a refinement assistant. On
narrow screens the same regions become tabs or drawers. This is a behavioral
layout contract rather than a requirement to copy any reference application's
dimensions or visual styling.

The refinement assistant can propose adding, removing, reordering, or changing
steps, inputs, prompts, dependencies, permissions, success checks, and result
display. Each response must produce a reviewable patch against an exact
`draft_revision`; the author accepts or rejects it before it becomes canonical.
The refinement conversation, its model label, and source context are
authoring-only data and never enter `miniapp.json` or an exported bundle.

### 6.4 Runtime core

The runtime core validates the schema, resolves references, evaluates restricted
conditions, executes steps, records events, enforces permissions, validates
outputs, and builds the declared result view.

It must be implemented below the web/desktop integration layer so the same core
can be used by:

- the FoxxyCode web/desktop process; and
- a standalone command-line executable.

### 6.5 Interpreter and executable builder

When built with `-tags miniapps`, the main `foxxycode` executable is also the
canonical interpreter and builder:

```text
foxxycode miniapps validate <miniapp.json|bundle>
foxxycode miniapps inspect <miniapp.json|bundle>
foxxycode miniapps requirements <miniapp.json|bundle>
foxxycode miniapps run <miniapp.json|bundle> [--input inputs.json]
foxxycode miniapps run --program - --input inputs.json
foxxycode miniapps build <miniapp.json|bundle> \
  --mode console|ui \
  --target windows/amd64 \
  --output <path> \
  --log-scope local|global
```

`--program -` reads the JSON program from standard input. A non-interactive run
requires all mandatory inputs through `--input` or declared bindings and writes
machine-readable progress to stderr and the result JSON to stdout. An
interactive console run renders the declared fields as terminal prompts. It
does not require the HTTP server, embedded SPA, or desktop build tags.

The builder creates an app-specific executable:

- `console` keeps the target operating system's console subsystem. It collects
  missing declared inputs from a TTY when available and otherwise behaves
  headlessly;
- `ui` starts a loopback-only mini-app HTTP endpoint and opens the existing
  FoxxyCode desktop window shell directly on an app-run route. The page is
  generated from the JSON input and display declarations and does not expose
  the normal chat/session interface;
- both modes execute the same embedded JSON and bundle through the same runtime
  core;
- both modes hide model reasoning, assistant scratch messages, and raw tool
  calls. Only operator-visible events are printed or rendered.

The first UI executable target is `windows/amd64`, matching the existing
WebView2 desktop shell. Console targets may be added for any supported
`GOOS/GOARCH` combination whose selected steps and dependencies are portable.

#### 6.5.1 Go packaging strategy

The v1 builder uses compile-time embedding instead of modifying an already
linked executable:

1. validate, sanitize, canonicalize, and integrity-lock the input bundle;
2. create an isolated temporary build workspace containing the
   version-matched mini-app runner source and vendored build dependencies;
3. place the canonical `.foxxyapp` payload under that workspace and include it
   in the runner with Go `//go:embed`;
4. invoke a controlled `go build` with `-tags miniapps` and the tags required by
   the selected mode;
5. for Windows `ui`, also use `-tags "miniapps http ui desktop"` and linker
   option `-H=windowsgui`; for `console`, omit `windowsgui`;
6. run `inspect` against the produced executable and verify the embedded
   manifest digest before reporting success.

This design follows the standard Go
[`embed`](https://pkg.go.dev/embed) and
[build-constraint](https://pkg.go.dev/cmd/go#hdr-Build_constraints)
mechanisms. The Windows console/GUI distinction is provided by the Go linker
[`-H windowsgui`](https://pkg.go.dev/cmd/link).

The builder may use an already installed compatible Go toolchain or silently
install a checksum-locked portable Go toolchain in the FoxxyCode runtime cache
under the reviewed `silent_private` policy. It must not depend on a developer
repository checkout.
The runner source/template and all required module sources must match the
FoxxyCode version that performs the build. The UI template contains prebuilt,
version-matched SPA assets, so building a particular mini app does not require
Node.js or npm.

The output is one distributable application file containing the interpreter,
`miniapp.json`, bundle files, skills, portable MCP assets, and optional UI
assets. Components that must exist as real files are verified and extracted at
first run into an app-specific runtime cache. Host dependencies and components
that cannot legally be redistributed remain explicit preflight requirements.

For Windows UI builds, “one executable” does not imply that the WebView2 Runtime
is compiled into the file. The app uses the Evergreen WebView2 Runtime when
available, detects its absence before opening the window, and follows
Microsoft's documented
[WebView2 distribution](https://learn.microsoft.com/en-us/microsoft-edge/webview2/concepts/distribution)
rules. A Fixed Version runtime may be an explicitly licensed bundled
dependency, but it is extracted and managed separately.

### 6.6 Run store

Every test and normal run receives a run id and persists:

- mini-app id and released version, or draft revision;
- start and finish timestamps;
- effective non-secret inputs;
- step events and logs;
- approvals and operator answers;
- produced result values and artifact paths;
- success-check results;
- duration and terminal status.

Secret values must never be written to the run history.

The run root is selected per app or build profile:

```text
# global
$FOXXYCODE_HOME/apps/<app-slug>--<short-id>/runs/<run-id>/

# local
<run-workspace>/.foxxycode/apps/<app-slug>--<short-id>/runs/<run-id>/
```

`$FOXXYCODE_HOME` is normally `~/.foxxycode`. A stable id suffix prevents two
apps with the same display name from sharing a directory. `local` requires an
explicit run workspace; a UI executable launched without one must ask for it or
fall back to `global` according to its reviewed build profile.

Each run directory contains at least:

```text
run.json
events.jsonl
execution.log
artifacts/
runtime/
```

The console and UI operator streams contain only lifecycle, sanitized progress,
declared interaction, validation, result, and artifact events. Raw
chain-of-thought/reasoning is neither displayed nor persisted. Tool invocations
are absent from the operator stream; `events.jsonl` may record tool name,
step id, start/finish, duration, outcome, and explicitly permitted
redacted/truncated arguments or results. Secret-bearing values are removed
before any sink, including diagnostic logs.

## 7. Storage and bundle layout

The global catalog and run root is:

```text
$FOXXYCODE_HOME/apps/
```

where `$FOXXYCODE_HOME` is the same configuration root used by FoxxyCode
(normally `~/.foxxycode`).

An app may instead be local to an explicitly selected workspace under
`<workspace>/.foxxycode/apps/`. The layout within either root is:

```text
apps/
  <app-slug>--<short-id>/
    app.json
    draft/
      miniapp.json
      files/
      skills/
      mcp/
      fixtures/
      sanitization.json
    releases/
      1.0.0/
        miniapp.json
        files/
        skills/
        mcp/
        manifest.lock.json
    runs/
      <run-id>/
        run.json
        events.jsonl
        execution.log
        artifacts/
        runtime/
```

`app.json` contains catalog-level mutable metadata and the current draft/release
pointers. `miniapp.json` is the portable program.

An exported bundle uses a single archive, with `.foxxyapp` as the recommended
extension. The archive is a ZIP-compatible container whose root contains
`miniapp.json`. Imported archives must be unpacked only after traversal, size,
file-count, symlink, and duplicate-path checks pass.

## 8. Lifecycle and versioning

### 8.1 Draft

- A new distillation creates a mutable draft.
- Editing a draft does not change the released version.
- Every successful draft save increments an internal `draft_revision`.
- Test runs identify the exact draft revision they executed.
- A draft may coexist with the latest release.

### 8.2 Released

- Releasing creates an immutable semantic version.
- The first release defaults to `1.0.0`.
- Editing a released mini app creates or updates its draft, never mutates the
  released files.
- Releasing that draft requires a version increase.
- The editor proposes:
  - patch for prompts, display settings, validations, and compatible fixes;
  - minor for backward-compatible input, output, or workflow additions;
  - major for removed/renamed inputs, changed output contracts, or incompatible
    execution semantics.
- The author may override the proposal, but duplicate or decreasing versions
  are rejected.

Only `draft` and `released` are lifecycle states in the MVP.

## 9. Distillation workflow

### 9.1 Entry conditions

**Create mini app** is enabled when:

- the session exists and is readable;
- no turn is currently running;
- the session contains at least one completed user task and assistant outcome;
- the current surface is web or desktop.

The user is shown that session data will be analyzed and that a test may repeat
actions. The test may not perform external side effects until the permissions
review has been accepted.

### 9.2 Scenario selection

The distiller analyzes the entire session but produces one concrete scenario per
draft. It must:

1. identify the likely task and final accepted outcome;
2. exclude exploratory failures unless they are required fallback knowledge;
3. present a short scenario summary for user confirmation;
4. allow the user to correct the task or desired outcome before generation.

If two independent tasks exist, the distiller asks the user to select one rather
than silently combining them.

### 9.3 Input inference

The distiller classifies session values as:

- fixed workflow constants;
- operator inputs;
- secrets supplied through runtime bindings;
- values produced by prior steps;
- environmental requirements;
- accidental source-session data that must be removed.

The generated draft stores the effective source-session values only in the local
test fixture. Fixture values containing detected secrets are replaced with
secret references.

### 9.4 Workflow synthesis

Observed tool calls and model actions are converted to the smallest explicit
workflow that reproduces the successful path. Repeated agent reasoning should be
replaced with deterministic steps when the session provides enough evidence.
An agent step remains only where dynamic planning is required.

### 9.5 Test and repair

The initial test uses the same effective inputs and isolated copies of required
source files. The distiller may perform multiple repair iterations:

1. validate the bundle and requirements;
2. create an isolated test workspace;
3. execute the workflow;
4. evaluate success criteria;
5. compare declared outputs with the expected source-session outcome;
6. revise the draft;
7. repeat until successful or until a configured limit is reached.

A distillation that reaches its limit remains a draft and reports the failing
steps and criteria. It must not be released automatically.

### 9.6 Sanitization and review

Before release, FoxxyCode scans the complete JSON and every bundled file for:

- API keys, access tokens, cookies, passwords, and private keys;
- absolute user-specific paths;
- source session ids and transcript fragments;
- personal or sensitive data;
- environment-specific endpoints and account ids;
- references to files that were not bundled;
- undeclared processes, network hosts, models, skills, or MCP servers.

Findings are displayed in a release review. Blocking findings must be resolved;
non-blocking findings require explicit acknowledgement.

## 10. JSON program contract

### 10.1 General rules

- The file is UTF-8 JSON.
- `schema_version` uses semantic versioning.
- Unknown required major versions are rejected.
- Unknown fields are rejected by default in security-sensitive objects.
- Every workflow step has a unique `id`.
- Step execution is sequential unless a `branch` chooses a nested path.
- Runtime values are JSON-compatible values plus typed file and directory
  handles.
- The interpreter must validate the whole program before executing the first
  side-effecting step.
- The JSON is canonical for execution; the visual editor is not a second source
  of behavior.
- General-purpose computation is expressed only by the versioned `program` step.
  Host effects from that step are possible only through declared typed imports.

### 10.2 Top-level shape

```json
{
  "schema_version": "1.0.0",
  "kind": "foxxycode.miniapp",
  "id": "weekly-repository-report",
  "version": "1.0.0",
  "state": "released",
  "metadata": {},
  "requirements": {},
  "permissions": {},
  "inputs": [],
  "workflow": [],
  "success": {},
  "outputs": [],
  "display": {},
  "runtime": {
    "log_scope": "global",
    "operator_event_level": "status",
    "diagnostic_tool_events": "sanitized",
    "persist_agent_reasoning": false
  }
}
```

`version` is omitted from an unreleased portable draft export or replaced by a
non-release marker such as `0.0.0-draft.<revision>`.

### 10.3 Metadata

```json
{
  "metadata": {
    "name": "Weekly repository report",
    "description": "Builds and summarizes a repository activity report.",
    "goal": "Produce a verified weekly report from operator-selected inputs.",
    "author": "Detected User",
    "tags": ["reporting", "git"],
    "estimated_duration_seconds": 90
  }
}
```

The author is inferred from the local FoxxyCode/user environment when possible
and remains editable. Source-session provenance is intentionally absent.
`description` explains the application to a catalog user; `goal` states the
result contract the distilled workflow is intended to achieve.

### 10.4 Value references

Typed references use an object and preserve the referenced JSON type:

```json
[
  { "$ref": "inputs.repository" },
  { "$ref": "steps.collect.outputs.items" },
  { "$ref": "secrets.github_token" }
]
```

String templates use:

```text
Report for {{ inputs.period }} contains {{ steps.collect.outputs.count }} items.
```

Reference roots in v1 are:

- `inputs.<id>`;
- `steps.<step-id>.outputs.<name>`;
- `secrets.<id>`;
- `run.id`, `run.workspace`, and `run.output_dir`;
- `app.id` and `app.version`.

References to undeclared or not-yet-produced values are validation errors.
Secrets may be passed to approved execution fields but may not be interpolated
into logs, display templates, or persisted outputs.

### 10.5 Restricted conditions

Conditions are data, not JavaScript or shell expressions:

```json
{
  "op": "and",
  "args": [
    {
      "op": "gt",
      "left": { "$ref": "steps.collect.outputs.count" },
      "right": 0
    },
    {
      "op": "eq",
      "left": { "$ref": "inputs.include_details" },
      "right": true
    }
  ]
}
```

Required v1 operators are `eq`, `ne`, `gt`, `gte`, `lt`, `lte`, `and`, `or`,
`not`, `exists`, `empty`, `contains`, and `matches`. `matches` uses a
runtime-limited regular expression implementation.

### 10.6 Runtime visibility and log policy

`runtime.log_scope` is `local` or `global` and selects the run-root rule from
section 6.6. `operator_event_level` is fixed to `status` in schema v1.
`diagnostic_tool_events` supports `none`, `metadata`, and `sanitized`.
`miniapps build --log-scope` overrides the portable default for that generated
executable and records the choice in its embedded build manifest.

`persist_agent_reasoning` must be `false` in v1. Raw model reasoning,
chain-of-thought, assistant scratch content, unredacted tool arguments, and
unredacted tool results are forbidden in console output, UI events, HTTP/SSE
operator streams, run history, and log files. This does not suppress declared
agent-step results: the step must produce an explicit typed output and a safe
operator summary.

## 11. Generated inputs and form controls

Each input declares data semantics separately from presentation:

```json
{
  "id": "format",
  "type": "string",
  "title": "Report format",
  "description": "Select the generated report format.",
  "required": true,
  "default": "markdown",
  "validation": {
    "enum": ["markdown", "json", "table"]
  },
  "ui": {
    "control": "radio",
    "order": 30
  },
  "visible_when": {
    "op": "eq",
    "left": { "$ref": "inputs.generate_report" },
    "right": true
  }
}
```

### 11.1 Input types

The MVP supports:

- `string`;
- `text`;
- `integer`;
- `number`;
- `boolean`;
- `date`;
- `datetime`;
- `enum`;
- `file`;
- `files`;
- `directory`;
- `secret`.

### 11.2 UI controls

The MVP supports:

- text input;
- multiline textarea;
- numeric input;
- checkbox;
- select;
- radio group;
- date/datetime picker;
- file picker;
- multi-file picker;
- directory picker;
- secret/password input.

The distiller chooses the initial control. The author may change the control when
it remains compatible with the input type.

### 11.3 Validation and dependencies

Supported validation includes:

- required;
- minimum and maximum;
- minimum and maximum length;
- enum;
- regex pattern;
- accepted file extensions and media types;
- maximum file count and total size;
- filesystem existence and file/directory kind;
- `visible_when`, `enabled_when`, and `required_when`.

The input dependency graph must be acyclic.

## 12. Workflow semantics

### 12.1 Common step fields

```json
{
  "id": "step-id",
  "kind": "script",
  "title": "Human-readable label",
  "when": { "op": "exists", "value": { "$ref": "inputs.source" } },
  "timeout_seconds": 120,
  "retry": {
    "max_attempts": 2,
    "backoff_seconds": 2
  },
  "on_error": "fail",
  "redact": ["token"]
}
```

`on_error` supports `fail`, `continue`, and `fallback`. A fallback contains an
explicit nested `steps` list. A step records `pending`, `running`, `waiting`,
`succeeded`, `failed`, `skipped`, or `cancelled`.

### 12.2 Operator input step

Top-level inputs are collected before execution. An `operator_input` step
collects data that is only known after earlier results:

```json
{
  "id": "choose_candidate",
  "kind": "operator_input",
  "fields": [
    {
      "id": "candidate",
      "type": "enum",
      "options": { "$ref": "steps.discover.outputs.candidates" },
      "ui": { "control": "radio" }
    }
  ]
}
```

### 12.3 Script and external command step

```json
{
  "id": "collect",
  "kind": "script",
  "runtime": "python",
  "runtime_version": "3.12",
  "source": {
    "type": "bundle",
    "path": "files/collect.py"
  },
  "args": [
    "--repository",
    { "$ref": "inputs.repository" }
  ],
  "env": {
    "API_TOKEN": { "$ref": "secrets.api_token" }
  },
  "outputs": {
    "protocol": "json",
    "schema": {
      "type": "object",
      "required": ["count", "items"]
    }
  }
}
```

`source.type` supports:

- `inline`, with a `code` field;
- `bundle`, with a safe bundle-relative `path`;
- `command`, with an executable name and argument array.

Shell concatenation is not implicit. Commands are executed as an executable plus
an argument array. A shell must be explicitly declared as a dependency and
permission when shell syntax is required.

### 12.4 Agent step

```json
{
  "id": "prepare_summary",
  "kind": "agent",
  "model": {
    "binding": "primary"
  },
  "system_prompt": "Produce a factual report from the supplied data.",
  "prompt": "Summarize:\n{{ steps.collect.outputs.items }}",
  "tools": ["bundle.skill.report-format"],
  "max_turns": 4,
  "output_schema": {
    "type": "object",
    "required": ["title", "markdown"]
  }
}
```

An agent step has a bounded turn count and an explicit tool allowlist. It does
not inherit the current FoxxyCode session, project tools, rules, memory, or MCP
connections.

### 12.5 HTTP API step

An `api` step declares method, templated URL, headers, query, body, accepted
status codes, timeout, retry, response decoding, and an output schema. Its host
must match declared network permissions. Secret headers are redacted.

### 12.6 MCP and skill steps

- An `mcp` step calls a tool from a bundled MCP component by stable server and
  tool ids.
- A `skill` step invokes a bundled skill entry point through the mini-app
  harness.
- The bundle contains the required skill content and portable MCP files or a
  locked, verifiable requirement.
- External credentials remain runtime secret bindings.
- An MCP server that cannot be redistributed must be declared as a host
  requirement rather than copied.

### 12.7 File operation step

The `file` step supports explicit `read`, `write`, `copy`, `move`, `mkdir`,
`list`, and `archive` operations. Destructive operations and writes outside the
run workspace require declared permissions and, when configured, confirmation.
Paths use typed file/directory handles or normalized paths under allowed roots.

### 12.8 Confirmation step

```json
{
  "id": "approve_publish",
  "kind": "confirm",
  "title": "Publish generated files?",
  "message": "The mini app will write {{ steps.render.outputs.count }} files.",
  "details": { "$ref": "steps.render.outputs.preview" },
  "required": true,
  "reject": "cancel"
}
```

The interpreter enters `waiting` until an operator responds. Headless execution
fails before starting if a required interaction has no supplied answer policy.

### 12.9 Branch step

```json
{
  "id": "select_rendering",
  "kind": "branch",
  "if": {
    "op": "eq",
    "left": { "$ref": "inputs.format" },
    "right": "markdown"
  },
  "then": [],
  "else": []
}
```

Only the selected branch executes. Nested step ids remain globally unique.

### 12.10 Mini-app call step

A `miniapp` step invokes an exact released mini-app id and version with mapped
inputs and named outputs. Version ranges and draft dependencies are rejected in
a released bundle so execution remains reproducible.

### 12.11 General-purpose JSON program step

The `program` step contains a JSON-native, stack-based instruction language. Its
MVP language id is `foxxy-vm/1`. The language supports functions, local values,
loops, conditional jumps, exceptions, arrays, objects, strings, numbers,
booleans, null, and calls to explicitly imported host capabilities. It is a
general-purpose programming language, not only a condition expression format.

Example:

```json
{
  "id": "copy_items",
  "kind": "program",
  "language": "foxxy-vm/1",
  "entry": "main",
  "imports": {},
  "functions": {
    "main": [
      { "op": "ref.get", "arg": "inputs.items" },
      { "op": "local.set", "arg": "items" },
      { "op": "const", "arg": 0 },
      { "op": "local.set", "arg": "index" },
      { "op": "const", "arg": [] },
      { "op": "local.set", "arg": "result" },
      { "op": "label", "arg": "loop" },
      { "op": "local.get", "arg": "index" },
      { "op": "local.get", "arg": "items" },
      { "op": "array.len" },
      { "op": "lt" },
      { "op": "jump_if_false", "arg": "done" },
      { "op": "local.get", "arg": "result" },
      { "op": "local.get", "arg": "items" },
      { "op": "local.get", "arg": "index" },
      { "op": "array.get" },
      { "op": "array.push" },
      { "op": "local.set", "arg": "result" },
      { "op": "local.get", "arg": "index" },
      { "op": "const", "arg": 1 },
      { "op": "add" },
      { "op": "local.set", "arg": "index" },
      { "op": "jump", "arg": "loop" },
      { "op": "label", "arg": "done" },
      { "op": "local.get", "arg": "result" },
      { "op": "return" }
    ]
  },
  "limits": {
    "instructions": 1000000,
    "wall_time_seconds": 30,
    "heap_bytes": 67108864,
    "stack_depth": 1024,
    "call_depth": 128
  },
  "output_schema": {
    "type": "array"
  }
}
```

The required instruction groups are:

- stack and values: `const`, `ref.get`, `local.get`, `local.set`, `dup`,
  `swap`, and `pop`;
- arithmetic, comparison, boolean, string, and explicit conversion operations;
- array and object creation, lookup, update, iteration, and length operations;
- control flow: labels, jumps, conditional jumps, function call/return, and
  structured `try`/`throw`;
- JSON Schema validation and typed result return;
- `host.call`, addressed only through a declared import id.

Before execution the validator resolves all labels and functions, rejects
unknown opcodes and invalid stack/control targets, validates imports, and
requires finite positive resource limits within engine policy. The VM consumes
instruction fuel and obeys wall-time, heap, stack, call-depth, and cancellation
limits. Reaching any limit fails the step predictably.

The VM is pure and deterministic unless it invokes `host.call`. An import maps a
stable id to an existing mini-app capability such as a file operation, HTTP
request, command, MCP/skill call, model call, time, or random source. Every
import has typed input/output schemas and is checked by the same permission,
secret, redaction, timeout, and logging layer as a first-class workflow step.
There is no `eval`, dynamic opcode loading, implicit process access, or implicit
filesystem/network access. The restricted condition language in section 11
remains separate and cannot perform host effects.

## 13. Dependencies and portable runtimes

`requirements` declares:

- operating systems and CPU architectures;
- interpreter engine version;
- required executables;
- language runtimes such as Python or Go;
- portable runtime packages;
- environment variables and secret bindings;
- model bindings, including exact providers/models or capability requirements;
- network connectivity;
- bundled or host-provided MCP servers and skills.

Example:

```json
{
  "requirements": {
    "engine": ">=1.0.0 <2.0.0",
    "platforms": ["windows/amd64", "linux/amd64"],
    "runtimes": [
      {
        "id": "python",
        "version": "3.12.x",
        "provision": {
          "mode": "portable",
          "interaction": "silent_private",
          "url": "https://example.invalid/python.zip",
          "sha256": "<required checksum>",
          "size_bytes": 52428800,
          "install_script": "files/install-python.ps1"
        }
      }
    ],
    "model_bindings": [
      {
        "id": "primary",
        "selection": "fixed",
        "provider": {
          "type": "openai",
          "base_url": "https://api.openai.com/v1",
          "scope": "remote"
        },
        "model": "exact-api-model-id",
        "credentials": {
          "source": "matched_provider"
        },
        "required_capabilities": ["responses", "tools"]
      },
      {
        "id": "local_summary",
        "selection": "fixed",
        "provider": {
          "type": "openai",
          "base_url": "http://127.0.0.1:11434/v1",
          "scope": "local",
          "adapter": "ollama"
        },
        "model": "exact-local-model-tag",
        "local_bootstrap": {
          "connect": true,
          "start": "if_declared",
          "ensure_model": "pull_if_missing",
          "load": "if_supported",
          "storage_scope": "app_cache",
          "timeout_seconds": 300
        }
      }
    ]
  }
}
```

### 13.1 Silent private provisioning

Provisioning rules:

- an already installed runtime is reused only when the declared compatibility
  and integrity policy permits it;
- `interaction: "silent_private"` performs preflight provisioning without an
  installation prompt during a run;
- silent provisioning may write only to the app-specific runtime cache and a
  same-filesystem staging directory; it must never use admin/root elevation,
  system package managers, service installation, global `PATH`/registry changes,
  or writes outside the private cache;
- every download has a reviewed HTTPS source, exact checksum, expected size or
  size ceiling, and locked artifact identity;
- declarative unpack/install actions and any bundled install script are visible
  during release/import review and execute with the app's already approved
  network, process, and filesystem authority;
- accepting or running that exact reviewed release authorizes its declared
  silent-private preflight; a new prompt is required only if a new release
  expands authority, changes an unlocked source, or requires interaction that
  the program declares explicitly;
- provisioning is transactional: download and verification happen in staging,
  successful content is atomically promoted, concurrent provisioning is locked,
  and a failure leaves the previous cache usable;
- the operator surface may show only a safe status such as **Preparing
  application runtime**; detailed redacted diagnostics go to the app run log;
- the runtime records the exact resolved versions and digests in
  `manifest.lock.json`;
- released bundle files and locked dependencies are integrity-checked before a
  run;
- if a dependency cannot be prepared within these constraints, preflight fails
  without attempting elevation or a system-wide fallback.

The same rules apply when an embedded executable extracts a bundled runtime and
when a local provider downloads a model. Cache reuse always requires an exact
locked digest or an explicitly allowed compatibility rule.

### 13.2 Provider and model bindings

An agent or model step references a binding id:

```json
{
  "id": "prepare_summary",
  "kind": "agent",
  "model": {
    "binding": "primary"
  }
}
```

`selection: "fixed"` binds the step to one provider identity and one exact
provider API model id. The interpreter must not replace it with a semantically
similar, newer, cheaper, or locally available model. Alternative models are
allowed only as an author-declared ordered fallback of explicit binding ids.
`selection: "capability"` retains capability-based resolution for applications
whose author intentionally allows it.

Provider resolution uses this algorithm:

1. Canonicalize the binding's effective `base_url`: lowercase scheme and host,
   convert the host to its ASCII form, remove the scheme's default port,
   normalize dot segments, and remove one trailing slash. User information,
   query, and fragment components are invalid. The path, including `/v1`, is
   significant.
2. Canonicalize every locally configured provider's `api_base` or documented
   native default in the same way. Local configuration names are aliases and
   are not part of identity.
3. Require an exact canonical URL match. Do not use prefix, substring, DNS, or
   `localhost`/loopback-alias equivalence.
4. For `openai` and `anthropic`, also require the declared provider type to
   match, because equal gateway URLs can expose different wire protocols. For a
   declared adapter, require the adapter identity as well.
5. Reuse the matching local provider's credential and proxy bindings; API keys
   and credential values are never stored in the app JSON or bundle.
6. Require the exact API model id. First match locally configured model entries,
   then use the provider's supported discovery API. A missing credential,
   unavailable exact model, or capability mismatch fails preflight.

For OpenAI providers the discovery operation is `GET /v1/models`, which returns
the currently available model ids ([OpenAI Models API][openai-models-api]).
For Anthropic providers the interpreter uses its native List Models API
([Anthropic List Models][anthropic-models-api]). A generic OpenAI-compatible
provider may be probed at `GET /v1/models`, but failure does not authorize model
substitution.

A provider is `scope: "local"` only when its canonical host is loopback or it
uses a supported local socket. Private-LAN addresses are not treated as local.
The interpreter:

1. probes the declared base URL with a short timeout;
2. starts a bundled or silently provisioned provider only when
   `local_bootstrap.start` and an adapter recipe explicitly allow it;
3. enumerates the exact model;
4. downloads it when `pull_if_missing` is declared and the locked identity,
   storage ceiling, network permission, and silent-private policy allow it;
5. loads it when the adapter supports explicit loading;
6. verifies the exact model id and required capabilities before the first model
   step.

The MVP adapters are:

- `ollama`: enumerate with `GET /api/tags`, pull with `POST /api/pull`, and use
  its OpenAI-compatible `/v1` API ([Ollama list][ollama-list],
  [pull][ollama-pull], [OpenAI compatibility][ollama-openai]);
- `lmstudio`: enumerate with `GET /api/v1/models`, load with
  `POST /api/v1/models/load`, and download only when the app declares that
  policy ([LM Studio REST API][lmstudio-rest],
  [load endpoint][lmstudio-load]);
- `generic`: probe only the declared compatible API; it cannot start, download,
  or load a model without a separately declared adapter recipe.

An adapter may derive a native management endpoint from `base_url` only through
its versioned built-in same-origin mapping; it must not concatenate an
author-supplied management path. Under `storage_scope: "app_cache"`, a model may
be pulled only into an app-managed provider store below the private cache. A
matching provider that is already running may be used immediately when the
exact model exists, but the interpreter must not silently mutate its shared
model store.

The runtime never guesses a model by display name. A local model tag must be
locked to a digest when the provider exposes one. If the provider is offline,
the shared provider lacks the model, or the exact model cannot be connected,
downloaded into the permitted app store, or loaded, preflight fails with the
required provider identity and model id while keeping credentials redacted.

[openai-models-api]: https://developers.openai.com/api/reference/resources/models/methods/list
[anthropic-models-api]: https://platform.claude.com/docs/en/api/models/list
[ollama-list]: https://docs.ollama.com/api/tags
[ollama-pull]: https://docs.ollama.com/api/pull
[ollama-openai]: https://docs.ollama.com/api/openai-compatibility
[lmstudio-rest]: https://lmstudio.ai/docs/developer/rest
[lmstudio-load]: https://lmstudio.ai/docs/developer/rest/load

## 14. Permissions and secrets

Permissions are declarative:

```json
{
  "permissions": {
    "filesystem": {
      "read": ["input-files", "selected-directory"],
      "write": ["run-output"]
    },
    "process": ["python", "git"],
    "network": ["api.example.com"],
    "models": ["text-reasoning"],
    "secrets": ["api_token"],
    "operator_confirmation": true
  }
}
```

The runtime shows the effective permission request before the first run and
again when a new released version expands authority.

Secret values:

- are supplied by the operator, environment, or a future secret store;
- are never copied from the source session;
- are never included in exports, logs, run inputs, prompts unless explicitly
  bound to that prompt field, or output display;
- are redacted from process arguments where the target supports stdin or
  environment-based delivery.

## 15. Success criteria

Success is separate from “all processes returned exit code zero.” A mini app
declares one or more checks:

- step status;
- output JSON Schema;
- file existence, kind, size, checksum, or content pattern;
- numeric or string predicate;
- HTTP status/response predicate;
- model-assisted judge prompt with a structured verdict;
- explicit operator acceptance.

Example:

```json
{
  "success": {
    "mode": "all",
    "checks": [
      {
        "kind": "step",
        "step": "prepare_summary",
        "status": "succeeded"
      },
      {
        "kind": "schema",
        "value": { "$ref": "steps.prepare_summary.outputs.result" },
        "schema": {
          "type": "object",
          "required": ["title", "markdown"]
        }
      },
      {
        "kind": "judge",
        "prompt": "Verify that the report answers the requested period and contains no unsupported claims.",
        "value": { "$ref": "steps.prepare_summary.outputs.result" },
        "output_schema": {
          "type": "object",
          "required": ["passed", "reason"],
          "properties": {
            "passed": { "type": "boolean" },
            "reason": { "type": "string" }
          }
        }
      }
    ]
  }
}
```

Model-assisted checks are nondeterministic and must be identified as such in the
test report. Release requires at least one non-model structural check whenever
the workflow has a structured or file output.

The draft editor provides an author-facing expectations field and a
**Generate expected result with LLM** action. The generator treats the draft
and expectations as untrusted data, produces a reusable `expected_result` and
`acceptance_criterion`, and stores an executable `prompt` check bound to a
declared fixed model. If the draft has no model binding, FoxxyCode snapshots the
currently configured provider identity, canonical `base_url`, and model into a
new fixed binding. Test and released runs ask that binding for a structured
`{"passed": boolean, "reason": string}` verdict; internal reasoning is neither
displayed nor persisted.

## 16. Outputs and result presentation

Each output has an id, value reference, type, optional schema, export behavior,
and renderer. Supported output types are:

- plain text;
- Markdown;
- JSON;
- table;
- file;
- files;
- directory;
- archive;
- generated media or other generated artifact.

Supported renderers include:

- text/Markdown panel;
- JSON tree;
- table with declared columns;
- file list with preview and download;
- image/audio/video or generic generated-artifact preview when the media type is
  supported;
- summary card composed from named outputs.

`display` controls title, description, layout, sections, collapsed details,
download actions, and the primary result. Normal operator UI does not render
execution logs inline; it may offer **Open run folder** for authorized local
diagnostics. Display configuration cannot execute code or read undeclared
files.

## 17. UI specification

### 17.1 Session action

- A **Create mini app** button appears in the active session header/action area
  in web and desktop mode only when the backend capability `miniapps` is true.
- After a completed session result, the same action may also appear as **Save as
  mini app** beside the result summary; both placements start the same operation.
- It is disabled while a turn is running.
- Activating it starts scenario identification and opens the generated draft
  when ready.
- Long distillation shows progress, current phase, elapsed time, cancel, and
  resumable failure details.

### 17.2 Mini Apps section

The main navigation adds **Mini Apps** with routes conceptually equivalent to:

```text
#/miniapps
#/miniapps/new
#/miniapps/<id>
#/miniapps/<id>/edit
#/miniapps/<id>/runs/<run-id>
```

The catalog provides status, latest version, author, tags, last run, measured
duration, and actions to run, edit draft, create a new version, export, import,
or view history.

An open session may display the catalog as a closable quick drawer without
navigating away. The drawer includes name/tag search and an explicit
**Show archived** control. Each compact card shows at least:

- icon or generated media-type mark;
- name and short description;
- exact release version and declared input count;
- compatibility/availability state and measured duration when known;
- a primary **Run** action;
- an overflow menu for edit/new-version, history, export, and archive/restore.

**Run** opens the generated input form for the resolved exact release. It must
not execute side effects directly from the card before input validation and
preflight.

### 17.3 Editor

The editor has the following views:

- Overview;
- Inputs and live form preview;
- Workflow;
- Dependencies and permissions;
- Success checks;
- Results/display;
- Bundle files;
- JSON;
- Test runs;
- Release review.

Invalid raw JSON does not replace the last valid canonical draft until it parses
and validates. Validation errors point to both a JSON location and the
corresponding visual editor field.

During distillation, the editor also provides:

- an ordered collapsible step list with explicit type badges and a separately
  labelled authoring/system context item;
- a read-only **Session result** card with accepted output, detected
  requirements, and candidate artifacts;
- a collapsible sanitized **Source session context** panel;
- directly editable name, description, goal, input id/label/type/required state,
  validation, and acceptance criteria;
- permission summary badges such as network, filesystem, process/Git, model,
  MCP, and skill, each opening the complete permission detail;
- a refinement assistant with its authoring model/provider label and draft patch
  history;
- explicit **Save draft**, **Release**, and close actions.

The authoring/system context item is not an implicit executable step. It becomes
portable only when the distiller turns required behavior into explicit prompts,
constants, bindings, or workflow steps. Closing warns about unsaved
local-invalid edits; a valid autosaved draft remains available.

### 17.4 Runner

The runner:

1. renders the declared input form;
2. validates inputs and dependencies;
3. previews permissions;
4. runs the workflow with sanitized live step status while writing execution
   logs to the selected run directory;
5. pauses for declared operator input or confirmation;
6. supports cancellation;
7. evaluates success;
8. renders declared results and downloadable artifacts;
9. renders total duration, model/API duration, input/output tokens, and resolved
   non-secret provider/model binding ids when available;
10. offers **Run again** with editable previous non-secret inputs.

### 17.5 Release review

The release screen shows:

- proposed version and compatibility impact;
- complete permission diff from the previous release;
- dependencies and install scripts;
- bundled skills and MCP components;
- sanitization findings;
- latest same-data test result;
- success-check report;
- bundle file list and integrity manifest.

Release is unavailable until the draft is valid, sanitization has no unresolved
blocking findings, and the current draft revision has a passing test.

## 18. Proposed HTTP API

These routes describe the product contract. The implemented MVP subset and its
exact response behavior are documented in `docs/http-api.md`; routes that
remain only in this section are forward-compatible requirements.

All routes in this section are registered only by builds with `miniapps`.
`GET /foxxycode/capabilities` advertises `"miniapps": true` for such a build and
false otherwise, and remains available independently of the optional module.

### 18.1 Distillation

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/foxxycode/sessions/{id}/miniapps/distill` | Start distillation for an explicitly selected session. |
| GET | `/foxxycode/miniapp-distillations/{job_id}` | Read phase, progress, scenario proposal, and result. |
| GET | `/foxxycode/miniapp-distillations/{job_id}/events` | Stream progress as SSE. |
| POST | `/foxxycode/miniapp-distillations/{job_id}/scenario` | Confirm or correct the proposed scenario. |
| POST | `/foxxycode/miniapp-distillations/{job_id}/cancel` | Cancel the job. |

### 18.2 Catalog and editing

| Method | Path | Purpose |
|--------|------|---------|
| GET/POST | `/foxxycode/miniapps` | List/search or create/import a mini app. |
| GET/PATCH/DELETE | `/foxxycode/miniapps/{id}` | Read/update catalog metadata or delete after confirmation. |
| GET/PUT | `/foxxycode/miniapps/{id}/draft` | Read or replace the validated canonical draft. |
| GET/PUT/DELETE | `/foxxycode/miniapps/{id}/files/{path...}` | Manage safe draft bundle files. |
| POST | `/foxxycode/miniapps/{id}/validate` | Validate schema, references, requirements, and permissions. |
| POST | `/foxxycode/miniapps/{id}/sanitize` | Scan the complete draft bundle. |
| POST | `/foxxycode/miniapps/{id}/release` | Create an immutable released version. |
| GET | `/foxxycode/miniapps/{id}/export` | Export a draft or exact release bundle. |
| POST | `/foxxycode/miniapps/import` | Validate and import a bundle. |

### 18.3 Runs

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/foxxycode/miniapps/{id}/test-runs` | Test an exact draft revision. |
| POST | `/foxxycode/miniapps/{id}/versions/{version}/runs` | Run an exact release. |
| GET | `/foxxycode/miniapp-runs/{run_id}` | Read run state and results. |
| GET | `/foxxycode/miniapp-runs/{run_id}/events` | Stream run events as SSE. |
| POST | `/foxxycode/miniapp-runs/{run_id}/input` | Answer a declared operator-input step. |
| POST | `/foxxycode/miniapp-runs/{run_id}/confirmation` | Resolve a declared confirmation step. |
| POST | `/foxxycode/miniapp-runs/{run_id}/cancel` | Cancel a run. |

Every externally visible route must be added to the served OpenAPI document and
`docs/http-api.md` when implemented.

## 19. Run behavior and recovery

- Only an exact draft revision or exact release version can run.
- Each step records structured start, progress, diagnostic log, output,
  waiting, finish, and error events. The operator event stream exposes only the
  sanitized public subset defined by `runtime.operator_event_level`.
- Cancellation propagates to child processes, HTTP requests, MCP calls, model
  calls, and nested mini apps.
- Completed step outputs are persisted incrementally.
- A process crash marks an active run `interrupted`.
- Resuming an interrupted run is allowed only when every already completed step
  declares its output durable and the next step is resumable. Otherwise the UI
  offers a fresh run with the same non-secret inputs.
- Retries and fallback are recorded as separate attempts in the event log.
- Runtime logs have size and retention limits; result artifacts have configurable
  quotas.
- Raw reasoning is never a run event. Raw tool calls are internal; only the
  reviewed diagnostic level may write redacted tool metadata or payload.

## 20. Reuse, modification, and ordinary-agent paths

The product recognizes three paths:

1. run a sufficiently matching released mini app;
2. create a new version of a close mini app;
3. continue with an ordinary agent session.

In the MVP:

- the catalog can be searched manually;
- measured mini-app run duration is displayed;
- the user chooses the path;
- modification always creates a draft for a new released version;
- FoxxyCode does not automatically estimate all three paths or route a request.

Future work may add semantic vectors, expected completion time, success
probability, token/currency cost, risk, and organizational usage signals. The
automatic router must remain advisory unless a later specification explicitly
authorizes autonomous selection.

## 21. Organizational collection

Future group/company collection follows these mandatory constraints:

- only explicitly opted-in sessions are analyzed;
- access follows the source session's organizational boundary;
- a human reviews and approves every proposed mini app;
- sanitization runs before the proposal enters a shared catalog;
- source-session content is not retained in the released artifact;
- administrators can define excluded projects, paths, tools, data classes, and
  network domains;
- contributors can see what data is being analyzed and cancel collection.

Background harvesting is not part of the MVP.

## 22. Validation and compatibility

A bundle is valid only when:

- its schema major version is supported;
- ids and references are unique and resolvable;
- input dependencies are acyclic;
- every nested branch and fallback validates;
- required assets exist and match their integrity entries;
- runtime and model requirements can be resolved;
- permissions cover every step without undeclared authority;
- secret references are declared and do not flow to forbidden sinks;
- output and success references resolve;
- released nested mini-app versions are exact;
- no path escapes the bundle or allowed runtime roots.

Minor schema versions may add optional fields. A major schema change requires an
explicit migration. Import never rewrites an immutable release in place.

## 23. MVP acceptance criteria

The feature is acceptable when all of the following are demonstrable:

1. A user completes a web/desktop session and activates **Create mini app**.
2. The distiller analyzes the session, confirms one scenario, and creates a
   draft without retaining source-session provenance in the portable program.
3. The draft contains generated form inputs, a valid workflow, requirements,
   permissions, success checks, outputs, and display settings.
4. The author can edit every generated field visually and through JSON.
5. The editor supports the declared v1 input controls and validation
   dependencies.
6. The interpreter executes every required v1 step kind, conditions, retry,
   fallback, and an exact-version nested mini app.
7. Inline scripts, bundled scripts, and explicitly declared external commands
   work.
8. Missing locked dependencies are prepared silently in the private app cache;
   no run-time installation prompt, elevation, or system-wide mutation occurs.
9. A same-data test runs in isolation and produces a persisted success report.
10. A failing test cannot be released.
11. Release review detects seeded secrets and source-specific paths in JSON and
    bundled files.
12. Releasing creates immutable version `1.0.0`; modifying it creates a draft
    that can only be released under a higher version.
13. A release exports to a portable bundle and imports on another compatible
    installation.
14. FoxxyCode and the standalone interpreter produce equivalent step/output
    semantics for the same bundle and inputs.
15. Runs expose live progress, declared interaction, cancellation, logs,
    duration, results, artifacts, and repeat.
16. Mini-app UI is absent from IDE-integrated surfaces.
17. The user can manually choose to run, version, or ignore a matching mini app;
    no automatic router is presented as implemented.
18. A build without `miniapps` has no interpreter/builder commands or mini-app
    HTTP routes and advertises no mini-app UI capability; a build with the tag
    exposes them.
19. FoxxyCode runs a JSON program without UI and builds both console and Windows
    desktop-UI executables that contain the canonical program and bundle.
20. Console and UI executables show only safe operator events; reasoning and raw
    tool calls remain hidden, while redacted execution diagnostics are stored
    under the selected local or global `.foxxycode/apps/<app>/runs/<run-id>`
    root.
21. A `foxxy-vm/1` program with functions and a bounded loop executes
    equivalently in the embedded interpreter and built executable, while an
    undeclared host call or exhausted resource limit fails predictably.
22. A fixed provider/model binding reuses local credentials only after an exact
    normalized `base_url`, provider protocol type, and API model-id match; it
    never silently substitutes another model.
23. An allowed local provider is probed, optionally started, and has its exact
    model pulled/loaded under the silent-private policy, or the run fails
    preflight without a model call.
24. The distillation workspace shows an ordered typed step navigator, session
    result, sanitized source context, editable inputs/goal/acceptance criteria,
    permission summaries, and a refinement assistant without exporting source
    evidence.
25. A refinement request produces a reviewable patch against an exact draft
    revision; accepting it creates a new revision but never bypasses test,
    sanitization, or human release gates.
26. From an open session, a quick catalog drawer searches names/tags, optionally
    shows archived apps, and presents cards with exact version, input count,
    description, **Run**, and overflow actions; **Run** opens the generated form
    before side effects.
27. Source-session, test, and normal-run views show available total duration,
    model/API duration, and input/output token metrics, while the released
    portable program contains no source-session metrics or provenance.

## 24. Implementation guidance

When implementation begins:

- define the JSON Schema and interpreter contract before UI-specific domain
  types;
- freeze the `foxxy-vm/1` opcode table, stack effects, numeric/error semantics,
  verifier, resource accounting, and cross-runtime conformance fixtures before
  accepting released programs;
- keep the interpreter and bundle validation in an inward package that does not
  depend on HTTP, desktop, or React code;
- place interpreter, builder, HTTP registration, and backend capability
  declarations behind `//go:build miniapps`; make the untagged capability false
  and do not register placeholder commands or routes;
- make the UI use the backend capability in addition to the existing
  web/desktop-versus-IDE surface check;
- implement executable creation with a version-matched runner build workspace
  and `//go:embed`, then verify the produced executable's embedded digest;
- add the happy path as a repository-root Gherkin feature before implementation;
- cover invalid schemas, unsafe archives, secret flows, path traversal,
  permission mismatches, VM verifier/limit failures, provider URL
  canonicalization, exact-model resolution, provisioning rollback, cancellation,
  and version conflicts with unit tests;
- add HTTP handlers only after storage, validation, and runtime behavior exist;
- update `external/httpserver/openapi.go`, `docs/http-api.md`, `DESIGN.md`, and
  `docs/ui.md` with the implemented contract;
- keep all web UI strings in both English and Russian localization dictionaries;
- build and test the UI against the repository's Chromium 104 baseline;
- run the full repository test and lint workflows before completion.

## 25. Deferred decisions

The following details may be decided during technical design without changing
the product contract above:

- the final standalone executable and bundle-extension names;
- the first officially supported portable Python and Go distributions;
- bundle signing and trusted-publisher policy;
- exact run-history retention and artifact quotas;
- additional UI targets beyond Windows WebView2 and additional console
  `GOOS/GOARCH` targets;
- the exact semantic-search and three-path routing design after the MVP.
