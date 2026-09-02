# Sparse checkout

Open the sparse-checkout transient with `>`. The available actions follow the
repository state, so enable is shown while sparse checkout is disabled and
disable/reapply are shown while it is enabled. These conditions are evaluated
dynamically from Git configuration rather than from static manifest labels.

## Modes and operations

- `-i` enables Git's sparse index for the next enable operation. This is useful
  for large repositories, but external tools that do not understand sparse
  indexes may require the full-index default.
- `e` enables sparse checkout and asks for cone directories (recommended) or
  advanced non-cone Git patterns.
- `s` replaces the current selection. `a` adds to it. Enter one path or pattern
  per line according to the enabled mode.
- `r` reapplies the current selection after working-tree changes.
- `d` disables sparse checkout and restores all tracked files.

Every state-changing operation presents the detected current mode and an exact
review before running. Replacing the selection and disabling sparse checkout
use operation-specific confirmations because they can remove or restore many
working-tree files. Inputs are bounded to 4 MiB and 100,000 lines and are passed
to Git as typed arguments rather than interpreted by a shell.
