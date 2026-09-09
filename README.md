# lazymagit

`lazymagit` is a standalone Go TUI inspired by Magit's status-buffer
workflow. It combines a section-oriented Git view with Doom Emacs's
Magit/Evil command bindings and navigation.

![Go](https://img.shields.io/badge/Go-1.25+-00ADD8)
![TUI](https://img.shields.io/badge/TUI-Bubble_Tea_v2-7D56F4)

## Screenshots

### Status and diff

![Lazymagit status view showing staged, unstaged, and untracked files beside a selected diff](docs/images/lazymagit-status.png)

### Command dispatcher

![Lazymagit command dispatcher showing Magit-style transient command groups](docs/images/lazymagit-commands.png)

## Features

- Porcelain-v2 status parsing with separate staged and unstaged state
- Safe handling of spaces, Unicode, leading dashes, and Git pathspec magic
- Stable, foldable sections for untracked, unstaged, staged, upstream, and log
- Persistent selectable log/graph rows with contextual cherry-pick, revert, reset, and revision inspection
- Selectable blame lines with exact commit drilldown and restored cursor position
- Three-stage conflict inspection with direct reviewed ours/theirs resolution
- Marked multi-file and reviewed file/hunk/multi-hunk/disjoint-region stage, unstage, and discard
- Stale-safe interactive patch reviews that revalidate the exact source diff before mutation and reject unresolved conflicts
- Commit creation, revision inspection, searchable branch switching, multi-remote browsing, and reviewed remote-branch publishing/deletion
- Vim-style commit and PR message buffers with wrapping, visual selection, undo/redo, paste, and persistent drafts
- Reviewed GitHub PR creation/editing through `gh`, with title/body, base branch, and draft/ready controls
- Reviewed merge, non-interactive rebase, cherry-pick/revert, reset, and bisect workflows with stale-state rejection
- Standard side-by-side and optional compact borderless status/diff layouts
- Universal status-row search with `/`, then `n` / `N` navigation
- Searchable worktree and branch browsers
- Bundled default, Tokyo Night, Catppuccin Mocha, Nord, Dracula, Gruvbox Dark, and Solarized Dark themes
- Responsive Magit-style command transients with explicit unavailable actions
- Asynchronous Git operations with stale-result protection
- Bounded Magit-style Git process transcript with terminal-safe clipboard copy
- Terminal-control sanitization for untrusted repository content

## Install

### Go

With Go 1.25 or newer installed:

```sh
go install github.com/richardrh/lazymagit/cmd/lazymagit@latest
```

The binary is installed into `$(go env GOPATH)/bin` unless `GOBIN` is set.

### Linux release packages

Each tagged GitHub release publishes `amd64` and `arm64` binaries plus native
Debian, RPM, and Arch Linux packages. Download the package for your architecture
from the [releases page](https://github.com/richardrh/lazymagit/releases), then
install it with the platform package manager:

```sh
# Ubuntu or Debian
sudo apt install ./lazymagit_*_linux_amd64.deb

# Fedora or another RPM-based distribution
sudo dnf install ./lazymagit_*_linux_amd64.rpm

# Arch Linux
sudo pacman -U ./lazymagit_*_linux_amd64.pkg.tar.zst
```

Generic Linux, macOS, and Windows archives and `checksums.txt` are published
by the release workflow for every `v*` tag. `go install` users can install
the module directly; there is not currently a Homebrew tap or formula.

## Build

Requirements: Go 1.25 or newer and Git available on `PATH`.
GitHub PR composition additionally requires the GitHub CLI (`gh`) with an existing authenticated session.

```sh
make check
CGO_ENABLED=0 go build -o lazymagit ./cmd/lazymagit
./lazymagit [--init] [--theme NAME] [--layout standard|compact] [repository]
```

`make check` runs formatting, vet, race-enabled tests, the complexity
threshold check, and generated-keybinding drift checks. To run only the Go
tests, use `go test ./...`.

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

## Documentation

The documentation site uses [Hugo Extended](https://gohugo.io/) with the
[Hextra](https://imfing.github.io/hextra/) theme. Site sources live under
`doc/content/`; the focused Markdown notes under `docs/` are synchronized into
the site during each build. See the [documentation site README](doc/README.md)
for prerequisites and maintenance details.

Preview or build the site from the repository root:

```sh
make -C doc serve
make -C doc build
```

Local site output is written to `doc/public/` and is not committed. The
development server is available at <http://localhost:1313/lazymagit/>.

`docs/keybindings.md` remains the canonical generated keybinding ledger. Do not
edit it by hand. Regenerate it with:

```sh
go run ./internal/keymap/cmd/keymapdoc
```

Check for generated-documentation drift with:

```sh
go run ./internal/keymap/cmd/keymapdoc -check
```

## Themes

Themes can be selected at startup with `--theme` or changed in the status
view with the `F2` theme picker. The default is `default`. The bundled themes
and their canonical names are:

| Theme | Name |
|---|---|
| Default | `default` |
| Tokyo Night | `tokyo-night` |
| Catppuccin Mocha | `catppuccin-mocha` |
| Nord | `nord` |
| Dracula | `dracula` |
| Gruvbox Dark | `gruvbox-dark` |
| Solarized Dark | `solarized-dark` |

Use either a canonical name or its display spelling. Names are
case-insensitive, and spaces or underscores are treated like hyphens.
`catppuccin` is also accepted as an alias for Catppuccin Mocha. For example:

```sh
./lazymagit --theme nord
./lazymagit --theme "Catppuccin Mocha" ./my-repository
./lazymagit --theme gruvbox_dark
```

Theme options must appear before the repository path. Restart the application
with a different `--theme` value to change the palette.

While in the status view, press `F2` to open the theme picker. Use `↑`/`↓`
or `j`/`k` to move, `Enter` to apply the selected theme, and `q`/`Esc` to
cancel.

## Keys

The application uses one Doom Magit binding model. There is no Vim/Magit mode
switch. Bindings follow [Doom's Magit configuration](https://github.com/doomemacs/modules/blob/main/modules/tools/magit/config.el)
on top of [Evil Collection's Magit bindings](https://github.com/emacs-evil/evil-collection/blob/master/modes/magit/evil-collection-magit.el).
Git transient suffixes retain Magit's bindings; Doom remaps conflicting
top-level commands around Vim navigation.

| Action | Keys |
|---|---|
| Move / first / last row | `j` / `k`, `gg` / `G` |
| Next / previous sibling section | `]` / `[`, `gj` / `gk` |
| Parent section | `gh` |
| Refresh | `gr`, `gR`, or `gz` |
| Toggle section | `Tab` or `za` |
| Open / close section | `zo` / `zc`; `zO` / `zC` recursively |
| Global section depth | `z1`, `z2`, `z3`, `z4`; `zr` expands |
| Position selected row at top / center / bottom | `zt` / `zz` / `zb` |
| Half-page / full-page movement | `Ctrl-d` / `Ctrl-u`, `Ctrl-f` / `Ctrl-b` |
| Detail viewport start / end | `0` / `$` |
| Stage / unstage | `s` / `u` |
| Select changed-line range | `v`, then `j` / `k`; `Space` pins a region |
| Select multiple hunks | `V` |
| Search / next / previous match | `/`, then `n` / `N` |
| Stage tracked modifications / unstage all | `S` / `U` |
| Change theme | `F2` |
| Reviewed discard | `x` |
| Commit | `c c` |
| Compose or edit the current branch's GitHub PR | `Alt-r` |
| Switch branch | `b b` |
| Checkout remote branch as local tracking branch | `b l` |
| Create branch / reviewed branch deletion | `b c` / `b k` |
| Fetch upstream / push remote | `f u` / `f p` |
| Fetch selected remote / all remotes | `f e` / `f a` |
| Add remote | `M a`; `M -f` enables fetching |
| Push | `p p` |
| Merge / rebase / cherry-pick | `m` / `r` / `A` |
| Revert without commit / revert transient | `-` / `_` |
| Quick reset / reset transient | `o` / `O` |
| Stash / worktree | `Z` / `*` |
| Submodule / subtree | `'` / `"` |
| Show refs / copy section value / copy revision | `yr` / `ys` / `yb` |
| Git processes | Backtick |
| Terminal blame / all-refs graph | `Alt-b` / `Alt-g` |
| Help / close or quit / quit all | `?` / `q` / `Q` |
| Cancel a prefix, selection, or dialog | `Esc` or `Ctrl-g` |

The `g`, `z`, and `y` prefixes wait for a suffix; a lone `g` never refreshes
after a delay. `$` and `0` pan the read-only detail viewport, not Git commands.
`g=` or `~` restores default diff context; `=` reduces context.
Doom reserves `gz` for refresh, not the upstream Evil Collection stash jump.

Pressing `b`, `c`, `f`, `p`, or `M` opens a grouped command transient. The
layout uses multiple columns when space permits and one vertically scrollable
column on narrow terminals; use arrows or `PageUp`/`PageDown`. Commands that
are not implemented are omitted from the interactive sheets. Implemented
actions unavailable in the current repository context remain visible with an
explicit marker. `q`, `Esc`, or `Ctrl-g` closes a transient.

Inside the `?` dispatcher, suffixes also remain stock Magit: `? P` opens Push,
`? z` opens Stash, `? Z` opens Worktree, and `? $` opens Git processes.
Normal-state Doom remaps apply again after the transient closes.

`Ctrl-c Ctrl-e` and `Ctrl-c Ctrl-o` open the selected file, revision, or stash
in the internal read-only detail pane; they never launch `$EDITOR`, `$BROWSER`,
or a URL handler. `Ctrl-c Ctrl-r` advances to the next visible decorated
commit row without wrapping. `Ctrl-c` is a command prefix, not quit.

Status navigation uses Doom bindings; commit and PR messages use the Vim-style
buffer described below. Other workflow forms retain their terminal controls.
This is not an embedded Vim/Neovim or full Evil/Forge implementation: Ex commands,
macros, plugins, and Emacs window management are not provided.

`S` stages all tracked modifications and deletions while intentionally leaving
untracked files alone. `U` clears all index changes while preserving worktree
content. Both keys work anywhere in the status view, independent of the
selected section.

Backtick opens a small process window at the bottom of the status view. It keeps a
bounded, chronological transcript of mutating Git commands, exit codes,
durations, stdout, and full useful stderr. Failed operations open the window at
the latest output automatically. Use arrows or `PageUp`/`PageDown` to scroll,
and `y` to request copying the entire plain, terminal-sanitized transcript via
OSC52. The confirmation says “Clipboard copy requested” because terminals do
not acknowledge OSC52 delivery. The displayed command line is an unambiguous
human representation of arguments, not a claim that a shell was used.
Use `q` or `Esc` to close it; backtick inside the process window is ignored,
matching Doom's protection against recursive process buffers.

`M a` opens separate name and URL fields and adds without fetching. Enable
`M -f` (or `Ctrl-f` in the modal) to explicitly request `git remote add -f`.
The command remains available after each add, so repositories can configure any
number of remotes. Remote choosers show each remote's effective fetch URL and a
distinct push URL when configured, rather than presenting ambiguous names only.
The URL is passed to Git exactly as entered; the remote name is trimmed.
`f u` fetches the upstream remote. `f p` fetches the configured push remote, or
opens a distinct chooser when none is configured; choosing one records
`branch.<current>.pushRemote` and then fetches it. `f e` is the fetch-only
remote chooser and `f a` runs `git fetch --all`. `f f` is intentionally unbound
to match Magit 4.7's exact sequences.

`b l` selects an exact remote-tracking ref, asks for a local branch name, then
creates and checks out a tracking branch. `b c` keeps local-only branch creation
and also offers a publish mode: choose an existing local branch, destination
remote, and remote branch name, review the exact local commit and observed
remote destination, then push with upstream configuration. `b k` includes both
non-current local branches and fetched remote-tracking branches. Remote deletion
reviews the server-advertised commit again immediately before `git push --delete`.
Remote rename, removal, configuration, and prune remain reviewed `M` workflows.

`p p` uses ordinary `git push` when the current branch already has an upstream.
Without an upstream, it pushes with `--set-upstream` to the configured push
remote. If no push remote is configured, a distinct chooser records
`branch.<current>.pushRemote` and then pushes the current branch while setting
its upstream; it does not fetch. The chooser configures the current branch.
There is not yet a separate prompt for configuring repository-wide
`remote.pushDefault`.

## Commit and PR message buffers

`c c` opens a commit buffer. `c a` and `c w` preload HEAD's message for amend
and reword. `Alt-r` opens the current branch's existing open GitHub PR, or
prepares a new PR with HEAD's message and an available local PR template.
`Alt-r` is a terminal extension, not a stock Magit suffix.

Write the title/subject on the first line, a blank line, then the body.
Buffers start in **INSERT** mode. **Esc enters NORMAL mode without closing
or discarding the message.**

| Action | Keys |
|---|---|
| Insert / append | `i`, `I`, `a`, `A` |
| Open a line below / above | `o` / `O` |
| Move | `h j k l`, arrows, `w b e`, `0 ^ $`, `gg`, `G`; counts such as `3j` |
| Delete / change | `x`, `X`, `dd`, `D`, `cc`, `C`, `dw`, `cw` |
| Character / line selection | `v` / `V`, then motions and `y`, `d`, or `c` |
| Yank line / put | `yy`, `p`, `P` (buffer-local register) |
| Undo / redo | `u` / `Ctrl-r` in Normal mode |
| Newline / paste | `Enter` in Insert mode; normal terminal paste |
| Move between message, options, and action | `Tab` / `Shift-Tab` |
| Save draft | `Ctrl-s` |
| Save draft and close | `Ctrl-g` or `Ctrl-c Ctrl-k` |
| Submit / review | `Ctrl-c Ctrl-c`, or focus the action and press `Enter` |
| Toggle commit diff / scroll diff or review | `Alt-d` / `Alt-j`, `Alt-k` |

Long lines wrap without changing the stored message, and the viewport follows
the cursor. The mode, position, title length, and action remain visible.
Wide commit buffers show a colored diff beside the message; narrow terminals
use `Alt-d` to switch between them.

Drafts autosave after 500 ms of inactivity and are also saved before closing
or submitting. They live in private, untracked Git metadata at
`<git-dir>/lazymagit/drafts/`, separated by branch and operation. Draft JSON is
limited to 1 MiB and is local plaintext, not encrypted. A rejected hook or
failed PR request keeps the buffer open with its text and undo history;
reopening after a restart restores the saved text and options. Successful
submission removes the corresponding draft.

For PRs, the first submission opens a review of title, body, base/head, and
draft status. A second `Ctrl-c Ctrl-c` or `Enter` publishes the reviewed values;
`Esc` returns to editing. New PRs default to draft. Existing PRs are edited
in place, including draft/ready changes, and the result URL is shown on success.
Push the branch first: PR publication uses an explicit `--head` and never
automatically pushes or forks. GitHub authentication/network failures are
reported rather than treated as an absent PR.

This provides GitHub PR message composition, not the full Forge interface:
there is no GitLab backend, PR review/comments/checks UI, Markdown preview,
or picker for multiple open PRs sharing the same head branch.

## Compatibility scope

This is not yet a behavior-exact port of all Magit. The implemented wave now covers the core status workflow, reviewed hunk/changed-line multi-selection mutations, bounded terminal-native conflict inspection and ours/theirs resolution, searchable branch and worktree browsers, compact layout, common commands, and reviewed history workflows. Terminal-native, reviewed interactive rebase todo editing is available for the bounded pick/reword/edit/squash/fixup/drop command set; `exec`, merge-topology todo commands, aliases, and external editors remain intentionally unavailable. Manual merge-buffer editing, binary/rename patch selection, semantic patch editing beyond typed region refinement, executable full transient option sets, and Emacs extension APIs remain out of scope. See [docs/compatibility.md](docs/compatibility.md) for upstream test traceability and [docs/parity.md](docs/parity.md) for the feature-by-feature parity matrix and [docs/keybindings.md](docs/keybindings.md) for the complete 98-key status ledger.

Whole-file destructive actions require confirmation and reject unsafe mixed-state cases. History operations resolve revisions to object IDs, show an immutable Review/Execute plan, revalidate HEAD, index, worktree, and operation administration state immediately before execution, and fail closed with a stale-plan error if repository state changed. Focused hunk, multi-hunk, and changed-line mutations use the same two-phase flow, reconstruct patches inside the backend, reject unresolved target conflicts, and revalidate the exact source diff before mutation.
