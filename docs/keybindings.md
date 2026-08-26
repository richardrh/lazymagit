# Magit v4.7 status keybinding ledger

Generated from the vendored manifest for Magit 4.7.0 (`v4.7.0`, `67f203853e74e926e2c99f60ed508840714f7ced`, clean checkout). Run `go run ./internal/keymap/cmd/keymapdoc` to update or add `-check` to verify drift.

| # | Upstream key | Canonical input | Upstream command | Kind | Domain | Layer | Source | Classification | Current status |
|---:|---|---|---|---|---|---|---|---|---|
| 1 | `<left-fringe> <mouse-1>` | `emacs:<left-fringe> <mouse-1>` | `magit-mouse-toggle-section` | binding | ui | `magit-section-mode-map` | `lisp/magit-section.el:463 (magit-section-mode-map)` | `not-applicable` | Emacs-only input or integration |
| 2 | `<left-fringe> <mouse-2>` | `emacs:<left-fringe> <mouse-2>` | `magit-mouse-toggle-section` | binding | ui | `magit-section-mode-map` | `lisp/magit-section.el:464 (magit-section-mode-map)` | `not-applicable` | Emacs-only input or integration |
| 3 | `TAB` | `tab` | `magit-section-toggle` | binding | ui | `magit-section-mode-map` | `lisp/magit-section.el:465 (magit-section-mode-map)` | `partial` | registered handler |
| 4 | `C-c TAB` | `ctrl+c tab` | `magit-section-cycle` | binding | ui | `magit-section-mode-map` | `lisp/magit-section.el:466 (magit-section-mode-map)` | `partial` | registered handler |
| 5 | `C-<tab>` | `ctrl+tab` | `magit-section-cycle` | binding | ui | `magit-section-mode-map` | `lisp/magit-section.el:467 (magit-section-mode-map)` | `partial` | registered handler |
| 6 | `<backtab>` | `<backtab>` | `magit-section-cycle-global` | binding | ui | `magit-section-mode-map` | `lisp/magit-section.el:470 (magit-section-mode-map)` | `missing` | not implemented |
| 7 | `^` | `^` | `magit-section-up` | binding | ui | `magit-section-mode-map` | `lisp/magit-section.el:471 (magit-section-mode-map)` | `missing` | not implemented |
| 8 | `p` | `p` | `magit-section-backward` | binding | ui | `magit-section-mode-map` | `lisp/magit-section.el:472 (magit-section-mode-map)` | `partial` | registered handler |
| 9 | `n` | `n` | `magit-section-forward` | binding | ui | `magit-section-mode-map` | `lisp/magit-section.el:473 (magit-section-mode-map)` | `partial` | registered handler |
| 10 | `M-p` | `alt+p` | `magit-section-backward-sibling` | binding | ui | `magit-section-mode-map` | `lisp/magit-section.el:474 (magit-section-mode-map)` | `missing` | not implemented |
| 11 | `M-n` | `alt+n` | `magit-section-forward-sibling` | binding | ui | `magit-section-mode-map` | `lisp/magit-section.el:475 (magit-section-mode-map)` | `missing` | not implemented |
| 12 | `1` | `1` | `magit-section-show-level-1` | binding | ui | `magit-section-mode-map` | `lisp/magit-section.el:476 (magit-section-mode-map)` | `partial` | registered handler |
| 13 | `2` | `2` | `magit-section-show-level-2` | binding | ui | `magit-section-mode-map` | `lisp/magit-section.el:477 (magit-section-mode-map)` | `partial` | registered handler |
| 14 | `3` | `3` | `magit-section-show-level-3` | binding | ui | `magit-section-mode-map` | `lisp/magit-section.el:478 (magit-section-mode-map)` | `partial` | registered handler |
| 15 | `4` | `4` | `magit-section-show-level-4` | binding | ui | `magit-section-mode-map` | `lisp/magit-section.el:479 (magit-section-mode-map)` | `missing` | not implemented |
| 16 | `M-1` | `alt+1` | `magit-section-show-level-1-all` | binding | ui | `magit-section-mode-map` | `lisp/magit-section.el:480 (magit-section-mode-map)` | `missing` | not implemented |
| 17 | `M-2` | `alt+2` | `magit-section-show-level-2-all` | binding | ui | `magit-section-mode-map` | `lisp/magit-section.el:481 (magit-section-mode-map)` | `missing` | not implemented |
| 18 | `M-3` | `alt+3` | `magit-section-show-level-3-all` | binding | ui | `magit-section-mode-map` | `lisp/magit-section.el:482 (magit-section-mode-map)` | `missing` | not implemented |
| 19 | `M-4` | `alt+4` | `magit-section-show-level-4-all` | binding | ui | `magit-section-mode-map` | `lisp/magit-section.el:483 (magit-section-mode-map)` | `missing` | not implemented |
| 20 | `C-<return>` | `ctrl+enter` | `magit-visit-thing` | binding | ui | `magit-mode-map` | `lisp/magit-mode.el:354 (magit-mode-map)` | `missing` | not implemented |
| 21 | `RET` | `enter` | `magit-visit-thing` | binding | ui | `magit-mode-map` | `lisp/magit-mode.el:355 (magit-mode-map)` | `missing` | not implemented |
| 22 | `M-TAB` | `emacs:M-TAB` | `magit-dired-jump` | binding | emacs | `magit-mode-map` | `lisp/magit-mode.el:356 (magit-mode-map)` | `not-applicable` | Emacs-only input or integration |
| 23 | `M-<tab>` | `alt+tab` | `magit-section-cycle-diffs` | binding | ui | `magit-mode-map` | `lisp/magit-mode.el:357 (magit-mode-map)` | `missing` | not implemented |
| 24 | `SPC` | `space` | `magit-diff-show-or-scroll-up` | binding | ui | `magit-mode-map` | `lisp/magit-mode.el:358 (magit-mode-map)` | `partial` | registered handler |
| 25 | `S-SPC` | `shift+space` | `magit-diff-show-or-scroll-down` | binding | ui | `magit-mode-map` | `lisp/magit-mode.el:359 (magit-mode-map)` | `partial` | registered handler |
| 26 | `DEL` | `backspace` | `magit-diff-show-or-scroll-down` | binding | ui | `magit-mode-map` | `lisp/magit-mode.el:360 (magit-mode-map)` | `missing` | not implemented |
| 27 | `+` | `+` | `magit-diff-more-context` | binding | ui | `magit-mode-map` | `lisp/magit-mode.el:361 (magit-mode-map)` | `missing` | not implemented |
| 28 | `-` | `-` | `magit-diff-less-context` | binding | ui | `magit-mode-map` | `lisp/magit-mode.el:362 (magit-mode-map)` | `missing` | not implemented |
| 29 | `0` | `0` | `magit-diff-default-context` | binding | ui | `magit-mode-map` | `lisp/magit-mode.el:363 (magit-mode-map)` | `missing` | not implemented |
| 30 | `a` | `a` | `magit-cherry-apply` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:364 (magit-mode-map)` | `missing` | not implemented |
| 31 | `A` | `A` | `magit-cherry-pick` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:364 (magit-mode-map)` | `partial` | registered transient |
| 32 | `b` | `b` | `magit-branch` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:366 (magit-mode-map)` | `partial` | registered transient |
| 33 | `B` | `B` | `magit-bisect` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:366 (magit-mode-map)` | `partial` | registered transient |
| 34 | `c` | `c` | `magit-commit` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:368 (magit-mode-map)` | `partial` | registered transient |
| 35 | `C` | `C` | `magit-clone` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:368 (magit-mode-map)` | `partial` | registered transient |
| 36 | `d` | `d` | `magit-diff` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:370 (magit-mode-map)` | `partial` | registered transient |
| 37 | `D` | `D` | `magit-diff-refresh` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:370 (magit-mode-map)` | `partial` | registered transient |
| 38 | `e` | `e` | `magit-ediff-dwim` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:372 (magit-mode-map)` | `missing` | not implemented |
| 39 | `E` | `E` | `magit-ediff` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:372 (magit-mode-map)` | `partial` | registered transient |
| 40 | `f` | `f` | `magit-fetch` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:374 (magit-mode-map)` | `partial` | registered transient |
| 41 | `F` | `F` | `magit-pull` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:374 (magit-mode-map)` | `partial` | registered transient |
| 42 | `g` | `g` | `magit-refresh` | binding | ui | `magit-mode-map` | `lisp/magit-mode.el:376 (magit-mode-map)` | `partial` | registered handler |
| 43 | `G` | `G` | `magit-refresh-all` | binding | ui | `magit-mode-map` | `lisp/magit-mode.el:376 (magit-mode-map)` | `partial` | registered handler |
| 44 | `h` | `h` | `magit-dispatch` | binding | ui | `magit-mode-map` | `lisp/magit-mode.el:378 (magit-mode-map)` | `partial` | registered transient |
| 45 | `?` | `?` | `magit-dispatch` | binding | ui | `magit-mode-map` | `lisp/magit-mode.el:379 (magit-mode-map)` | `partial` | registered transient |
| 46 | `H` | `H` | `magit-describe-section` | binding | ui | `magit-mode-map` | `lisp/magit-mode.el:378 (magit-mode-map)` | `missing` | not implemented |
| 47 | `i` | `i` | `magit-gitignore` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:381 (magit-mode-map)` | `partial` | registered transient |
| 48 | `I` | `I` | `magit-init` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:381 (magit-mode-map)` | `missing` | not implemented |
| 49 | `J` | `J` | `magit-display-repository-buffer` | binding | ui | `magit-mode-map` | `lisp/magit-mode.el:383 (magit-mode-map)` | `missing` | not implemented |
| 50 | `k` | `k` | `magit-delete-thing` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:385 (magit-mode-map)` | `partial` | registered handler |
| 51 | `K` | `K` | `magit-file-untrack` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:385 (magit-mode-map)` | `missing` | not implemented |
| 52 | `l` | `l` | `magit-log` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:387 (magit-mode-map)` | `partial` | registered transient |
| 53 | `L` | `L` | `magit-log-refresh` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:387 (magit-mode-map)` | `partial` | registered transient |
| 54 | `m` | `m` | `magit-merge` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:389 (magit-mode-map)` | `partial` | registered transient |
| 55 | `M` | `M` | `magit-remote` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:389 (magit-mode-map)` | `partial` | registered transient |
| 56 | `o` | `o` | `magit-submodule` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:393 (magit-mode-map)` | `partial` | registered transient |
| 57 | `O` | `O` | `magit-subtree` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:393 (magit-mode-map)` | `partial` | registered transient |
| 58 | `P` | `P` | `magit-push` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:395 (magit-mode-map)` | `partial` | registered transient |
| 59 | `q` | `q` | `magit-mode-bury-buffer` | binding | ui | `magit-mode-map` | `lisp/magit-mode.el:397 (magit-mode-map)` | `partial` | registered handler |
| 60 | `Q` | `Q` | `magit-git-command` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:397 (magit-mode-map)` | `missing` | not implemented |
| 61 | `:` | `:` | `magit-git-command` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:399 (magit-mode-map)` | `missing` | not implemented |
| 62 | `r` | `r` | `magit-rebase` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:400 (magit-mode-map)` | `partial` | registered transient |
| 63 | `R` | `R` | `magit-file-rename` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:400 (magit-mode-map)` | `missing` | not implemented |
| 64 | `s` | `s` | `magit-stage-files` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:402 (magit-mode-map)` | `partial` | registered handler |
| 65 | `S` | `S` | `magit-stage-modified` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:402 (magit-mode-map)` | `partial` | registered handler |
| 66 | `t` | `t` | `magit-tag` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:404 (magit-mode-map)` | `partial` | registered transient |
| 67 | `T` | `T` | `magit-notes` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:404 (magit-mode-map)` | `partial` | registered transient |
| 68 | `u` | `u` | `magit-unstage-files` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:406 (magit-mode-map)` | `partial` | registered handler |
| 69 | `U` | `U` | `magit-unstage-all` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:406 (magit-mode-map)` | `partial` | registered handler |
| 70 | `v` | `v` | `magit-revert-no-commit` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:408 (magit-mode-map)` | `missing` | not implemented |
| 71 | `V` | `V` | `magit-revert` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:408 (magit-mode-map)` | `partial` | registered transient |
| 72 | `w` | `w` | `magit-am` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:410 (magit-mode-map)` | `partial` | registered transient |
| 73 | `W` | `W` | `magit-patch` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:410 (magit-mode-map)` | `partial` | registered transient |
| 74 | `x` | `x` | `magit-reset-quickly` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:412 (magit-mode-map)` | `missing` | not implemented |
| 75 | `X` | `X` | `magit-reset` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:412 (magit-mode-map)` | `partial` | registered transient |
| 76 | `y` | `y` | `magit-show-refs` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:414 (magit-mode-map)` | `partial` | registered transient |
| 77 | `Y` | `Y` | `magit-cherry` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:414 (magit-mode-map)` | `missing` | not implemented |
| 78 | `z` | `z` | `magit-stash` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:416 (magit-mode-map)` | `partial` | registered transient |
| 79 | `Z` | `Z` | `magit-worktree` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:416 (magit-mode-map)` | `partial` | registered transient |
| 80 | `%` | `%` | `magit-worktree` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:418 (magit-mode-map)` | `partial` | registered transient |
| 81 | `$` | `$` | `magit-process-buffer` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:419 (magit-mode-map)` | `partial` | registered handler |
| 82 | `!` | `!` | `magit-run` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:420 (magit-mode-map)` | `partial` | registered transient |
| 83 | `>` | `>` | `magit-sparse-checkout` | binding | git | `magit-mode-map` | `lisp/magit-mode.el:421 (magit-mode-map)` | `partial` | registered transient |
| 84 | `C-c C-c` | `ctrl+c ctrl+c` | `magit-dispatch` | binding | ui | `magit-mode-map` | `lisp/magit-mode.el:422 (magit-mode-map)` | `partial` | registered transient |
| 85 | `C-c C-r` | `ctrl+c ctrl+r` | `magit-next-reference` | binding | ui | `magit-mode-map` | `lisp/magit-mode.el:423 (magit-mode-map)` | `missing` | not implemented |
| 86 | `C-c C-e` | `ctrl+c ctrl+e` | `magit-edit-thing` | binding | ui | `magit-mode-map` | `lisp/magit-mode.el:424 (magit-mode-map)` | `missing` | not implemented |
| 87 | `C-c C-o` | `ctrl+c ctrl+o` | `magit-browse-thing` | binding | ui | `magit-mode-map` | `lisp/magit-mode.el:425 (magit-mode-map)` | `missing` | not implemented |
| 88 | `C-c C-w` | `ctrl+c ctrl+w` | `magit-copy-thing` | binding | ui | `magit-mode-map` | `lisp/magit-mode.el:426 (magit-mode-map)` | `partial` | registered handler |
| 89 | `C-w` | `ctrl+w` | `magit-copy-section-value` | binding | ui | `magit-mode-map` | `lisp/magit-mode.el:427 (magit-mode-map)` | `partial` | registered handler |
| 90 | `M-w` | `alt+w` | `magit-copy-buffer-revision` | binding | ui | `magit-mode-map` | `lisp/magit-mode.el:428 (magit-mode-map)` | `partial` | registered handler |
| 91 | `<remap> <mouse-set-point>` | `emacs:<remap> <mouse-set-point>` | `magit-mouse-set-point` | binding | ui | `magit-mode-map` | `lisp/magit-mode.el:429 (magit-mode-map)` | `not-applicable` | Emacs-only input or integration |
| 92 | `<remap> <back-to-indentation>` | `emacs:<remap> <back-to-indentation>` | `magit-back-to-indentation` | binding | ui | `magit-mode-map` | `lisp/magit-mode.el:430 (magit-mode-map)` | `not-applicable` | Emacs-only input or integration |
| 93 | `<remap> <previous-line>` | `emacs:<remap> <previous-line>` | `magit-previous-line` | binding | ui | `magit-mode-map` | `lisp/magit-mode.el:431 (magit-mode-map)` | `not-applicable` | Emacs-only input or integration |
| 94 | `<remap> <next-line>` | `emacs:<remap> <next-line>` | `magit-next-line` | binding | ui | `magit-mode-map` | `lisp/magit-mode.el:432 (magit-mode-map)` | `not-applicable` | Emacs-only input or integration |
| 95 | `<remap> <evil-previous-line>` | `emacs:<remap> <evil-previous-line>` | `evil-previous-visual-line` | binding | emacs | `magit-mode-map` | `lisp/magit-mode.el:433 (magit-mode-map)` | `not-applicable` | Emacs-only input or integration |
| 96 | `<remap> <evil-next-line>` | `emacs:<remap> <evil-next-line>` | `evil-next-visual-line` | binding | emacs | `magit-mode-map` | `lisp/magit-mode.el:434 (magit-mode-map)` | `not-applicable` | Emacs-only input or integration |
| 97 | `j` | `j` | `magit-status-jump` | binding | ui | `magit-status-mode-map` | `lisp/magit-status.el:422 (magit-status-mode-map)` | `partial` | registered transient |
| 98 | `<remap> <dired-jump>` | `emacs:<remap> <dired-jump>` | `magit-dired-jump` | binding | emacs | `magit-status-mode-map` | `lisp/magit-status.el:423 (magit-status-mode-map)` | `not-applicable` | Emacs-only input or integration |

## Manifest identity

- Schema: **1.0.0**
- Mode: **magit-status-mode**
- Effective map chain: `magit-status-mode-map` → `magit-mode-map` → `magit-section-mode-map`
- Effective top-level status bindings: **98**
- Recursively reachable transients: **44**
- Transient entry occurrences: **554**

## Recursively reachable transients

All manifest transients are generated from effective status bindings and transient suffix edges. Every occurrence is retained, including infixes, conditional duplicate keys, provenance, and multi-token suffixes. Infixes are actionable only when static capability data declares a consumer.

### `w` — magit-am (16 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `-3` | `transient:magit-am:--3way` | **infix** | Arguments | if-not: magit-am-in-progress-p | `missing` | infix: argument editing is not implemented |
| `-R` | `transient:magit-am:--reject` | **infix** | Arguments | if-not: magit-am-in-progress-p | `missing` | infix: argument editing is not implemented |
| `-p` | `magit-apply:-p` | **infix** | Arguments | if-not: magit-am-in-progress-p | `missing` | infix: argument editing is not implemented |
| `-c` | `transient:magit-am:--scissors` | **infix** | Arguments | if-not: magit-am-in-progress-p | `missing` | infix: argument editing is not implemented |
| `-k` | `transient:magit-am:--keep` | **infix** | Arguments | if-not: magit-am-in-progress-p | `missing` | infix: argument editing is not implemented |
| `-b` | `transient:magit-am:--keep-non-patch` | **infix** | Arguments | if-not: magit-am-in-progress-p | `missing` | infix: argument editing is not implemented |
| `-d` | `transient:magit-am:--committer-date-is-author-date` | **infix** | Arguments | if-not: magit-am-in-progress-p | `missing` | infix: argument editing is not implemented |
| `-t` | `transient:magit-am:--ignore-date` | **infix** | Arguments | if-not: magit-am-in-progress-p | `missing` | infix: argument editing is not implemented |
| `-S` | `magit:--gpg-sign` | **infix** | Arguments | if-not: magit-am-in-progress-p | `missing` | infix: argument editing is not implemented |
| `+s` | `magit:--signoff` | **infix** | Arguments | if-not: magit-am-in-progress-p | `missing` | infix: argument editing is not implemented |
| `m` | `magit-am-apply-maildir` | **suffix** | Apply | if-not: magit-am-in-progress-p | `missing` | suffix: not implemented |
| `w` | `magit-am-apply-patches` | **suffix** | Apply | if-not: magit-am-in-progress-p | `missing` | suffix: not implemented |
| `a` | `magit-patch-apply` | **suffix** | Apply | if-not: magit-am-in-progress-p | `partial` |  |
| `w` | `magit-am-continue` | **suffix** | Actions | if: magit-am-in-progress-p | `missing` | suffix: not implemented |
| `s` | `magit-am-skip` | **suffix** | Actions | if: magit-am-in-progress-p | `missing` | suffix: not implemented |
| `a` | `magit-am-abort` | **suffix** | Actions | if: magit-am-in-progress-p | `missing` | suffix: not implemented |

### `B` — magit-bisect (12 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `-n` | `transient:magit-bisect:--no-checkout` | **infix** | Arguments | if-not: magit-bisect-in-progress-p | `missing` | infix: argument editing is not implemented |
| `-p` | `transient:magit-bisect:--first-parent` | **infix** | Arguments | if-not: magit-bisect-in-progress-p; if: #[nil ((magit-git-version>= "2.29")) (t)] | `missing` | infix: argument editing is not implemented |
| `=o` | `magit-bisect:--term-old` | **infix** | Arguments | if-not: magit-bisect-in-progress-p | `missing` | infix: argument editing is not implemented |
| `=n` | `magit-bisect:--term-new` | **infix** | Arguments | if-not: magit-bisect-in-progress-p | `missing` | infix: argument editing is not implemented |
| `B` | `magit-bisect-start` | **suffix** | Actions | if-not: magit-bisect-in-progress-p | `missing` | suffix: not implemented |
| `s` | `magit-bisect-run` | **suffix** | Actions | if-not: magit-bisect-in-progress-p | `missing` | suffix: not implemented |
| `B` | `magit-bisect-bad` | **suffix** | Actions | if: magit-bisect-in-progress-p | `missing` | suffix: not implemented |
| `g` | `magit-bisect-good` | **suffix** | Actions | if: magit-bisect-in-progress-p | `missing` | suffix: not implemented |
| `m` | `magit-bisect-mark` | **suffix** | Actions | if: magit-bisect-in-progress-p | `missing` | suffix: not implemented |
| `k` | `magit-bisect-skip` | **suffix** | Actions | if: magit-bisect-in-progress-p | `missing` | suffix: not implemented |
| `r` | `magit-bisect-reset` | **suffix** | Actions | if: magit-bisect-in-progress-p | `missing` | suffix: not implemented |
| `s` | `magit-bisect-run` | **suffix** | Actions | if: magit-bisect-in-progress-p | `missing` | suffix: not implemented |

### `b` — magit-branch (24 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `d` | `magit-branch.<branch>.description` | **infix** | #[nil ((concat (propertize "Configure " 'face 'transient-heading) (propertize (transient-scope) 'face 'magit-branch-local))) (t)] | if: #[nil ((and magit-branch-direct-configure (transient-scope))) (t)] | `partial` | actionable in corresponding Configure dialog |
| `u` | `magit-branch.<branch>.merge/remote` | **infix** | #[nil ((concat (propertize "Configure " 'face 'transient-heading) (propertize (transient-scope) 'face 'magit-branch-local))) (t)] | if: #[nil ((and magit-branch-direct-configure (transient-scope))) (t)] | `partial` | actionable in corresponding Configure dialog |
| `r` | `magit-branch.<branch>.rebase` | **infix** | #[nil ((concat (propertize "Configure " 'face 'transient-heading) (propertize (transient-scope) 'face 'magit-branch-local))) (t)] | if: #[nil ((and magit-branch-direct-configure (transient-scope))) (t)] | `partial` | actionable in corresponding Configure dialog |
| `p` | `magit-branch.<branch>.pushRemote` | **infix** | #[nil ((concat (propertize "Configure " 'face 'transient-heading) (propertize (transient-scope) 'face 'magit-branch-local))) (t)] | if: #[nil ((and magit-branch-direct-configure (transient-scope))) (t)] | `partial` | actionable in corresponding Configure dialog |
| `R` | `magit-pull.rebase` | **infix** | Configure repository defaults | if-non-nil: magit-branch-direct-configure | `partial` | actionable in corresponding Configure dialog |
| `P` | `magit-remote.pushDefault` | **infix** | Configure repository defaults | if-non-nil: magit-branch-direct-configure | `partial` | actionable in corresponding Configure dialog |
| `B` | `magit-update-default-branch` | **suffix** | Configure repository defaults | if-non-nil: magit-branch-direct-configure; inapt-if-not: magit-get-some-remote; inapt-if-not: magit-get-some-remote | `missing` | suffix: not implemented |
| `-r` | `transient:magit-branch:--recurse-submodules` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `b` | `magit-checkout` | **suffix** | Checkout |  | `partial` | TUI workflow handler (startup-validated) |
| `l` | `magit-branch-checkout` | **suffix** | Checkout |  | `partial` | TUI workflow handler (startup-validated) |
| `o` | `magit-branch-orphan` | **suffix** | Checkout |  | `partial` | TUI workflow handler (startup-validated) |
| `r` | `magit-checkout-remote-ref` | **suffix** | Checkout |  | `missing` | suffix: not implemented |
| `c` | `magit-branch-and-checkout` | **suffix** |  |  | `partial` | TUI workflow handler (startup-validated) |
| `s` | `magit-branch-spinoff` | **suffix** |  |  | `missing` | backend explicitly reports this branch workflow as unsupported |
| `w` | `magit-worktree-checkout` | **suffix** |  |  | `partial` | TUI workflow handler (startup-validated) |
| `n` | `magit-branch-create` | **suffix** | Create |  | `partial` | TUI workflow handler (startup-validated) |
| `S` | `magit-branch-spinout` | **suffix** | Create |  | `missing` | backend explicitly reports this branch workflow as unsupported |
| `W` | `magit-worktree-branch` | **suffix** | Create |  | `partial` | TUI workflow handler (startup-validated) |
| `C` | `magit-branch-configure` | **suffix** | Do |  | `partial` |  |
| `m` | `magit-branch-rename` | **suffix** | Do |  | `partial` | TUI workflow handler (startup-validated) |
| `x` | `magit-branch-reset` | **suffix** | Do |  | `partial` | TUI workflow handler (startup-validated) |
| `k` | `magit-branch-delete` | **suffix** | Do |  | `partial` | TUI workflow handler (startup-validated) |
| `h` | `magit-branch-shelve` | **suffix** |  |  | `missing` | suffix: not implemented |
| `H` | `magit-branch-unshelve` | **suffix** |  |  | `missing` | suffix: not implemented |

### `b C` — magit-branch-configure (9 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `d` | `magit-branch.<branch>.description` | **infix** | #[nil ((concat (propertize "Configure " 'face 'transient-heading) (propertize (transient-scope) 'face 'magit-branch-local))) (t)] |  | `missing` | infix: argument editing is not implemented |
| `u` | `magit-branch.<branch>.merge/remote` | **infix** | #[nil ((concat (propertize "Configure " 'face 'transient-heading) (propertize (transient-scope) 'face 'magit-branch-local))) (t)] |  | `missing` | infix: argument editing is not implemented |
| `r` | `magit-branch.<branch>.rebase` | **infix** | #[nil ((concat (propertize "Configure " 'face 'transient-heading) (propertize (transient-scope) 'face 'magit-branch-local))) (t)] |  | `missing` | infix: argument editing is not implemented |
| `p` | `magit-branch.<branch>.pushRemote` | **infix** | #[nil ((concat (propertize "Configure " 'face 'transient-heading) (propertize (transient-scope) 'face 'magit-branch-local))) (t)] |  | `missing` | infix: argument editing is not implemented |
| `R` | `magit-pull.rebase` | **infix** | Configure repository defaults |  | `missing` | infix: argument editing is not implemented |
| `P` | `magit-remote.pushDefault` | **infix** | Configure repository defaults |  | `missing` | infix: argument editing is not implemented |
| `B` | `magit-update-default-branch` | **suffix** | Configure repository defaults | inapt-if-not: magit-get-some-remote; inapt-if-not: magit-get-some-remote | `missing` | suffix: not implemented |
| `a m` | `magit-branch.autoSetupMerge` | **infix** | Configure branch creation |  | `missing` | infix: argument editing is not implemented |
| `a r` | `magit-branch.autoSetupRebase` | **infix** | Configure branch creation |  | `missing` | infix: argument editing is not implemented |

### `A` — magit-cherry-pick (17 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `-m` | `magit-cherry-pick:--mainline` | **infix** | Arguments | if-not: magit-sequencer-in-progress-p | `missing` | infix: argument editing is not implemented |
| `=s` | `magit-merge:--strategy` | **infix** | Arguments | if-not: magit-sequencer-in-progress-p | `missing` | infix: argument editing is not implemented |
| `-F` | `transient:magit-cherry-pick:--ff` | **infix** | Arguments | if-not: magit-sequencer-in-progress-p | `missing` | infix: argument editing is not implemented |
| `-x` | `transient:magit-cherry-pick:-x` | **infix** | Arguments | if-not: magit-sequencer-in-progress-p | `missing` | infix: argument editing is not implemented |
| `-e` | `transient:magit-cherry-pick:--edit` | **infix** | Arguments | if-not: magit-sequencer-in-progress-p | `missing` | infix: argument editing is not implemented |
| `-S` | `magit:--gpg-sign` | **infix** | Arguments | if-not: magit-sequencer-in-progress-p | `missing` | infix: argument editing is not implemented |
| `+s` | `magit:--signoff` | **infix** | Arguments | if-not: magit-sequencer-in-progress-p | `missing` | infix: argument editing is not implemented |
| `A` | `magit-cherry-copy` | **suffix** | Apply here | if-not: magit-sequencer-in-progress-p | `missing` | suffix: not implemented |
| `a` | `magit-cherry-apply` | **suffix** | Apply here | if-not: magit-sequencer-in-progress-p | `missing` | suffix: not implemented |
| `h` | `magit-cherry-harvest` | **suffix** | Apply here | if-not: magit-sequencer-in-progress-p | `missing` | suffix: not implemented |
| `m` | `magit-merge-squash` | **suffix** | Apply here | if-not: magit-sequencer-in-progress-p | `missing` | suffix: not implemented |
| `d` | `magit-cherry-donate` | **suffix** | Apply elsewhere | if-not: magit-sequencer-in-progress-p | `missing` | suffix: not implemented |
| `n` | `magit-cherry-spinout` | **suffix** | Apply elsewhere | if-not: magit-sequencer-in-progress-p | `missing` | suffix: not implemented |
| `s` | `magit-cherry-spinoff` | **suffix** | Apply elsewhere | if-not: magit-sequencer-in-progress-p | `missing` | suffix: not implemented |
| `A` | `magit-sequencer-continue` | **suffix** | Actions | if: magit-sequencer-in-progress-p | `missing` | suffix: not implemented |
| `s` | `magit-sequencer-skip` | **suffix** | Actions | if: magit-sequencer-in-progress-p | `missing` | suffix: not implemented |
| `a` | `magit-sequencer-abort` | **suffix** | Actions | if: magit-sequencer-in-progress-p | `missing` | suffix: not implemented |

### `C` — magit-clone (18 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `-B` | `transient:magit-clone:--single-branch` | **infix** | Fetch arguments |  | `missing` | infix: argument editing is not implemented |
| `-n` | `transient:magit-clone:--no-tags` | **infix** | Fetch arguments |  | `missing` | infix: argument editing is not implemented |
| `-S` | `transient:magit-clone:--recurse-submodules` | **infix** | Fetch arguments |  | `missing` | infix: argument editing is not implemented |
| `-l` | `transient:magit-clone:--no-local` | **infix** | Fetch arguments |  | `missing` | infix: argument editing is not implemented |
| `-o` | `transient:magit-clone:--origin=` | **infix** | Setup arguments |  | `missing` | infix: argument editing is not implemented |
| `-b` | `transient:magit-clone:--branch=` | **infix** | Setup arguments |  | `missing` | infix: argument editing is not implemented |
| `-f` | `magit-clone:--filter` | **infix** | Setup arguments |  | `missing` | infix: argument editing is not implemented |
| `-g` | `transient:magit-clone:--separate-git-dir=` | **infix** | Setup arguments |  | `missing` | infix: argument editing is not implemented |
| `-t` | `transient:magit-clone:--template=` | **infix** | Setup arguments |  | `missing` | infix: argument editing is not implemented |
| `-s` | `transient:magit-clone:--shared` | **infix** | Local sharing arguments |  | `missing` | infix: argument editing is not implemented |
| `-h` | `transient:magit-clone:--no-hardlinks` | **infix** | Local sharing arguments |  | `missing` | infix: argument editing is not implemented |
| `C` | `magit-clone-regular` | **suffix** | Clone |  | `missing` | suffix: not implemented |
| `s` | `magit-clone-shallow` | **suffix** | Clone |  | `missing` | suffix: not implemented |
| `d` | `magit-clone-shallow-since` | **suffix** | Clone |  | `missing` | suffix: not implemented |
| `e` | `magit-clone-shallow-exclude` | **suffix** | Clone |  | `missing` | suffix: not implemented |
| `>` | `magit-clone-sparse` | **suffix** | Clone |  | `missing` | suffix: not implemented |
| `b` | `magit-clone-bare` | **suffix** | Clone |  | `missing` | suffix: not implemented |
| `m` | `magit-clone-mirror` | **suffix** | Clone |  | `missing` | suffix: not implemented |

### `c` — magit-commit (26 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `-a` | `transient:magit-commit:--all` | **infix** | Arguments |  | `partial` | actionable TUI option |
| `-e` | `transient:magit-commit:--allow-empty` | **infix** | Arguments |  | `partial` | actionable TUI option |
| `-v` | `transient:magit-commit:--verbose` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-n` | `transient:magit-commit:--no-verify` | **infix** | Arguments |  | `partial` | actionable TUI option |
| `-R` | `transient:magit-commit:--reset-author` | **infix** | Arguments |  | `partial` | actionable TUI option |
| `-A` | `magit:--author` | **infix** | Arguments |  | `partial` | actionable TUI option |
| `-D` | `magit-commit:--date` | **infix** | Arguments |  | `partial` | actionable TUI option |
| `-S` | `magit:--gpg-sign` | **infix** | Arguments |  | `partial` | actionable TUI option |
| `+s` | `magit:--signoff` | **infix** | Arguments |  | `partial` | actionable TUI option |
| `-C` | `magit-commit:--reuse-message` | **infix** | Arguments |  | `partial` | actionable TUI option |
| `-c` | `magit-commit:--reedit-message` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `c` | `magit-commit-create` | **suffix** | Create |  | `partial` | TUI workflow handler (startup-validated) |
| `e` | `magit-commit-extend` | **suffix** | Edit HEAD |  | `partial` | TUI workflow handler (startup-validated) |
| `a` | `magit-commit-amend` | **suffix** | Edit HEAD |  | `partial` | TUI workflow handler (startup-validated) |
| `w` | `magit-commit-reword` | **suffix** | Edit HEAD |  | `partial` | TUI workflow handler (startup-validated) |
| `d` | `magit-commit-reshelve` | **suffix** | Edit HEAD |  | `missing` | suffix: not implemented |
| `f` | `magit-commit-fixup` | **suffix** | Edit |  | `partial` | TUI workflow handler (startup-validated) |
| `s` | `magit-commit-squash` | **suffix** | Edit |  | `partial` | TUI workflow handler (startup-validated) |
| `A` | `magit-commit-alter` | **suffix** | Edit |  | `partial` | TUI workflow handler (startup-validated) |
| `n` | `magit-commit-augment` | **suffix** | Edit |  | `partial` | TUI workflow handler (startup-validated) |
| `W` | `magit-commit-revise` | **suffix** | Edit |  | `partial` | TUI workflow handler (startup-validated) |
| `F` | `magit-commit-instant-fixup` | **suffix** | Edit and rebase |  | `missing` | suffix: not implemented |
| `S` | `magit-commit-instant-squash` | **suffix** | Edit and rebase |  | `missing` | suffix: not implemented |
| `R` | `magit-rebase-reword-commit` | **suffix** | Edit and rebase |  | `missing` | suffix: not implemented |
| `x` | `magit-commit-autofixup` | **suffix** | Spread across commits |  | `partial` |  |
| `X` | `magit-commit-absorb-modules` | **suffix** | Spread across commits |  | `partial` |  |

### `c X` — magit-commit-absorb (3 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `-f` | `transient:magit-commit-absorb:--force` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-v` | `transient:magit-commit-absorb:--verbose` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `x` | `magit-commit-absorb` | **suffix** | Actions |  | `partial` |  |

### `c x` — magit-commit-autofixup (4 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `-c` | `magit-autofixup:--context` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-s` | `magit-autofixup:--strict` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-v` | `transient:magit-commit-autofixup:-vv` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `x` | `magit-commit-autofixup` | **suffix** | Actions |  | `partial` |  |

### `d` — magit-diff (8 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `d` | `magit-diff-dwim` | **suffix** | Actions |  | `missing` | suffix: not implemented |
| `r` | `magit-diff-range` | **suffix** | Actions |  | `missing` | suffix: not implemented |
| `p` | `magit-diff-paths` | **suffix** | Actions |  | `missing` | suffix: not implemented |
| `u` | `magit-diff-unstaged` | **suffix** | Actions |  | `missing` | suffix: not implemented |
| `s` | `magit-diff-staged` | **suffix** | Actions |  | `missing` | suffix: not implemented |
| `w` | `magit-diff-working-tree` | **suffix** | Actions |  | `missing` | suffix: not implemented |
| `c` | `magit-show-commit` | **suffix** | Actions |  | `missing` | suffix: not implemented |
| `t` | `magit-stash-show` | **suffix** | Actions |  | `missing` | suffix: not implemented |

### `D` — magit-diff-refresh (9 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `g` | `magit-diff-refresh` | **suffix** | Refresh |  | `partial` |  |
| `s` | `transient-set-and-exit` | **suffix** | Refresh |  | `missing` | suffix: not implemented |
| `w` | `transient-save-and-exit` | **suffix** | Refresh |  | `missing` | suffix: not implemented |
| `t` | `magit-diff-toggle-refine-hunk` | **suffix** | Toggle |  | `missing` | suffix: not implemented |
| `T` | `magit-diff-toggle-fontify-hunk` | **suffix** | Toggle |  | `missing` | suffix: not implemented |
| `F` | `magit-diff-toggle-file-filter` | **suffix** | Toggle |  | `missing` | suffix: not implemented |
| `b` | `magit-toggle-buffer-lock` | **suffix** | Toggle | if-mode: (magit-diff-mode magit-revision-mode magit-stash-mode) | `missing` | suffix: not implemented |
| `r` | `magit-diff-switch-range-type` | **suffix** | Do | if-mode: magit-diff-mode | `missing` | suffix: not implemented |
| `f` | `magit-diff-flip-revs` | **suffix** | Do | if-mode: magit-diff-mode | `missing` | suffix: not implemented |

### `h` — magit-dispatch (51 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `A` | `magit-cherry-pick` | **suffix** | Transient and dwim commands |  | `partial` |  |
| `b` | `magit-branch` | **suffix** | Transient and dwim commands |  | `partial` |  |
| `B` | `magit-bisect` | **suffix** | Transient and dwim commands |  | `partial` |  |
| `c` | `magit-commit` | **suffix** | Transient and dwim commands |  | `partial` |  |
| `C` | `magit-clone` | **suffix** | Transient and dwim commands |  | `partial` |  |
| `d` | `magit-diff` | **suffix** | Transient and dwim commands |  | `partial` |  |
| `D` | `magit-diff-refresh` | **suffix** | Transient and dwim commands |  | `partial` |  |
| `e` | `magit-ediff-dwim` | **suffix** | Transient and dwim commands |  | `missing` | suffix: not implemented |
| `E` | `magit-ediff` | **suffix** | Transient and dwim commands |  | `partial` |  |
| `f` | `magit-fetch` | **suffix** | Transient and dwim commands |  | `partial` |  |
| `F` | `magit-pull` | **suffix** | Transient and dwim commands |  | `partial` |  |
| `h` | `magit-info` | **suffix** | Transient and dwim commands |  | `missing` | suffix: not implemented |
| `H` | `magit-describe-section` | **suffix** | Transient and dwim commands | if-derived: magit-mode | `missing` | suffix: not implemented |
| `i` | `magit-gitignore` | **suffix** | Transient and dwim commands |  | `partial` |  |
| `I` | `magit-init` | **suffix** | Transient and dwim commands |  | `missing` | suffix: not implemented |
| `j` | `magit-status-jump` | **suffix** | Transient and dwim commands | if-mode: magit-status-mode | `partial` |  |
| `j` | `magit-status-quick` | **suffix** | Transient and dwim commands | if-not-mode: magit-status-mode | `missing` | suffix: not implemented |
| `J` | `magit-display-repository-buffer` | **suffix** | Transient and dwim commands |  | `missing` | suffix: not implemented |
| `l` | `magit-log` | **suffix** | Transient and dwim commands |  | `partial` |  |
| `L` | `magit-log-refresh` | **suffix** | Transient and dwim commands |  | `partial` |  |
| `m` | `magit-merge` | **suffix** | Transient and dwim commands |  | `partial` |  |
| `M` | `magit-remote` | **suffix** | Transient and dwim commands |  | `partial` |  |
| `o` | `magit-submodule` | **suffix** | Transient and dwim commands |  | `partial` |  |
| `O` | `magit-subtree` | **suffix** | Transient and dwim commands |  | `partial` |  |
| `P` | `magit-push` | **suffix** | Transient and dwim commands |  | `partial` |  |
| `Q` | `magit-git-command` | **suffix** | Transient and dwim commands |  | `missing` | suffix: not implemented |
| `r` | `magit-rebase` | **suffix** | Transient and dwim commands |  | `partial` |  |
| `t` | `magit-tag` | **suffix** | Transient and dwim commands |  | `partial` |  |
| `T` | `magit-notes` | **suffix** | Transient and dwim commands |  | `partial` |  |
| `V` | `magit-revert` | **suffix** | Transient and dwim commands |  | `partial` |  |
| `w` | `magit-am` | **suffix** | Transient and dwim commands |  | `partial` |  |
| `W` | `magit-patch` | **suffix** | Transient and dwim commands |  | `partial` |  |
| `X` | `magit-reset` | **suffix** | Transient and dwim commands |  | `partial` |  |
| `y` | `magit-show-refs` | **suffix** | Transient and dwim commands |  | `partial` |  |
| `Y` | `magit-cherry` | **suffix** | Transient and dwim commands |  | `missing` | suffix: not implemented |
| `z` | `magit-stash` | **suffix** | Transient and dwim commands |  | `partial` |  |
| `Z` | `magit-worktree` | **suffix** | Transient and dwim commands |  | `partial` |  |
| `!` | `magit-run` | **suffix** | Transient and dwim commands |  | `partial` |  |
| `a` | `magit-apply` | **suffix** | Applying changes | if-derived: magit-mode | `missing` | suffix: not implemented |
| `v` | `magit-reverse` | **suffix** | Applying changes | if-derived: magit-mode | `missing` | suffix: not implemented |
| `k` | `magit-discard` | **suffix** | Applying changes | if-derived: magit-mode | `missing` | suffix: not implemented |
| `s` | `magit-stage` | **suffix** | Applying changes | if-derived: magit-mode | `missing` | suffix: not implemented |
| `u` | `magit-unstage` | **suffix** | Applying changes | if-derived: magit-mode | `missing` | suffix: not implemented |
| `S` | `magit-stage-modified` | **suffix** | Applying changes | if-derived: magit-mode | `missing` | suffix: not implemented |
| `U` | `magit-unstage-all` | **suffix** | Applying changes | if-derived: magit-mode | `missing` | suffix: not implemented |
| `g` | `magit-refresh` | **suffix** | Essential commands | if-derived: magit-mode | `missing` | suffix: not implemented |
| `q` | `magit-mode-bury-buffer` | **suffix** | Essential commands | if-derived: magit-mode | `missing` | suffix: not implemented |
| `<tab>` | `magit-section-toggle` | **suffix** | Essential commands | if-derived: magit-mode | `missing` | suffix: not implemented |
| `<return>` | `magit-visit-thing` | **suffix** | Essential commands | if-derived: magit-mode | `missing` | suffix: not implemented |
| `C-x m` | `describe-mode` | **suffix** | Essential commands | if-derived: magit-mode | `missing` | suffix: not implemented |
| `C-x i` | `magit-info` | **suffix** | Essential commands | if-derived: magit-mode | `missing` | suffix: not implemented |

### `E` — magit-ediff (11 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `E` | `magit-ediff-dwim` | **suffix** | Ediff |  | `missing` | suffix: not implemented |
| `s` | `magit-ediff-stage` | **suffix** | Ediff | inapt-if-not: magit-anything-modified-p | `missing` | suffix: not implemented |
| `m` | `magit-ediff-resolve-rest` | **suffix** | Ediff | inapt-if-not: magit-anything-unmerged-p | `missing` | suffix: not implemented |
| `M` | `magit-ediff-resolve-all` | **suffix** | Ediff | inapt-if-not: magit-anything-unmerged-p | `missing` | suffix: not implemented |
| `t` | `magit-git-mergetool` | **suffix** | Ediff |  | `partial` |  |
| `u` | `magit-ediff-show-unstaged` | **suffix** | Ediff | inapt-if-not: magit-anything-unstaged-p | `missing` | suffix: not implemented |
| `i` | `magit-ediff-show-staged` | **suffix** | Ediff | inapt-if-not: magit-anything-staged-p | `missing` | suffix: not implemented |
| `w` | `magit-ediff-show-working-tree` | **suffix** | Ediff | inapt-if-not: magit-anything-modified-p | `missing` | suffix: not implemented |
| `c` | `magit-ediff-show-commit` | **suffix** | Ediff |  | `missing` | suffix: not implemented |
| `r` | `magit-ediff-compare` | **suffix** | Ediff |  | `missing` | suffix: not implemented |
| `z` | `magit-ediff-show-stash` | **suffix** | Ediff | inapt-if-not: magit-list-stashes | `missing` | suffix: not implemented |

### `f` — magit-fetch (12 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `-p` | `transient:magit-fetch:--prune` | **infix** | Arguments |  | `partial` | actionable TUI option |
| `-t` | `transient:magit-fetch:--tags` | **infix** | Arguments |  | `partial` | actionable TUI option |
| `-u` | `transient:magit-fetch:--unshallow` | **infix** | Arguments |  | `partial` | actionable TUI option |
| `-F` | `transient:magit-fetch:--force` | **infix** | Arguments |  | `partial` | actionable TUI option |
| `p` | `magit-fetch-from-pushremote` | **suffix** | Fetch from |  | `partial` | TUI workflow handler (startup-validated) |
| `u` | `magit-fetch-from-upstream` | **suffix** | Fetch from | if: #[nil ((magit-get-current-remote t)) (t)] | `partial` | TUI workflow handler (startup-validated) |
| `e` | `magit-fetch-other` | **suffix** | Fetch from |  | `partial` | TUI workflow handler (startup-validated) |
| `a` | `magit-fetch-all` | **suffix** | Fetch from |  | `partial` | TUI workflow handler (startup-validated) |
| `o` | `magit-fetch-branch` | **suffix** | Fetch |  | `partial` | TUI workflow handler (startup-validated) |
| `r` | `magit-fetch-refspec` | **suffix** | Fetch |  | `partial` | TUI workflow handler (startup-validated) |
| `m` | `magit-fetch-modules` | **suffix** | Fetch |  | `partial` |  |
| `C` | `magit-branch-configure` | **suffix** | Configure |  | `partial` |  |

### `f m` — magit-fetch-modules (3 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `-v` | `transient:magit-fetch-modules:--verbose` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-j` | `transient:magit-fetch-modules:--jobs=` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `m` | `magit-fetch-modules` | **suffix** | Action |  | `partial` |  |

### `E t` — magit-git-mergetool (8 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `-t` | `magit-git-mergetool:--tool` | **infix** | Settings |  | `missing` | infix: argument editing is not implemented |
| `=t` | `magit-merge.guitool` | **infix** | Settings |  | `missing` | infix: argument editing is not implemented |
| `=T` | `magit-merge.tool` | **infix** | Settings |  | `missing` | infix: argument editing is not implemented |
| `-r` | `magit-mergetool.hideResolved` | **infix** | Settings |  | `missing` | infix: argument editing is not implemented |
| `-b` | `magit-mergetool.keepBackup` | **infix** | Settings |  | `missing` | infix: argument editing is not implemented |
| `-k` | `magit-mergetool.keepTemporaries` | **infix** | Settings |  | `missing` | infix: argument editing is not implemented |
| `-w` | `magit-mergetool.writeToTemp` | **infix** | Settings |  | `missing` | infix: argument editing is not implemented |
| ` m` | `magit-git-mergetool` | **suffix** | Actions |  | `partial` |  |

### `i` — magit-gitignore (8 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `t` | `magit-gitignore-in-topdir` | **suffix** | Gitignore |  | `missing` | suffix: not implemented |
| `s` | `magit-gitignore-in-subdir` | **suffix** | Gitignore |  | `missing` | suffix: not implemented |
| `p` | `magit-gitignore-in-gitdir` | **suffix** | Gitignore |  | `missing` | suffix: not implemented |
| `g` | `magit-gitignore-on-system` | **suffix** | Gitignore | inapt-if-not: #[nil ((magit-get "core.excludesfile")) (t)] | `missing` | suffix: not implemented |
| `w` | `magit-skip-worktree` | **suffix** | Skip worktree |  | `missing` | suffix: not implemented |
| `W` | `magit-no-skip-worktree` | **suffix** | Skip worktree |  | `missing` | suffix: not implemented |
| `u` | `magit-assume-unchanged` | **suffix** | Assume unchanged |  | `missing` | suffix: not implemented |
| `U` | `magit-no-assume-unchanged` | **suffix** | Assume unchanged |  | `missing` | suffix: not implemented |

### `l` — magit-log (17 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `l` | `magit-log-current` | **suffix** | Log |  | `missing` | suffix: not implemented |
| `o` | `magit-log-other` | **suffix** | Log |  | `missing` | suffix: not implemented |
| `h` | `magit-log-head` | **suffix** | Log |  | `missing` | suffix: not implemented |
| `u` | `magit-log-related` | **suffix** | Log |  | `missing` | suffix: not implemented |
| `L` | `magit-log-branches` | **suffix** |  |  | `missing` | suffix: not implemented |
| `b` | `magit-log-all-branches` | **suffix** |  |  | `missing` | suffix: not implemented |
| `a` | `magit-log-all` | **suffix** |  |  | `missing` | suffix: not implemented |
| `R` | `magit-log-reflog` | **suffix** |  |  | `missing` | suffix: not implemented |
| `B` | `magit-log-matching-branches` | **suffix** |  |  | `missing` | suffix: not implemented |
| `T` | `magit-log-matching-tags` | **suffix** |  |  | `missing` | suffix: not implemented |
| `m` | `magit-log-merged` | **suffix** |  |  | `missing` | suffix: not implemented |
| `r` | `magit-reflog-current` | **suffix** | Reflog |  | `missing` | suffix: not implemented |
| `O` | `magit-reflog-other` | **suffix** | Reflog |  | `missing` | suffix: not implemented |
| `H` | `magit-reflog-head` | **suffix** | Reflog |  | `missing` | suffix: not implemented |
| `i` | `magit-wip-log-index` | **suffix** | Wiplog | if-non-nil: magit-wip-mode | `missing` | suffix: not implemented |
| `w` | `magit-wip-log-worktree` | **suffix** | Wiplog | if-non-nil: magit-wip-mode | `missing` | suffix: not implemented |
| `s` | `magit-shortlog` | **suffix** | Other |  | `partial` |  |

### `L` — magit-log-refresh (13 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `-n` | `magit-log:-n` | **infix** | Arguments | if-not-mode: magit-log-mode | `missing` | infix: argument editing is not implemented |
| `-o` | `magit-log:--*-order` | **infix** | Arguments | if-not-mode: magit-log-mode | `missing` | infix: argument editing is not implemented |
| `-g` | `transient:magit-log-refresh:--graph` | **infix** | Arguments | if-not-mode: magit-log-mode | `missing` | infix: argument editing is not implemented |
| `-c` | `transient:magit-log-refresh:--color` | **infix** | Arguments | if-not-mode: magit-log-mode | `missing` | infix: argument editing is not implemented |
| `-d` | `transient:magit-log-refresh:--decorate` | **infix** | Arguments | if-not-mode: magit-log-mode | `missing` | infix: argument editing is not implemented |
| `g` | `magit-log-refresh` | **suffix** | Refresh |  | `partial` |  |
| `s` | `transient-set-and-exit` | **suffix** | Refresh |  | `missing` | suffix: not implemented |
| `w` | `transient-save-and-exit` | **suffix** | Refresh |  | `missing` | suffix: not implemented |
| `L` | `magit-toggle-margin` | **suffix** | Margin |  | `missing` | suffix: not implemented |
| `l` | `magit-cycle-margin-style` | **suffix** | Margin |  | `missing` | suffix: not implemented |
| `d` | `magit-toggle-margin-details` | **suffix** | Margin |  | `missing` | suffix: not implemented |
| `x` | `magit-toggle-log-margin-style` | **suffix** | Margin |  | `missing` | suffix: not implemented |
| `b` | `magit-toggle-buffer-lock` | **suffix** | Toggle | if-mode: magit-log-mode | `missing` | suffix: not implemented |

### `m` — magit-merge (18 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `-f` | `transient:magit-merge:--ff-only` | **infix** | Arguments | if-not: magit-merge-in-progress-p | `missing` | infix: argument editing is not implemented |
| `-n` | `transient:magit-merge:--no-ff` | **infix** | Arguments | if-not: magit-merge-in-progress-p | `missing` | infix: argument editing is not implemented |
| `-s` | `magit-merge:--strategy` | **infix** | Arguments | if-not: magit-merge-in-progress-p | `missing` | infix: argument editing is not implemented |
| `-X` | `magit-merge:--strategy-option` | **infix** | Arguments | if-not: magit-merge-in-progress-p | `missing` | infix: argument editing is not implemented |
| `-b` | `transient:magit-merge:-Xignore-space-change` | **infix** | Arguments | if-not: magit-merge-in-progress-p | `missing` | infix: argument editing is not implemented |
| `-w` | `transient:magit-merge:-Xignore-all-space` | **infix** | Arguments | if-not: magit-merge-in-progress-p | `missing` | infix: argument editing is not implemented |
| `-A` | `magit-diff:--diff-algorithm` | **infix** | Arguments | if-not: magit-merge-in-progress-p | `missing` | infix: argument editing is not implemented |
| `-S` | `magit:--gpg-sign` | **infix** | Arguments | if-not: magit-merge-in-progress-p | `missing` | infix: argument editing is not implemented |
| `+s` | `magit:--signoff` | **infix** | Arguments | if-not: magit-merge-in-progress-p | `missing` | infix: argument editing is not implemented |
| `m` | `magit-merge-plain` | **suffix** | Actions | if-not: magit-merge-in-progress-p | `missing` | suffix: not implemented |
| `e` | `magit-merge-editmsg` | **suffix** | Actions | if-not: magit-merge-in-progress-p | `missing` | suffix: not implemented |
| `n` | `magit-merge-nocommit` | **suffix** | Actions | if-not: magit-merge-in-progress-p | `missing` | suffix: not implemented |
| `a` | `magit-merge-absorb` | **suffix** | Actions | if-not: magit-merge-in-progress-p | `missing` | suffix: not implemented |
| `p` | `magit-merge-preview` | **suffix** | Actions | if-not: magit-merge-in-progress-p | `missing` | suffix: not implemented |
| `s` | `magit-merge-squash` | **suffix** | Actions | if-not: magit-merge-in-progress-p | `missing` | suffix: not implemented |
| `d` | `magit-merge-dissolve` | **suffix** | Actions | if-not: magit-merge-in-progress-p | `missing` | suffix: not implemented |
| `m` | `magit-commit-create` | **suffix** | Actions | if: magit-merge-in-progress-p | `missing` | suffix: not implemented |
| `a` | `magit-merge-abort` | **suffix** | Actions | if: magit-merge-in-progress-p | `missing` | suffix: not implemented |

### `T` — magit-notes (13 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `c` | `magit-core.notesRef` | **infix** | Configure local settings |  | `missing` | infix: argument editing is not implemented |
| `d` | `magit-notes.displayRef` | **infix** | Configure local settings |  | `missing` | infix: argument editing is not implemented |
| `C` | `magit-global-core.notesRef` | **infix** | Configure global settings |  | `missing` | infix: argument editing is not implemented |
| `D` | `magit-global-notes.displayRef` | **infix** | Configure global settings |  | `missing` | infix: argument editing is not implemented |
| `-n` | `transient:magit-notes:--dry-run` | **infix** | Arguments for prune | if-not: magit-notes-merging-p | `missing` | infix: argument editing is not implemented |
| `-r` | `magit-notes:--ref` | **infix** | Arguments for edit and remove | if-not: magit-notes-merging-p | `missing` | infix: argument editing is not implemented |
| `-s` | `magit-notes:--strategy` | **infix** | Arguments for merge | if-not: magit-notes-merging-p | `missing` | infix: argument editing is not implemented |
| `T` | `magit-notes-edit` | **suffix** | Actions | if-not: magit-notes-merging-p | `missing` | suffix: not implemented |
| `r` | `magit-notes-remove` | **suffix** | Actions | if-not: magit-notes-merging-p | `missing` | suffix: not implemented |
| `m` | `magit-notes-merge` | **suffix** | Actions | if-not: magit-notes-merging-p | `missing` | suffix: not implemented |
| `p` | `magit-notes-prune` | **suffix** | Actions | if-not: magit-notes-merging-p | `missing` | suffix: not implemented |
| `c` | `magit-notes-merge-commit` | **suffix** | Actions | if: magit-notes-merging-p | `missing` | suffix: not implemented |
| `a` | `magit-notes-merge-abort` | **suffix** | Actions | if: magit-notes-merging-p | `missing` | suffix: not implemented |

### `W` — magit-patch (5 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `c` | `magit-patch-create` | **suffix** | Actions |  | `partial` |  |
| `w` | `magit-am` | **suffix** | Actions |  | `partial` |  |
| `a` | `magit-patch-apply` | **suffix** | Actions |  | `partial` |  |
| `s` | `magit-patch-save` | **suffix** | Actions |  | `missing` | suffix: not implemented |
| `r` | `magit-request-pull` | **suffix** | Actions |  | `missing` | suffix: not implemented |

### `w a` — magit-patch-apply (4 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `-i` | `transient:magit-patch-apply:--index` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-c` | `transient:magit-patch-apply:--cached` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-3` | `transient:magit-patch-apply:--3way` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `a` | `magit-patch-apply` | **suffix** | Actions |  | `partial` |  |

### `W c` — magit-patch-create (23 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `C-m C-r` | `magit-format-patch:--in-reply-to` | **infix** | Mail arguments |  | `missing` | infix: argument editing is not implemented |
| `C-m s  ` | `magit-format-patch:--thread` | **infix** | Mail arguments |  | `missing` | infix: argument editing is not implemented |
| `C-m C-f` | `magit-format-patch:--from` | **infix** | Mail arguments |  | `missing` | infix: argument editing is not implemented |
| `C-m C-t` | `magit-format-patch:--to` | **infix** | Mail arguments |  | `missing` | infix: argument editing is not implemented |
| `C-m C-c` | `magit-format-patch:--cc` | **infix** | Mail arguments |  | `missing` | infix: argument editing is not implemented |
| `C-m b  ` | `magit-format-patch:--base` | **infix** | Patch arguments |  | `missing` | infix: argument editing is not implemented |
| `C-m v  ` | `magit-format-patch:--reroll-count` | **infix** | Patch arguments |  | `missing` | infix: argument editing is not implemented |
| `C-m d i` | `magit-format-patch:--interdiff` | **infix** | Patch arguments |  | `missing` | infix: argument editing is not implemented |
| `C-m d r` | `magit-format-patch:--range-diff` | **infix** | Patch arguments |  | `missing` | infix: argument editing is not implemented |
| `C-m p  ` | `magit-format-patch:--subject-prefix` | **infix** | Patch arguments |  | `missing` | infix: argument editing is not implemented |
| `C-m r  ` | `transient:magit-patch-create:--rfc` | **infix** | Patch arguments |  | `missing` | infix: argument editing is not implemented |
| `C-m l  ` | `transient:magit-patch-create:--cover-letter` | **infix** | Patch arguments |  | `missing` | infix: argument editing is not implemented |
| `C-m D  ` | `magit-format-patch:--cover-from-description` | **infix** | Patch arguments |  | `missing` | infix: argument editing is not implemented |
| `C-m n  ` | `magit-format-patch:--notes` | **infix** | Patch arguments |  | `missing` | infix: argument editing is not implemented |
| `C-m o  ` | `magit-format-patch:--output-directory` | **infix** | Patch arguments |  | `missing` | infix: argument editing is not implemented |
| `-U` | `magit-diff:-U` | **infix** | Diff arguments |  | `missing` | infix: argument editing is not implemented |
| `-M` | `magit-diff:-M` | **infix** | Diff arguments |  | `missing` | infix: argument editing is not implemented |
| `-C` | `magit-diff:-C` | **infix** | Diff arguments |  | `missing` | infix: argument editing is not implemented |
| `-A` | `magit-diff:--diff-algorithm` | **infix** | Diff arguments |  | `missing` | infix: argument editing is not implemented |
| `--` | `magit:--` | **infix** | Diff arguments |  | `missing` | infix: argument editing is not implemented |
| `-b` | `transient:magit-patch-create:--ignore-space-change` | **infix** | Diff arguments |  | `missing` | infix: argument editing is not implemented |
| `-w` | `transient:magit-patch-create:--ignore-all-space` | **infix** | Diff arguments |  | `missing` | infix: argument editing is not implemented |
| `c` | `magit-patch-create` | **suffix** | Actions |  | `partial` |  |

### `F` — magit-pull (15 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `-f` | `transient:magit-pull:--ff-only` | **infix** | #[nil ((if magit-pull-or-fetch "Pull arguments" "Arguments")) (t)] |  | `missing` | infix: argument editing is not implemented |
| `-r` | `magit-pull:--rebase` | **infix** | #[nil ((if magit-pull-or-fetch "Pull arguments" "Arguments")) (t)] |  | `missing` | infix: argument editing is not implemented |
| `-A` | `transient:magit-pull:--autostash` | **infix** | #[nil ((if magit-pull-or-fetch "Pull arguments" "Arguments")) (t)] |  | `missing` | infix: argument editing is not implemented |
| `-F` | `transient:magit-pull:--force` | **infix** | #[nil ((if magit-pull-or-fetch "Pull arguments" "Arguments")) (t)] |  | `missing` | infix: argument editing is not implemented |
| `p` | `magit-pull-from-pushremote` | **suffix** | #[nil ((let (anon2595) (if (setq anon2595 (magit-get-current-branch)) (let ((branch anon2595)) (concat (propertize "Pull into " 'face 'transient-heading) (propertize branch 'face 'magit-branch-local) (propertize " from" 'face 'transient-heading))) (propertize "Pull from" 'face 'transient-heading)))) (t)] | if: magit-get-current-branch | `missing` | suffix: not implemented |
| `u` | `magit-pull-from-upstream` | **suffix** | #[nil ((let (anon2595) (if (setq anon2595 (magit-get-current-branch)) (let ((branch anon2595)) (concat (propertize "Pull into " 'face 'transient-heading) (propertize branch 'face 'magit-branch-local) (propertize " from" 'face 'transient-heading))) (propertize "Pull from" 'face 'transient-heading)))) (t)] | if: magit-get-current-branch | `missing` | suffix: not implemented |
| `e` | `magit-pull-branch` | **suffix** | #[nil ((let (anon2595) (if (setq anon2595 (magit-get-current-branch)) (let ((branch anon2595)) (concat (propertize "Pull into " 'face 'transient-heading) (propertize branch 'face 'magit-branch-local) (propertize " from" 'face 'transient-heading))) (propertize "Pull from" 'face 'transient-heading)))) (t)] |  | `missing` | suffix: not implemented |
| `U` | `magit-pull-into-upstream` | **suffix** | #[nil ((format (propertize "Pull into %s from" 'face 'transient-heading) (magit-get-upstream-branch))) (t)] | if: magit-pull--upstreams | `missing` | suffix: not implemented |
| `f` | `magit-fetch-all-no-prune` | **suffix** | Fetch from | if-non-nil: magit-pull-or-fetch | `missing` | suffix: not implemented |
| `F` | `magit-fetch-all-prune` | **suffix** | Fetch from | if-non-nil: magit-pull-or-fetch | `missing` | suffix: not implemented |
| `o` | `magit-fetch-branch` | **suffix** | Fetch | if-non-nil: magit-pull-or-fetch | `missing` | suffix: not implemented |
| `s` | `magit-fetch-refspec` | **suffix** | Fetch | if-non-nil: magit-pull-or-fetch | `missing` | suffix: not implemented |
| `m` | `magit-fetch-modules` | **suffix** | Fetch | if-non-nil: magit-pull-or-fetch | `partial` |  |
| `r` | `magit-branch.<branch>.rebase` | **infix** | Configure | if: magit-get-current-branch | `missing` | infix: argument editing is not implemented |
| `C` | `magit-branch-configure` | **suffix** | Configure |  | `partial` |  |

### `P` — magit-push (18 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `-f` | `transient:magit-push:--force-with-lease` | **infix** | Arguments |  | `partial` | actionable TUI option |
| `-F` | `transient:magit-push:--force` | **infix** | Arguments |  | `partial` | actionable TUI option |
| `-h` | `transient:magit-push:--no-verify` | **infix** | Arguments |  | `partial` | actionable TUI option |
| `-n` | `transient:magit-push:--dry-run` | **infix** | Arguments |  | `partial` | actionable TUI option |
| `-u` | `transient:magit-push:--set-upstream` | **infix** | Arguments |  | `partial` | actionable TUI option |
| `-T` | `transient:magit-push:--tags` | **infix** | Arguments |  | `partial` | actionable TUI option |
| `-t` | `transient:magit-push:--follow-tags` | **infix** | Arguments |  | `partial` | actionable TUI option |
| `-o` | `magit-push:--push-option` | **infix** | Arguments |  | `partial` | actionable TUI option |
| `p` | `magit-push-current-to-pushremote` | **suffix** | #[nil ((format (propertize "Push %s to" 'face 'transient-heading) (propertize (magit-get-current-branch) 'face 'magit-branch-local))) (t)] | if: magit-get-current-branch; if: magit-get-current-branch | `partial` | TUI workflow handler (startup-validated) |
| `u` | `magit-push-current-to-upstream` | **suffix** | #[nil ((format (propertize "Push %s to" 'face 'transient-heading) (propertize (magit-get-current-branch) 'face 'magit-branch-local))) (t)] | if: magit-get-current-branch; if: magit-get-current-branch | `partial` | TUI workflow handler (startup-validated) |
| `e` | `magit-push-current` | **suffix** | #[nil ((format (propertize "Push %s to" 'face 'transient-heading) (propertize (magit-get-current-branch) 'face 'magit-branch-local))) (t)] | if: magit-get-current-branch | `partial` | TUI workflow handler (startup-validated) |
| `o` | `magit-push-other` | **suffix** | Push |  | `partial` | TUI workflow handler (startup-validated) |
| `r` | `magit-push-refspecs` | **suffix** | Push |  | `partial` | TUI workflow handler (startup-validated) |
| `m` | `magit-push-matching` | **suffix** | Push |  | `partial` | TUI workflow handler (startup-validated) |
| `T` | `magit-push-tag` | **suffix** | Push |  | `partial` | TUI workflow handler (startup-validated) |
| `t` | `magit-push-tags` | **suffix** | Push |  | `partial` | TUI workflow handler (startup-validated) |
| `n` | `magit-push-notes-ref` | **suffix** | Push |  | `partial` | TUI workflow handler (startup-validated) |
| `C` | `magit-branch-configure` | **suffix** | Configure |  | `partial` |  |

### `r` — magit-rebase (31 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `-k` | `transient:magit-rebase:--keep-empty` | **infix** | Arguments | if-not: magit-rebase-in-progress-p | `missing` | infix: argument editing is not implemented |
| `-p` | `transient:magit-rebase:--preserve-merges` | **infix** | Arguments | if-not: magit-rebase-in-progress-p; if: #[nil ((magit-git-version< "2.33.0")) (t)] | `missing` | infix: argument editing is not implemented |
| `-r` | `transient:magit-rebase:--rebase-merges=` | **infix** | Arguments | if-not: magit-rebase-in-progress-p | `missing` | infix: argument editing is not implemented |
| `-u` | `transient:magit-rebase:--update-refs` | **infix** | Arguments | if-not: magit-rebase-in-progress-p; if: #[nil ((magit-git-version>= "2.38.0")) (t)] | `missing` | infix: argument editing is not implemented |
| `-s` | `magit-merge:--strategy` | **infix** | Arguments | if-not: magit-rebase-in-progress-p | `missing` | infix: argument editing is not implemented |
| `-X` | `magit-merge:--strategy-option` | **infix** | Arguments | if-not: magit-rebase-in-progress-p | `missing` | infix: argument editing is not implemented |
| `=X` | `magit-diff:--diff-algorithm` | **infix** | Arguments | if-not: magit-rebase-in-progress-p | `missing` | infix: argument editing is not implemented |
| `-f` | `transient:magit-rebase:--force-rebase` | **infix** | Arguments | if-not: magit-rebase-in-progress-p | `missing` | infix: argument editing is not implemented |
| `-d` | `transient:magit-rebase:--committer-date-is-author-date` | **infix** | Arguments | if-not: magit-rebase-in-progress-p | `missing` | infix: argument editing is not implemented |
| `-t` | `transient:magit-rebase:--ignore-date` | **infix** | Arguments | if-not: magit-rebase-in-progress-p | `missing` | infix: argument editing is not implemented |
| `-a` | `transient:magit-rebase:--autosquash` | **infix** | Arguments | if-not: magit-rebase-in-progress-p | `missing` | infix: argument editing is not implemented |
| `-A` | `transient:magit-rebase:--autostash` | **infix** | Arguments | if-not: magit-rebase-in-progress-p | `missing` | infix: argument editing is not implemented |
| `-i` | `transient:magit-rebase:--interactive` | **infix** | Arguments | if-not: magit-rebase-in-progress-p | `missing` | infix: argument editing is not implemented |
| `-h` | `transient:magit-rebase:--no-verify` | **infix** | Arguments | if-not: magit-rebase-in-progress-p | `missing` | infix: argument editing is not implemented |
| `-x` | `magit-rebase:--exec` | **infix** | Arguments | if-not: magit-rebase-in-progress-p | `missing` | infix: argument editing is not implemented |
| `-S` | `magit:--gpg-sign` | **infix** | Arguments | if-not: magit-rebase-in-progress-p | `missing` | infix: argument editing is not implemented |
| `+s` | `magit:--signoff` | **infix** | Arguments | if-not: magit-rebase-in-progress-p | `missing` | infix: argument editing is not implemented |
| `p` | `magit-rebase-onto-pushremote` | **suffix** | #[nil ((format (propertize "Rebase %s onto" 'face 'transient-heading) (propertize (or (magit-get-current-branch) "HEAD") 'face 'magit-branch-local))) (t)] | if-not: magit-rebase-in-progress-p; if: magit-get-current-branch | `missing` | suffix: not implemented |
| `u` | `magit-rebase-onto-upstream` | **suffix** | #[nil ((format (propertize "Rebase %s onto" 'face 'transient-heading) (propertize (or (magit-get-current-branch) "HEAD") 'face 'magit-branch-local))) (t)] | if-not: magit-rebase-in-progress-p; if: magit-get-current-branch | `missing` | suffix: not implemented |
| `e` | `magit-rebase-branch` | **suffix** | #[nil ((format (propertize "Rebase %s onto" 'face 'transient-heading) (propertize (or (magit-get-current-branch) "HEAD") 'face 'magit-branch-local))) (t)] | if-not: magit-rebase-in-progress-p | `missing` | suffix: not implemented |
| `i` | `magit-rebase-interactive` | **suffix** | Rebase | if-not: magit-rebase-in-progress-p | `missing` | suffix: not implemented |
| `s` | `magit-rebase-subset` | **suffix** | Rebase | if-not: magit-rebase-in-progress-p | `missing` | suffix: not implemented |
| `m` | `magit-rebase-edit-commit` | **suffix** | Rebase | if-not: magit-rebase-in-progress-p | `missing` | suffix: not implemented |
| `w` | `magit-rebase-reword-commit` | **suffix** | Rebase | if-not: magit-rebase-in-progress-p | `missing` | suffix: not implemented |
| `k` | `magit-rebase-remove-commit` | **suffix** | Rebase | if-not: magit-rebase-in-progress-p | `missing` | suffix: not implemented |
| `f` | `magit-rebase-autosquash` | **suffix** | Rebase | if-not: magit-rebase-in-progress-p | `missing` | suffix: not implemented |
| `t` | `magit-reshelve-since` | **suffix** | Rebase | if-not: magit-rebase-in-progress-p | `missing` | suffix: not implemented |
| `r` | `magit-rebase-continue` | **suffix** | Actions | if: magit-rebase-in-progress-p | `missing` | suffix: not implemented |
| `s` | `magit-rebase-skip` | **suffix** | Actions | if: magit-rebase-in-progress-p | `missing` | suffix: not implemented |
| `e` | `magit-rebase-edit` | **suffix** | Actions | if: magit-rebase-in-progress-p | `missing` | suffix: not implemented |
| `a` | `magit-rebase-abort` | **suffix** | Actions | if: magit-rebase-in-progress-p | `missing` | suffix: not implemented |

### `M` — magit-remote (15 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `u` | `magit-remote.<remote>.url` | **infix** | Variables | if: #[nil ((and magit-remote-direct-configure (transient-scope))) (t)] | `partial` | actionable in corresponding Configure dialog |
| `U` | `magit-remote.<remote>.fetch` | **infix** | Variables | if: #[nil ((and magit-remote-direct-configure (transient-scope))) (t)] | `partial` | actionable in corresponding Configure dialog |
| `s` | `magit-remote.<remote>.pushurl` | **infix** | Variables | if: #[nil ((and magit-remote-direct-configure (transient-scope))) (t)] | `partial` | actionable in corresponding Configure dialog |
| `S` | `magit-remote.<remote>.push` | **infix** | Variables | if: #[nil ((and magit-remote-direct-configure (transient-scope))) (t)] | `partial` | actionable in corresponding Configure dialog |
| `O` | `magit-remote.<remote>.tagopt` | **infix** | Variables | if: #[nil ((and magit-remote-direct-configure (transient-scope))) (t)] | `partial` | actionable in corresponding Configure dialog |
| `h` | `magit-remote.<remote>.followremotehead` | **infix** | Variables | if: #[nil ((and magit-remote-direct-configure (transient-scope))) (t)] | `partial` | actionable in corresponding Configure dialog |
| `-f` | `transient:magit-remote:-f` | **infix** | Arguments for add |  | `partial` | actionable TUI option |
| `a` | `magit-remote-add` | **suffix** | Actions |  | `partial` | TUI workflow handler (startup-validated) |
| `r` | `magit-remote-rename` | **suffix** | Actions |  | `partial` | TUI workflow handler (startup-validated) |
| `k` | `magit-remote-remove` | **suffix** | Actions |  | `partial` | TUI workflow handler (startup-validated) |
| `C` | `magit-remote-configure` | **suffix** | Actions |  | `partial` |  |
| `p` | `magit-remote-prune` | **suffix** | Actions |  | `partial` | TUI workflow handler (startup-validated) |
| `P` | `magit-remote-prune-refspecs` | **suffix** | Actions |  | `missing` | suffix: not implemented |
| `z` | `magit-remote-unshallow` | **suffix** | Actions |  | `partial` | TUI workflow handler (startup-validated) |
| `d u` | `magit-update-default-branch` | **suffix** | Actions | inapt-if-not: magit-get-some-remote | `partial` | TUI workflow handler (startup-validated) |

### `M C` — magit-remote-configure (6 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `u` | `magit-remote.<remote>.url` | **infix** | #[nil ((concat (propertize "Configure " 'face 'transient-heading) (propertize (transient-scope) 'face 'magit-branch-remote))) (t)] |  | `missing` | infix: argument editing is not implemented |
| `U` | `magit-remote.<remote>.fetch` | **infix** | #[nil ((concat (propertize "Configure " 'face 'transient-heading) (propertize (transient-scope) 'face 'magit-branch-remote))) (t)] |  | `missing` | infix: argument editing is not implemented |
| `s` | `magit-remote.<remote>.pushurl` | **infix** | #[nil ((concat (propertize "Configure " 'face 'transient-heading) (propertize (transient-scope) 'face 'magit-branch-remote))) (t)] |  | `missing` | infix: argument editing is not implemented |
| `S` | `magit-remote.<remote>.push` | **infix** | #[nil ((concat (propertize "Configure " 'face 'transient-heading) (propertize (transient-scope) 'face 'magit-branch-remote))) (t)] |  | `missing` | infix: argument editing is not implemented |
| `O` | `magit-remote.<remote>.tagopt` | **infix** | #[nil ((concat (propertize "Configure " 'face 'transient-heading) (propertize (transient-scope) 'face 'magit-branch-remote))) (t)] |  | `missing` | infix: argument editing is not implemented |
| `h` | `magit-remote.<remote>.followremotehead` | **infix** | #[nil ((concat (propertize "Configure " 'face 'transient-heading) (propertize (transient-scope) 'face 'magit-branch-remote))) (t)] |  | `missing` | infix: argument editing is not implemented |

### `X` — magit-reset (8 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `b` | `magit-branch-reset` | **suffix** | Reset |  | `missing` | suffix: not implemented |
| `f` | `magit-file-checkout` | **suffix** | Reset |  | `missing` | suffix: not implemented |
| `m` | `magit-reset-mixed` | **suffix** | Reset this |  | `missing` | suffix: not implemented |
| `s` | `magit-reset-soft` | **suffix** | Reset this |  | `missing` | suffix: not implemented |
| `h` | `magit-reset-hard` | **suffix** | Reset this |  | `missing` | suffix: not implemented |
| `k` | `magit-reset-keep` | **suffix** | Reset this |  | `missing` | suffix: not implemented |
| `i` | `magit-reset-index` | **suffix** | Reset this |  | `missing` | suffix: not implemented |
| `w` | `magit-reset-worktree` | **suffix** | Reset this |  | `missing` | suffix: not implemented |

### `V` — magit-revert (11 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `-m` | `magit-cherry-pick:--mainline` | **infix** | Arguments | if-not: magit-sequencer-in-progress-p | `missing` | infix: argument editing is not implemented |
| `-e` | `transient:magit-revert:--edit` | **infix** | Arguments | if-not: magit-sequencer-in-progress-p | `missing` | infix: argument editing is not implemented |
| `-E` | `transient:magit-revert:--no-edit` | **infix** | Arguments | if-not: magit-sequencer-in-progress-p | `missing` | infix: argument editing is not implemented |
| `=s` | `magit-merge:--strategy` | **infix** | Arguments | if-not: magit-sequencer-in-progress-p | `missing` | infix: argument editing is not implemented |
| `-S` | `magit:--gpg-sign` | **infix** | Arguments | if-not: magit-sequencer-in-progress-p | `missing` | infix: argument editing is not implemented |
| `+s` | `magit:--signoff` | **infix** | Arguments | if-not: magit-sequencer-in-progress-p | `missing` | infix: argument editing is not implemented |
| `V` | `magit-revert-and-commit` | **suffix** | Actions | if-not: magit-sequencer-in-progress-p | `missing` | suffix: not implemented |
| `v` | `magit-revert-no-commit` | **suffix** | Actions | if-not: magit-sequencer-in-progress-p | `missing` | suffix: not implemented |
| `V` | `magit-sequencer-continue` | **suffix** | Actions | if: magit-sequencer-in-progress-p | `missing` | suffix: not implemented |
| `s` | `magit-sequencer-skip` | **suffix** | Actions | if: magit-sequencer-in-progress-p | `missing` | suffix: not implemented |
| `a` | `magit-sequencer-abort` | **suffix** | Actions | if: magit-sequencer-in-progress-p | `missing` | suffix: not implemented |

### `!` — magit-run (9 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `!` | `magit-git-command-topdir` | **suffix** | Run git subcommand |  | `missing` | suffix: not implemented |
| `p` | `magit-git-command` | **suffix** | Run git subcommand |  | `missing` | suffix: not implemented |
| `s` | `magit-shell-command-topdir` | **suffix** | Run shell command |  | `missing` | suffix: not implemented |
| `S` | `magit-shell-command` | **suffix** | Run shell command |  | `missing` | suffix: not implemented |
| `k` | `magit-run-gitk` | **suffix** | Launch |  | `missing` | suffix: not implemented |
| `a` | `magit-run-gitk-all` | **suffix** | Launch |  | `missing` | suffix: not implemented |
| `b` | `magit-run-gitk-branches` | **suffix** | Launch |  | `missing` | suffix: not implemented |
| `g` | `magit-run-git-gui` | **suffix** | Launch |  | `missing` | suffix: not implemented |
| `m` | `magit-git-mergetool` | **suffix** | Launch |  | `partial` |  |

### `l s` — magit-shortlog (8 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `-n` | `transient:magit-shortlog:--numbered` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-s` | `transient:magit-shortlog:--summary` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-e` | `transient:magit-shortlog:--email` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-g` | `transient:magit-shortlog:--group=` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-f` | `transient:magit-shortlog:--format=` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-w` | `transient:magit-shortlog:-w` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `s` | `magit-shortlog-since` | **suffix** | Shortlog |  | `missing` | suffix: not implemented |
| `r` | `magit-shortlog-range` | **suffix** | Shortlog |  | `missing` | suffix: not implemented |

### `y` — magit-show-refs (10 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `-c` | `magit-for-each-ref:--contains` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-M` | `transient:magit-show-refs:--merged=` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-m` | `transient:magit-show-refs:--merged` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-N` | `transient:magit-show-refs:--no-merged=` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-n` | `transient:magit-show-refs:--no-merged` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-s` | `magit-for-each-ref:--sort` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `y` | `magit-show-refs-head` | **suffix** | Actions |  | `missing` | suffix: not implemented |
| `c` | `magit-show-refs-current` | **suffix** | Actions |  | `missing` | suffix: not implemented |
| `o` | `magit-show-refs-other` | **suffix** | Actions |  | `missing` | suffix: not implemented |
| `r` | `magit-refs-set-show-commit-count` | **suffix** | Actions | if-derived: magit-refs-mode | `missing` | suffix: not implemented |

### `>` — magit-sparse-checkout (6 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `-i` | `transient:magit-sparse-checkout:--sparse-index` | **infix** | Arguments for enabling | if-not: magit-sparse-checkout-enabled-p | `missing` | infix: argument editing is not implemented |
| `e` | `magit-sparse-checkout-enable` | **suffix** | Actions | if-not: magit-sparse-checkout-enabled-p | `missing` | suffix: not implemented |
| `d` | `magit-sparse-checkout-disable` | **suffix** | Actions | if: magit-sparse-checkout-enabled-p | `missing` | suffix: not implemented |
| `r` | `magit-sparse-checkout-reapply` | **suffix** | Actions | if: magit-sparse-checkout-enabled-p | `missing` | suffix: not implemented |
| `s` | `magit-sparse-checkout-set` | **suffix** | Actions |  | `missing` | suffix: not implemented |
| `a` | `magit-sparse-checkout-add` | **suffix** | Actions |  | `missing` | suffix: not implemented |

### `z` — magit-stash (19 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `-u` | `transient:magit-stash:--include-untracked` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-a` | `transient:magit-stash:--all` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `z` | `magit-stash-both` | **suffix** | Stash |  | `missing` | suffix: not implemented |
| `i` | `magit-stash-index` | **suffix** | Stash |  | `missing` | suffix: not implemented |
| `w` | `magit-stash-worktree` | **suffix** | Stash |  | `missing` | suffix: not implemented |
| `x` | `magit-stash-keep-index` | **suffix** | Stash |  | `missing` | suffix: not implemented |
| `P` | `magit-stash-push` | **suffix** | Stash |  | `partial` |  |
| `Z` | `magit-snapshot-both` | **suffix** | Snapshot |  | `missing` | suffix: not implemented |
| `I` | `magit-snapshot-index` | **suffix** | Snapshot |  | `missing` | suffix: not implemented |
| `W` | `magit-snapshot-worktree` | **suffix** | Snapshot |  | `missing` | suffix: not implemented |
| `r` | `magit-wip-commit` | **suffix** | Snapshot |  | `missing` | suffix: not implemented |
| `a` | `magit-stash-apply` | **suffix** | Use |  | `missing` | suffix: not implemented |
| `p` | `magit-stash-pop` | **suffix** | Use |  | `missing` | suffix: not implemented |
| `k` | `magit-stash-drop` | **suffix** | Use |  | `missing` | suffix: not implemented |
| `l` | `magit-stash-list` | **suffix** | Inspect |  | `missing` | suffix: not implemented |
| `v` | `magit-stash-show` | **suffix** | Inspect |  | `missing` | suffix: not implemented |
| `b` | `magit-stash-branch` | **suffix** | Transform |  | `missing` | suffix: not implemented |
| `B` | `magit-stash-branch-here` | **suffix** | Transform |  | `missing` | suffix: not implemented |
| `f` | `magit-stash-format-patch` | **suffix** | Transform |  | `missing` | suffix: not implemented |

### `z P` — magit-stash-push (6 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `--` | `magit:--` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-u` | `transient:magit-stash-push:--include-untracked` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-a` | `transient:magit-stash-push:--all` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-k` | `transient:magit-stash-push:--keep-index` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-K` | `transient:magit-stash-push:--no-keep-index` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `P` | `magit-stash-push` | **suffix** | Actions |  | `partial` |  |

### `j` — magit-status-jump (13 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `z ` | `magit-jump-to-stashes` | **suffix** | Jump to | if: #[nil ((memq 'magit-insert-stashes (symbol-value (intern (format "%s-sections-hook" (substring (symbol-name major-mode) 0 -5)))))) (t)]; inapt-if-not: #[nil ((magit-get-section (cons (cons 'stashes "refs/stash") (magit-section-ident magit-root-section)))) (t)] | `missing` | suffix: not implemented |
| `t ` | `magit-jump-to-tracked` | **suffix** | Jump to | if: #[nil ((memq 'magit-insert-tracked-files (symbol-value (intern (format "%s-sections-hook" (substring (symbol-name major-mode) 0 -5)))))) (t)]; inapt-if-not: #[nil ((magit-get-section (cons (cons 'tracked nil) (magit-section-ident magit-root-section)))) (t)] | `missing` | suffix: not implemented |
| `n ` | `magit-jump-to-untracked` | **suffix** | Jump to | if: #[nil ((memq 'magit-insert-untracked-files (symbol-value (intern (format "%s-sections-hook" (substring (symbol-name major-mode) 0 -5)))))) (t)]; inapt-if-not: #[nil ((magit-get-section (cons (cons 'untracked nil) (magit-section-ident magit-root-section)))) (t)] | `missing` | suffix: not implemented |
| `i ` | `magit-jump-to-ignored` | **suffix** | Jump to | if: #[nil ((memq 'magit-insert-ignored-files (symbol-value (intern (format "%s-sections-hook" (substring (symbol-name major-mode) 0 -5)))))) (t)]; inapt-if-not: #[nil ((magit-get-section (cons (cons 'ignored nil) (magit-section-ident magit-root-section)))) (t)] | `missing` | suffix: not implemented |
| `u ` | `magit-jump-to-unstaged` | **suffix** | Jump to | if: #[nil ((memq 'magit-insert-unstaged-changes (symbol-value (intern (format "%s-sections-hook" (substring (symbol-name major-mode) 0 -5)))))) (gravatar-size t)]; inapt-if-not: #[nil ((magit-get-section (cons (cons 'unstaged nil) (magit-section-ident magit-root-section)))) (gravatar-size t)] | `missing` | suffix: not implemented |
| `s ` | `magit-jump-to-staged` | **suffix** | Jump to | if: #[nil ((memq 'magit-insert-staged-changes (symbol-value (intern (format "%s-sections-hook" (substring (symbol-name major-mode) 0 -5)))))) (gravatar-size t)]; inapt-if-not: #[nil ((magit-get-section (cons (cons 'staged nil) (magit-section-ident magit-root-section)))) (gravatar-size t)] | `missing` | suffix: not implemented |
| `fu` | `magit-jump-to-unpulled-from-upstream` | **suffix** |  | if: #[nil ((memq 'magit-insert-unpulled-from-upstream (symbol-value (intern (format "%s-sections-hook" (substring (symbol-name major-mode) 0 -5)))))) (t)]; inapt-if-not: #[nil ((magit-get-section (cons (cons 'unpulled "..@{upstream}") (magit-section-ident magit-root-section)))) (t)] | `missing` | suffix: not implemented |
| `fp` | `magit-jump-to-unpulled-from-pushremote` | **suffix** |  | if: #[nil ((memq 'magit-insert-unpulled-from-pushremote (symbol-value (intern (format "%s-sections-hook" (substring (symbol-name major-mode) 0 -5)))))) (t)]; inapt-if-not: #[nil ((magit-get-section (cons (cons 'unpulled "..@{push}") (magit-section-ident magit-root-section)))) (t)] | `missing` | suffix: not implemented |
| `pu` | `magit-jump-to-unpushed-to-upstream` | **suffix** |  | if: #[nil ((or (memq 'magit-insert-unpushed-to-upstream-or-recent magit-status-sections-hook) (memq 'magit-insert-unpushed-to-upstream magit-status-sections-hook))) (t)]; inapt-if-not: #[nil ((magit-get-section (cons (cons 'unpushed "@{upstream}..") (magit-section-ident magit-root-section)))) (t)] | `missing` | suffix: not implemented |
| `pp` | `magit-jump-to-unpushed-to-pushremote` | **suffix** |  | if: #[nil ((memq 'magit-insert-unpushed-to-pushremote (symbol-value (intern (format "%s-sections-hook" (substring (symbol-name major-mode) 0 -5)))))) (t)]; inapt-if-not: #[nil ((magit-get-section (cons (cons 'unpushed "@{push}..") (magit-section-ident magit-root-section)))) (t)] | `missing` | suffix: not implemented |
| `a ` | `magit-jump-to-assume-unchanged` | **suffix** |  | if: #[nil ((memq 'magit-insert-assume-unchanged-files (symbol-value (intern (format "%s-sections-hook" (substring (symbol-name major-mode) 0 -5)))))) (t)]; inapt-if-not: #[nil ((magit-get-section (cons (cons 'assume-unchanged nil) (magit-section-ident magit-root-section)))) (t)] | `missing` | suffix: not implemented |
| `w ` | `magit-jump-to-skip-worktree` | **suffix** |  | if: #[nil ((memq 'magit-insert-skip-worktree-files (symbol-value (intern (format "%s-sections-hook" (substring (symbol-name major-mode) 0 -5)))))) (t)]; inapt-if-not: #[nil ((magit-get-section (cons (cons 'skip-worktree nil) (magit-section-ident magit-root-section)))) (t)] | `missing` | suffix: not implemented |
| `j` | `imenu` | **suffix** | Jump using |  | `missing` | suffix: not implemented |

### `o` — magit-submodule (16 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `-f` | `transient:magit-submodule:--force` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-r` | `transient:magit-submodule:--recursive` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-N` | `transient:magit-submodule:--no-fetch` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-C` | `transient:magit-submodule:--checkout` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-R` | `transient:magit-submodule:--rebase` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-M` | `transient:magit-submodule:--merge` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-U` | `transient:magit-submodule:--remote` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `a` | `magit-submodule-add` | **suffix** | One module actions |  | `missing` | suffix: not implemented |
| `r` | `magit-submodule-register` | **suffix** | One module actions |  | `missing` | suffix: not implemented |
| `p` | `magit-submodule-populate` | **suffix** | One module actions |  | `missing` | suffix: not implemented |
| `u` | `magit-submodule-update` | **suffix** | One module actions |  | `missing` | suffix: not implemented |
| `s` | `magit-submodule-synchronize` | **suffix** | One module actions |  | `missing` | suffix: not implemented |
| `d` | `magit-submodule-unpopulate` | **suffix** | One module actions |  | `missing` | suffix: not implemented |
| `k` | `magit-submodule-remove` | **suffix** | One module actions |  | `missing` | suffix: not implemented |
| `l` | `magit-list-submodules` | **suffix** | Populated modules actions |  | `missing` | suffix: not implemented |
| `f` | `magit-fetch-modules` | **suffix** | Populated modules actions |  | `partial` |  |

### `O` — magit-subtree (2 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `i` | `magit-subtree-import` | **suffix** | Subtree actions |  | `partial` |  |
| `e` | `magit-subtree-export` | **suffix** | Subtree actions |  | `partial` |  |

### `O e` — magit-subtree-export (8 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `-P` | `magit-subtree:--prefix` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-a` | `magit-subtree:--annotate` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-b` | `magit-subtree:--branch` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-o` | `magit-subtree:--onto` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-i` | `transient:magit-subtree-export:--ignore-joins` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-j` | `transient:magit-subtree-export:--rejoin` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `p` | `magit-subtree-push` | **suffix** | Subtree export actions |  | `missing` | suffix: not implemented |
| `s` | `magit-subtree-split` | **suffix** | Subtree export actions |  | `missing` | suffix: not implemented |

### `O i` — magit-subtree-import (7 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `-P` | `magit-subtree:--prefix` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-m` | `magit-subtree:--message` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-s` | `transient:magit-subtree-import:--squash` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `a` | `magit-subtree-add` | **suffix** | Subtree import actions |  | `missing` | suffix: not implemented |
| `c` | `magit-subtree-add-commit` | **suffix** | Subtree import actions |  | `missing` | suffix: not implemented |
| `m` | `magit-subtree-merge` | **suffix** | Subtree import actions |  | `missing` | suffix: not implemented |
| `f` | `magit-subtree-pull` | **suffix** | Subtree import actions |  | `missing` | suffix: not implemented |

### `t` — magit-tag (9 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `-f` | `transient:magit-tag:--force` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-e` | `transient:magit-tag:--edit` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-a` | `transient:magit-tag:--annotate` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-s` | `transient:magit-tag:--sign` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `-u` | `magit-tag:--local-user` | **infix** | Arguments |  | `missing` | infix: argument editing is not implemented |
| `t` | `magit-tag-create` | **suffix** | Create |  | `missing` | suffix: not implemented |
| `r` | `magit-tag-release` | **suffix** | Create |  | `missing` | suffix: not implemented |
| `k` | `magit-tag-delete` | **suffix** | Do |  | `missing` | suffix: not implemented |
| `p` | `magit-tag-prune` | **suffix** | Do |  | `missing` | suffix: not implemented |

### `Z` — magit-worktree (5 occurrences)

| Key | Command | Kind | Group | Conditions | Classification | Current status |
|---|---|---|---|---|---|---|
| `b` | `magit-worktree-checkout` | **suffix** | Create new |  | `missing` | suffix: not implemented |
| `c` | `magit-worktree-branch` | **suffix** | Create new |  | `missing` | suffix: not implemented |
| `m` | `magit-worktree-move` | **suffix** | Commands |  | `missing` | suffix: not implemented |
| `k` | `magit-worktree-delete` | **suffix** | Commands |  | `missing` | suffix: not implemented |
| `g` | `magit-worktree-status` | **suffix** | Commands |  | `missing` | suffix: not implemented |
