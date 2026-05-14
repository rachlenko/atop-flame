# Usage

`atop-flame` consumes stdin and renders one section per metric category found
in the input. It does not run `atop` itself — pipe it from a live capture or
from a recorded dump.

## Flags

| Flag | Default | Description |
|---|---|---|
| `--help` | — | show usage text and exit |
| `--version` | — | show version (from build flags or module info) and exit |
| `--html-output` | off | render to `./atop-flame.html` instead of stdout |
| `--browser PATH` | — | executable to open the HTML file in (omit to just write the file) |
| `--top N` | `30` | maximum rows per section |

## Examples

### Live capture, terminal output

```bash
atop -P ALL 5 12 | atop-flame
```

`-P ALL 5 12` asks atop for one snapshot every 5 s, 12 snapshots in total
(≈ 1 minute), in the parseable label format. Pipe straight into `atop-flame`.

### Recorded log, HTML report

```bash
atop -r /var/log/atop/atop_$(date +%Y%m%d) \
     -b $(date -d '10 minutes ago' '+%H:%M') \
     -e $(date '+%H:%M') \
     -P ALL | atop-flame --html-output
```

The HTML file lands in the current directory:

```console
chart saved: atop-flame.html
```

### Open the HTML chart in a specific browser

```bash
atop -P ALL | atop-flame --html-output --browser firefox
```

Without `--browser`, the file is written but no application is launched —
intentional, so scripts and CI runs are predictable.

### Limit the table size

```bash
atop -P ALL | atop-flame --top 10        # only top 10 per section
```

`--top` applies to every section uniformly (CPU, RSS, disk I/O, network, GPU,
DSK, LVM, MDD, NET, IFB).

### Replay a dump for analysis

```bash
cat ./test/data/output_full.raw | atop-flame
```

Useful in CI: capture an atop snapshot during a flaky test, archive the raw
dump as a CI artifact, then re-render it locally with the same binary version
to investigate.

## Exit codes

| Code | When |
|---|---|
| `0` | success — at least one section rendered, or `--help` / `--version` |
| `1` | unrecoverable error (cannot create output file, browser launch failed) |

A stream that contains nothing recognizable is **not** an error; the binary
prints `no recognizable atop data found in input` to stderr and exits zero.

## What `--version` reports

The version is resolved at runtime from three sources, in priority order:

1. **ldflags injection** — `-X main.appVersion=…` set by the Makefile or the
   release CI workflow. Takes precedence when present.
2. **Embedded module info** — populated by `go install …@v0.0.2`. Used when
   ldflags weren't supplied.
3. **`dev`** — fallback when neither is available (e.g. `go run .` from a
   non-VCS checkout).

This makes `atop-flame --version` honest in every install path: tagged
releases show their tag, Makefile builds show the git-derived revision,
ad-hoc compiles show `dev`.
