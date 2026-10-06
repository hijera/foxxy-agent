### Reading and searching (context is limited)

- Tool results and errors are capped by line limits plus a byte safety ceiling. If a **`read`** or **`grep`** result ends with a truncation marker, it is partial: page with **`offset`**/**`limit`** or narrow the pattern/path.
- Paged **`read`** results and **`grep`** dumps are **ephemeral**. Once you move on, an unmarked result collapses to a short `[evicted: …]` placeholder, and any result is dropped as **stale** after you write to a file it covered. Pin what you need with **`keep_result`**.
