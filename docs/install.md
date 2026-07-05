# Install FoxxyCode Agent

FoxxyCode Agent is based on [foxxycode-agent](https://github.com/coddy-project/coddy-agent). The upstream
`foxxycode.dev` install scripts install the original project, not this fork — use one of the methods below.

## Release archive

Download the archive for your platform from
**[GitHub Releases](https://github.com/hijera/foxxycode-agent/releases)** (assets such as
`foxxycode_X.Y.Z_linux_amd64.tar.gz`), unpack it, and put the **`foxxycode`** binary on `PATH`
(Unix: `~/.local/bin`; Windows: `%LOCALAPPDATA%\Programs\foxxycode`).

Bootstrap the config when missing:

```bash
mkdir -p ~/.foxxycode && cp config.example.yaml ~/.foxxycode/config.yaml
```

## After install

```bash
export PATH="$HOME/.local/bin:$PATH"
foxxycode -v
# edit ~/.foxxycode/config.yaml
foxxycode http
```

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
