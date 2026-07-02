# Updating Foxxy Agent

Use **`foxxy update`** to download official release binaries from [GitHub Releases](https://github.com/hijera/foxxy-agent/releases) and replace the **`foxxy`** executable you are running.

## What gets installed

CI publishes one archive per platform on each SemVer tag **`X.Y.Z`**:

| Archive | Platform |
|---------|----------|
| **`foxxy_X.Y.Z_linux_amd64.tar.gz`** | Linux x86_64 |
| **`foxxy_X.Y.Z_linux_arm64.tar.gz`** | Linux arm64 |
| **`foxxy_X.Y.Z_windows_amd64.zip`** | Windows x86_64 (**`foxxy.exe`**) |
| **`foxxy_X.Y.Z_darwin_amd64.tar.gz`** | macOS Intel |
| **`foxxy_X.Y.Z_darwin_arm64.tar.gz`** | macOS Apple Silicon |

Each binary is built with **`http`**, **`ui`**, **`scheduler`**, and **`memory`** (same as **`make build TAGS="http ui scheduler memory"`** and the default Docker image). See [Build from source](build.md#release-binaries-ci) for the release pipeline.

## Which file is replaced

**`foxxy update`** resolves **`os.Executable()`** (symlinks followed) and overwrites that path. Examples:

- After **`make install`** as a regular user, that is usually **`~/.local/bin/foxxy`**.
- When you run **`./build/foxxy update`**, it updates **`build/foxxy`** in the repo.

This differs from **`make install`**, which always copies to **`~/.local/bin`** or **`/usr/local/bin`**. To update the binary on **`PATH`**, invoke the same **`foxxy`** that **`which foxxy`** prints.

## Commands

Check for a newer release (exit **0** if up to date, **1** if a newer **`X.Y.Z`** exists):

```bash
foxxy -v
foxxy update --check
```

Install the latest release (prompt **`[y/N]`** unless **`-y`**):

```bash
foxxy update
foxxy update -y
```

Install a specific tag:

```bash
foxxy update --version 0.9.3
foxxy update --version 0.9.3 -y
```

Override the GitHub repository (default **`hijera/foxxy-agent`**):

```bash
foxxy update --repo hijera/foxxy-agent
```

All flags:

```bash
foxxy update --help
```

## Version comparison

**`foxxy -v`** may show a git describe string (for example **`0.9.2-5-gb6b7d31-dirty`**). **`foxxy update`** compares the leading **`X.Y.Z`** prefix to the release tag. A local **`dev`** build is treated as older than any published SemVer release.

## Other upgrade paths

| Method | When to use |
|--------|-------------|
| **`foxxy update`** | You already have a release binary on disk and want the next (or a specific) GitHub release. |
| **`make install`** | You built from a clone and want **`build/foxxy`** on **`PATH`**. |
| **`make build TAGS="..."`** | You need custom tags or local changes not in releases. |
| **Docker** | **`docker compose pull`** / image tag **`X.Y.Z`** on [GHCR](https://github.com/hijera/foxxy-agent/pkgs/container/foxxy-agent). |
| **`go install ...@latest`** | Quick install without release assets; default module tags only (no **`http`** / UI unless you build from source). |

## Limitations

- Only platforms listed in the release table are supported; others get a clear error.
- On Windows the running process may lock the executable; close other **`foxxy`** instances if replace fails.
- **`foxxy update`** needs outbound HTTPS to **`api.github.com`** and the asset CDN (GitHub release downloads).
- Config under **`$CODDY_HOME`** is not modified; only the binary is replaced.
