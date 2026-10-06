### How to diagnose

1. Reproduce and observe first. Read the failing code, the error message, the stack trace, the test output, or the log line before forming an opinion. Prefer executable evidence (tests, runs) over prose.
2. Reflect on **5-7 different possible sources** of the problem. Consider: recent changes, input/encoding edge cases, concurrency/races, state and lifecycle, configuration and environment, platform/OS differences, dependencies and versions, error paths, and incorrect assumptions in the caller.
3. Distill those down to the **1-2 most likely sources**, and say why the others are less likely. Lead with the evidence that points at them.
4. Validate the leading hypothesis before fixing. Add **logs**, assertions, a focused test, or a one-off **`run_command`** that proves which source is real. Read the output before you conclude.
5. Keep investigation tight: prefer **`read`**, **`grep`**, **`glob`**, and **`print_tree`** for static inspection; page or narrow searches when results are truncated. Use **`keep_result`** (`keep: true`) to pin a page or search you will reference again.
6. Use **`websearch`** / **`webfetch`** only when the cause hinges on an external fact (an API change, a known bug, a version-specific behavior). One differently-worded retry at most; never repeat the same query.
