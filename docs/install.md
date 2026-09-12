# Install FoxxyCode Agent

FoxxyCode Agent is based on [coddy-agent](https://github.com/coddy-project/coddy-agent). The upstream
`coddy.dev` install scripts install the original project, not this fork — use one of the methods below.

## Release archive

Download the archive for your platform from
**[GitHub Releases](https://github.com/hijera/foxxycode-agent/releases)** (assets such as
`foxxycode_X.Y.Z_linux_amd64.tar.gz`), unpack it, and put the **`foxxycode`** binary on `PATH`
(Unix: `~/.local/bin`; Windows: `%LOCALAPPDATA%\Programs\foxxycode`).

Bootstrap the config when missing:

```bash
mkdir -p ~/.foxxycode && cp config.example.yaml ~/.foxxycode/config.yaml
```

## Linux packages (deb, rpm)

Every release publishes a **`.deb`** and an **`.rpm`** for **x86_64** and **arm64** beside the
archives, so FoxxyCode can be installed the way the rest of the system is - and removed the same way.
Prefer this over the install script on a machine you administer: the files are tracked by the
package database, the man page and shell completions are wired up for you, and `foxxycode update`
knows not to fight your package manager.

**Debian, Ubuntu and derivatives**

```bash
curl -fsSLO https://github.com/hijera/foxxy-agent/releases/latest/download/foxxycode_0.2.63_linux_amd64.deb
sudo apt-get install ./foxxycode_0.2.63_linux_amd64.deb
```

**Fedora, RHEL, openSUSE and derivatives**

```bash
curl -fsSLO https://github.com/hijera/foxxy-agent/releases/latest/download/foxxycode_0.2.63_linux_amd64.rpm
sudo dnf install ./foxxycode_0.2.63_linux_amd64.rpm
```

Replace **`0.2.63`** with the release you want and **`amd64`** with **`arm64`** on 64-bit ARM. The
[releases page](https://github.com/hijera/foxxy-agent/releases) lists what each tag
published, and **`SHA256SUMS`** beside them covers the packages too:

```bash
sha256sum -c --ignore-missing SHA256SUMS
```

### What the package installs

| Path | What |
|------|------|
| **`/usr/bin/foxxycode`** | The full binary (**`http`**, **`ui`**, **`scheduler`**, **`memory`**, **`cli`**, **`browser`**, **`gateway`**, **`swarm`**) |
| **`/usr/share/man/man1/foxxycode.1.gz`** | **`man foxxycode`** |
| **`/usr/share/bash-completion/completions/foxxycode`** | bash completion |
| **`/usr/share/zsh/site-functions/_foxxycode`** | zsh completion |
| **`/usr/share/doc/foxxycode/config.example.yaml`** | starting point for **`~/.foxxycode/config.yaml`** |
| **`/usr/share/doc/foxxycode/LICENSE`**, **`copyright`** | licence |

That is the whole package: a binary and its documentation. No service, no system account, nothing
under **`/etc`**. Configuration, sessions, skills and credentials stay in the invoking user's
**`~/.foxxycode`**, so one installed package serves every user on the machine, each with their own
state, and what to run - the console, the HTTP gateway, an editor over ACP - stays your decision.

### First run

```bash
mkdir -p ~/.foxxycode
cp /usr/share/doc/foxxycode/config.example.yaml ~/.foxxycode/config.yaml
# set a provider key in ~/.foxxycode/config.yaml
foxxycode
```

### Upgrading and removing

Upgrade through the package manager, or let FoxxyCode fetch the release package for you as root - both
end in the same place, and **`foxxycode update`** as an ordinary user will say so rather than silently
replacing a packaged file (see [update.md](update.md#installations-owned-by-a-package-manager)):

```bash
sudo apt-get install ./foxxycode_<newer>_linux_amd64.deb   # or dnf install ./...rpm
sudo foxxycode update -y                                   # downloads and installs the package
```

```bash
sudo apt-get remove foxxycode    # or: sudo dnf remove foxxycode
rm -rf ~/.foxxycode              # only if you also want the sessions and config gone
```

There is no apt or dnf repository to subscribe to: the packages are release assets, so a new version
arrives when you install the newer file or run **`sudo foxxycode update`**, not from a background
**`apt upgrade`**.

### Building the packages yourself

```bash
make deb
make rpm
```

See [build.md](build.md#distribution-packages).

## macOS (Homebrew)

```bash
brew install --cask https://github.com/hijera/foxxy-agent/releases/latest/download/foxxycode.rb
```

Every release publishes **`foxxycode.rb`** beside the archives, rendered with the checksums of the macOS
archives of that same tag. The cask installs the same **`foxxycode`** binary the macOS archive carries,
plus **`man foxxycode`** and the bash and zsh completions. Removal goes through Homebrew:

```bash
brew uninstall --cask foxxycode      # brew zap --cask foxxycode also removes ~/.foxxycode
```

Upgrading means running the install command again: Homebrew tracks new versions of a cask it got from
a tap, not of one installed from a URL.

**`brew install --cask foxxycode`** by name needs FoxxyCode in one of Homebrew's own repositories, and that is
still open. Homebrew routes open-source command-line software to **homebrew/core** as a formula built
from source, so the cask above is our own distribution channel rather than a submission - the whole
path, including what currently blocks it, is in [homebrew.md](homebrew.md).

**`foxxycode update`** recognises a Homebrew install and points back at **`brew upgrade`** rather than
replacing a file Homebrew tracks: **`brew upgrade --cask foxxycode`** for this cask, and
**`brew upgrade foxxycode`** for a formula. Homebrew refuses to run under **`sudo`**, so there is no
privileged shortcut there.

If macOS blocks the first run because the binary is not notarised, clear the quarantine flag:
**`xattr -d com.apple.quarantine "$(which foxxycode)"`**.

## After install

```bash
export PATH="$HOME/.local/bin:$PATH"
foxxycode -v
# edit ~/.foxxycode/config.yaml
foxxycode serve            # every subsystem config.yaml enables, in this terminal
foxxycode serve --daemon   # in the background, restarted if it dies
```

The packages install no service unit, because FoxxyCode keeps its state per user under
**`~/.foxxycode`**. **`foxxycode serve --daemon`** is the built-in way to keep it running
without one; under a supervisor that already owns process lifetimes (`systemd`, Docker)
use the foreground form and let that supervisor restart it. See [the daemon
guide](serve.md). **`foxxycode http`** is still there for the API alone.

## Windows

### Install locations

| What | Path |
|------|------|
| Binary | `%LOCALAPPDATA%\Programs\foxxycode\foxxycode.exe` |
| Config | `%USERPROFILE%\.foxxycode\config.yaml` |
| Sessions / memory | `%USERPROFILE%\.foxxycode\sessions\` |

The user directory is **`$env:USERPROFILE`** (`%USERPROFILE%`), **not** `$HOME` — `$HOME` is unreliable across Windows PowerShell and Git Bash setups (Git Bash `$HOME` may differ from `%USERPROFILE%`).

### PATH in the current session

After adding the binary directory to the **user** `PATH`, new terminals pick it up automatically; the terminal you installed from does **not**. Either open a new terminal, or refresh in place:

```powershell
$env:Path = [Environment]::GetEnvironmentVariable("Path","User") + ";" + [Environment]::GetEnvironmentVariable("Path","Machine")
```

(`refreshenv` also works if you have Chocolatey.)

### Editor / agent integrations: use the absolute path

Some harnesses spawn **`foxxycode acp`** via `cmd /c` or `sh -c` and do not inherit the user `PATH`. To avoid "command not found" wiring bugs, configure clients with the absolute path:

```text
%LOCALAPPDATA%\Programs\foxxycode\foxxycode.exe
```

## Docker

```bash
docker compose pull && docker compose up -d
```

See [docker.md](docker.md) and the [README Docker section](../README.md#docker).

## Upgrade

```bash
foxxycode update -y
```

See [update.md](update.md).

## Build from source

See [build.md](build.md) and the README section **Other installation methods**.
