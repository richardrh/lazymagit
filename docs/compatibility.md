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

## Initial compatibility boundary

The initial standalone application supports repository discovery, status
sections, whole-file stage/unstage/discard, commit creation, structured log and
upstream sections, branch listing and switching, fetch, and push. Branch
creation is implemented and integration-tested in the backend but is not yet
exposed by the TUI. The interface uses Magit's familiar command prefixes and
offers separate Vim and Magit key schemes where their single-key bindings
conflict. Prefixes and help use responsive, grouped Magit-style transients.
Important Magit suffixes outside the implemented compatibility boundary remain
visible with an explicit unavailable marker; selecting one has no Git effect.

Aggregate `S` stages tracked modifications and deletions but excludes untracked
files; aggregate `U` clears index changes without changing worktree content.
The `$` status command opens a bounded bottom process pane containing sanitized
mutating-command transcripts and useful stderr. Failed operations open it at
the newest output. Transcript copy uses Bubble Tea's OSC52 request and cannot
confirm that the receiving terminal accepted the clipboard payload. This is a
practical process-output slice, not compatibility with Magit's complete process
buffer lifecycle and controls.

Status history follows Magit's bounded presentation: ten recent commits and at
most 256 commits on each upstream side, with `256+` shown when a side is known
to be truncated. Push-remote fetch honors a branch setting or
`remote.pushDefault`; when neither exists, the TUI can set the current branch's
push remote before fetching. A dedicated `remote.pushDefault` prompt is not
implemented.

`P p` retains Git's normal push behavior for a branch with an upstream. For a
branch without one, it uses the configured push remote with `--set-upstream`,
or offers a **Push and set upstream** destination chooser. The chooser does not
write push-remote configuration before pushing; a successful
`git push --set-upstream` establishes the branch upstream itself, while a failed
or cancelled push leaves no new branch remote configuration. Detached, unborn, and Git command
failures remain backend errors and are shown through the process pane.

Magit's Emacs APIs, package extensions, complete conflict resolution, submodule
management, worktree management, forge integration, and every transient option
are not yet exact-compatible. The history transients provide reviewed merge,
non-interactive rebase, cherry-pick/revert, reset, and bisect paths; revisions
are resolved to object IDs and execution rejects stale repository state.
Interactive rebase has a terminal-native multiline todo editor with reviewed,
revision-bound pick/reword/edit/squash/fixup/drop instructions and active
rebase todo editing plus continue/skip/abort. It never invokes `$EDITOR` or a
user shell: a sealed lazymagit callback installs the reviewed todo. `exec`,
merge-topology commands, aliases, and autosquash rewriting remain unavailable.
`bisect run` likewise remains unavailable rather than accepting arbitrary
command execution. Unsafe discard of mixed staged and
unstaged content is intentionally rejected.
