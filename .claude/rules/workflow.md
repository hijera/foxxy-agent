---
description: BDD-style workflow, UI screenshots in the PR, HTTP OpenAPI and config schema sync before lint, final checks
paths:
  - "**/*.go"
  - "docs/**/*.md"
  - "README.md"
  - "external/ui/**/*"
---

# Workflow (features, bugs, finish)

## New behavior (TDD / BDD)

When adding or changing behavior (including words like feature, add, implement, фича, добавить):

1. Add or extend a **failing** test that asserts the observable outcome (red).
2. Run the narrowest test scope that proves the failure is real.
3. Implement the smallest change that makes the test pass (green).
4. Run **`make test`** - the express run: **`ui-build`**, **`ui-test`**, then one **`go test`** over the whole tree with every optional module compiled in (**`http,ui,scheduler,memory,cli,browser,gateway,swarm`**). Everything must pass. Do **not** walk the tag combinations locally: that matrix (**`make test-matrix`**) runs on GitHub Actions for every pull request, one job per combination. When the change moved a build-tag boundary (a **`_stub.go`**, an **`Available`** const, a **`//go:build`** line), run that one combination by hand - **`go test -tags=<set> ./...`** - and leave the rest to CI.
5. **UI screenshots in the PR** - if the change touches the SPA (**`external/ui/**`**: `.tsx`, `styles.css`, rendered markup), attach screenshots of **every** changed surface to the PR description. Per edit, not one image per PR.
   - Screenshot the **running build** you already verified per **`.claude/rules/ui-verification.md`** (**`npx vite`** in **`external/ui`**, browser tools). Never a mockup, a hand-drawn approximation, or a re-used older image.
   - One image per affected **view and state** - a new dialog needs open *and* the surface it returns to; a changed row needs the row in each state the edit reaches.
   - Post **before/after** pairs for surfaces that already existed, so the visual diff is readable without checking out the branch.
   - Add **narrow (390px)** and **wide (1280px)** when layout differs between them, and **light** plus **dark** when the change adds or edits colors (the repo ships 7 themes; cover any whose tokens the change touches).
   - If a surface genuinely cannot be captured (no browser available, backend-gated screen), say so **explicitly** in the PR and state what was verified instead - do not silently omit it.
6. **HTTP OpenAPI narrative** - If you changed the optional OpenAI-compatible HTTP API (routes, methods, headers, request or response bodies, status codes, or anything reflected in the served spec), update **`external/httpserver/openapi.go`** (`openAPISpec`) so it matches **`external/httpserver/server.go`** handlers and tests. Align **`docs/reference/http-api.md`** (and **`README.md`** HTTP bullets) when user-facing descriptions change.
7. **Config schema sync** - If you changed the YAML config surface (**`internal/config`** structs: added, renamed, retyped, or removed a yaml-tagged field, enum value, or default), update **`internal/config/config.schema.json`** (the copy embedded into the binary: it is what `foxxycode -t` validates against, and `make site-schema` republishes it to **`docs/config.schema.json`**, the file GitHub Pages serves) and run **`make docs`**: the field tables of **`docs/reference/config.md`** are generated from the schema's `description` strings and the loader's defaults, so a key without a description ships an empty row. **`TestDocsConfigSchemaMatchesStructs`** (**`internal/config/docs_schema_test.go`**) catches key/type drift; the prose in the Notes section of that page and the guide **`docs/getting-started/configuration.md`** are kept by hand. Mirror user-facing fields in **`config.example.yaml`** and **`UISchemaMap()`** (**`internal/config/ui_schema.go`**) as well. The same change must also update the bundled self-configuration skill **`internal/skills/bundled/configure-foxxycode/SKILL.md`** (its "Configuration areas" catalog and command examples are the agent-facing view of the schema) - schema edits that skip the skill ship an agent that configures against a stale surface.
8. **Publish the schema** - **`make site-schema`** copies the embedded schema to **`docs/config.schema.json`**, which GitHub Pages serves at the address FoxxyCode writes as a modeline into every config it saves. **`make site-schema-check`** reports drift without writing, and **`TestPublishedSchemaMatchesTheEmbeddedOne`** fails on it. Do **not** publish a **renamed or removed** key ahead of the release that understands it: the schema sets **`additionalProperties: false`**, so the new file marks the old key as an error in every config already on disk. Adding an optional key is safe immediately.
9. Update documentation and specs if needed.
10. **Documentation of the change** - a capability a user will notice, or a change to anything a
    page describes, is not finished until the documentation says so, in the same pull request. The
    layout is **`docs/nav.yaml`** (the map: every page with a one-line summary), **`docs/<group>/<page>.md`**
    (`getting-started`, `surfaces`, `operate`, `features`, `reference`, `tutorials`, `contributing`, `plans`) and
    **`docs/assets/`** (what the pages embed); **`docs/contributing/documentation.md`** has the page
    types, the capture recipes and the checks.
    - **New capability** - a page under **`docs/features/`**, **`docs/surfaces/`** or **`docs/operate/`**
      (or a section of the page that owns the area), an entry in **`docs/nav.yaml`**, a line in the
      feature list of both READMEs when it is something a newcomer chooses FoxxyCode for, and a row in the
      reference that lists things of its kind (**`docs/reference/tools.md`** for a tool,
      **`slash-commands.md`** for a command, **`environment-variables.md`** for a variable,
      **`keyboard.md`** for a key).
    - **Changed behaviour** - the page that describes it, found with `git grep -n '<key or command>' docs/`,
      updated in the same commit. A config key also flows through step 7, a CLI flag through step 11.
    - **Web UI and console features** - the page shows the feature, not only describes it: a screenshot
      of the real surface (headless Chrome against a live **`http,ui`** build, or the pty capture scripts
      under **`examples/cli/`** for the console) embedded next to the paragraph that explains it, Dark
      theme 1280 px wide as the default, named `<feature>-<state>-<theme>-<width>.png`, under
      **`docs/assets/<page>/`** when the page needs several and at the top level of **`docs/assets/`**
      otherwise, with a one-line italic caption.
    - **Assets** - every file under **`docs/assets/`** is referenced by a page, the README, **`DESIGN.md`**
      or the **`Dockerfile`**; **`make docs-check`** fails on one that is not, so a screenshot leaves
      together with the feature it showed. Screenshots that only prove a pull request go to the pull
      request (the orphan **`screenshots`** branch, linked by raw URL), never under **`docs/assets/`**.
    - **Generated pages** - **`make docs`** regenerates **`docs/README.md`**,
      the field tables of **`docs/reference/config.md`**, the help screens of
      **`docs/reference/cli.md`** and the inventory of **`docs/assets/INDEX.md`**; commit the result.
      **`make docs-check`** (the CI job **Documentation**, and the pre-commit hook for documentation
      commits) fails on drift, on a page missing from the map, on a broken relative link or anchor and
      on an unused asset. **`make docs-fast`** refreshes everything but the CLI reference, which follows
      the `--help` screens of a full-tag binary and is regenerated on Linux or WSL, where CI checks it.
      `llms.txt` and `llms-full.txt` are **not kept in this repository**: a concatenation of every page conflicted in every branch that touched one. The website build renders them (`go run ./cmd/docsgen -publish -skip-cli`, run by `make site` / `make site-check` and by the **Website** workflow) into the checkout it publishes, where they are served at **`https://hijera.github.io/foxxy-agent/llms.txt`** and **`https://hijera.github.io/foxxy-agent/llms-full.txt`**; `.gitignore` covers the paths, so a local copy never reaches a commit.
    - **Design records** - **`docs/plans/**`** keep decisions as they were taken and are not rewritten
      to match a later rename; only their link targets are repaired when a page moves. A page that
      moves takes its address with it: there are no redirect stubs, and an old link breaks.
11. **CLI command set** - if the change added, renamed or removed a subcommand, a **`serve`** verb or a flag that **`printUsage`** (**`cmd/foxxycode/main.go`**) lists, carry the same change into **`packaging/man/foxxycode.1`**, **`packaging/completions/foxxycode.bash`** and **`packaging/completions/foxxycode.zsh`**, and into the **`topLevelCommands`** list of **`cmd/foxxycode/usage_test.go`**. Those three files are what every install route ships as the description of the command set - the release archive, the **`.deb`** and the **`.rpm`**, the Homebrew formula - and nothing generates them from the code. A completer that offers a command the binary no longer answers is a user-visible bug, so this belongs to the change, not to the release.
12. Run **`make lint`** (`golangci-lint`). Fix reported issues.

Then report briefly: goal, tests added or changed, `make test` and `make lint` outcome, files touched, and the CI matrix verdict once the pull request is up.

## Bug fixes

1. Add a regression test that fails on the broken code.
2. Fix the code; confirm the new test passes.
3. Run **`make test`** (the express run; the tag matrix is CI's).
4. If the fix changes anything the user sees in the SPA, complete step 5 (UI screenshots) from the feature flow - a visual bug fix without before/after images in the PR is not reviewable.
5. If the bug or fix touches the HTTP API surface, complete step 6 (OpenAPI and docs) from the feature flow.
6. If it touches **`internal/config`** yaml-tagged structs, complete steps 7 and 8 (config schema sync, and publishing it) from the feature flow.
7. If the fix touched what **`printUsage`** lists - a subcommand, a **`serve`** verb, a flag - complete step 11 (man page and completions) from the feature flow.
8. If the fix changes anything a documentation page describes, complete step 10 (documentation of the change) from the feature flow.
9. Run **`make lint`**.

## Before calling work done

- **`make test`** green locally. The tag matrix is not a local step: after the push, read the **Tests on PR** run (**`gh pr checks`**) and fix whichever combination it names.
- **Screenshots of every changed UI surface attached to the PR** when **`external/ui/**`** changed, or an explicit note saying why a surface could not be captured.
- OpenAPI and HTTP docs updated when the HTTP API changed.
- **`internal/config/config.schema.json`** and **`docs/reference/config.md`** (regenerated with **`make docs`**) updated when `internal/config` yaml fields changed, and **`make site-schema-check`** clean so the copy published at **`hijera.github.io/foxxy-agent/config.schema.json`** is not stale.
- **Man page and completions match the usage text** when the CLI surface changed: `go test ./cmd/foxxycode -run 'TestUsage|TestPackaging'` green.
- **`make docs-check`** clean: every new page in **`docs/nav.yaml`**, the generated pages regenerated with **`make docs`**, no broken relative link or anchor, no asset without a page. The page of every user-visible change updated, with a screenshot on the page when the change is visible in the web UI or the console.
- **`make lint`** clean.
- **Rules sync** — if any `.claude/rules/*.md` file was added or changed, propagate the **edit** to `.cursor/rules/`: the same `.mdc` file, with `paths:` replaced by Cursor-compatible `globs:`/`alwaysApply:` (files without `paths:` get `alwaysApply: true`). Refresh the index in **`.codex/rules.md`** when a rule file is added, renamed, or removed.
  - **Port the edit, not the whole body.** Copying the `.md` body over the `.mdc` looks equivalent and is not, in two ways that are easy to miss. Sibling references are spelled for their own dialect — `@architecture.mdc` in a `.mdc` file — so a body copy leaves Cursor pointing at files it cannot resolve. And the two copies have **drifted in both directions**: `core-modules` carries a paragraph in the `.md` that is absent from the `.mdc` and another in the `.mdc` that is absent from the `.md`, so a copy in either direction deletes text nobody meant to delete. Diff the pair before assuming they are the same file.
- **Changelog** — if the change is something a plugin user can observe, add a Russian `## Unreleased — <date>` entry before opening the PR: to **`editors/intellij/CHANGELOG.md`** for what IntelliJ users see, to **`editors/vscode/CHANGELOG.md`** for what VS Code users see, to **both** for SPA / agent behaviour changes (both plugins host the same UI). Write the same section in English in the **`CHANGELOG.en.md`** next to each file you touched: the project website reads the twins, and the changelog tests fail when their headings or entries drift from the Russian file. Merging releases immediately and the build stamps the heading with the tag, so the notes are part of the PR, not a later step. See **`.claude/rules/release-changelog.md`**.

## BDD specs (Gherkin)

The **happy path** of a feature (and a bug's reproduction) belongs in a Gherkin `.feature`
file in the **repo-root `features/`** directory. The godog step definitions stay in the
package under test (e.g. `external/httpserver/bdd_*_test.go`, `internal/agent/bdd_*_test.go`)
and point `Options.Paths` at `../../features/<name>.feature`. **Edge and error cases go in
ordinary unit tests**, not in `features/`.
