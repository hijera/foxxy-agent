# Install Coddy

Install scripts and the landing page: **https://coddy.dev/**

## One-line install

**Linux / macOS**

```bash
curl -fsSL https://coddy.dev/install.sh | bash
```

**Windows (PowerShell)**

```powershell
irm https://coddy.dev/install.ps1 | iex
```

Creates **`~/.coddy/config.yaml`** from the release **`config.example.yaml`** when missing.

## After install

```bash
export PATH="$HOME/.local/bin:$PATH"
coddy -v
# edit ~/.coddy/config.yaml
coddy http
```

## Docker

```bash
docker compose pull && docker compose up -d
```

See [docker.md](docker.md) and the [README Docker section](../README.md#docker).

## Upgrade

```bash
coddy update -y
```

See [update.md](update.md).

## Build from source

See [build.md](build.md) and the README section **Other installation methods**.
