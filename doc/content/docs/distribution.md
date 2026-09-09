---
title: Distribution
linkTitle: Distribution
weight: 4
---

`lazymagit` is distributed through the Go module proxy, GitHub Releases, and
Linux package artifacts produced by GoReleaser. The GitHub Actions release
workflow runs for every tag matching `v*`.

## Go install

No registry submission is required for `go install`. Once a version tag is
available, install the command directly from the module path:

```sh
go install github.com/richardrh/lazymagit/cmd/lazymagit@latest
go install github.com/richardrh/lazymagit/cmd/lazymagit@v0.1.1
```

The Go toolchain resolves the module through the public Go module proxy. `@latest`
selects the latest semver release tag; use an explicit version for reproducible
installs.

## GitHub Releases

Pushing a `v*` tag starts `.github/workflows/release.yml`. GoReleaser publishes:

- `amd64` and `arm64` binaries for Linux, macOS, and Windows;
- `.tar.gz` archives for Unix platforms and `.zip` archives for Windows;
- `checksums.txt`; and
- Debian, RPM, and Arch Linux packages for Linux.

The release workflow requires the repository's standard `GITHUB_TOKEN` and
publishes to the GitHub Releases page. Release configuration is in
`.goreleaser.yaml`.

## Homebrew

There is not currently a Homebrew formula or tap configured for `lazymagit`.
A Homebrew distribution requires a separate tap repository, such as
`homebrew-lazymagit`, or a GoReleaser `brews` configuration that publishes to
an existing tap. Until that is added, macOS users can use `go install` or a
GitHub release archive.

## Other package managers

There is no Scoop, Chocolatey, Nix, or MacPorts publishing configuration in this
repository. The GitHub release archives remain the portable installation path;
package-manager integrations can be added independently once a maintainer-owned
repository and publishing credentials exist.
