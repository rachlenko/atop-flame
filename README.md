# atop-flame

Reads `atop -P PRC` output from stdin and renders a CPU flame chart.
Non-matching lines are silently ignored.

## Install

The fastest way — fetches and builds a tagged release into `$GOBIN` (or `$HOME/go/bin`):

```bash
go install github.com/rachlenko/atop-flame@latest          # latest tag
go install github.com/rachlenko/atop-flame@v0.0.2          # a specific tag
```

`--version` reports the installed semver tag automatically (read from the binary's embedded module info).

### Build from source

```bash
git clone https://github.com/rachlenko/atop-flame
cd atop-flame
make build            # writes .bin/atop-flame using vendored deps
# or, plain Go:
go build -o atop-flame .
```

### Prebuilt binaries

See the [Releases](https://github.com/rachlenko/atop-flame/releases) page for static binaries for linux/darwin × amd64/arm64.

### Docker

```bash
docker pull ghcr.io/rachlenko/atop-flame:latest
atop -P ALL | docker run --rm -i ghcr.io/rachlenko/atop-flame:latest
```

## Usage

```bash
# terminal (colored ASCII bars)
atop -P PRC | ./atop-flame

# from a recorded log file, 10-minute window
atop -r /var/log/atop/atop_$(date +%Y%m%d) \
     -b $(date -d '10 minutes ago' '+%H:%M') \
     -e $(date '+%H:%M') \
     -P PRC | ./atop-flame

# HTML chart, written to ./atop-flame.html
atop -P PRC | ./atop-flame --html-output

# HTML chart, open in firefox
atop -P PRC | ./atop-flame --html-output --browser firefox

# show top 50 processes
atop -P PRC | ./atop-flame --top 50

# help / version
./atop-flame --help
./atop-flame --version
```

## Flags

| Flag | Description |
|---|---|
| `--help` | show help and exit |
| `--version` | show version and exit |
| `--html-output` | render HTML chart to `./atop-flame.html` |
| `--browser PATH` | browser executable to open the HTML file in (omit to just write the file) |
| `--top N` | show top N processes by CPU ticks (default: 30) |

## Documentation

The long-form docs live under [`docs/`](docs/) and are built with
[Zensical](https://zensical.org), a Rust + Python static site generator from
the Material for MkDocs team:

```bash
python3 -m venv .venv && source .venv/bin/activate
pip install zensical
zensical serve           # http://localhost:8000
zensical build           # writes site/ for deployment
```

Pages:

- [`docs/index.md`](docs/index.md) — overview + hero screenshot
- [`docs/installation.md`](docs/installation.md) — `go install`, Docker, source
- [`docs/usage.md`](docs/usage.md) — every flag with examples
- [`docs/metrics.md`](docs/metrics.md) — full atop-label catalog, one screenshot per section
- [`docs/architecture.md`](docs/architecture.md) — parser → aggregate → renderers

Zensical is pre-1.0; if config keys in `zensical.toml` get renamed upstream,
update them per the [Zensical docs](https://zensical.org/docs/) before re-running.

## Dependencies

- [`github.com/go-echarts/go-echarts/v2`](https://github.com/go-echarts/go-echarts) — HTML chart rendering
- [`github.com/lucasb-eyer/go-colorful`](https://github.com/lucasb-eyer/go-colorful) — 24-bit ANSI colors for terminal output

## Integration with freeze detection

Add this to any process before it might freeze, to leave a CPU snapshot in the log:

```bash
atop -r /var/log/atop/atop_$(date +%Y%m%d) \
     -b $(date -d '10 minutes ago' '+%H:%M') \
     -e $(date '+%H:%M') \
     -P PRC | ./atop-flame >> /var/log/pre-freeze-snapshot.log
```
