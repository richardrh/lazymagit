# lazymagit

`lazymagit` is a standalone Go TUI inspired by Magit's status-buffer
workflow. It combines a section-oriented Git view and familiar Magit command
prefixes with an optional Vim navigation layer.

![Go](https://img.shields.io/badge/Go-1.25+-00ADD8)
![TUI](https://img.shields.io/badge/TUI-Bubble_Tea_v2-7D56F4)

## Features

- Porcelain-v2 status parsing with separate staged and unstaged state
- Safe handling of spaces, Unicode, leading dashes, and Git pathspec magic
- Stable, foldable sections for untracked, unstaged, staged, upstream, and log
- Whole-file stage, unstage, aggregate tracked staging, and confirmed discard
- Commit creation, revision inspection, local branch switching, remote add, fetch, and push
- Side-by-side status and change-colored diff views on wide terminals
- Responsive stacked view on narrow terminals
- Responsive Magit-style command transients with explicit unavailable actions
- Asynchronous Git operations with stale-result protection
- Bounded Magit-style Git process transcript with terminal-safe clipboard copy
- Terminal-control sanitization for untrusted repository content

## Build

Requirements: Go 1.25 or newer and Git available on `PATH`.

```sh
go test ./...
CGO_ENABLED=0 go build -o lazymagit ./cmd/lazymagit
./lazymagit [--init] [repository]
```

The resulting executable contains the Go application and TUI dependencies in
one binary. Like Magit itself, it invokes the system Git executable for Git
semantics, hooks, configuration, and authentication.

When the selected directory is not a repository, an interactive invocation
asks before running `git init`. A non-interactive invocation never prompts;
pass `--init` to initialize an existing directory explicitly. The explicit
form initializes the exact directory given, even when that directory is inside
another repository. Existing repositories (including bare repositories) are
never reinitialized. Use `--` before a repository path that begins with a dash,
for example `./lazymagit -- --project`.

## Keys

The default key scheme preserves Vim navigation. Press `F2` to switch to the
Magit scheme. This explicit mode is necessary because Vim's `k` (move up) and
`G` (last row) conflict with Magit's `k` (discard) and `G` (refresh all).

| Action | Vim scheme | Magit scheme |
|---|---|---|
| Move | `j` / `k`, `gg` / `G` | `n` / `p` |
| Refresh | `g` after a short delay | `g` or `G` |
| Toggle section | `Tab` | `Tab` |
| Show depth | `1`, `2`, `3` | `1`, `2`, `3` |
| Stage / unstage | `s` / `u` | `s` / `u` |
| Stage modified / unstage all | `S` / `U` | `S` / `U` |
| Confirmed discard | `x` | `k` |
| Commit | `c c` | `c c` |
| Switch branch | `b b` | `b b` |
| Fetch upstream / push remote | `f u` / `f p` | `f u` / `f p` |
| Fetch chosen remote / all remotes | `f e` / `f a` | `f e` / `f a` |
| Add remote (`M -f` to fetch) | `M a` | `M a` |
| Push | `P p` | `P p` |
| Toggle Git processes | `$` | `$` |
| Help / quit | `?` / `q` | `?` / `q` |

Pressing `b`, `c`, `f`, `P`, or `M` opens a grouped command transient. The
layout uses multiple columns when space permits and one vertically scrollable
column on narrow terminals; use arrows or `PageUp`/`PageDown`. Actions shown as
unavailable are catalogued for discoverability but are never executed. `q` or
`Esc` closes a transient, and `?` opens the columnar command dispatcher.

In Magit mode, `x` is recognized as Magit's quick-reset command and reports its
explicit `missing` status; it never falls through to Vim discard behavior.

`S` stages all tracked modifications and deletions while intentionally leaving
untracked files alone. `U` clears all index changes while preserving worktree
content. Both keys work anywhere in the status view, independent of the
selected section.

`$` toggles a small process window at the bottom of the status view. It keeps a
bounded, chronological transcript of mutating Git commands, exit codes,
durations, stdout, and full useful stderr. Failed operations open the window at
the latest output automatically. Use arrows or `PageUp`/`PageDown` to scroll,
and `y` to request copying the entire plain, terminal-sanitized transcript via
OSC52. The confirmation says “Clipboard copy requested” because terminals do
not acknowledge OSC52 delivery. The displayed command line is an unambiguous
human representation of arguments, not a claim that a shell was used.

`M a` opens separate name and URL fields and adds without fetching. Enable
`M -f` (or `Ctrl-f` in the modal) to explicitly request `git remote add -f`.
The URL is passed to Git exactly as entered; the remote name is trimmed.
`f u` fetches the upstream remote. `f p` fetches the configured push remote, or
opens a distinct chooser when none is configured; choosing one records
`branch.<current>.pushRemote` and then fetches it. `f e` is the fetch-only
remote chooser and `f a` runs `git fetch --all`. `f f` is intentionally unbound
to match Magit 4.7's exact sequences.

`P p` uses ordinary `git push` when the current branch already has an upstream.
Without an upstream, it pushes with `--set-upstream` to the configured push
remote. If no push remote is configured, a distinct chooser records
`branch.<current>.pushRemote` and then pushes the current branch while setting
its upstream; it does not fetch. The chooser configures the current branch.
There is not yet a separate prompt for configuring repository-wide
`remote.pushDefault`.

## Compatibility scope

This is not yet a behavior-exact port of all Magit. The implemented first wave
covers the core status workflow and common commands. Hunk/region operations,
interactive rebase, stashes, conflict resolution, submodule/worktree
management, executable full transient option sets, and Emacs extension APIs remain out of
scope. See [docs/compatibility.md](docs/compatibility.md) for upstream test
traceability and [docs/parity.md](docs/parity.md) for the feature-by-feature
parity matrix and [docs/keybindings.md](docs/keybindings.md) for the complete
98-key status ledger.

Destructive actions operate on whole files, require confirmation, treat paths
literally, and reject mixed staged/unstaged discard when preserving one side
cannot be guaranteed.
