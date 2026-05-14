# atop-flame

Reads `atop -P PRC` output from stdin and renders a CPU flame chart.
Non-matching lines are silently ignored.

## Build

```bash
go mod tidy
go build -o atop-flame .
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

# HTML chart, auto-open in default browser
atop -P PRC | ./atop-flame --html-output

# HTML chart, specific output file and browser
atop -P PRC | ./atop-flame \
     --html-output \
     --output /tmp/report.html \
     --browser firefox

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
| `--html-output` | generate HTML chart and open in browser |
| `--output FILE` | output file path for `--html-output` (default: temp file) |
| `--browser PATH` | browser executable (default: `xdg-open` / `open` / `start`) |
| `--top N` | show top N processes by CPU ticks (default: 30) |

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
