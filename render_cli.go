package main

import (
	"fmt"
	"math"
	"os"
	"strings"

	colorful "github.com/lucasb-eyer/go-colorful"
)

var (
	cSys   = mustHex("#BA7517") // amber  — sys ticks bar
	cUsr   = mustHex("#1D9E75") // teal   — usr ticks bar
	cMem   = mustHex("#7A6AC9") // violet — RSS bar
	cDisk  = mustHex("#3F8DBF") // blue   — disk I/O bar
	cNet   = mustHex("#C964A4") // pink   — net activity
	cGpu   = mustHex("#5DA130") // green  — GPU
	cName  = mustHex("#5F5E5A") // gray   — process / device name
	cEmpty = mustHex("#D3D1C7") // light gray — empty bar fill
	cHead  = mustHex("#888780") // gray   — section header
	cRed   = mustHex("#E24B4A") // state R
	cAmb   = mustHex("#EF9F27") // state D
	cGrn   = mustHex("#1D9E75") // state S
	cGray  = mustHex("#888780") // state I / default
)

const barWidth = 36

func mustHex(h string) colorful.Color {
	c, err := colorful.Hex(h)
	if err != nil {
		panic(err)
	}
	return c
}

func ansi(c colorful.Color, s string) string {
	if s == "" {
		return ""
	}
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
	case "":
		return ansi(cGray, "[-]")
	default:
		return ansi(cGray, "[I]")
	}
}

func intRound(f float64) int { return int(math.Round(f)) }

func renderCLI(agg *Aggregate, topN int) {
	if !agg.HasAny() {
		fmt.Fprintln(os.Stderr, "no recognizable atop data found in input")
		return
	}

	if cpu := agg.topProcs(func(p *ProcAgg) int64 { return p.CPUTotal() }, topN); len(cpu) > 0 {
		renderProcCPUSection(cpu)
	}
	if mem := agg.topProcs(func(p *ProcAgg) int64 { return p.RSS() }, topN); len(mem) > 0 {
		renderProcBarSection("Memory — top processes by RSS", "RSS",
			mem, cMem,
			func(p *ProcAgg) int64 { return p.RSS() },
			formatKiB,
		)
	}
	if disk := agg.topProcs(func(p *ProcAgg) int64 { return p.DiskTotal() }, topN); len(disk) > 0 {
		renderProcBarSection("Disk I/O — top processes by reads+writes", "OPS",
			disk, cDisk,
			func(p *ProcAgg) int64 { return p.DiskTotal() },
			formatInt,
		)
	}
	if nets := agg.topProcs(func(p *ProcAgg) int64 { return p.NetTotal() }, topN); len(nets) > 0 {
		renderProcBarSection("Network — top processes by activity", "NET",
			nets, cNet,
			func(p *ProcAgg) int64 { return p.NetTotal() },
			formatInt,
		)
	}
	if gpu := agg.topProcs(func(p *ProcAgg) int64 { return p.GPU() }, topN); len(gpu) > 0 {
		renderProcBarSection("GPU — top processes by utilization", "%",
			gpu, cGpu,
			func(p *ProcAgg) int64 { return p.GPU() },
			func(v int64) string { return fmt.Sprintf("%d%%", v) },
		)
	}

	if disks := agg.topDevices(agg.disks, agg.diskOrder, topN); len(disks) > 0 {
		renderDeviceSection("Disks (DSK) — total I/O", disks, cDisk)
	}
	if lvms := agg.topDevices(agg.lvms, agg.lvmOrder, topN); len(lvms) > 0 {
		renderDeviceSection("Logical volumes (LVM) — total I/O", lvms, cDisk)
	}
	if mdds := agg.topDevices(agg.mdds, agg.mddOrder, topN); len(mdds) > 0 {
		renderDeviceSection("MD RAID (MDD) — total I/O", mdds, cDisk)
	}
	if nets := agg.topNets(agg.nets, agg.netOrder, topN); len(nets) > 0 {
		renderNetSection("Network interfaces (NET)", nets, cNet)
	}
	if ifbs := agg.topNets(agg.ifbs, agg.ifbOrder, topN); len(ifbs) > 0 {
		renderNetSection("InfiniBand interfaces (IFB)", ifbs, cNet)
	}
	if len(agg.gpus) > 0 {
		renderGPUSection(agg)
	}

	renderSystemSummary(agg)
}

// ── per-process sections ──────────────────────────────────────────────────────

func renderProcCPUSection(procs []*ProcAgg) {
	header := fmt.Sprintf("CPU — top %d processes by sys+usr ticks", len(procs))
	maxVal := int64(0)
	maxName := 16
	for _, p := range procs {
		if v := p.CPUTotal(); v > maxVal {
			maxVal = v
		}
		if n := len(p.Name); n > maxName {
			maxName = n
		}
	}
	if maxName > 30 {
		maxName = 30
	}
	printSectionHeader(header)
	fmt.Printf("  %-*s  %-3s  %-*s  %s\n",
		maxName, "PROCESS", "ST",
		barWidth, ansi(cSys, "■ sys")+" "+ansi(cUsr, "■ usr"),
		"TICKS",
	)
	fmt.Println("  " + strings.Repeat("─", maxName+barWidth+18))

	for _, p := range procs {
		total := p.CPUTotal()
		sysBars, usrBars := 0, 0
		if maxVal > 0 {
			sysBars = intRound(float64(p.SysTicks) / float64(maxVal) * barWidth)
			usrBars = intRound(float64(p.UsrTicks) / float64(maxVal) * barWidth)
		}
		empty := barWidth - sysBars - usrBars
		if empty < 0 {
			empty = 0
		}

		bar := ansi(cSys, strings.Repeat("█", sysBars)) +
			ansi(cUsr, strings.Repeat("█", usrBars)) +
			ansi(cEmpty, strings.Repeat("░", empty))

		threads := ""
		if p.Threads > 1 {
			threads = fmt.Sprintf(" ×%d", p.Threads)
		}
		fmt.Printf("  %s  %s  %s  %d%s\n",
			ansi(cName, padName(p.Name, maxName)),
			stateTag(p.State),
			bar,
			total,
			threads,
		)
	}
	fmt.Println()
}

func renderProcBarSection(
	title, unit string,
	procs []*ProcAgg,
	barColor colorful.Color,
	score func(*ProcAgg) int64,
	format func(int64) string,
) {
	maxVal := int64(0)
	maxName := 16
	for _, p := range procs {
		if v := score(p); v > maxVal {
			maxVal = v
		}
		if n := len(p.Name); n > maxName {
			maxName = n
		}
	}
	if maxName > 30 {
		maxName = 30
	}
	printSectionHeader(title)
	fmt.Printf("  %-*s  %-3s  %-*s  %s\n",
		maxName, "PROCESS", "ST",
		barWidth, ansi(barColor, "■ "+strings.ToLower(unit)),
		unit,
	)
	fmt.Println("  " + strings.Repeat("─", maxName+barWidth+18))

	for _, p := range procs {
		v := score(p)
		filled := 0
		if maxVal > 0 {
			filled = intRound(float64(v) / float64(maxVal) * barWidth)
		}
		empty := barWidth - filled
		bar := ansi(barColor, strings.Repeat("█", filled)) +
			ansi(cEmpty, strings.Repeat("░", empty))
		fmt.Printf("  %s  %s  %s  %s\n",
			ansi(cName, padName(p.Name, maxName)),
			stateTag(p.State),
			bar,
			format(v),
		)
	}
	fmt.Println()
}

// ── device sections ───────────────────────────────────────────────────────────

func renderDeviceSection(title string, devs []*DeviceAgg, barColor colorful.Color) {
	maxVal := int64(0)
	maxName := 8
	for _, d := range devs {
		if v := d.IOTotal(); v > maxVal {
			maxVal = v
		}
		if n := len(d.Name); n > maxName {
			maxName = n
		}
	}
	if maxName > 24 {
		maxName = 24
	}
	printSectionHeader(title)
	fmt.Printf("  %-*s  %-*s  %s\n",
		maxName, "DEVICE",
		barWidth, ansi(barColor, "■ I/O"),
		"BYTES",
	)
	fmt.Println("  " + strings.Repeat("─", maxName+barWidth+14))
	for _, d := range devs {
		v := d.IOTotal()
		filled := 0
		if maxVal > 0 {
			filled = intRound(float64(v) / float64(maxVal) * barWidth)
		}
		empty := barWidth - filled
		bar := ansi(barColor, strings.Repeat("█", filled)) +
			ansi(cEmpty, strings.Repeat("░", empty))
		fmt.Printf("  %s  %s  %s\n",
			ansi(cName, padName(d.Name, maxName)),
			bar,
			formatBytes(d.IOBytes()),
		)
	}
	fmt.Println()
}

func renderNetSection(title string, nets []*NetAgg, barColor colorful.Color) {
	maxVal := int64(0)
	maxName := 8
	for _, n := range nets {
		if v := n.TotalBytes(); v > maxVal {
			maxVal = v
		}
		if l := len(n.Name); l > maxName {
			maxName = l
		}
	}
	if maxName > 24 {
		maxName = 24
	}
	printSectionHeader(title)
	fmt.Printf("  %-*s  %-*s  %s\n",
		maxName, "IFACE",
		barWidth, ansi(barColor, "■ rx+tx"),
		"BYTES",
	)
	fmt.Println("  " + strings.Repeat("─", maxName+barWidth+14))
	for _, n := range nets {
		v := n.TotalBytes()
		filled := 0
		if maxVal > 0 {
			filled = intRound(float64(v) / float64(maxVal) * barWidth)
		}
		empty := barWidth - filled
		bar := ansi(barColor, strings.Repeat("█", filled)) +
			ansi(cEmpty, strings.Repeat("░", empty))
		fmt.Printf("  %s  %s  %s\n",
			ansi(cName, padName(n.Name, maxName)),
			bar,
			formatBytes(v),
		)
	}
	fmt.Println()
}

func renderGPUSection(agg *Aggregate) {
	printSectionHeader("GPU devices")
	for _, name := range agg.gpuOrder {
		g := agg.gpus[name]
		fmt.Printf("  %s  util %d%%  mem %s\n",
			ansi(cName, name),
			g.UtilPct,
			formatKiB(int64(g.MemKiB)),
		)
	}
	fmt.Println()
}

// ── system summary ────────────────────────────────────────────────────────────

func renderSystemSummary(agg *Aggregate) {
	if agg.CPU == nil && agg.CPL == nil && agg.MEM == nil && agg.SWP == nil &&
		agg.PSI == nil && agg.PAG == nil && agg.NFS == nil && len(agg.PerCore) == 0 {
		return
	}
	printSectionHeader("System summary")

	if c := agg.CPU; c != nil {
		fmt.Printf("  %-10s sys: %.1f%%  usr: %.1f%%  wait: %.1f%%  idle: %.1f%%  (cores: %d)\n",
			ansi(cHead, "CPU"), c.Sys, c.Usr, c.Wait, c.Idle, c.NumCPU)
	}
	if len(agg.PerCore) > 0 {
		var parts []string
		for i, c := range agg.PerCore {
			if c == nil {
				continue
			}
			parts = append(parts, fmt.Sprintf("cpu%d: %.0f%% busy", i, 100-c.Idle))
		}
		if len(parts) > 0 {
			fmt.Printf("  %-10s %s\n", ansi(cHead, "per-core"), strings.Join(parts, "  "))
		}
	}
	if l := agg.CPL; l != nil {
		fmt.Printf("  %-10s 1m: %.2f  5m: %.2f  15m: %.2f   ctxsw: %d  intr: %d\n",
			ansi(cHead, "load"), l.Load1, l.Load5, l.Load15, l.CtxSwitches, l.Interrupts)
	}
	if m := agg.MEM; m != nil {
		fmt.Printf("  %-10s used: %s / %s   cache: %s   buffer: %s   slab: %s\n",
			ansi(cHead, "memory"),
			formatBytes(m.UsedBytes()), formatBytes(m.TotalBytes),
			formatBytes(m.CacheBytes), formatBytes(m.BufferBytes), formatBytes(m.SlabBytes))
	}
	if s := agg.SWP; s != nil {
		fmt.Printf("  %-10s used: %s / %s   committed: %s\n",
			ansi(cHead, "swap"),
			formatBytes(s.UsedBytes()), formatBytes(s.TotalBytes), formatBytes(s.CommittedBytes))
	}
	if p := agg.PAG; p != nil {
		fmt.Printf("  %-10s swapin: %d  swapout: %d  stalls: %d\n",
			ansi(cHead, "paging"), p.SwapIn, p.SwapOut, p.PageStalls)
	}
	if p := agg.PSI; p != nil {
		fmt.Printf("  %-10s cpu: %.1f%%  mem: %.1f%%  io: %.1f%%  (10s avg)\n",
			ansi(cHead, "pressure"), p.CPUSome10, p.MemSome10, p.IOSome10)
	}
	if n := agg.NFS; n != nil {
		role := []string{}
		if n.HasClient {
			role = append(role, "client")
		}
		if n.HasServer {
			role = append(role, "server")
		}
		fmt.Printf("  %-10s activity: %s\n", ansi(cHead, "NFS"), strings.Join(role, "+"))
	}
	if agg.Snapshots > 0 {
		fmt.Printf("  %-10s %d\n", ansi(cHead, "snapshots"), agg.Snapshots)
	}
	fmt.Println()
}

// ── formatting helpers ───────────────────────────────────────────────────────

func printSectionHeader(s string) {
	fmt.Println()
	fmt.Println(ansi(cHead, "─── "+s+" "+strings.Repeat("─", maxLineRule(s))))
}

func maxLineRule(s string) int {
	n := 70 - len(s) - 5
	if n < 4 {
		n = 4
	}
	return n
}

func padName(name string, width int) string {
	if len(name) > width {
		name = name[:width-1] + "…"
	}
	return fmt.Sprintf("%-*s", width, name)
}

func formatInt(v int64) string {
	return fmt.Sprintf("%d", v)
}

func formatKiB(v int64) string {
	return formatBytes(v * 1024)
}

func formatBytes(v int64) string {
	if v <= 0 {
		return "0 B"
	}
	const k = 1024
	switch {
	case v >= k*k*k*k:
		return fmt.Sprintf("%.1f TiB", float64(v)/float64(k*k*k*k))
	case v >= k*k*k:
		return fmt.Sprintf("%.1f GiB", float64(v)/float64(k*k*k))
	case v >= k*k:
		return fmt.Sprintf("%.1f MiB", float64(v)/float64(k*k))
	case v >= k:
		return fmt.Sprintf("%.1f KiB", float64(v)/float64(k))
	}
	return fmt.Sprintf("%d B", v)
}
