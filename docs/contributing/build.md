# Building FoxxyCode from source

This page is the detailed reference for local builds. For a short version, see [Installation](../../README.en.md#install) in the root **README**.

## Prerequisites

- **Go** - match `go` in [`go.mod`](../../go.mod) (currently **1.25**).
- **Git** - the Makefile embeds a version string from tags or `git describe` when available.
- **Node.js and npm** - required when you build with both **`http`** and **`ui`**, because the Makefile runs **`ui-build`** (see [`Makefile`](../../Makefile)) to produce the assets that **`go:embed`** picks up, and by **`make test`** and **`make lint`**, which run the SPA's vitest suite and its TypeScript check.

Optional:

- **`golangci-lint` v2.x** (built with Go **1.25** or newer) - for **`make lint`**, which runs an untagged pass plus one per optional build tag (**`cli`**, **`browser`**, **`gateway`**, **`swarm`**, **`http,scheduler,memory,gateway,swarm`**); **`make lint-ui`** adds the embedded-SPA pass and **`make lint-windows`** the Windows one. CI installs the pinned version with **`go install`** and calls the same targets, so local and CI coverage cannot drift.
- **Python 3.8+** - only for the interactive build wizard ([`scripts/build.py`](../../scripts/build.py)); stdlib only, no `pip` packages.

## Interactive build wizard

On **Windows** (and anywhere without GNU Make on `PATH`), use the Russian-language console wizard
[`scripts/build.py`](../../scripts/build.py). It wraps the same steps as **`make build`**, Gradle
**`buildPlugin`**, and VS Code **`vsce package`** without duplicating Go compile logic.

**Interactive** (no arguments — step-by-step menus for target, platform, tags, IDE plugins):

```bash
python scripts/build.py
```

**Non-interactive** (CI or scripts):

```bash
# Full-feature CLI for the current host (same as make build TAGS="http ui scheduler memory cli browser gateway swarm")
python scripts/build.py --target cli --preset full

# Lean ACP-only binary (no npm step)
python scripts/build.py --target cli --preset lean

# Cross-compile one release target
python scripts/build.py --target cli --goos linux --goarch arm64 --preset full --dry-run

# All five release platforms + dist/*.tar.gz|zip + SHA256SUMS
python scripts/build.py --target cli --all-release --preset full --ldflags-strip

# IntelliJ plugin zip (JDK 17+, Go, npm). The wizard auto-detects every JDK 17+
# available: $JAVA_HOME, then java on PATH, then the bundled JBR of any
# installed JetBrains IDE (IntelliJ IDEA, PyCharm, WebStorm, ...). When several
# IDEs are found it prints them all and, in interactive mode, asks which one to
# use as JAVA_HOME for Gradle — handy on Windows boxes that ship Java 8 on PATH
# but have PyCharm + PhpStorm + ... installed.
python scripts/build.py --target intellij --plugin-version 1.2.3 --production

# VS Code VSIX for one platform (scaffold extension)
python scripts/build.py --target vscode --vscode-target darwin-arm64

# CLI release matrix + IntelliJ + VS Code
python scripts/build.py --target all --preset full
```

| Wizard choice | Output |
|---------------|--------|
| CLI (single platform) | **`build/foxxycode`** or **`build/foxxycode.exe`** |
| CLI (all release) | **`dist/foxxycode_<version>_<os>_<arch>.{tar.gz,zip}`** + **`dist/SHA256SUMS`** |
| IntelliJ | **`editors/intellij/build/distributions/*.zip`** |
| VS Code | **`editors/vscode/*.vsix`** (one per **`--vscode-target`**) |

**Tag presets:** **`lean`** (no tags), **`full`** (`http ui scheduler memory cli browser gateway` - the
set the release CLI archives ship, matching the **`Makefile`**'s **`FULL_TAGS`**), **`gateway`**
(like full, but only the Telegram adapter via **`gateway.telegram`**), **`desktop`** (swaps in **`desktop`**). Custom tags: **`--tags http,scheduler`** (comma-separated; **`ui`**
requires **`http`**).

Run **`python scripts/build.py --help`** for the full flag list (Russian descriptions).


Build with **`memory`** to link long-term memory (`external/memory`). Enable behavior at runtime with **`memory.enable`** in config (see [`external/memory/README.md`](../../external/memory/README.md)).

The **HTTP gateway**, **embedded SPA**, **scheduler**, and **memory** are controlled by Go build tags. For a single binary that matches the default **Docker** image and includes every optional feature:

```bash
make build TAGS="http ui scheduler memory cli browser gateway"
```

Output: **`build/foxxycode`**.

Equivalent **`go build`** (after `ui-build` when you use **`ui`**, or use **`make build`**, which runs **`ui-build`** automatically when **`TAGS`** contains both **`http`** and **`ui`**):

```bash
make ui-build   # only when using -tags=...,ui,... with http; Makefile runs this for you on `make build`
VERSION="$(make -s print-version)"
go build -tags=http,ui,scheduler,memory,cli,browser,gateway \
  -ldflags "-X github.com/hijera/foxxycode-agent/internal/version.Version=${VERSION}" \
  -o build/foxxycode \
  ./cmd/foxxycode/
```

The [**Dockerfile**](../../Dockerfile) uses the same idea: comma-separated tags via **`BUILD_TAGS`** (default **`http,scheduler,ui,memory,gateway,cli,browser,swarm`**) and strips debug symbols with **`-ldflags "-s -w ..."`** in addition to the version **`X`** flag.

## Fork-owned SPA styles

The common stylesheet stays at `external/ui/src/styles.css`, matching the
upstream path. FoxxyCode's SVN cards, provider onboarding and guided tour,
provider import/export controls and IDE embed chrome live in
`external/ui/src/styles/fork/`.

A top-level comment such as `/* foxxycode:styles svn */` marks the original
cascade position: `foxxycode:styles` plus exactly one name, the file
`styles/fork/<name>.css`. `external/ui/postcss-fork-styles.mjs` expands that
feature's CSS before `postcss-resolve-color-mix.mjs` runs. Keep the marker in
place when porting upstream CSS; edit the feature file for fork-specific
styling. Move its media queries and state rules together. A longer comment that
only names the mechanism is documentation and stays in the composed stylesheet
untouched. Feature files cannot include one another and cannot use `@import`.
An unknown name, an unreadable file, a nested include and an `@import` all fail
the build.

The same composition runs in Vite development and production. Included files
are registered as dependencies for hot reload. The browser receives one
stylesheet with no include markers or runtime imports, and `scripts-sync-to-go.mjs`
continues publishing the same embedded `styles.css`. All theme variables remain
visible to the Chromium 104 color transform. CSS contract tests read the
composed source through `readStyleSource()` from the same module — fork rules
count for the phone layout and reduced-effects contracts like for any other —
and `forkStyles.test.ts` pins that every file under `styles/fork/` has its
marker in `styles.css`, every marker its file, and every feature the rules its
surface renders with.

For a mechanical extraction, compare the built CSS before and after the move:
the bytes and SHA-256 must match. Run `make ui-test`, `npm run test:layout` in
`external/ui`, the Chromium 104 compatibility guard and the browser checks for
the affected surfaces. Capture before/after views in narrow/wide and dark/light
layouts, then rebuild the binary as described above.

## Install on your PATH

**`make install`** copies **`build/foxxycode`** onto your **`PATH`**:

- If **`build/foxxycode`** already exists (for example after **`make build TAGS="http ui scheduler memory cli browser gateway swarm"`**), it is installed as-is without rebuilding.
- If the binary is missing, **`make install`** runs **`make build TAGS="http ui scheduler memory cli browser gateway swarm"`** first.

- **root** - **`/usr/local/bin/foxxycode`**
- **non-root** - **`~/.local/bin/foxxycode`** (ensure that directory is on **`PATH`**)

```bash
make build TAGS="http ui scheduler memory cli browser gateway swarm"
make install
```

## Update from GitHub Releases

See **[docs/getting-started/update.md](../getting-started/update.md)** for **`foxxycode update`**, release asset names, and how that differs from **`make install`**.

## Lean build (ACP-focused, smaller binary)

Plain **`make build`** (empty **`TAGS`**) omits **`external/httpserver`**, the embedded UI, **`external/scheduler`**, and **`external/memory`**. You still get **`foxxycode acp`**, core tools, and MCP.

```bash
make build
```

Use this when you only need stdio ACP and want fewer dependencies and no **`npm`** step.

## Version string (`LDFLAGS`, `print-version`)

The Makefile sets:

```text
LDFLAGS := -X github.com/hijera/foxxycode-agent/internal/version.Version=$(VERSION)
```

**`VERSION`** is resolved from git (tag at **HEAD**, else **`git describe`**, else **`dev`**). Print the same value the next **`make build`** would embed:

```bash
make -s print-version
```

Manual one-liner aligned with **`make build`**:

```bash
go build \
  -tags=http,ui,scheduler,memory,cli \
  -ldflags "-X github.com/hijera/foxxycode-agent/internal/version.Version=$(make -s print-version)" \
  -o build/foxxycode \
  ./cmd/foxxycode/
```

## **`TAGS` vs `go build -tags`**

In **`Makefile`**, **`TAGS`** is **space-separated**:

```bash
make build TAGS="http ui scheduler memory cli browser gateway swarm"
```

**`go build`** expects a **comma-separated** list (no spaces):

```bash
go build -tags=http,ui,scheduler,memory,cli ...
```

Order does not matter for these tags.

## Build tags reference

| Tag | Enables | Documentation |
|-----|---------|----------------|
| **`memory`** | Long-term memory copilot; with **`http`**, **`/foxxycode/sessions/{id}/memory/*`** REST; toggle runtime behavior with **`memory.enable`** | [`external/memory/README.md`](../../external/memory/README.md) |
| **`http`** | **`foxxycode http`**, OpenAI-shaped REST gateway, **`/docs`**, **`/openapi.yaml`** | [`docs/reference/http-api.md`](../reference/http-api.md) · [`external/httpserver/`](../../external/httpserver) |
| **`ui`** | Embedded SPA on **`/`** (requires **`http`**; **`/`** returns **404** with **`http`** only) | [`docs/surfaces/web-ui.md`](../surfaces/web-ui.md) · [`DESIGN.md`](../../DESIGN.md) |
| **`scheduler`** | Scheduler daemon hooks, **`foxxycode_scheduler_*`** tools; with **`http`**, **`/foxxycode/scheduler`** REST | [`docs/operate/scheduler.md`](../operate/scheduler.md) · [`external/scheduler/README.md`](../../external/scheduler/README.md) |
| **`gateway.telegram`** | **`foxxycode gateway`** subcommand with Telegram bot adapter; per-user/group sessions, access control | [`docs/surfaces/gateway.md`](../surfaces/gateway.md) · [`external/gateway/`](../../external/gateway) |
| **`gateway`** | All messenger adapters (superset of **`gateway.telegram`**; includes future Discord, Slack adapters) | [`docs/surfaces/gateway.md`](../surfaces/gateway.md) |
| **`desktop`** | Windows WebView2 desktop app (**`foxxycode desktop`**); combine with **`http`**, **`ui`** | [`docs/contributing/build.md`](build.md#desktop-windows-webview2) |
| **`cli`** | Interactive console TUI (bare **`foxxycode`**, **`foxxycode cli`**) | [`docs/surfaces/console.md`](../surfaces/console.md) · [`external/cli/`](../../external/cli) |
| **`browser`** | Interactive browser tool (**`browser_action`**, chromedp) | [`docs/features/browser-tool.md`](../features/browser-tool.md) |
| **`swarm`** | Swarm relay served by **`foxxycode serve`**; agents register into it and relays chain | [`docs/operate/swarm.md`](../operate/swarm.md) · [`external/swarm/`](../../external/swarm) |

Tests come at two speeds. **`make test`** is the express run: the SPA assets and suite, then one
**`go test`** over the whole tree with every optional module compiled in — what the shipped binary
contains. The combinations themselves live in **`TEST_TAG_SETS`** ([`Makefile`](../../Makefile)):
**`make test-matrix`** walks them in sequence, and CI runs one job per combination —
**`.github/workflows/tests-on-pr.yaml`** reads the list through **`make print-test-tag-sets`**, so
it cannot drift from the Makefile. Locally, reach for a single combination instead:
**`go test -tags=<set> ./...`**. Two of those jobs also start the built binary: `cli` drives the
console through a real pty (`examples/cli/cli_e2e_startup.py`), `gateway` runs the Telegram bot
against the offline stand of `cmd/tgfake` (`examples/gateway/tg_e2e_offline.sh`).

**`make test-race`** runs the race detector over every non-UI Go tag in one pass. CI runs it on pull requests; the UI tag only embeds the already built SPA.

## Desktop (Windows WebView2)

Build a GUI **`foxxycode-desktop.exe`** that embeds the SPA in an Edge WebView2 window (no separate browser, no console window):

```bash
make build-desktop
# -> build/foxxycode-desktop.exe
```

Or cross-compile from Linux/macOS (pure Go, **`CGO_ENABLED=0`**):

```bash
python scripts/build.py --target cli --preset desktop
```

**Requirements:** Windows 10+, [WebView2 Runtime](https://developer.microsoft.com/en-us/microsoft-edge/webview2/) (preinstalled on most Win10/11 images). Logs go to **`~/.foxxycode/desktop.log`** because **`-H=windowsgui`** hides stdout/stderr. Owning no console also means every console tool a turn runs (git, ripgrep, the shell behind **`run_command`**) would be given a console window of its own, so they are all started windowless through **`platform.HideConsoleWindow`**; see **`docs/contributing/architecture.md`**.

**First run:** double-click opens **`/#/chat`** with a provider picker modal (pattern inspired by NeuralDeskApp). Save writes **`config.yaml`** via **`PUT /foxxycode/config`**.

**Licensing note:** desktop builds link [`github.com/jchv/go-webview2`](https://github.com/jchv/go-webview2) (license **`Other/NOASSERTION`** in **`go.mod`**). Review before redistribution.

**Manual smoke:** delete **`%USERPROFILE%\.foxxycode\config.yaml`**, run **`build\foxxycode-desktop.exe`**, complete onboarding, restart and confirm the modal stays closed.

## Distribution packages

**`make deb`** and **`make rpm`** build the Linux packages a release publishes; **`make brew`**
renders the Homebrew cask for one.

```bash
make deb
make rpm
```

Output: **`dist/foxxycode_<version>_linux_<arch>.deb`** and **`.rpm`**. Knobs:

| Variable | Default | What |
|----------|---------|------|
| **`PKG_ARCHS`** | host **`GOARCH`** | architectures to package, e.g. **`"amd64 arm64"`** |
| **`PKG_TAGS`** | **`FULL_TAGS`** (**`http ui scheduler memory cli browser gateway swarm`**) | build tags for the packaged binary |
| **`DIST_DIR`** | **`dist`** | where the packages land |

```bash
make deb PKG_ARCHS="amd64 arm64"
make rpm PKG_TAGS="http cli"      # lean binary, no npm step
```

The recipe is **`packaging/nfpm.yaml`**, driven by **`scripts/build-packages.sh`**, which stages the
man page (**`packaging/man/foxxycode.1`**), the shell completions (**`packaging/completions/`**),
the systemd user unit (**`packaging/systemd/foxxycode.service`**), the maintainer scripts
(**`packaging/scripts/postinstall.sh`**, **`preremove.sh`**),
**`config.example.yaml`** and **`LICENSE`** into one directory and runs
[nfpm](https://nfpm.goreleaser.com/) over it. nfpm is not a module dependency: the script uses the
**`nfpm`** on **`PATH`** when there is one and otherwise fetches the pinned version with
**`go run`**, so there is nothing to install first.

The user unit is installed but not enabled, and the post-install message says so; each user who
wants the service runs **`foxxycode serve install`**. No system service, system account or files under
**`/etc`** are created, because each user keeps state under **`~/.foxxycode`**. The unit is not
written by hand: **`serve.PackagedUnitFile()`** (**`internal/serve/systemd.go`**) renders it, the
same function that writes the unit for a script install, and **`TestPackagedUnitIsTheRenderedOne`**
fails until **`packaging/systemd/foxxycode.service`** matches it byte for byte. The **Distribution
packages** CI job then checks that both package formats carry that file and the maintainer
scripts.

Version strings are normalised for the two formats by **`scripts/package-version.sh`** - rpm forbids
**`-`** in a version and dpkg reads the last one as the start of the Debian revision, so
**`0.2.59-5-gb6b7d31-dirty`** is packaged as **`0.2.59+5.gb6b7d31.dirty`**, which both accept and both
sort after **`0.2.59`**.

### Homebrew cask

```bash
make brew VERSION=0.2.63
```

**`scripts/build-homebrew-cask.sh`** fills **`packaging/homebrew/foxxycode.rb.tmpl`** with the version
and the SHA-256 of both macOS archives, writing **`dist/foxxycode.rb`**. It takes those archives from
**`DIST_DIR`** when they are there (which is the case in the release job, right after the
cross-compile) and downloads them from the GitHub release otherwise - a cask pins checksums, so it
can only be rendered for a version whose archives exist.

Install the rendered file to try it:

```bash
brew install --cask dist/foxxycode.rb
```

The cask links **`foxxycode`**, **`foxxycode.1`** and both completion scripts, which is why the release
**`darwin`** and **`linux`** archives carry those files beside the binary. Each release publishes
**`foxxycode.rb`** as an asset, and **`brew install --cask <url>`** installs from it.

### Homebrew formula

```bash
make brew-formula VERSION=0.2.63
make brew-check VERSION=0.2.63
```

**`scripts/build-homebrew-formula.sh`** fills **`packaging/homebrew/foxxycode-formula.rb.tmpl`** with the
version and the SHA-256 of that tag's **source** archive, writing **`dist/formula/foxxycode.rb`**. It
downloads the archive to hash it, so the version has to be a published tag.

That file is the artefact a [homebrew/core](https://github.com/Homebrew/homebrew-core) pull request
carries. Homebrew routes open-source command-line software there as a formula built from source and
keeps homebrew/cask for native applications and binary-only software, so the cask above is our own
channel and the formula is the submission. The formula builds the release tag set, which is why
**`node`** joins **`go`** as a build dependency: the embedded SPA is generated rather than committed.

**`scripts/check-homebrew-submission.sh`** (**`make brew-check`**) is the preflight - token
availability, notability thresholds, the release, and the rendered formula. It exits non-zero when
something blocks the submission. The full path, including the notability arithmetic that blocks a
self-submission today, is in [homebrew.md](../getting-started/homebrew.md).

What the packages install, and how they interact with **`foxxycode update`**, is documented in
[install.md](../getting-started/install.md#linux-packages-deb-rpm) and
[update.md](../getting-started/update.md#installations-owned-by-a-package-manager).

## Windows regression checks

Windows test runs support CRLF checkouts: CSS source assertions normalize line
endings before matching selectors, and embedded-documentation heading assertions
accept either line ending. The SPA-wide source audit has a bounded
15-second timeout for slower filesystem scans under parallel load. These test
accommodations do not change the shipped UI. Reverse-tunnel tests also exercise
an explicitly initialized HTTP/2 transport, including the Go 1.27 implementation
of `x/net/http2`, rather than assuming a zero-value transport can adopt a socket.
The background-wake permission scenario selects a file-creation command for the
detected shell, so it does not require Unix `touch` on Windows. The MCP reload
test allows five seconds for each subprocess response while still checking that
the connection survives cancellation of the reload context.

For a fresh Go run without reusing test results, use:

```bash
go test -count=1 "-tags=http,ui,scheduler,memory,cli,browser,gateway,swarm" ./...
```

Build the embedded SPA first (`make ui-build`); `make test` also runs the UI suite.

## Preparing an upstream port

The repository helper [scripts/upstream.py](../../scripts/upstream.py) inventories
changes from coddy-agent before a manual port into FoxxyCode. It needs Python
3.10+, Git and PyYAML for reading the port ledger. Install the dependency with
`python -m pip install -r scripts/requirements-upstream.txt`. Run it from the repository
root; use `--repo <checkout>` before the subcommand to inspect another checkout.

```bash
python scripts/upstream.py scan --from <upstream-base-sha> --to <upstream-target-sha>
python scripts/upstream.py preview --from <upstream-base-sha> --to <upstream-target-sha> --fork-base HEAD
python scripts/upstream.py check --base <fork-base-sha>
python scripts/upstream.py ledger-check
```

Fetch upstream explicitly before scanning a remote ref. `scan`, `check` and
`ledger-check` are read-only apart from an optional output report. `preview`
can also write an explicitly requested artifact bundle. The helper
never fetches, applies patches, commits, pushes or runs tests. `scan`
requires an explicit ancestor range and records full commit IDs. Its candidate
list includes merge commits (first-parent diffs), so related entries can overlap.
It retains paths touched by intermediate commits even if a later commit reverts
them, and includes both old and new paths for renames.

`check` compares the fork base with the working tree, including staged changes
and non-ignored untracked paths. Both commands report matching divergence
registry entries, named guard tests and follow-up checks. The checker also reads
the base registry and source markers so deleting a guard during a port does not
hide the review requirement. Reports map `coddy` to `foxxycode`, `Coddy` to
`FoxxyCode` and `CODDY` to `FOXXYCODE` in directory and file names, checking both
original and mapped paths for protected behavior. For example,
`cmd/coddy/main.go` is checked against `cmd/foxxycode/main.go`. JSON exposes
`path_mappings`, `fork_files` and the `branding` replacement rules.

Apply the same rebranding to imported code: identifiers, imports, string
literals, environment prefixes, CLI names and routes, with corresponding
references, tests and documentation. Preserve upstream provenance and explicit
compatibility contracts; module/repository owner changes must use the fork's
actual addresses. The helper reports mappings without rewriting files. Inspect
other renames, mapping collisions and indirect dependencies manually. A clean
report does not prove that fork behavior is preserved.

Add `--format json` for machine-readable output, or `--output <new-file>` to a
read-only command to
write a UTF-8 report. Existing output files are never overwritten. Exit status
0 means a report was produced, not that checks passed; input, Git and output
errors return 2. Reports explicitly indicate that no tests were executed.

The versioned [ports.yaml](../../ports.yaml) records reviewed commits. Historical
decisions cover the journal's waves from `bc1afb9` through `4b091359` (upstream
1.1.61), selected earlier ports identified by fork commit messages, and the
partially ported PR #351. Full Git IDs and fork ancestry were checked during
migration. An `adapted` historical record describes a consolidated wave or PR,
including deliberate fork differences; it does not claim a patch-identical
cherry-pick or a fresh semantic audit. The ledger notes state its coverage.
An absent entry is `unreviewed`, not necessarily unported. Do not infer port
status from Git ancestry after a manual or adapted port. A record has this shape
(replace the SHA placeholders with actual full lowercase Git commit IDs):

```yaml
version: 1
upstream: https://github.com/coddy-project/coddy-agent
ports:
  - upstream_sha: '<full upstream SHA>'
    status: partial
    fork_commits: ['<full fork SHA>']
    reason: Recovery was adapted to the fork continuation path.
    remaining:
      - Port the provider status API.
```

Allowed statuses are `ported`, `adapted`, `partial`, `deferred` and `skipped`.
Every record requires a reason. Ported/adapted/partial records require fork
commits as evidence; partial/deferred records require a nonempty remaining list.
Completed/skipped records have `remaining: []`. Update a record when its status
changes; duplicate upstream SHAs, duplicate YAML keys and unknown fields are
errors. Optional top-level `notes` holds text about ledger coverage.

`scan` retains all commits for review and adds their `port` status. JSON also
exposes `pending_commits` (unreviewed/partial/deferred SHAs in the range) and
`backlog` (all partial/deferred entries outside it, whose relevance to the target
must be checked manually). File and divergence reports still cover the whole
range, including recorded ports. `check` also displays the open backlog.

`--ledger <file>` selects a ledger relative to the inspected repository. An
absent default ledger is tolerated by scan/check; a missing explicit ledger or
a missing ledger for `ledger-check` is an error. `ledger-check` validates YAML
and record structure only: it does not fetch or verify commit existence,
ancestry, or behavior. Edit the ledger after verifying the port, then run the
validator. The helper never advances a synchronization checkpoint automatically.

### Preview normalized three-way merges

```bash
python scripts/upstream.py preview --from 1.2.18 --to 1.3.0 --fork-base HEAD --bundle port-preview
```

`preview` compares the net upstream range against a committed fork revision
without creating a branch or worktree, applying results, or updating the ledger.
Fetch the refs explicitly first; the base must be an ancestor of the target.
All three refs are pinned to full commit IDs in the report. Uncommitted changes
are excluded even when `--fork-base` is `HEAD`.

The trial maps upstream path branding, normalizes UTF-8 text from CRLF to LF,
and mechanically rebrands the upstream base and target. Existing fork text
receives only line-ending normalization, so fork compatibility values such as
`engine: coddy` are not rewritten merely because they occur in ours. Content
mappings distinguish the Go module
`github.com/hijera/foxxycode-agent` from repository URLs pointing to
`https://github.com/hijera/foxxy-agent`, for both upstream owners
`EvilFreelancer` and `coddy-project`. These mappings also appear in scan/check
JSON and apply to UTF-8 hunk contents in `prepare`'s adapted patch. Other owners,
raw/download URLs, semantic values, aliases and provenance still need review.

Git's `merge-file --diff3` merges regular text blobs using the explicit upstream
base. The Markdown and JSON reports distinguish clean merges, text conflicts,
clean additions/deletions, add/add conflicts, modify/delete conflicts, rename
destination collisions, binary files, non-UTF-8 content, and special Git entries
such as symlinks and submodules. Renames use Git's similarity detection; both
paths, input object IDs and modes are retained. The proposed executable mode is
recorded for merged candidates. An upstream rename whose old and destination
paths both exist in the fork requires manual review. No external decoder is
used for Git blobs; undecodable content is retained as raw bytes.

Without `--bundle`, the command writes only its report to stdout or an exclusive
`--output` file. With `--bundle`, it creates a new directory relative to the
inspected repository (or the absolute path supplied), containing `preview.json`,
`preview.md` and numbered `files/` directories. Each row names its normalized
base/ours/theirs artifacts and a merged candidate when available. Unsupported
content is saved as `.raw`; special entries are never materialized as links.
Numbered artifact paths avoid interpreting repository filenames as output paths.
Existing bundle paths, Git metadata destinations and upstream branding path
collisions are rejected. A failed write reports the incomplete bundle's location
and leaves it for inspection.

Exit 0 means the preview completed, including when conflicts require review;
Git, input or output errors return 2. Conflict counts measure textual overlap,
not porting effort or semantic correctness. The net range includes previously
recorded ports; review the ledger and the scan's divergence/guard report before
using any candidate. Do not apply both merge-commit patches and their overlapping
constituent patches. The preview does not fetch, run tests, advance a checkpoint,
or prove that a fork policy survived.

### Prepare a worktree and patches

```bash
python scripts/upstream.py prepare --wave recovery --from <upstream-base> --to <target> --fork-base HEAD --worktree worktrees/recovery --dry-run
```

Remove `--dry-run` to create a fresh branch `codex/upstream-recovery` and worktree.
The worktree path is relative to the inspected repository unless absolute.
Existing paths or branches and branding path collisions are rejected before
creation. The source checkout can be dirty: only the committed `--fork-base`
(default `HEAD`) is checked out, and local edits are not copied or discarded.

The returned `bundle` path is inside the new worktree's private Git directory,
so reports do not appear as source changes. It contains `original.patch`,
`adapted.patch`, `scan.json`, `scan.md` and `manifest.json` with pinned SHAs,
plus a `preview/` directory with the three-way inputs, candidates and reports
described above. The manifest includes preview disposition and conflict counts.
`--dry-run` does not compute or write the three-way trial.
The patches describe the net upstream range, including recorded ports; choose
the needed hunks after reviewing partial/completed entries. The adapted patch
mechanically rebrands paths and UTF-8 text, leaving binary and non-UTF-8 payloads
unchanged with warnings. It is a review candidate, not a semantic port: correct
compatibility values, provenance and any repository addresses outside the explicit
owner mappings before applying it.

Run `git apply --check <bundle>/adapted.patch` in the new worktree after review.
Adapted text patches omit obsolete blob IDs and must not use `--3way`. Neither
patch is applied automatically. A bundle write failure leaves the new worktree
and branch in place and reports their location, rather than deleting possible
work. Git's ordinary worktree lifecycle applies; preserve any needed bundle
before removing the worktree.

### Record a reviewed result

```bash
python scripts/upstream.py record --upstream-commit <source> --status adapted --fork-commit <result> --reason "Kept fork continuation behavior" --dry-run
```

Remove `--dry-run` to update the existing ledger. Refs are resolved to full SHAs;
every fork commit must be an ancestor of the inspected checkout's `HEAD`.
Repeat `--fork-commit` for several result commits and `--remaining` for individual
unfinished items. `--update` is required to replace a different existing record
and supplies the entire replacement decision. An identical record is a no-op.
Commit existence and ancestry do not prove semantic correctness.

The ledger must be a regular file within the inspected repository. Recording
uses an exclusive `<ledger>.lock`, a same-directory temporary file and atomic
replacement; invalid input or an existing lock leaves the ledger intact.
Concurrent edits detected before replacement are rejected. Remove a stale lock
only after confirming its writer is no longer running. YAML is serialized again,
so formatting and comments may change; top-level notes and other records remain.
`--dry-run` neither locks nor writes. If the worktree does not yet contain the
helper, invoke it from the original checkout with `--repo <worktree>`.

The repository skill
[upstream-port](../../.claude/skills/upstream-port/SKILL.md) guides the manual
workflow and refers to the existing Cursor rules. The
[upstream-review](../../.claude/skills/upstream-review/SKILL.md) skill audits the
original upstream changes against the fork implementation and ledger evidence.
Agents that do not discover
`.claude/skills` can be asked to read that file explicitly. Run the helper's
offline integration suite with:

```bash
python -m unittest discover -s scripts -p "test_upstream*.py"
```

The [Upstream helper workflow](../../.github/workflows/upstream-helper.yaml) runs
this suite on Linux and Windows with Python 3.10 when helper sources change.

Prompt-cache regressions run separately with `make test-cache`; context and timing benchmarks run with `make test-perf`.

## Release binaries (CI)

On each SemVer git tag **`X.Y.Z`** that is on **`main`**, the [**Release binaries**](../../.github/workflows/release-binaries.yaml) workflow (separate from Docker CI) uploads archives to the matching **GitHub Release**:

| Archive | Platform |
|---------|----------|
| **`foxxycode_X.Y.Z_linux_amd64.tar.gz`** | Linux x86_64 |
| **`foxxycode_X.Y.Z_linux_arm64.tar.gz`** | Linux arm64 |
| **`foxxycode_X.Y.Z_windows_amd64.zip`** | Windows x86_64 (**`foxxycode.exe`**) |
| **`foxxycode_X.Y.Z_darwin_amd64.tar.gz`** | macOS Intel |
| **`foxxycode_X.Y.Z_darwin_arm64.tar.gz`** | macOS Apple Silicon |
| **`SHA256SUMS`** | Checksums for the archives above |

Tags match the full feature set: **`http`**, **`ui`**, **`scheduler`**, **`memory`**, **`cli`**, **`browser`**, **`gateway`**. Manual run after a tag exists:

```bash
gh workflow run "Release binaries" --ref X.Y.Z -f tag=X.Y.Z
```

## **`go install` from upstream**

```bash
go install github.com/hijera/foxxycode-agent/cmd/foxxycode@latest
```

That compiles whatever the module default is **without** your local **`TAGS`**. For a known set of features (HTTP, UI, scheduler, memory), clone the repo and use **`make build TAGS="http ui scheduler memory cli browser gateway swarm"`** (or **`go build -tags=...`** as above).
