package main_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var (
	binPath    string
	samplePath string
)

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "atop-flame-e2e-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "TestMain: mktemp: %v\n", err)
		os.Exit(2)
	}

	binPath = filepath.Join(tmpDir, "atop-flame")
	if runtime.GOOS == "windows" {
		binPath += ".exe"
	}

	build := exec.Command("go", "build", "-o", binPath, ".")
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		os.RemoveAll(tmpDir)
		fmt.Fprintf(os.Stderr, "TestMain: build failed: %v\n", err)
		os.Exit(2)
	}

	samplePath, err = filepath.Abs(filepath.Join("test", "data", "output.raw"))
	if err != nil {
		os.RemoveAll(tmpDir)
		fmt.Fprintf(os.Stderr, "TestMain: abs sample path: %v\n", err)
		os.Exit(2)
	}
	if _, err := os.Stat(samplePath); err != nil {
		os.RemoveAll(tmpDir)
		fmt.Fprintf(os.Stderr, "TestMain: sample data missing at %s: %v\n", samplePath, err)
		os.Exit(2)
	}

	code := m.Run()
	os.RemoveAll(tmpDir)
	os.Exit(code)
}

// runPipingSample executes the built binary in workDir with the given args and
// the PRC-only sample piped into stdin (mirroring `cat test/data/output.raw | ./atop-flame ...`).
func runPipingSample(t *testing.T, workDir string, args ...string) (stdout, stderr string) {
	t.Helper()
	return runPipingFile(t, samplePath, workDir, args...)
}

// runPipingFile is like runPipingSample but takes an arbitrary input file.
func runPipingFile(t *testing.T, inputPath, workDir string, args ...string) (stdout, stderr string) {
	t.Helper()

	f, err := os.Open(inputPath)
	if err != nil {
		t.Fatalf("open %s: %v", inputPath, err)
	}
	defer f.Close()

	cmd := exec.Command(binPath, args...)
	cmd.Stdin = f
	cmd.Dir = workDir
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		t.Fatalf("run %s %v failed: %v\nstdout: %s\nstderr: %s",
			binPath, args, err, outBuf.String(), errBuf.String())
	}
	return outBuf.String(), errBuf.String()
}

// TestPipedSample_CLIGraph mirrors:  cat test/data/output.raw | ./atop-flame
// Default (no flags) must render the colored ASCII bar chart to stdout.
func TestPipedSample_CLIGraph(t *testing.T) {
	stdout, _ := runPipingSample(t, t.TempDir())

	if !strings.Contains(stdout, "PROCESS") {
		t.Errorf("expected 'PROCESS' table header in stdout, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "CPU — top") {
		t.Errorf("expected 'CPU — top …' section header in stdout, got:\n%s", stdout)
	}
	// 24-bit ANSI true-color escape — produced by the colored bar renderer.
	if !strings.Contains(stdout, "\x1b[38;2;") {
		t.Errorf("expected ANSI true-color escape sequences in CLI output, got:\n%s", stdout)
	}

	// at least one known process name from the sample should appear
	found := false
	for _, name := range []string{"systemd", "sshd", "atop", "kthreadd", "qemu-ga"} {
		if strings.Contains(stdout, name) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected at least one known process name in CLI output, got:\n%s", stdout)
	}
}

// TestPipedSample_HTMLOutput mirrors:  cat test/data/output.raw | ./atop-flame --html-output
// Must write the HTML chart to ./atop-flame.html in cwd and emit a "chart saved:" notice on stderr.
// Must NOT open a browser (no --browser flag).
func TestPipedSample_HTMLOutput(t *testing.T) {
	workDir := t.TempDir()
	stdout, stderr := runPipingSample(t, workDir, "--html-output")

	if stdout != "" {
		t.Errorf("--html-output should not write to stdout, got:\n%s", stdout)
	}
	if !strings.Contains(stderr, "chart saved:") {
		t.Errorf("expected 'chart saved:' notice on stderr, got: %q", stderr)
	}

	htmlPath := filepath.Join(workDir, "atop-flame.html")
	info, err := os.Stat(htmlPath)
	if err != nil {
		t.Fatalf("expected %s to be created: %v", htmlPath, err)
	}
	if info.Size() == 0 {
		t.Fatalf("html file %s is empty", htmlPath)
	}

	content, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("read html: %v", err)
	}
	body := string(content)

	for _, needle := range []string{
		"<!DOCTYPE html>",
		"echarts",
		"CPU — top processes by sys+usr ticks",
	} {
		if !strings.Contains(body, needle) {
			t.Errorf("expected %q in HTML output", needle)
		}
	}
}

// TestPipedFullSample_CLI verifies that every metric category present in
// output_full.raw (CPU/PRC/PRM/PRD/DSK/CPL/MEM/SWP/PAG/PSI) produces a
// section in CLI mode and that absent ones (NET/IFB/LVM/MDD/NFS/GPU/PRN/PRG)
// are silently skipped.
func TestPipedFullSample_CLI(t *testing.T) {
	full := filepath.Join(filepath.Dir(samplePath), "output_full.raw")
	if _, err := os.Stat(full); err != nil {
		t.Skipf("output_full.raw not present: %v", err)
	}

	stdout, _ := runPipingFile(t, full, t.TempDir())

	expectPresent := []string{
		"CPU — top",                            // PRC section
		"Memory — top processes by RSS",        // PRM section
		"Disk I/O — top processes by reads",    // PRD section
		"Disks (DSK) — total I/O",              // DSK device section
		"System summary",                       // CPU/CPL/MEM/SWP/PAG/PSI summary
		"ctxsw:",                               // CPL summary line
		"cache:",                               // MEM summary line
		"(10s avg)",                            // PSI summary line
	}
	for _, want := range expectPresent {
		if !strings.Contains(stdout, want) {
			t.Errorf("expected %q in CLI output for full sample, missing", want)
		}
	}

	expectAbsent := []string{
		"Network interfaces (NET)",
		"InfiniBand interfaces (IFB)",
		"Logical volumes (LVM)",
		"MD RAID (MDD)",
		"GPU — top",
		"GPU devices",
	}
	for _, want := range expectAbsent {
		if strings.Contains(stdout, want) {
			t.Errorf("expected NO %q in CLI output (label absent from sample), got it", want)
		}
	}
}

// TestPipedFullSample_HTML verifies that the same labels-present / labels-absent
// invariant holds for the HTML page: one go-echarts chart per present category.
func TestPipedFullSample_HTML(t *testing.T) {
	full := filepath.Join(filepath.Dir(samplePath), "output_full.raw")
	if _, err := os.Stat(full); err != nil {
		t.Skipf("output_full.raw not present: %v", err)
	}

	workDir := t.TempDir()
	_, _ = runPipingFile(t, full, workDir, "--html-output")

	html, err := os.ReadFile(filepath.Join(workDir, "atop-flame.html"))
	if err != nil {
		t.Fatalf("read html: %v", err)
	}
	body := string(html)

	expectPresent := []string{
		"CPU — top processes by sys+usr ticks",
		"Memory — top processes by RSS",
		"Disk — top processes by reads+writes",
		"Disks (DSK) — total I/O",
		"System summary",
	}
	for _, want := range expectPresent {
		if !strings.Contains(body, want) {
			t.Errorf("expected chart title %q in HTML, missing", want)
		}
	}

	expectAbsent := []string{
		"Network interfaces (NET)",
		"InfiniBand interfaces (IFB)",
		"Logical volumes (LVM)",
		"MD RAID (MDD)",
	}
	for _, want := range expectAbsent {
		if strings.Contains(body, want) {
			t.Errorf("expected NO chart title %q in HTML (label absent from sample), got it", want)
		}
	}
}

// TestParserIgnoresJunk feeds the binary a stream that mixes valid PRC lines
// with various malformed and unknown labels — the recognized rows must still
// produce the CPU section, and the binary must exit zero.
func TestParserIgnoresJunk(t *testing.T) {
	junk := strings.Join([]string{
		"",                          // blank
		"this is not atop output",   // garbage
		"PRC malformed-line",        // truncated PRC
		"PRC host 1 d t 1 7 (a) S 100 4 8",
		"WAT host 1 d t 1 unknown label payload",
		"SEP",
		"PRC host 1 d t 1 8 (b) R 100 2 6",
	}, "\n") + "\n"

	dir := t.TempDir()
	inFile := filepath.Join(dir, "mixed.raw")
	if err := os.WriteFile(inFile, []byte(junk), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, _ := runPipingFile(t, inFile, t.TempDir())
	if !strings.Contains(stdout, "CPU — top") {
		t.Errorf("expected CPU section even with junk mixed in, got:\n%s", stdout)
	}
	for _, name := range []string{"a", "b"} {
		if !strings.Contains(stdout, name) {
			t.Errorf("expected process name %q to survive parsing", name)
		}
	}
}

// TestPipedSample_HTMLBrowserMissing exercises the --browser opt-in path:
// when a non-existent browser is given, the HTML file is still written, but
// the command exits non-zero with a useful error mentioning the bad browser.
func TestPipedSample_HTMLBrowserMissing(t *testing.T) {
	workDir := t.TempDir()

	f, err := os.Open(samplePath)
	if err != nil {
		t.Fatalf("open sample: %v", err)
	}
	defer f.Close()

	cmd := exec.Command(binPath,
		"--html-output",
		"--browser", "this-browser-does-not-exist-zzz",
	)
	cmd.Stdin = f
	cmd.Dir = workDir
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err = cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit when --browser is missing executable, got success")
	}

	// HTML file should still be on disk despite the launch failure.
	htmlPath := filepath.Join(workDir, "atop-flame.html")
	if _, statErr := os.Stat(htmlPath); statErr != nil {
		t.Errorf("expected %s to exist even when browser launch fails: %v", htmlPath, statErr)
	}

	stderr := errBuf.String()
	if !strings.Contains(stderr, "this-browser-does-not-exist-zzz") {
		t.Errorf("expected stderr to mention the bad browser name, got: %s", stderr)
	}
}
