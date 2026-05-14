package main

import (
	"bufio"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"

	colorful "github.com/lucasb-eyer/go-colorful"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/components"
	"github.com/go-echarts/go-echarts/v2/opts"
)

const (
	appVersion = "1.0.0"
	appName    = "atop-flame"
)

var helpText = fmt.Sprintf(`%s %s — atop PRC CPU flame chart

Reads atop -P PRC lines from stdin. Non-matching lines are silently ignored.
Renders a CPU flame chart in the terminal (default) or as an HTML page.

USAGE
  atop ... -P PRC | %s [flags]
  cat saved_prc.txt | %s [flags]

FLAGS
  --help              show this help and exit
  --version           show version and exit
  --html-output       generate an HTML chart and open it in a browser
  --output FILE       output file path for --html-output
                      (default: auto-generated temp file)
  --browser PATH      browser executable to open the HTML file
                      (default: xdg-open on Linux, open on macOS, start on Windows)
  --top N             show only top N processes by total CPU ticks (default: 30)

PROCESS STATES
  [R] running (red)    [D] uninterruptible I/O (amber)
  [S] sleeping (teal)  [I] idle kernel thread (gray)

CHART COLUMNS
  ■ sys   kernel-space ticks (amber bars)
  ■ usr   user-space ticks   (teal bars)

EXAMPLES
  # live capture, terminal output
  atop -P PRC | %s

  # from recorded log, HTML output, auto-open in default browser
  atop -r /var/log/atop/atop_20260514 -b 12:00 -e 12:10 -P PRC | %s --html-output

  # specify output file and browser
  atop -P PRC | %s --html-output --output /tmp/report.html --browser firefox
`,
	appName, appVersion,
	appName, appName,
	appName, appName, appName,
)

// ── data model ────────────────────────────────────────────────────────────────

// Process aggregates CPU tick data across all threads of one named process.
type Process struct {
	Name        string
	State       string // most-active state seen across threads
	SysTicks    int    // kernel-space centiseconds
	UsrTicks    int    // user-space centiseconds
	ThreadCount int    // number of PRC lines merged
}

func (p *Process) Total() int { return p.SysTicks + p.UsrTicks }

// stateRank returns an ordering so we keep the most active state
// when merging multiple threads: R > D > S > I.
func stateRank(s string) int {
	switch s {
	case "R":
		return 4
	case "D":
		return 3
	case "S":
		return 2
	case "I":
		return 1
	}
	return 0
}

// ── parsing ───────────────────────────────────────────────────────────────────

// prcRe matches one atop -P PRC output line.
//
// Format (space-separated):
//   PRC  hostname  epoch  date  time  interval  pid  (name)  state  cpupct  sys  usr  ...
//
// We capture: pid(1) name(2) state(3) sys(4) usr(5).
var prcRe = regexp.MustCompile(
	`^PRC\s+\S+\s+\d+\s+\S+\s+\S+\s+\d+\s+(\d+)\s+\(([^)]+)\)\s+([RSDIZ])\s+\d+\s+(\d+)\s+(\d+)`,
)

func parseStdin(topN int) []Process {
	sc := bufio.NewScanner(os.Stdin)
	agg := map[string]*Process{}
	order := []string{} // insertion order for stable de-dup

	for sc.Scan() {
		m := prcRe.FindStringSubmatch(sc.Text())
		if m == nil {
			continue // silently skip non-matching lines
		}
		// m[1]=pid  m[2]=name  m[3]=state  m[4]=sys  m[5]=usr
		name := m[2]
		state := m[3]
		sys, _ := strconv.Atoi(m[4])
		usr, _ := strconv.Atoi(m[5])

		p, exists := agg[name]
		if !exists {
			p = &Process{Name: name, State: state}
			agg[name] = p
			order = append(order, name)
		}
		p.SysTicks += sys
		p.UsrTicks += usr
		p.ThreadCount++
		if stateRank(state) > stateRank(p.State) {
			p.State = state
		}
	}

	if err := sc.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: stdin read error: %v\n", err)
	}

	// sort by total ticks descending
	sort.Slice(order, func(i, j int) bool {
		return agg[order[i]].Total() > agg[order[j]].Total()
	})

	result := make([]Process, 0, topN)
	for _, name := range order {
		if len(result) >= topN {
			break
		}
		if agg[name].Total() == 0 {
			break // everything after this is also zero
		}
		result = append(result, *agg[name])
	}
	return result
}

// ── CLI rendering (go-colorful) ───────────────────────────────────────────────

var (
	cSys   = mustHex("#BA7517") // amber  — sys ticks bar
	cUsr   = mustHex("#1D9E75") // teal   — usr ticks bar
	cName  = mustHex("#5F5E5A") // gray   — process name
	cEmpty = mustHex("#D3D1C7") // light gray — empty bar fill
	cRed   = mustHex("#E24B4A") // state R
	cAmb   = mustHex("#EF9F27") // state D
	cGrn   = mustHex("#1D9E75") // state S
	cGray  = mustHex("#888780") // state I / default
)

func mustHex(h string) colorful.Color {
	c, err := colorful.Hex(h)
	if err != nil {
		panic(err)
	}
	return c
}

// ansi wraps text in 24-bit ANSI foreground color.
func ansi(c colorful.Color, s string) string {
	r, g, b := c.RGB255()
	return fmt.Sprintf("\033[38;2;%d;%d;%dm%s\033[0m", r, g, b, s)
}

func stateTag(state string) string {
	switch state {
	case "R":
		return ansi(cRed, "[R]")
	case "D":
		return ansi(cAmb, "[D]")
	case "S":
		return ansi(cGrn, "[S]")
	default:
		return ansi(cGray, "[I]")
	}
}

func renderCLI(procs []Process) {
	if len(procs) == 0 {
		fmt.Fprintln(os.Stderr, "no processes with CPU activity found in input")
		return
	}

	maxTotal := 0
	maxNameLen := 16
	for _, p := range procs {
		if t := p.Total(); t > maxTotal {
			maxTotal = t
		}
		if n := len(p.Name); n > maxNameLen {
			maxNameLen = n
		}
	}
	if maxNameLen > 30 {
		maxNameLen = 30
	}

	const barWidth = 42

	// ── header ──
	fmt.Println()
	fmt.Printf("  %-*s  %-3s  %-*s  %s\n",
		maxNameLen, "PROCESS",
		"ST",
		barWidth, ansi(cSys, "■ sys")+" "+ansi(cUsr, "■ usr"),
		"TICKS",
	)
	fmt.Println("  " + strings.Repeat("─", maxNameLen+barWidth+18))

	// ── rows ──
	for _, p := range procs {
		total := p.Total()
		sysBars, usrBars := 0, 0
		if maxTotal > 0 {
			sysBars = intRound(float64(p.SysTicks) / float64(maxTotal) * barWidth)
			usrBars = intRound(float64(p.UsrTicks) / float64(maxTotal) * barWidth)
		}
		empty := barWidth - sysBars - usrBars
		if empty < 0 {
			empty = 0
		}

		name := p.Name
		if len(name) > maxNameLen {
			name = name[:maxNameLen-1] + "…"
		}
		namePad := fmt.Sprintf("%-*s", maxNameLen, name)

		bar := ansi(cSys, strings.Repeat("█", sysBars)) +
			ansi(cUsr, strings.Repeat("█", usrBars)) +
			ansi(cEmpty, strings.Repeat("░", empty))

		threads := ""
		if p.ThreadCount > 1 {
			threads = fmt.Sprintf(" ×%d", p.ThreadCount)
		}

		fmt.Printf("  %s  %s  %s  %d%s\n",
			ansi(cName, namePad),
			stateTag(p.State),
			bar,
			total,
			threads,
		)
	}
	fmt.Println()
}

func intRound(f float64) int {
	return int(math.Round(f))
}

// ── HTML rendering (go-echarts) ───────────────────────────────────────────────

func renderHTML(procs []Process, outputPath string) error {
	if len(procs) == 0 {
		return fmt.Errorf("no processes with CPU activity to render")
	}

	// Reverse so highest bar appears at the top of the horizontal chart.
	rev := make([]Process, len(procs))
	for i, p := range procs {
		rev[len(procs)-1-i] = p
	}

	names := make([]string, len(rev))
	sysData := make([]opts.BarData, len(rev))
	usrData := make([]opts.BarData, len(rev))

	for i, p := range rev {
		label := p.Name
		if p.ThreadCount > 1 {
			label += fmt.Sprintf(" ×%d", p.ThreadCount)
		}
		names[i] = label
		sysData[i] = opts.BarData{
			Value:     p.SysTicks,
			ItemStyle: &opts.ItemStyle{Color: "#BA7517"},
		}
		usrData[i] = opts.BarData{
			Value:     p.UsrTicks,
			ItemStyle: &opts.ItemStyle{Color: "#1D9E75"},
		}
	}

	chartH := fmt.Sprintf("%dpx", maxInt(480, len(procs)*32+140))

	bar := charts.NewBar()
	bar.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title: "atop PRC — CPU Flame Chart",
			Subtitle: fmt.Sprintf(
				"top %d processes by CPU ticks  ·  ■ sys (amber)  ■ usr (teal)",
				len(procs),
			),
		}),
		charts.WithInitializationOpts(opts.Initialization{
			Width:  "100%",
			Height: chartH,
		}),
		charts.WithTooltipOpts(opts.Tooltip{
			Show:    opts.Bool(true),
			Trigger: "axis",
		}),
		charts.WithLegendOpts(opts.Legend{
			Show: opts.Bool(true),
			Data: []string{"sys ticks", "usr ticks"},
		}),
		charts.WithGridOpts(opts.Grid{
			Left:   "22%",
			Right:  "6%",
			Top:    "80px",
			Bottom: "30px",
		}),
		charts.WithXAxisOpts(opts.XAxis{
			Name: "CPU ticks (centiseconds)",
			NameLocation: "middle",
			NameGap: 25,
		}),
		charts.WithYAxisOpts(opts.YAxis{
			AxisLabel: &opts.AxisLabel{
				FontFamily: "monospace",
				FontSize:   11,
			},
		}),
	)

	bar.SetXAxis(names).
		AddSeries("sys ticks", sysData,
			charts.WithBarChartOpts(opts.BarChart{Stack: "total"}),
		).
		AddSeries("usr ticks", usrData,
			charts.WithBarChartOpts(opts.BarChart{Stack: "total"}),
		)
	bar.XYReversal()

	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create output file %q: %w", outputPath, err)
	}
	defer f.Close()

	page := components.NewPage()
	page.AddCharts(bar)
	return page.Render(f)
}

// ── browser opener ────────────────────────────────────────────────────────────

func openInBrowser(browser, filePath string) error {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		absPath = filePath
	}
	url := "file://" + absPath

	if browser != "" {
		resolved, lookErr := exec.LookPath(browser)
		if lookErr != nil {
			return fmt.Errorf(
				"browser %q was not found:\n"+
					"  error: %v\n\n"+
					"Troubleshooting:\n"+
					"  • Make sure the browser is installed and the name is correct\n"+
					"  • Provide the full path, e.g.:  --browser /usr/bin/firefox\n"+
					"  • Common browser names: firefox, chromium, google-chrome,\n"+
					"    brave-browser, microsoft-edge, epiphany\n"+
					"  • On Windows, try: --browser \"C:\\Program Files\\Mozilla Firefox\\firefox.exe\"\n"+
					"\n"+
					"The HTML file was saved and can be opened manually:\n"+
					"  %s",
				browser, lookErr, absPath,
			)
		}
		return exec.Command(resolved, url).Start()
	}

	// No browser specified — use OS default opener.
	switch runtime.GOOS {
	case "linux":
		xdg, lookErr := exec.LookPath("xdg-open")
		if lookErr != nil {
			return fmt.Errorf(
				"default browser opener 'xdg-open' was not found on this system.\n\n"+
					"Troubleshooting:\n"+
					"  • Install xdg-utils:\n"+
					"      sudo apt install xdg-utils    (Debian/Ubuntu)\n"+
					"      sudo dnf install xdg-utils    (Fedora/RHEL)\n"+
					"  • Or specify a browser directly, e.g.:  --browser firefox\n"+
					"  • Or open the file manually in any browser:\n"+
					"      %s",
				absPath,
			)
		}
		return exec.Command(xdg, url).Start()

	case "darwin":
		return exec.Command("open", url).Start()

	case "windows":
		return exec.Command("cmd", "/c", "start", "", url).Start()

	default:
		return fmt.Errorf(
			"unsupported OS %q: cannot determine the default browser opener.\n"+
				"Use --browser to specify one, e.g.:  --browser firefox\n"+
				"The HTML file was saved to:\n  %s",
			runtime.GOOS, absPath,
		)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ── main ──────────────────────────────────────────────────────────────────────

func main() {
	var (
		fHelp       = flag.Bool("help", false, "show help and exit")
		fVersion    = flag.Bool("version", false, "show version and exit")
		fHTMLOutput = flag.Bool("html-output", false, "generate HTML chart")
		fOutput     = flag.String("output", "", "output HTML file (default: temp file)")
		fBrowser    = flag.String("browser", "", "browser executable for --html-output")
		fTop        = flag.Int("top", 30, "max processes to display")
	)

	flag.Usage = func() { fmt.Print(helpText) }
	flag.Parse()

	if *fHelp {
		fmt.Print(helpText)
		return
	}
	if *fVersion {
		fmt.Printf("%s %s\n", appName, appVersion)
		return
	}

	procs := parseStdin(*fTop)

	if *fHTMLOutput {
		outPath := *fOutput
		if outPath == "" {
			tmp, err := os.CreateTemp("", "atop-flame-*.html")
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: cannot create temp file: %v\n", err)
				os.Exit(1)
			}
			outPath = tmp.Name()
			tmp.Close()
		}

		if err := renderHTML(procs, outPath); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "chart saved: %s\n", outPath)

		if err := openInBrowser(*fBrowser, outPath); err != nil {
			fmt.Fprintf(os.Stderr, "\nerror: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Default: colored terminal output.
	renderCLI(procs)
}
