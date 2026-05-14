# Architecture

`atop-flame` is intentionally small — five Go files, no goroutines, no
network I/O. It reads stdin, builds an in-memory aggregate, and walks that
aggregate twice (once per renderer).

## File map

```
.
├── main.go          flags, --help / --version, dispatch
├── parser.go        line-level regex + per-label field extractor
├── aggregate.go     ProcAgg, DeviceAgg, NetAgg, GpuAgg, system singletons
├── render_cli.go    24-bit ANSI bar sections + textual system summary
├── render_html.go   go-echarts multi-chart page
├── go.mod / go.sum  module path: github.com/rachlenko/atop-flame
└── vendor/          deps committed for hermetic builds
```

## Pipeline

```
                ┌───────────────────┐
   stdin ──────▶│  parser.go        │
                │  parseInput(r)    │
                │  per-label regex  │
                │  → *Aggregate     │
                └────────┬──────────┘
                         │
                  ┌──────┴────────────────┐
                  ▼                       ▼
            render_cli.go           render_html.go
            renderCLI(agg)          renderHTML(agg, path)
            ANSI to stdout          go-echarts page
```

### 1. Parsing

`parser.go` exposes a single entry point:

```go
func parseInput(r io.Reader) *Aggregate
```

Two regexes split lines into the common prefix
(`LABEL host epoch date time interval …`) plus a per-label tail. Each label
has a dedicated branch that pulls the relevant numeric fields out of the
tail. Unknown labels and short / malformed tails are silently skipped —
robustness over strictness, because atop's column layout has changed
across versions.

`SEP` separator lines bump `Aggregate.Snapshots`, which the renderers use
to label the system-summary block.

### 2. Aggregation

`aggregate.go` defines four maps keyed by name (with parallel `*Order`
slices for stable insertion-order iteration):

| Field | Holds | Keyed by |
|---|---|---|
| `procs` | `*ProcAgg` | process name |
| `disks` / `lvms` / `mdds` | `*DeviceAgg` | device name |
| `nets` / `ifbs` | `*NetAgg` | interface name |
| `gpus` | `*GpuAgg` | GPU id |

System singletons (`CPU`, `CPL`, `MEM`, `SWP`, `PAG`, `PSI`, `NFS`,
`PerCore`) are pointers — `nil` when the corresponding label was never
seen.

Merging rules per metric are documented in [Metrics](metrics.md#aggregation-rules).
The headline ones:

- `PRC` ticks → **sum** (per-thread atop output → one row per process)
- `PRM` RSS / VSize → **max** (threads share memory)
- `PRD` / `PRN` → **sum**
- `PRG` → **max**

### 3. Rendering

Both renderers share the same shape:

```go
for each section:
    list := agg.topProcs(scoreFn, topN)   // or topDevices / topNets
    if len(list) == 0 { continue }        // ← absent-section skip
    drawSection(list)
```

`agg.HasAny()` short-circuits the whole pipeline when stdin contained nothing
recognizable — the binary prints a stderr notice and exits zero.

**CLI** (`render_cli.go`):

- 24-bit ANSI escapes via [`go-colorful`](https://github.com/lucasb-eyer/go-colorful).
- Per-section bar widths normalized to 36 columns; the longest row in the
  section fills the full bar.
- The system summary at the end is plain text, not a bar.

**HTML** (`render_html.go`):

- [`go-echarts v2`](https://github.com/go-echarts/go-echarts) horizontal stacked bars.
- One `*charts.Bar` per section, combined into a `components.NewPage()`.
- Page is rendered to disk; no JS bundler, no asset pipeline — output is a
  single ~100 KB HTML file.

## Why the labels merge the way they do

For per-thread metrics, atop emits one line per thread under the same
process name. `PRC` ticks are independent per thread (each thread eats its
own CPU), so summing reflects the parent process's CPU usage. `PRM`,
however, reports virtual / resident size per thread, and threads in one
process **share** their address space — summing those would multiply a
single process's memory by its thread count. The merge rule per metric is
chosen so that the bar length matches the user's mental model of "how much
of this resource did this program use."

## Version reporting

`main.go` exposes a small `resolveVersion()` helper that returns the most
specific version it can find:

1. The ldflags-injected `appVersion` value (CI / Makefile builds).
2. The module version stamped into the binary by `go install …@vX.Y.Z`,
   read at runtime from
   [`runtime/debug.ReadBuildInfo`](https://pkg.go.dev/runtime/debug#ReadBuildInfo).
3. The literal string `dev` as a last resort.

This keeps `atop-flame --version` truthful in every install path without
needing per-path build glue.

## What's intentionally absent

- **No `--output FILE`.** HTML mode always writes `./atop-flame.html` so
  there's one canonical artifact name to find and clean up.
- **No automatic browser launch.** Side effects on the host are opt-in via
  `--browser <executable>`. Scripts and CI runs are predictable by default.
- **No per-snapshot time series.** Aggregating-then-charting matches the
  CI freeze-debugging use case (one snapshot, "who was eating resources")
  much better than line graphs. A time-series view is on the roadmap.
- **No external state.** No config file, no env vars, no cache. The
  binary's behavior is a function of its flags and stdin.
