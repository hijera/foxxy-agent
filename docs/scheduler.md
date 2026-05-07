## Scheduler

### Overview

The scheduler is an optional cron-like runner that scans a directory for job markdown files and executes them when due.
It is linked into the binary only when built with the `scheduler` build tag.

The scheduler has two parts

- `external/scheduler` starts the daemon in the background and runs jobs
- `external/scheduler/tools` exposes `coddy_scheduler_*` tools to create and manage jobs

The cron parser uses five fields and interprets schedules in UTC.

### Build

- Build with scheduler only
  - `go build -tags=scheduler ./cmd/coddy`
- Build with HTTP and scheduler
  - `go build -tags=http,scheduler ./cmd/coddy`

### Enabling

The scheduler is active for a process when any of these is true

- `scheduler.enabled: true` in config
- `coddy acp -scheduler-enabled` or `coddy http -scheduler-enabled`

When active, the scheduler daemon runs in the background for both ACP and HTTP commands.

### Job directory

Jobs are `*.md` files under `scheduler.dir`.

- If `scheduler.dir` is empty or omitted
  - it defaults to `${CODDY_HOME}/scheduler`

Sidecar files live next to the job file

- `basename.lock` during a running job
- `basename.state` with the last scheduled slot timestamp

### Job file format

A job is a markdown file with YAML frontmatter and a markdown body.

Frontmatter fields

- `description` string
  - Human readable summary
- `schedule` string
  - Five field crontab in UTC
- `cwd` string
  - Empty or omitted means use the coddy process cwd
  - Relative paths are resolved against the coddy process cwd
- `model` string
  - Optional model override for the scheduled session
- `mode` string
  - `agent` or `plan`
  - When omitted, defaults to `agent`

Body

- The markdown body is used as the one-shot instruction text for the agent turn

### Examples

#### Minute tick that writes a file

```md
---
description: "Minute tick"
schedule: "* * * * *"
cwd: ""
mode: agent
---

In the session working directory run

bash -lc 'date -u +%FT%TZ > tick.txt'
```

### Scheduler tools

Tools are available only when the scheduler is effectively enabled.

- `coddy_scheduler_list`
  - List job files with last and next scheduled times
- `coddy_scheduler_read`
  - Read one job file by relative path under `scheduler.dir`
- `coddy_scheduler_write`
  - Create or replace a job file under `scheduler.dir`
- `coddy_scheduler_delete`
  - Delete a job file and its `.lock` and `.state`
- `coddy_scheduler_validate`
  - Validate a schedule string and print next run times
