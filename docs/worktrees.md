# Worktree workflows

Open the worktree transient with `Z` (or `%`, where Magit's alternate binding is available).

## Create

- `b` creates a worktree from an existing branch or revision. The reviewed dialog supports detached HEAD, `--no-checkout`, lock-on-create with an optional reason, and explicit force.
- `c` creates a new branch and worktree. It supports `--no-checkout`, lock-on-create with an optional reason, and explicit force.

The confirmation screen binds the destination, resolved commit, branch, and all options. Execution rejects a stale review if the revision changes or the destination becomes unsafe.

`--no-checkout` creates the administrative worktree and branch without populating tracked files. Lock-on-create is useful for worktrees on removable or intermittently mounted storage.

## Manage

The pinned Magit keys remain available: `g` lists and filters worktrees, `m` performs a reviewed move, and `k` performs a reviewed removal. Primary and bare worktrees are excluded from destructive choices, and reviewed move and removal operations reject changed state before execution.
