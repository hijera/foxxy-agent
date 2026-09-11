# Updating FoxxyCode Agent

Use **`foxxycode update`** to download official release binaries from [GitHub Releases](https://github.com/hijera/foxxycode-agent/releases) and replace the **`foxxycode`** executable you are running.

On Windows, FoxxyCode starts a short-lived helper from the system temporary directory. The helper waits for `foxxycode update` to exit, keeps a backup of the installed executable, replaces it, and starts the updated FoxxyCode again. The helper's status lines continue in the same `cmd.exe` or PowerShell console after the `update` command returns, so `foxxycode update` reports success once the download is staged, not once the swap has happened.

Every download is checked against the `SHA256SUMS` asset the release publishes before anything is unpacked or installed. A release without that asset is reported and installed anyway.

## What gets installed

CI publishes one archive per platform on each SemVer tag **`X.Y.Z`**:

| Archive | Platform |
|---------|----------|
| **`foxxycode_X.Y.Z_linux_amd64.tar.gz`** | Linux x86_64 |
| **`foxxycode_X.Y.Z_linux_arm64.tar.gz`** | Linux arm64 |
| **`foxxycode_X.Y.Z_windows_amd64.zip`** | Windows x86_64 (**`foxxycode.exe`**) |
| **`foxxycode_X.Y.Z_darwin_amd64.tar.gz`** | macOS Intel |
| **`foxxycode_X.Y.Z_darwin_arm64.tar.gz`** | macOS Apple Silicon |

Each binary is built with **`http`**, **`ui`**, **`scheduler`**, and **`memory`** (same as **`make build TAGS="http ui scheduler memory"`** and the default Docker image). See [Build from source](build.md#release-binaries-ci) for the release pipeline.

## Which file is replaced

**`foxxycode update`** resolves **`os.Executable()`** (symlinks followed) and overwrites that path. Examples:

- After **`make install`** as a regular user, that is usually **`~/.local/bin/foxxycode`**.
- When you run **`./build/foxxycode update`**, it updates **`build/foxxycode`** in the repo.

This differs from **`make install`**, which always copies to **`~/.local/bin`** or **`/usr/local/bin`**. To update the binary on **`PATH`**, invoke the same **`foxxycode`** that **`which foxxycode`** prints.

## Installations owned by a package manager

A **`foxxycode`** that **`apt`** or **`dnf`** put on disk is listed in the package database, file by file. Overwriting **`/usr/bin/foxxycode`** in place would leave that database describing a build that is gone, the next **`apt upgrade`** or **`dnf reinstall`** would quietly put the old version back, and **`dpkg --verify`** would report a checksum mismatch nobody asked for. So **`foxxycode update`** does not replace a packaged executable. It asks **`dpkg-query -S`** and **`rpm -qf`** who owns the file it is about to write and takes one of two other routes:

- **as an ordinary user** - it changes nothing, names the package that owns the installation, and prints the command that upgrades it (**`sudo apt-get install --only-upgrade foxxycode`**, **`sudo dnf upgrade foxxycode`**, ...). The exit code is **1**, so a script notices;
- **as root** - it downloads the **`.deb`** or **`.rpm`** for this architecture, verifies it against **`SHA256SUMS`** like any other asset, and hands it to the package manager it found (**`apt-get`**, **`apt`**, **`dnf`**, **`yum`**, **`zypper`**, else **`dpkg`** or **`rpm`**). The package database stays correct.

**Homebrew** is recognised too, on macOS and on Linux: an executable that resolves into a **`Caskroom`** or **`Cellar`** directory belongs to brew. There is no privileged route there - Homebrew refuses to run under **`sudo`** - so both root and an ordinary user are pointed at **`brew upgrade`**. Which of the two markers matched decides the flag: a **`Caskroom`** path is a cask and takes **`brew upgrade --cask foxxycode`**, a **`Cellar`** path is a formula and takes **`brew upgrade foxxycode`**. The distinction is not cosmetic - **`--cask`** on a formula install fails, there being no cask by that name to upgrade.

```bash
sudo foxxycode update -y
```

Ownership is asked of the package database, not guessed from the path, so a binary you copied out of **`/usr/bin`** keeps updating itself, a distribution's own rebuild of FoxxyCode is recognised like ours, and a build under **`~/.local/bin`** is untouched by any of this.

A release published before this pipeline has archives but no packages. Root is told exactly that and pointed back at the package manager, rather than being handed an archive that would overwrite the packaged file.

## Commands

Check for a newer release (exit **0** if up to date, **1** if a newer **`X.Y.Z`** exists):

```bash
foxxycode -v
foxxycode update --check
```

Install the latest release (prompt **`[y/N]`** unless **`-y`**):

```bash
foxxycode update
foxxycode update -y
```

Install a specific tag:

```bash
foxxycode update --version 0.9.3
foxxycode update --version 0.9.3 -y
```

Override the GitHub repository (default **`hijera/foxxycode-agent`**):

```bash
foxxycode update --repo hijera/foxxycode-agent
```

Install on Windows without starting FoxxyCode again afterwards - useful from a script or a CI step, where the restarted process has no console to run in:

```bash
foxxycode update -y --no-restart
```

All flags:

```bash
foxxycode update --help
```

## Version comparison

**`foxxycode -v`** may show a git describe string (for example **`0.9.2-5-gb6b7d31-dirty`**). **`foxxycode update`** compares the leading **`X.Y.Z`** prefix to the release tag. A local **`dev`** build is treated as older than any published SemVer release.

## Other upgrade paths

| Method | When to use |
|--------|-------------|
| **`foxxycode update`** | You already have a release binary on disk and want the next (or a specific) GitHub release. |
| **`make install`** | You built from a clone and want **`build/foxxycode`** on **`PATH`**. |
| **`make build TAGS="..."`** | You need custom tags or local changes not in releases. |
| **Docker** | **`docker compose pull`** / image tag **`X.Y.Z`** on [GHCR](https://github.com/hijera/foxxycode-agent/pkgs/container/foxxycode-agent). |
| **`go install ...@latest`** | Quick install without release assets; default module tags only (no **`http`** / UI unless you build from source). |

## Limitations

- Only platforms listed in the release table are supported; others get a clear error.
- On Windows, FoxxyCode waits up to 30 seconds for another process to release the executable. A permission failure reports much sooner and names the directory: installing into `Program Files` needs an elevated console. If the update cannot be installed, the current executable is left in place; if the updated binary cannot be started, FoxxyCode restores the backup.
- The Windows helper deletes itself through the FoxxyCode it just installed. Installing a release older than that handoff - `foxxycode update --version` walking backwards - starts the older build directly instead, and its helper stays in `%TEMP%` until the next `foxxycode update` sweeps it.
- Asset downloads resume after a temporary connection failure (up to three attempts). GitHub supports the HTTP range requests FoxxyCode uses to resume; a server that does not support ranges is downloaded again from the beginning, and one that resumes at the wrong offset fails the download rather than installing a spliced archive.
- **`foxxycode update`** needs outbound HTTPS to **`api.github.com`** and the asset CDN (GitHub release downloads).
- Config under **`$FOXXYCODE_HOME`** is not modified; only the binary is replaced.
