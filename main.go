package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// appVersion is overridden at build time via -ldflags "-X main.appVersion=…"
// in CI (release.yml) and via the Makefile's GIT_REV-derived value. The
// literal here is only used for bare `go run`/`go build .` without ldflags.
var appVersion = "0.0.1"

const (
	appName           = "atop-flame"
	defaultHTMLOutput = "atop-flame.html"
)

var helpText = fmt.Sprintf(`%s %s — atop -P flame charts

Reads atop -P output from stdin and renders flame charts for every metric
category present in the input. Unknown or malformed lines are silently
ignored, and chart sections with no data are skipped.

USAGE
  atop ... -P ALL | %s [flags]
  cat saved_atop_dump.raw | %s [flags]

FLAGS
  --help              show this help and exit
  --version           show version and exit
  --html-output       render charts as one HTML page instead of CLI bars
                      (written to ./atop-flame.html in the current directory)
  --browser PATH      browser executable to open the HTML file in
                      (omit to just write the file and exit)
  --top N             max rows per section (default: 30)

SUPPORTED LABELS
  Per-process:  PRC (CPU), PRM (memory), PRD (disk), PRN (net), PRG (GPU)
  System:       CPU, cpu, CPL, MEM, SWP, PAG, PSI, NFS, NFSC, NFSS
  Devices:      DSK, LVM, MDD, NET, IFB, GPU

CHART READING
  ■ sys (amber)   kernel-space ticks
  ■ usr (teal)    user-space ticks
  ■ rss (violet)  resident set size
  ■ ops (blue)    disk reads + writes

EXAMPLES
  # live capture, terminal output
  atop -P ALL | %s

  # from recorded log, write HTML chart to ./atop-flame.html
  atop -r /var/log/atop/atop_20260514 -b 12:00 -e 12:10 -P ALL | %s --html-output

  # render HTML and open it in firefox
  atop -P ALL | %s --html-output --browser firefox
`,
	appName, appVersion,
	appName, appName,
	appName, appName, appName,
)

func main() {
	var (
		fHelp       = flag.Bool("help", false, "show help and exit")
		fVersion    = flag.Bool("version", false, "show version and exit")
		fHTMLOutput = flag.Bool("html-output", false, "render HTML chart to "+defaultHTMLOutput)
		fBrowser    = flag.String("browser", "", "browser executable to open the HTML file in")
		fTop        = flag.Int("top", 30, "max rows per section")
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

	agg := parseInput(os.Stdin)

	if *fHTMLOutput {
		if err := renderHTML(agg, defaultHTMLOutput, *fTop); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "chart saved: %s\n", defaultHTMLOutput)

		if *fBrowser != "" {
			if err := openInBrowser(*fBrowser, defaultHTMLOutput); err != nil {
				fmt.Fprintf(os.Stderr, "\nerror: %v\n", err)
				os.Exit(1)
			}
		}
		return
	}

	renderCLI(agg, *fTop)
}

func openInBrowser(browser, filePath string) error {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		absPath = filePath
	}
	url := "file://" + absPath

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
