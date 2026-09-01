# Magit v4.7 parity matrix

Behavioral baseline: Magit v4.7.0 at
`67f203853e74e926e2c99f60ed508840714f7ced`.

This matrix prevents a matching key label from being mistaken for matching
behavior. A feature is **exact** only when its key sequence, repository result,
important prompts, and safety behavior have black-box tests. **Partial** means
the useful first path exists but Magit's transient options or edge cases do
not.

| Area | State | Current behavior | Important gap |
|---|---|---|---|
| Repository discovery/init | Exact slice | Nested discovery, bare detection, safe init prompt | Bare status UI |
| Status file sections | Partial | Magit headings/order, independent staged/unstaged state, stable folds, and unresolved-path selection | Inline hunks and operation headers |
| Status commit sections | Partial | Magit hash/ref/subject rows, recent fallback, upstream ranges; `C-c C-r` advances visible decorated rows without wrapping | Full decoration ordering/faces and configurable log arguments |
| Top-level thing controls | Adapted | `C-c C-e`/`C-c C-o` open typed status items in the read-only detail pane | No ambient editor/browser/URL handler or Emacs buffer-local remaps |
| Revision inspection | Partial | Metadata, changed-file stat/names, bounded full patch, selectable `Ctrl-B` blame lines, and selectable bounded commit logs/ASCII graphs; navigation selects exact commits, `Enter` inspects them, history transients use the selected revision, `p` opens its first parent, and `Esc` restores the prior graph or blame cursor | Merge-parent selection, branch/ref actions from graph rows, and parent/earlier-revision blame traversal |
| Whole-file stage/unstage | Partial | `Alt-M` marks independent status rows; `s`/`u` batch compatible marked files and `x` reviews a marked batch discard; `S` stages all tracked changes excluding untracked; `U` clears index changes while preserving the worktree | Cross-section commands beyond stage/unstage/discard and Magit's context-sensitive defaults |
| Hunk/region operations | Partial | `[`/`]` focus hunks; `V` toggles noncontiguous hunks; `v`/`Space` collects disjoint typed changed-line regions; reviewed stage, unstage, and discard regenerate and stale-check the exact patch before mutation | Binary/rename region operations and semantic patch editing beyond typed region refinement |
| Discard | Partial | Confirmed safe whole-file discard plus reviewed unstaged/staged hunk, multi-hunk, and changed-line discard; unresolved target conflicts are rejected | Mixed-state whole-file preservation edge cases |
| Commit | Partial | `c` transient supports create, extend, amend, reword, fixup, squash, alter, augment, and revise; typed author/date/signoff/reuse/reedit/signing options; bounded terminal-native multiline message editing; hooks and explicit signing consent | `--verbose` preview, external editor workflows, and all Magit message-editing edge cases |
| Branch | Partial | Checked-out/local/remote revision selection, create/create-and-checkout/orphan, rename, reviewed delete/reset, branch/upstream/push/pull configuration, and new worktree creation | Exact Magit spin-off semantics and editor-backed branch-description workflows |
| Remote | Partial | Add with optional fetch, rename, reviewed removal/prune/unshallow, default-branch update, and typed fetch/push URL/refspec/tag/follow configuration; branch Configure exposes `remote.pushDefault` | Full Git remote command surface and arbitrary remote-helper execution |
| Fetch | Partial | `f u`, `f p`, `f e`, `f a`, explicit remote/branch/refspec/module fetch, push-remote configuration, and typed prune/tags/unshallow/force options | Remaining Git fetch flags and arbitrary command-line forms |
| Pull | Partial | `F` transient supports upstream, push-remote, or selected remote branch with merge/rebase/ff-only, autostash, and force choices; related fetch/configure suffixes route to their typed workflows | Broader pull configuration and all Git/Magit mode combinations |
| Push | Partial | Reviewed push to push-remote/upstream/selected remote, another branch, explicit refspecs, matching branches, one/all tags, and notes; typed force-with-lease/force/no-verify/dry-run/set-upstream/tags/follow-tags/push-option support; persistent branch push-remote changes are reviewed | All Git push modes and repository-wide configuration UI beyond Configure |
| Diff/log | Partial | `d`/`l` provide bounded status/revision/range diffs, unified terminal comparisons, selectable graph/decorated history, current/other/all/branch/tag matching logs, reflogs, shortlogs, and refs | Richer persistent filters and Magit's full graph/revision UI |
| Conflicts | Partial | `e` safely shows bounded base/ours/theirs index blobs; `1` identifies inspect-only base, `2`/`3` select ours/theirs, and `r` opens the stale-safe reviewed resolver directly; `E t m` remains available; continuation controls are operation-aware | Base is inspect-only because stock Git provides no base checkout mode; no manual merge-buffer editing, optional external mergetool, or bulk resolve actions |
| Patch workflows | Partial | `w`/`W` support reviewed `git am`, checked/reviewed plain `git apply`, and bounded reviewed `format-patch` publication; format dialogs expose numbering, signoff, shallow/deep threading, RFC/subject/reroll/start/base metadata, bounded From/To/Cc/In-Reply-To headers, and a terminal-native editable cover letter bound to the review token | No request-pull, GPG, external editor, mail sending, arbitrary shell, mail parsing controls beyond 3-way/scissors/signoff/keep-CR, or format-patch diff-algorithm/interdiff/range-diff/notes/cover-from-description knobs |
| Stash | Partial | `z` transient workflows plus folded status section, `j z` jump, stable OID rows, and bounded lazy patch detail | Remaining stash suffixes and exact section presentation edge cases |
| Merge | Partial | `m` transient: reviewed target selection, plain/no-commit/squash/preview, `--ff-only`/`--no-ff`, typed strategy/signing options, and stale-safe reviewed conflict continue/abort | Editor-backed merge messages, absorb/dissolve |
| Rebase | Partial | `r` transient: reviewed non-interactive current-branch rebase, upstream/push-remote targets, typed keep-empty/rebase-merges/update-refs/autostash/force/strategy/signoff options, and terminal-native reviewed interactive todo editing (`pick`/`reword`/`edit`/`squash`/`fixup`/`drop`) with active edits and continue/skip/abort | `exec`, aliases, merge-topology todo commands, and autosquash rewriting remain unavailable; interactive merge topology is rejected |
| Cherry-pick/revert | Partial | `A`/`a` and `V`/`v`: reviewed resolved commits with mainline/strategy/signoff, non-editor apply, and reviewed continue/skip/abort | Multi-commit selection and editor-backed message editing |
| Reset | Partial | `x` quick mixed reset and `X` transient: reviewed mixed/soft/hard/keep/index/worktree/file reset with stale-state rejection | Magit's context-sensitive defaults and revision browser |
| Bisect | Partial | `B` transient: reviewed start/good/bad/mark/skip/reset, typed `--no-checkout`/`--first-parent`, and paired custom old/new terms; later marks safely use the active terms | `bisect run` stays unavailable because arbitrary command execution is unsafe |
| Tags | Partial | `t` creates lightweight/annotated/signed/release tags with reviewed force replacement; reviewed delete and remote-comparison prune; push transient handles one/all tags | Multi-tag selection and the full remote-tag management surface |
| Submodules/worktrees | Partial | `o` manages add/init/update/sync/deinit/remove/list/fetch submodules; `Z` manages reviewed add/new-branch/move/remove/prune plus list/lock/unlock worktrees; subtree and sparse-checkout transients provide typed terminal-native workflows, including reviewed cone/non-cone setup, sparse index, set/add/reapply, and explicit disable restoration | Submodule status presentation, recursive multi-module selection, and every Git worktree/subtree/sparse option |
| Process output | Partial | `$` bottom process pane, bounded sanitized chronological command/output transcript, failure auto-open, OSC52 copy request | Magit's full process lifecycle controls and persistent process buffer behavior |
| Emacs Lisp extension API | Not applicable | Standalone Go program | No claim of Emacs package compatibility |

## Audit process

1. Pin one Magit revision; never compare against an unspecified moving target.
2. Extract top-level keymaps and every transient prefix/suffix into a manifest.
3. Capture real Magit buffers from deterministic temporary repositories.
4. For each action, run Magit and lazymagit against equivalent fixtures and
   compare Git plumbing state (`diff`, `diff --cached`, refs, config, and log),
   not screenshots alone.
5. Test the full key sequence and prompts in the TUI state machine.
6. Keep destructive edge cases as mandatory regression tests before marking a
   row exact.

The next core parity milestones are branch/ref actions from persistent history, multi-file selection, merge-parent selection, and deeper terminal-native blame controls. These provide more daily terminal value than adding Emacs-only integrations out of order.
