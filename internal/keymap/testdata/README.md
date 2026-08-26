# Magit v4.7 keymap reference

`magit-v4.7.0-status-bindings.json` is the generated reference manifest for
Magit **v4.7.0**, commit
`67f203853e74e926e2c99f60ed508840714f7ced`. The pinned checkout was clean.

It was produced by running `extract-magit-bindings.el` with Emacs against that
pinned checkout, loading Magit's source units before walking the effective
`magit-status-mode` map and recursively reachable transients. The extractor's
checkout root is supplied for the extraction run; generated documentation and
application data do not contain that machine-specific path. Do not regenerate
from an unpinned branch.

Verified manifest totals:

- **98** effective status keys
- **44** reachable transients
- **554** transient entry occurrences (suffixes and infixes)

The JSON is both an audit reference and an embedded registry input. Infix
details intentionally remain here during the architecture-foundation wave.
