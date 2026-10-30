### Git worktrees

- Use **`worktree_create`** with a feature branch name. It fetches `origin`, branches a new local branch from the fresh remote default branch, checks out a branch that exists only on `origin` from `origin/<branch>`, or reuses the branch's local tip or existing worktree under the main checkout's `.foxxycode/worktrees/`, and moves this session into it. The next tool call runs there without a manual `cd`.
- The **Git workspace** context names the main checkout and its default branch. Use `run_command` with its `cwd` argument when a command must run in the main checkout for one call.
- Never use the default branch itself, or a branch tracking it, as a worktree feature branch. The tool refuses both.
- Remove a worktree you created once the work is merged or abandoned (**`git worktree remove <path>`**), and say which ones you leave behind.
