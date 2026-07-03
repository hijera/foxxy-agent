# Building Coddy from source

This page is the detailed reference for local builds. For a short version, see [Installation](../README.md#installation) in the root **README**.

## Prerequisites

- **Go** - match `go` in [`go.mod`](../go.mod) (currently **1.25**).
- **Git** - the Makefile embeds a version string from tags or `git describe` when available.
- **Node.js and npm** - required only when you build with both **`http`** and **`ui`**, because the Makefile runs **`ui-build`** (see [`Makefile`](../Makefile)) to produce the assets that **`go:embed`** picks up.

Optional:

- **`golangci-lint` v2.x** (built with Go **1.25** or newer) - for **`make lint`**. CI uses **`golangci/golangci-lint-action@v7`** or newer (v6 supports only golangci-lint v1).
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
# Full-feature CLI for the current host (same as make build TAGS="http ui scheduler memory")
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
| CLI (single platform) | **`build/foxxy`** or **`build/foxxy.exe`** |
| CLI (all release) | **`dist/foxxy_<version>_<os>_<arch>.{tar.gz,zip}`** + **`dist/SHA256SUMS`** |
| IntelliJ | **`editors/intellij/build/distributions/*.zip`** |
| VS Code | **`editors/vscode/*.vsix`** (one per **`--vscode-target`**) |

**Tag presets:** **`lean`** (no tags), **`full`** (`http ui scheduler memory`), **`gateway`**
(adds **`gateway.telegram`**). Custom tags: **`--tags http,scheduler`** (comma-separated; **`ui`**
requires **`http`**).

Run **`python scripts/build.py --help`** for the full flag list (Russian descriptions).


Build with **`memory`** to link long-term memory (`external/memory`). Enable behavior at runtime with **`memory.enabled`** in config (see [`external/memory/README.md`](../external/memory/README.md)).

The **HTTP gateway**, **embedded SPA**, **scheduler**, and **memory** are controlled by Go build tags. For a single binary that matches the default **Docker** image and includes every optional feature:

```bash
make build TAGS="http ui scheduler memory"
```

Output: **`build/coddy`**.

Equivalent **`go build`** (after `ui-build` when you use **`ui`**, or use **`make build`**, which runs **`ui-build`** automatically when **`TAGS`** contains both **`http`** and **`ui`**):

```bash
make ui-build   # only when using -tags=...,ui,... with http; Makefile runs this for you on `make build`
VERSION="$(make -s print-version)"
go build -tags=http,ui,scheduler,memory \
  -ldflags "-X github.com/hijera/foxxy-agent/internal/version.Version=${VERSION}" \
  -o build/coddy \
  ./cmd/coddy/
```

The [**Dockerfile**](../Dockerfile) uses the same idea: comma-separated tags via **`BUILD_TAGS`** (default **`http,scheduler,ui,memory`**) and strips debug symbols with **`-ldflags "-s -w ..."`** in addition to the version **`X`** flag.

## Install on your PATH

**`make install`** copies **`build/coddy`** onto your **`PATH`**:

- If **`build/coddy`** already exists (for example after **`make build TAGS="http ui scheduler memory"`**), it is installed as-is without rebuilding.
- If the binary is missing, **`make install`** runs **`make build TAGS="http ui scheduler memory"`** first.

- **root** - **`/usr/local/bin/coddy`**
- **non-root** - **`~/.local/bin/coddy`** (ensure that directory is on **`PATH`**)

```bash
make build TAGS="http ui scheduler memory"
make install
```

## Update from GitHub Releases

See **[docs/update.md](update.md)** for **`coddy update`**, release asset names, and how that differs from **`make install`**.

## Lean build (ACP-focused, smaller binary)

Plain **`make build`** (empty **`TAGS`**) omits **`external/httpserver`**, the embedded UI, **`external/scheduler`**, and **`external/memory`**. You still get **`coddy acp`**, core tools, and MCP.

```bash
make build
```

Use this when you only need stdio ACP and want fewer dependencies and no **`npm`** step.

## Version string (`LDFLAGS`, `print-version`)

The Makefile sets:

```text
LDFLAGS := -X github.com/hijera/foxxy-agent/internal/version.Version=$(VERSION)
```

**`VERSION`** is resolved from git (tag at **HEAD**, else **`git describe`**, else **`dev`**). Print the same value the next **`make build`** would embed:

```bash
make -s print-version
```

Manual one-liner aligned with **`make build`**:

```bash
go build \
  -tags=http,ui,scheduler,memory \
  -ldflags "-X github.com/hijera/foxxy-agent/internal/version.Version=$(make -s print-version)" \
  -o build/coddy \
  ./cmd/coddy/
```

## **`TAGS` vs `go build -tags`**

In **`Makefile`**, **`TAGS`** is **space-separated**:

```bash
make build TAGS="http ui scheduler memory"
```

**`go build`** expects a **comma-separated** list (no spaces):

```bash
go build -tags=http,ui,scheduler,memory ...
```

Order does not matter for these tags.

## Build tags reference

| Tag | Enables | Documentation |
|-----|---------|----------------|
| **`memory`** | Long-term memory copilot; with **`http`**, **`/coddy/sessions/{id}/memory/*`** REST; toggle runtime behavior with **`memory.enabled`** | [`external/memory/README.md`](../external/memory/README.md) |
| **`http`** | **`coddy http`**, OpenAI-shaped REST gateway, **`/docs`**, **`/openapi.yaml`** | [`docs/http-api.md`](http-api.md) · [`external/httpserver/`](../external/httpserver/) |
| **`ui`** | Embedded SPA on **`/`** (requires **`http`**; **`/`** returns **404** with **`http`** only) | [`docs/ui.md`](ui.md) · [`DESIGN.md`](../DESIGN.md) |
| **`scheduler`** | Scheduler daemon hooks, **`coddy_scheduler_*`** tools; with **`http`**, **`/coddy/scheduler`** REST | [`docs/scheduler.md`](scheduler.md) · [`external/scheduler/README.md`](../external/scheduler/README.md) |
| **`gateway.telegram`** | **`coddy gateway`** subcommand with Telegram bot adapter; per-user/group sessions, access control | [`docs/gateway.md`](gateway.md) · [`external/gateway/`](../external/gateway/) |
| **`gateway`** | All messenger adapters (superset of **`gateway.telegram`**; includes future Discord, Slack adapters) | [`docs/gateway.md`](gateway.md) |

**`make test`** exercises tag combinations (see **`test`** target in [`Makefile`](../Makefile)).

## Release binaries (CI)

On each SemVer git tag **`X.Y.Z`** that is on **`main`**, the [**Release binaries**](../.github/workflows/release-binaries.yaml) workflow (separate from Docker CI) uploads archives to the matching **GitHub Release**:

| Archive | Platform |
|---------|----------|
| **`coddy_X.Y.Z_linux_amd64.tar.gz`** | Linux x86_64 |
| **`coddy_X.Y.Z_linux_arm64.tar.gz`** | Linux arm64 |
| **`coddy_X.Y.Z_windows_amd64.zip`** | Windows x86_64 (**`coddy.exe`**) |
| **`coddy_X.Y.Z_darwin_amd64.tar.gz`** | macOS Intel |
| **`coddy_X.Y.Z_darwin_arm64.tar.gz`** | macOS Apple Silicon |
| **`SHA256SUMS`** | Checksums for the archives above |

Tags match the full feature set: **`http`**, **`ui`**, **`scheduler`**, **`memory`**. Manual run after a tag exists:

```bash
gh workflow run "Release binaries" --ref X.Y.Z -f tag=X.Y.Z
```

## **`go install` from upstream**

```bash
go install github.com/hijera/foxxy-agent/cmd/coddy@latest
```

That compiles whatever the module default is **without** your local **`TAGS`**. For a known set of features (HTTP, UI, scheduler, memory), clone the repo and use **`make build TAGS="http ui scheduler memory"`** (or **`go build -tags=...`** as above).
