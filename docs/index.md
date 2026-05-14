# atop-flame

> Flame-style "who eats the most resources" charts from `atop -P` output.

`atop-flame` is a small Go CLI that consumes the parseable output of
[atop](https://www.atoptool.nl/) (`atop -P …`) from stdin and renders one
horizontal bar section per metric category. The longest bar in each section is
the heaviest offender — so a single glance tells you which process is burning
CPU, which one is hoarding RSS, which disk is saturated, which interface is
flooded.

![CPU flame chart](assets/cli-cpu.png)

## Two output modes

| Mode | How to enable | What you get |
|---|---|---|
| **CLI** (default) | no flags | colored 24-bit ANSI bars to stdout |
| **HTML** | `--html-output` | self-contained `atop-flame.html` (go-echarts) in the current directory |
| **HTML + browser** | `--html-output --browser firefox` | as above, then opens the file |

## Design principles

- **Stream-friendly.** Reads stdin until EOF; pipes cleanly from `atop -P` or from a recorded dump.
- **Skip absent metrics.** Sections only appear when the corresponding atop label is present in the input — your CI snapshot without PSI/GPU won't pollute the report with empty boxes.
- **Tolerant parser.** Malformed lines, unknown labels, version drift between atop releases — silently skipped, never fatal.
- **One artifact, zero JS frameworks.** HTML mode produces a single file you can email or attach to a Jira ticket.

## Quick start

```bash
go install github.com/rachlenko/atop-flame@latest

# live capture
atop -P ALL | atop-flame

# recorded log, HTML report
cat /var/log/atop/dump.raw | atop-flame --html-output
```

See **[Installation](installation.md)** for Docker, prebuilt binaries, and
source builds; **[Usage](usage.md)** for every flag; **[Metrics](metrics.md)** for
the full label table with screenshots; **[Architecture](architecture.md)** for
internals.
