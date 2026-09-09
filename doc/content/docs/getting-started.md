---
title: Getting started
linkTitle: Getting started
weight: 2
---

## Install with Go

With Go 1.25 or newer:

```sh
go install github.com/richardrh/lazymagit/cmd/lazymagit@latest
```

The binary is installed into `$(go env GOPATH)/bin` unless `GOBIN` is set.

## Build from source

From the repository root:

```sh
make check
CGO_ENABLED=0 go build -o lazymagit ./cmd/lazymagit
```

Run it against the current directory or pass a repository path:

```sh
./lazymagit [--init] [--theme NAME] [--layout standard|compact] [repository]
```

`make check` runs formatting, vet, race-enabled tests, the complexity threshold
check, and generated-keybinding drift checks.

## Themes

Choose a bundled theme at startup with `--theme`, or press `F2` in the status
view to open the runtime theme picker. Use `↑`/`↓` or `j`/`k` to move, `Enter`
to apply, and `q`/`Esc` to cancel.

Available canonical names:

- `default`
- `tokyo-night`
- `catppuccin-mocha`
- `nord`
- `dracula`
- `gruvbox-dark`
- `solarized-dark`

Names are case-insensitive; spaces and underscores are treated like hyphens.
`catppuccin` is accepted as an alias for `catppuccin-mocha`.

## First controls

| Action | Keys |
|---|---|
| Move | `j` / `k` |
| Refresh | `gr`, `gR`, or `gz` |
| Toggle section | `Tab` or `za` |
| Stage / unstage | `s` / `u` |
| Commit | `c c` |
| Branch transient | `b` |
| Fetch transient | `f` |
| Push transient | `p` or `P` |
| Commands/help | `?` |
| Quit | `q` or `Q` |

See the [complete keybinding ledger](/docs/keybindings/) and the
[compatibility notes](/docs/compatibility/) for the full behavior and known
gaps.
