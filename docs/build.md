# Building FoxxyCode from source

This page is the detailed reference for local builds. For a short version, see [Installation](../README.md#installation) in the root **README**.

## Prerequisites

- **Go** - match `go` in [`go.mod`](../go.mod) (currently **1.25**).
- **Git** - the Makefile embeds a version string from tags or `git describe` when available.
- **Node.js and npm** - required only when you build with both **`http`** and **`ui`**, because the Makefile runs **`ui-build`** (see [`Makefile`](../Makefile)) to produce the assets that **`go:embed`** picks up.

Optional:

- **`golangci-lint` v2.x** (built with Go **1.25** or newer) - for **`make lint`**, which runs an untagged pass plus one per optional build tag (**`cli`**, **`browser`**, **`gateway`**, **`http,scheduler,memory,gateway`**); **`make lint-ui`** adds the embedded-SPA pass and **`make lint-windows`** the Windows one. CI installs the pinned version with **`go install`** and calls the same targets, so local and CI coverage cannot drift.
- **Python 3.8+** - only for the interactive build wizard ([`scripts/build.py`](../scripts/build.py)); stdlib only, no `pip` packages.

## Interactive build wizard

On **Windows** (and anywhere without GNU Make on `PATH`), use the Russian-language console wizard
[`scripts/build.py`](../scripts/build.py). It wraps the same steps as **`make build`**, Gradle
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


Build with **`memory`** to link long-term memory (`external/memory`). Enable behavior at runtime with **`memory.enabled`** in config (see [`external/memory/README.md`](../external/memory/README.md)).

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

The [**Dockerfile**](../Dockerfile) uses the same idea: comma-separated tags via **`BUILD_TAGS`** (default **`http,scheduler,ui,memory,gateway,cli,browser,swarm`**) and strips debug symbols with **`-ldflags "-s -w ..."`** in addition to the version **`X`** flag.

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

See **[docs/update.md](update.md)** for **`foxxycode update`**, release asset names, and how that differs from **`make install`**.

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
| **`memory`** | Long-term memory copilot; with **`http`**, **`/foxxycode/sessions/{id}/memory/*`** REST; toggle runtime behavior with **`memory.enabled`** | [`external/memory/README.md`](../external/memory/README.md) |
| **`http`** | **`foxxycode http`**, OpenAI-shaped REST gateway, **`/docs`**, **`/openapi.yaml`** | [`docs/http-api.md`](http-api.md) · [`external/httpserver/`](../external/httpserver/) |
| **`ui`** | Embedded SPA on **`/`** (requires **`http`**; **`/`** returns **404** with **`http`** only) | [`docs/ui.md`](ui.md) · [`DESIGN.md`](../DESIGN.md) |
| **`scheduler`** | Scheduler daemon hooks, **`foxxycode_scheduler_*`** tools; with **`http`**, **`/foxxycode/scheduler`** REST | [`docs/scheduler.md`](scheduler.md) · [`external/scheduler/README.md`](../external/scheduler/README.md) |
| **`gateway.telegram`** | **`foxxycode gateway`** subcommand with Telegram bot adapter; per-user/group sessions, access control | [`docs/gateway.md`](gateway.md) · [`external/gateway/`](../external/gateway/) |
| **`gateway`** | All messenger adapters (superset of **`gateway.telegram`**; includes future Discord, Slack adapters) | [`docs/gateway.md`](gateway.md) |
| **`desktop`** | Windows WebView2 desktop app (**`foxxycode desktop`**); combine with **`http`**, **`ui`** | [`docs/build.md`](build.md#desktop-windows-webview2) |

**`make test`** exercises tag combinations (see **`test`** target in [`Makefile`](../Makefile)).

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

**Requirements:** Windows 10+, [WebView2 Runtime](https://developer.microsoft.com/en-us/microsoft-edge/webview2/) (preinstalled on most Win10/11 images). Logs go to **`~/.foxxycode/desktop.log`** because **`-H=windowsgui`** hides stdout/stderr. Owning no console also means every console tool a turn runs (git, ripgrep, the shell behind **`run_command`**) would be given a console window of its own, so they are all started windowless through **`platform.HideConsoleWindow`**; see **`docs/architecture.md`**.

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
**`config.example.yaml`** and **`LICENSE`** into one directory and runs
[nfpm](https://nfpm.goreleaser.com/) over it. nfpm is not a module dependency: the script uses the
**`nfpm`** on **`PATH`** when there is one and otherwise fetches the pinned version with
**`go run`**, so there is nothing to install first.

The package installs a binary and its documentation and nothing else - no service, no system
account, no files under **`/etc`** - because FoxxyCode's state lives in the invoking user's
**`~/.foxxycode`**.

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
self-submission today, is in [homebrew.md](homebrew.md).

What the packages install, and how they interact with **`foxxycode update`**, is documented in
[install.md](install.md#linux-packages-deb-rpm) and
[update.md](update.md#installations-owned-by-a-package-manager).

## Release binaries (CI)

On each SemVer git tag **`X.Y.Z`** that is on **`main`**, the [**Release binaries**](../.github/workflows/release-binaries.yaml) workflow (separate from Docker CI) uploads archives to the matching **GitHub Release**:

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
