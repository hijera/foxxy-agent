# Docker

Run Coddy as **`coddy http`** inside a minimal **`scratch`** image. Memory support is always compiled in (same as local **`make build`**); HTTP, embedded UI, and scheduler are controlled by **Go build tags**, mirrored by **`BUILD_TAGS`** / **`CODDY_BUILD_TAGS`**.

Related files:

- [`Dockerfile`](../Dockerfile) - multi-stage build (**Node** UI bundle, **Go** binary, **`scratch`** runtime)
- [`docker-compose.yml`](../docker-compose.yml) - sample service, ports, volumes, optional API key env vars
- [`.dockerignore`](../.dockerignore) - keeps context small; never commit **`config.yaml`** with secrets
- [`examples/httpserver/docker.sh`](../examples/httpserver/docker.sh) - smoke test (**`docker compose up`**, **`curl`**, Python gateway checks)

General build instructions without Docker - **[docs/build.md](build.md)**.

## Prerequisites

- **Docker** with **Compose V2** (**`docker compose`**, not only legacy **`docker-compose`**)
- A **`config.yaml`** you mount read-only into the container (start from **`config.example.yaml`**). Do not commit secrets.

## What the image contains by default

**`Dockerfile`** **`ARG BUILD_TAGS`** defaults to **`http,scheduler,ui`** (comma-separated, same meaning as **`go build -tags=`**).

- **`http`** - **`coddy http`** and REST gateway (see **[docs/http-api.md](http-api.md)**).
- **`ui`** - embedded SPA on **`/`** (needs **`http`**).
- **`scheduler`** - scheduler subsystem (**[docs/scheduler.md](scheduler.md)**).
- Memory copilot - always linked; toggle at runtime via **`memory.enabled`** (**[external/memory/README.md](../external/memory/README.md)**).

To build an image **without** the embedded UI (still **`http`**), override **`BUILD_TAGS`** (for example **`http,scheduler`**) via **`docker compose` `args`** or **`docker build --build-arg`**.

## Build the image

From the repository root:

```bash
docker compose build coddy
```

Optional version label and tags (same variables **`docker-compose.yml`** uses):

```bash
export CODDY_VERSION="$(git describe --tags --dirty 2>/dev/null || echo dev)"
export CODDY_BUILD_TAGS="http,scheduler,ui"
docker compose build coddy
```

**`CODDY_BUILD_TAGS`** must stay **comma-separated** with **no spaces**, matching **`go build -tags=`**.

## Run with Compose

**1.** Create **`config.yaml`** beside the compose file (or point env vars at a file path). Minimal flow:

```bash
cp config.example.yaml config.yaml
# edit providers, models, keys; keep http server bound to 0.0.0.0 inside the container if you expose the port
```

**2.** Start the service (builds if needed):

```bash
docker compose up -d --build coddy
```

**3.** Sanity check:

```bash
curl -sS http://127.0.0.1:12345/v1/models | head
```

Default published port is **12345** (see **`docker-compose.yml`** **`ports`** mapping).

## Volumes and environment

**`docker-compose.yml`** mounts:

| Mount | Purpose |
|-------|---------|
| **`${CODDY_CONFIG:-./config.yaml}`** → **`/home/user/.coddy.yaml`** | Read-only config file path **inside** the container (**`CODDY_CONFIG`**) |
| **`${CODDY_CWD:-./workspace}`** → **`/workspace`** | Workspace (**`CODDY_CWD`**) |
| **`${CODDY_HOME:-./coddy_home}`** → **`/home/user/.coddy`** | Sessions, skills, scheduler data (**`CODDY_HOME`**) |

Override host paths:

```bash
export CODDY_CONFIG="$PWD/my-coddy.yaml"
export CODDY_CWD="$PWD/myproject"
export CODDY_HOME="$PWD/coddy-state"
docker compose up -d coddy
```

The sample Compose file sets **`CODDY_CONFIG`** inside the container to a **mounted file** path (**`/home/user/.coddy.yaml`**). That file can live anywhere on the host; it does not have to be named **`config.yaml`** or sit next to **`$CODDY_HOME`**. On a normal host install without **`CODDY_CONFIG`**, the loader prefers **`$CODDY_HOME/config.yaml`** (see **`docs/config.md`**).

Optional provider keys can be passed as environment variables (see **`docker-compose.yml`** **`environment`**). Prefer **mounted config** or your secret manager for production; **do not** commit real keys.

## How the Dockerfile stages work

1. **`ui-builder` (Node)** - runs **`npm ci`** and **`npm run build:go`** under **`external/ui`**, producing the static bundle copied into the Go tree for **`go:embed`** when **`ui`** is in **`BUILD_TAGS`**.
2. **`build` (Go)** - **`CGO_ENABLED=0`**, **`go build -tags="$BUILD_TAGS"`** with **`-trimpath`** and **`-ldflags "-s -w -X ...Version=..."`**, writes **`/out/coddy`**, copies **`ca-certificates.crt`** for HTTPS clients.
3. **`scratch`** - only the binary and CA bundle; **`ENTRYPOINT`** **`/bin/coddy`**, default **`CMD`** **`http -H 0.0.0.0 -P 12345`**.

## Automated smoke test

```bash
./examples/httpserver/docker.sh
```

The script builds a temporary **`config.yaml`**, brings up **`coddy`** with **`docker compose`**, waits for **`/v1/models`**, then runs **`examples/httpserver/http_smoke_gateway.py`**.
