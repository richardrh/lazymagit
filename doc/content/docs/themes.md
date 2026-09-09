---
title: Themes
linkTitle: Themes
weight: 3
---

Themes can be selected at startup or changed from the status view.

## Runtime picker

Press `F2` in the status view. Use `↑`/`↓` or `j`/`k` to select a theme,
`Enter` to apply it immediately, and `q`/`Esc` to cancel.
The picker shows both the display name and canonical slug.

## Startup selection

```sh
./lazymagit --theme nord
./lazymagit --theme "Catppuccin Mocha" ./my-repository
./lazymagit --theme gruvbox_dark
```

Theme options must appear before the repository path. Restarting with a
`--theme` value is equivalent to using the runtime picker.

| Theme | Canonical name |
|---|---|
| Default | `default` |
| Tokyo Night | `tokyo-night` |
| Catppuccin Mocha | `catppuccin-mocha` |
| Nord | `nord` |
| Dracula | `dracula` |
| Gruvbox Dark | `gruvbox-dark` |
| Solarized Dark | `solarized-dark` |

Names are case-insensitive, and spaces or underscores are treated like
hyphens. `catppuccin` is an alias for `catppuccin-mocha`.
