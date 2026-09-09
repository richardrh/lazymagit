# Documentation site

This [Hugo](https://gohugo.io/) site uses the
[Hextra](https://imfing.github.io/hextra/) theme. Guides live in
`content/`; the focused Markdown notes under the repository's `docs/` directory
are synchronized into the site during each build. Generated output goes to
`public/` and is not committed.

## Prerequisites

- Go 1.26 or later for the documentation sync check
- Hugo Extended 0.158.0 or later

On macOS with Homebrew:

```sh
brew install hugo go
```

## Preview locally

From the repository root:

```sh
make -C doc serve
```

Open <http://localhost:1313/lazymagit/>.

## Build

```sh
make -C doc build
```

The build verifies that generated `docs/keybindings.md` is current before
copying the repository Markdown notes into the Hugo content tree. Clean local
site output with:

```sh
make -C doc clean
```

The GitHub Actions workflow in `.github/workflows/docs.yml` builds the site on
pull requests and deploys the `master` build to GitHub Pages. Deployment
requires GitHub Pages to use GitHub Actions as its source.

To update the pinned Hextra module intentionally:

```sh
hugo mod tidy --source doc
```

Review the resulting `doc/go.mod` and `doc/go.sum` changes together.
