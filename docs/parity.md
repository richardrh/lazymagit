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
| Status file sections | Partial | Magit headings/order, independent staged/unstaged state, stable folds | Inline hunks, conflicts, operation headers |
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
| Conflicts | Missing workflow | Unmerged state parses | Ours/theirs/base, resolve, continue/abort |
| Stash | Partial | `z` transient workflows plus folded status section, `j z` jump, stable OID rows, and bounded lazy patch detail | Remaining stash suffixes and exact section presentation edge cases |
| Merge/rebase/cherry-pick | Missing | — | Start, sequence status, continue/skip/abort |
| Reset | Missing | Magit `x`/`X` are recognized and report their upstream missing status; Vim `x` remains the adapted discard key | Quick reset and `X` transient |
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

The next core parity milestone is practical conflict and history-editing workflows, followed by richer multi-selection patch operations. These provide more Magit value than adding rarely used top-level commands out of order.
