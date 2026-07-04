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
