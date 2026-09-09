# Magit compatibility notes

This project is an independent Go implementation of Magit's status-buffer
workflow. The behavioral reference is Magit v4.7.0, commit
`67f203853e74e926e2c99f60ed508840714f7ced`.

Upstream test references:

- `magit-toplevel:basic`, `magit-toplevel:submodule`, `magit-in-bare-repo`
- `magit-status:file-sections`
- `magit-status:log-sections`
- `magit-status:section-commands`
- `magit-list-{|local-|remote-}branch-names`
- `magit-get` and `magit-get-boolean`

Source: <https://github.com/magit/magit/blob/v4.7.0/test/magit-tests.el>

The Go tests are independently written black-box tests of observable repository
state; they are not translations of Emacs Lisp implementation code.

Runtime bindings follow [Doom's Magit module](https://github.com/doomemacs/modules/blob/main/modules/tools/magit/config.el)
and [Evil Collection's Magit integration](https://github.com/emacs-evil/evil-collection/blob/master/modes/magit/evil-collection-magit.el),
including Doom's `gz` refresh override. The pinned stock Magit manifest remains
the provenance reference for Git command identities and transient suffixes,
not a selectable input scheme. See the README for the runtime binding table.

## Initial compatibility boundary

The initial standalone application supports repository discovery, status
sections, whole-file stage/unstage/discard, commit creation, structured log and
upstream sections, branch listing and switching, fetch, and push. Branch
creation and create-and-checkout are exposed by the TUI. The interface uses
Doom Emacs's Magit/Evil bindings, including `p` for Push, `Z` for Stash, `*`
for Worktree, `z` folds, and `gr` refresh. There is no runtime scheme toggle.
Prefixes and help use responsive, grouped Magit-style transients. Incomplete
Magit commands remain documented in this compatibility matrix but are omitted
from interactive command sheets. Implemented commands that are unavailable only
in the current repository context remain visible with an explicit marker and
have no Git effect when selected.

Commit and GitHub PR messages use a shared terminal-native Vim-style buffer:
Insert/Normal/Visual modes, cursor motions, operators, undo/redo, bracketed
paste, wrapping, and cursor-following scrolling. `Esc` changes editor mode,
not dialog lifetime; `C-g` or `C-c C-k` saves and closes, and `C-c C-c`
submits. Drafts persist in Git metadata and survive failed operations.
Amend/reword preload HEAD's message. Other multiline workflows, including
rebase todo and patch cover letters, retain their existing form editor.

`Alt-r` is a terminal extension for creating or editing the current branch's
GitHub PR through authenticated `gh`. Publication has a distinct review step
and never pushes or forks automatically. This is not full Forge parity.

Aggregate `S` stages tracked modifications and deletions but excludes untracked
files; aggregate `U` clears index changes without changing worktree content.
The backtick status command opens a bounded bottom process pane containing sanitized
mutating-command transcripts and useful stderr. Failed operations open it at
the newest output. Transcript copy uses Bubble Tea's OSC52 request and cannot
confirm that the receiving terminal accepted the clipboard payload. This is a
practical process-output slice, not compatibility with Magit's complete process
buffer lifecycle and controls.

Status history follows Magit's bounded presentation: ten recent commits and at
most 256 commits on each upstream side, with `256+` shown when a side is known
to be truncated. `C-c C-r` advances through visible
commit rows carrying Git `%D` decorations and stops at the final visible
reference. `C-c C-e` and `C-c C-o` open a selected file, revision, or stash in
the read-only terminal detail pane. They deliberately do not invoke an ambient
editor, browser, or URL handler; this standalone status model has no safe
equivalent for Magit's buffer-local remaps. `C-c` remains a prefix. Push-remote
fetch honors a branch setting or
`remote.pushDefault`; when neither exists, the TUI can set the current branch's
push remote before fetching. A dedicated `remote.pushDefault` prompt is not
implemented.

`p p` retains Git's normal push behavior for a branch with an upstream. For a
branch without one, it uses the configured push remote with `--set-upstream`,
or offers a **Push and set upstream** destination chooser. The chooser does not
write push-remote configuration before pushing; a successful
`git push --set-upstream` establishes the branch upstream itself, while a failed
or cancelled push leaves no new branch remote configuration. Detached, unborn, and Git command
failures remain backend errors and are shown through the process pane.

Magit's Emacs APIs, package extensions, submodule management, worktree management, forge integration, and every transient option are not yet exact-compatible. For an unresolved path, `e` renders bounded base/ours/theirs index blobs and `E t m` provides a reviewed, stale-safe terminal-native ours/theirs checkout followed by staging that exact path. This deliberately never launches an external mergetool, editor, or shell. Base remains inspect-only: stock Git has no base checkout mode, so the UI does not pretend otherwise. Merge continuation is reviewed against the prepared index tree; rebase, cherry-pick, and revert continue/abort controls use their reviewed sequencer state.

The history transients provide reviewed merge, non-interactive rebase, cherry-pick/revert, reset, and bisect paths; revisions are resolved to object IDs and execution rejects stale repository state. Interactive rebase has a terminal-native multiline todo editor with reviewed, revision-bound pick/reword/edit/squash/fixup/drop instructions and active rebase todo editing plus continue/skip/abort. It never invokes `$EDITOR` or a user shell: a sealed lazymagit callback installs the reviewed todo. `exec`, merge-topology commands, aliases, and autosquash rewriting remain unavailable. `bisect run` likewise remains unavailable rather than accepting arbitrary command execution. Unsafe discard of mixed staged and unstaged content is intentionally rejected.
