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
| Status commit sections | Partial | Magit hash/ref/subject rows, recent fallback, upstream ranges | Full decoration ordering/faces and configurable log arguments |
| Revision inspection | Partial | Metadata, changed-file stat/names, bounded full patch | Revision-local commands and parent navigation |
| Whole-file stage/unstage | Partial | `s`/`u`; `S` stages all tracked changes excluding untracked; `U` clears index changes while preserving the worktree; unborn branches, renames/deletions, literal paths | Multi-selection operations |
| Hunk/region operations | Partial | `[`/`]` focus hunks; `v` selects changed lines; reviewed stage, unstage, and discard regenerate and stale-check the exact patch before mutation | Split/refine hunks, arbitrary disjoint selections, binary/rename region operations |
| Discard | Partial | Confirmed safe whole-file discard plus reviewed unstaged/staged hunk and changed-line discard | Mixed-state whole-file preservation edge cases and multi-selection discard |
| Commit | Partial | `c c`, hooks, one-line message, responsive command catalog | Multiline editor and executable amend/fixup/squash/reword/signoff |
| Branch | Partial | `b b` local switch; backend create/list | TUI create/delete/rename/upstream/spin-off |
| Remote | Partial | `M a`, separate name/URL, fetch-on-add default | `remote.pushDefault` prompt, rename/remove/set-url/prune |
| Fetch | Partial | `f u`, `f p`, `f e`, `f a`; push-remote configuration; unavailable suffixes marked | Executable transient flags and refspec/branch fetch |
| Pull | Missing | — | `F` transient, merge/rebase choices |
| Push | Partial | `P p` preserves plain push with an existing upstream; otherwise uses the configured push remote or a **Push and set upstream** destination chooser without pre-writing push-remote config | Arbitrary destination/upstream prompts, repository-wide push-default prompt, tags, force-with-lease transient |
| Diff/log | Partial | File diffs and structured status logs | General `d`/`l` views, ranges, graph, filters |
| Conflicts | Partial | `e` safely shows bounded base/ours/theirs index blobs for a selected unresolved path; `E t m` provides reviewed Git-native ours/theirs resolution and stages that path; merge/rebase/cherry-pick/revert retain their appropriate continue/abort controls | Base is inspect-only because stock Git provides no base checkout mode; no external mergetool/editor, manual merge-buffer editing, or bulk Ediff resolve actions |
| Patch workflows | Partial | `w`/`W` support reviewed `git am`, checked/reviewed plain `git apply`, and bounded reviewed `format-patch` publication; format dialogs expose numbering, cover/signoff, threading, subject, reroll/start numbers, and To/Cc | No request-pull, GPG/editor flows, mail parsing controls beyond 3-way/scissors/signoff/keep-CR, or format-patch diff-algorithm/interdiff/range-diff knobs; thread style is intentionally reduced to Git's safe default |
| Stash | Partial | `z` transient workflows plus folded status section, `j z` jump, stable OID rows, and bounded lazy patch detail | Remaining stash suffixes and exact section presentation edge cases |
| Merge | Partial | `m` transient: reviewed target selection, plain/no-commit/squash/preview, `--ff-only`/`--no-ff`, typed strategy/signing options, and stale-safe reviewed conflict continue/abort | Editor-backed merge messages, absorb/dissolve |
| Rebase | Partial | `r` transient: reviewed non-interactive current-branch rebase, upstream/push-remote targets, typed keep-empty/rebase-merges/update-refs/autostash/force/strategy/signoff options, and terminal-native reviewed interactive todo editing (`pick`/`reword`/`edit`/`squash`/`fixup`/`drop`) with active edits and continue/skip/abort | `exec`, aliases, merge-topology todo commands, and autosquash rewriting remain unavailable; interactive merge topology is rejected |
| Cherry-pick/revert | Partial | `A`/`a` and `V`/`v`: reviewed resolved commits with mainline/strategy/signoff, non-editor apply, and reviewed continue/skip/abort | Multi-commit selection and editor-backed message editing |
| Reset | Partial | `x` quick mixed reset and `X` transient: reviewed mixed/soft/hard/keep/index/worktree/file reset with stale-state rejection | Magit's context-sensitive defaults and revision browser |
| Bisect | Partial | `B` transient: reviewed start/good/bad/mark/skip/reset plus typed `--no-checkout` and `--first-parent` start options | `bisect run` stays unavailable because arbitrary command execution is unsafe; custom terms are not exposed |
| Tags | Partial | Decorations display | `t` create/delete/prune/push |
| Submodules/worktrees | Missing workflow | Repository discovery generally works | `o`/`Z` management transients |
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

The next core parity milestone is richer history editing and multi-selection patch operations. These provide more Magit value than adding rarely used top-level commands out of order.
