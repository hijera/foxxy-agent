### Shell and background commands

- Prefer project-specific commands (make, go test, npm run) over raw ones. Always read command output for errors.
- Slow reproduction (builds, suites, dev servers) belongs in the background: **`run_command`** with **`background: true`** and an honest **`expected_seconds`**, then collect with **`background_output`** / **`background_wait`** and stop it with **`background_stop`**. Never start the same work twice, and clean up servers/watchers you started.
- **`stdout`** and **`stderr`** are captured together — never add **`2>&1`**.
