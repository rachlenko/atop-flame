# Installation

## `go install` (recommended)

Requires Go 1.21+.

```bash
go install github.com/rachlenko/atop-flame@latest          # latest tag
go install github.com/rachlenko/atop-flame@v0.0.2          # a specific tag
```

The binary lands in `$GOBIN` (or `$HOME/go/bin` if unset). `--version` reports
the installed semver tag automatically — `atop-flame` reads it from the
binary's embedded module info, so no extra build flags are needed:

```console
$ atop-flame --version
atop-flame v0.0.2
```

## Docker

Multi-arch image (`linux/amd64`, `linux/arm64`) on GitHub Container Registry:

```bash
docker pull ghcr.io/rachlenko/atop-flame:v0.0.2
atop -P ALL | docker run --rm -i ghcr.io/rachlenko/atop-flame:v0.0.2
```

Tags published per release:

- `:v0.0.2` — immutable, points at the tagged build
- `:0.0.2` — same, without the `v` prefix
- `:latest` — moving pointer to the most recent release

## Prebuilt binaries

Static stripped binaries are attached to every GitHub Release:

- `atop-flame-v0.0.2-linux-amd64`
- `atop-flame-v0.0.2-linux-arm64`
- `atop-flame-v0.0.2-darwin-amd64`
- `atop-flame-v0.0.2-darwin-arm64`

Each binary ships next to a `.sha256` file. Grab one from
[Releases](https://github.com/rachlenko/atop-flame/releases) and
`chmod +x` it.

## Build from source

```bash
git clone https://github.com/rachlenko/atop-flame
cd atop-flame
make build              # → .bin/atop-flame, using vendored deps
make build-all          # cross-compile for linux/darwin × amd64/arm64
```

`make build` injects a version string of the form
`vX.Y.Z-<hash>-<timestamp>` via `-ldflags "-X main.appVersion=…"`, so the
binary reports a useful version even when built from a non-tagged commit:

```console
$ .bin/atop-flame --version
atop-flame v0.0.2-9819a56-20260514T194911
```

For a plain `go build .` (no Makefile), Go's
[`runtime/debug.BuildInfo`](https://pkg.go.dev/runtime/debug) takes over and
fills the version from the local VCS state — including a `+dirty` suffix when
the working tree has uncommitted changes.
