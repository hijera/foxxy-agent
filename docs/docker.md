# Docker

Run FoxxyCode as **`foxxycode http`** inside a minimal **`scratch`** image. The default image ships the embedded web UI (**`ui`** build tag), OpenAI-compatible REST (**`http`**), scheduler (**`scheduler`**), and long-term memory (**`memory`**).

Related files:

- [`Dockerfile`](../Dockerfile) - multi-stage build (**Node** UI bundle, **Go** binary, **`scratch`** runtime)
- [`docker-compose.yml`](../docker-compose.yml) - run **`ghcr.io/hijera/foxxycode-agent`** (default **`docker compose`**)
- [`docker-compose.dev.yml`](../docker-compose.dev.yml) - build from source, publish port **12345**, volumes
- [`.dockerignore`](../.dockerignore) - keeps context small; never commit **`config.yaml`** with secrets
- [`examples/httpserver/docker.sh`](../examples/httpserver/docker.sh) - automated smoke test

Published images: **[foxxycode-agent on GHCR](https://github.com/hijera/foxxycode-agent/pkgs/container/foxxycode-agent)** (`ghcr.io/hijera/foxxycode-agent`). CI builds **multi-arch** manifests (**`linux/amd64`**, **`linux/arm64`**) on SemVer tags and pushes floating aliases (**`latest`**, **`MAJOR.MINOR`**, **`MAJOR`**) when appropriate - see [`.github/workflows/docker-build-push.yaml`](../.github/workflows/docker-build-push.yaml).

On Apple Silicon or arm64 Linux hosts, pull the image as usual; Docker selects **`arm64`** automatically. To pin a platform explicitly:

```bash
docker pull --platform linux/arm64 ghcr.io/hijera/foxxycode-agent:latest
docker pull --platform linux/amd64 ghcr.io/hijera/foxxycode-agent:latest
```

General build instructions without Docker - **[docs/build.md](build.md)**.

## Prerequisites

- **Docker** with **Compose V2** (**`docker compose`**, not only legacy **`docker-compose`**)
- A **`config.yaml`** you mount read-only into the container (start from **`config.example.yaml`**). Do not commit secrets.
- For the web UI, a browser on the machine that can reach the published host port (default **12345**)

## Docker Compose

Compose is the recommended way to run the published GHCR image or a locally built one. Both files define a **single service** named **`foxxycode`** - no database or Redis sidecar. The process is **`foxxycode http`** bound to **`0.0.0.0:12345`** inside the container; Compose maps that port to the host.

| File | When to use |
|------|-------------|
| [`docker-compose.yml`](../docker-compose.yml) | Pull a release from GHCR (**`docker compose pull`**) - day-to-day and production-like runs without building on the host |
| [`docker-compose.dev.yml`](../docker-compose.dev.yml) | **`build:`** from the repo [`Dockerfile`](../Dockerfile) - hacking on FoxxyCode, reproducing CI, or running **`examples/httpserver/docker.sh`** |

Compose V2 merges an optional **`docker-compose.override.yml`** in the same directory (git-ignored by convention) so you can pin an image tag, change ports, or add **`environment`** without editing the tracked file.

### What the `foxxycode` service runs

| Setting | Default compose | Dev compose |
|---------|-----------------|-------------|
| **Image** | **`${FOXXYCODE_IMAGE:-ghcr.io/hijera/foxxycode-agent:latest}`** | **`foxxycode-agent:${FOXXYCODE_VERSION:-dev}`** (built locally) |
| **Command** | Image **`CMD`**: **`http -H 0.0.0.0 -P 12345`** | Same |
| **Published port** | **`${FOXXYCODE_HTTP_PORT:-12345}:12345`** | Same |
| **Working dir** | **`/workspace`** (**`FOXXYCODE_CWD`**) | Same |

**Healthcheck** - **`foxxycode --version`** every 5s (container "healthy" once the binary responds; HTTP readiness is separate - use **`curl /v1/models`** after **`up`**).

**Logging** - **`json-file`** driver with **`max-size: 100k`** per file to avoid unbounded log growth on long-lived hosts.

### Host paths and environment

Bind mounts (override host paths with env vars before **`up`**):

| Host (default) | Container | Variable |
|----------------|-------------|----------|
| **`./config.yaml`** | **`/home/user/.foxxycode.yaml`** (read-only) | **`FOXXYCODE_CONFIG`** |
| **`./workspace`** | **`/workspace`** | **`FOXXYCODE_CWD`** |
| **`./foxxycode_home`** | **`/home/user/.foxxycode`** | **`FOXXYCODE_HOME`** |

Fixed inside the container (set by compose **`environment`**):

| Variable | Value | Role |
|----------|-------|------|
| **`FOXXYCODE_CONFIG`** | **`/home/user/.foxxycode.yaml`** | Points the loader at the mounted file (not **`$FOXXYCODE_HOME/config.yaml`**) |
| **`FOXXYCODE_HOME`** | **`/home/user/.foxxycode`** | Sessions, skills, scheduler store |
| **`FOXXYCODE_CWD`** | **`/workspace`** | Default cwd for new sessions and tool paths |

Provider keys can be injected from the host shell (optional, empty if unset):

- **`OPENAI_API_KEY`**, **`ANTHROPIC_API_KEY`**, **`DEEPSEEK_API_KEY`**

Prefer mounting secrets via config or your orchestrator; do not commit real keys.

### Compose commands

**Published image** (from repo root or any directory where you keep **`config.yaml`**, **`workspace/`**, **`foxxycode_home/`**):

```bash
docker compose pull
docker compose up -d
docker compose ps
docker compose logs -f foxxycode
curl -sS http://127.0.0.1:12345/v1/models | head
docker compose restart foxxycode
docker compose down
```

Pin a GHCR tag without editing YAML:

```bash
export FOXXYCODE_IMAGE=ghcr.io/hijera/foxxycode-agent:0.2.0
docker compose pull
docker compose up -d
```

Change only the host port:

```bash
export FOXXYCODE_HTTP_PORT=8080
docker compose up -d
# UI: http://127.0.0.1:8080/
```

**Build from source** (dev compose file):

```bash
docker compose -f docker-compose.dev.yml build foxxycode
docker compose -f docker-compose.dev.yml up -d --build
docker compose -f docker-compose.dev.yml logs -f foxxycode
docker compose -f docker-compose.dev.yml down
```

Optional build args on the dev file:

```bash
export FOXXYCODE_VERSION="$(git describe --tags --dirty 2>/dev/null || echo dev)"
export FOXXYCODE_BUILD_TAGS="http,scheduler,ui,memory"
docker compose -f docker-compose.dev.yml build foxxycode
```

**`FOXXYCODE_BUILD_TAGS`** must stay comma-separated with **no spaces**, matching **`go build -tags=`**.

### Override example

Create **`docker-compose.override.yml`** next to **`docker-compose.yml`** (Compose loads it automatically):

```yaml
services:
  foxxycode:
    image: ghcr.io/hijera/foxxycode-agent:0.2.0
    ports:
      - "8080:12345"
    environment:
      OPENAI_API_KEY: ${OPENAI_API_KEY}
```

Use overrides for machine-specific ports, pinned tags, or extra **`environment`** - keep secrets out of git.

### Upgrade and rollback

```bash
export FOXXYCODE_IMAGE=ghcr.io/hijera/foxxycode-agent:NEW_TAG
docker compose pull
docker compose up -d
```

Sessions and config on the host (**`./foxxycode_home`**, mounted **`config.yaml`**) survive image swaps. If a new binary fails health checks, inspect **`docker compose logs foxxycode`** and roll back **`FOXXYCODE_IMAGE`** to the previous tag.

## Quick start (Compose + web UI)

From a checkout of this repository (or any folder where you keep **`config.yaml`**, **`workspace/`**, and **`foxxycode_home/`**):

**1. Prepare config and directories**

```bash
cp config.example.yaml config.yaml
mkdir -p workspace foxxycode_home
```

Edit **`config.yaml`**: configure at least one entry under **`providers`** and **`models`**, and set **`agent.model`** to a listed model id. You can leave **`api_key`** empty and pass **`OPENAI_API_KEY`** (or **`NAME_API_KEY`** for provider **`name`**) through compose **`environment`** instead.

Optional: set **`httpserver.host`** to **`0.0.0.0`** and **`httpserver.port`** to **12345** in YAML. The container **`CMD`** already runs **`foxxycode http -H 0.0.0.0 -P 12345`**, so flags apply even if **`httpserver`** is omitted from the file.

**2. Start FoxxyCode** (published image from GHCR):

```bash
docker compose pull
docker compose up -d
```

**3. Connect with the bundled UI**

Open in a browser on the host (same machine as Docker, unless you forwarded the port):

```text
http://127.0.0.1:12345/
```

What you get:

| URL | Purpose |
|-----|---------|
| **`/`** | Embedded SPA (chat composer, sessions, tools in transcript) |
| **`/#/settings`** | Live **`config.yaml`** editor (**`GET/PUT /foxxycode/config`**) |
| **`/docs/`** | Swagger UI |
| **`/v1/models`** | Model and mode list (also used by the SPA model picker) |

Typical first-time flow in the UI:

1. Confirm the page loads (static assets from **`go:embed`**).
2. In the composer toolbar, select a **backend model** (rows with **`owned_by`** other than **`foxxycode`** come from your YAML **`models`** list).
3. Switch **agent** vs **plan** mode if needed (session operating profiles).
4. Type a message and send. The UI calls **`POST /v1/responses`** with **`stream: true`** and shows streaming assistant output.
5. Files created or edited by tools land under the mounted workspace (**host `./workspace`** → container **`/workspace`**, **`FOXXYCODE_CWD`**).

Sessions and skills state persist under **`./foxxycode_home`** on the host (**`FOXXYCODE_HOME`** in the container).

**4. Sanity check (optional)**

```bash
curl -sS http://127.0.0.1:12345/v1/models | head
docker compose logs -f foxxycode
```

**Security:** **`foxxycode http`** has no application-level auth. Treat port **12345** like any admin API - bind to localhost, use a firewall, or put a reverse proxy with TLS and authentication in front for remote access.

For a local **`Dockerfile`** build, use **`docker-compose.dev.yml`** - see [Docker Compose](#docker-compose) above.

## What the image contains by default

**`Dockerfile`** **`ARG BUILD_TAGS`** defaults to **`http,scheduler,ui,memory`** (comma-separated, same meaning as **`go build -tags=`**).

- **`http`** - **`foxxycode http`** and REST gateway (see **[docs/http-api.md](http-api.md)**).
- **`ui`** - embedded SPA on **`/`** (needs **`http`**).
- **`scheduler`** - scheduler subsystem (**[docs/scheduler.md](scheduler.md)**).
- **`memory`** - long-term memory copilot and session memory REST (**[external/memory/README.md](../external/memory/README.md)**); toggle runtime behavior via **`memory.enabled`**.

To build an image **without** memory or the embedded UI, override **`BUILD_TAGS`** (for example **`http,scheduler,ui`** or **`http,scheduler`**) via **`docker compose` `args`** or **`docker build --build-arg`**.

Volume and environment details for Compose are in [Docker Compose](#docker-compose). On a bare-metal install without **`FOXXYCODE_CONFIG`**, the loader prefers **`$FOXXYCODE_HOME/config.yaml`** (see **`docs/config.md`**).

## How the Dockerfile stages work

1. **`ui-builder` (Node)** - runs **`npm ci`** and **`npm run build:go`** under **`external/ui`**, producing the static bundle copied into the Go tree for **`go:embed`** when **`ui`** is in **`BUILD_TAGS`**.
2. **`build` (Go)** - **`CGO_ENABLED=0`**, **`GOOS`/`GOARCH`** from BuildKit **`TARGETOS`/`TARGETARCH`** (CI builds **`linux/amd64`** and **`linux/arm64`**), **`go build -tags="$BUILD_TAGS"`** with **`-trimpath`** and **`-ldflags "-s -w -X ...Version=..."`**, writes **`/out/foxxycode`**, copies **`ca-certificates.crt`** for HTTPS clients.
3. **`scratch`** - only the binary and CA bundle; **`ENTRYPOINT`** **`/bin/foxxycode`**, default **`CMD`** **`http -H 0.0.0.0 -P 12345`**.

## Automated smoke test

```bash
./examples/httpserver/docker.sh
```

The script builds a temporary **`config.yaml`**, brings up **`foxxycode`** with **`docker-compose.dev.yml`**, waits for **`/v1/models`**, then runs **`examples/httpserver/http_smoke_gateway.py`**.
