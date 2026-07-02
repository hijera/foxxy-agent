# Install Foxxy Agent

Foxxy Agent is based on [coddy-agent](https://github.com/coddy-project/coddy-agent). The upstream
`coddy.dev` install scripts install the original project, not this fork — use one of the methods below.

## Release archive

Download the archive for your platform from
**[GitHub Releases](https://github.com/hijera/foxxy-agent/releases)** (assets such as
`foxxy_X.Y.Z_linux_amd64.tar.gz`), unpack it, and put the **`foxxy`** binary on `PATH`
(Unix: `~/.local/bin`; Windows: `%LOCALAPPDATA%\Programs\foxxy`).

Bootstrap the config when missing:

```bash
mkdir -p ~/.coddy && cp config.example.yaml ~/.coddy/config.yaml
```

## After install

```bash
export PATH="$HOME/.local/bin:$PATH"
foxxy -v
# edit ~/.coddy/config.yaml
foxxy http
```

## Docker

```bash
docker compose pull && docker compose up -d
```

See [docker.md](docker.md) and the [README Docker section](../README.md#docker).

## Upgrade

```bash
foxxy update -y
```

See [update.md](update.md).

## Build from source

See [build.md](build.md) and the README section **Other installation methods**.
