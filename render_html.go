package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/components"
	"github.com/go-echarts/go-echarts/v2/opts"
)

// Hex palette mirrored from the CLI renderer so HTML and terminal output
// stay visually consistent.
const (
	htmlSys  = "#BA7517"
	htmlUsr  = "#1D9E75"
	htmlMem  = "#7A6AC9"
	htmlDisk = "#3F8DBF"
	htmlNet  = "#C964A4"
	htmlGpu  = "#5DA130"
)

// renderHTML writes a multi-section page to outputPath. A chart is rendered
// only for metric categories where the aggregate actually has data.
func renderHTML(agg *Aggregate, outputPath string, topN int) error {
	if !agg.HasAny() {
		return fmt.Errorf("no recognizable atop data to render")
	}

	page := components.NewPage()
	page.PageTitle = "atop-flame — resource flame charts"

	if cpu := agg.topProcs(func(p *ProcAgg) int64 { return p.CPUTotal() }, topN); len(cpu) > 0 {
		page.AddCharts(buildProcCPUChart(cpu))
	}
	if mem := agg.topProcs(func(p *ProcAgg) int64 { return p.RSS() }, topN); len(mem) > 0 {
		page.AddCharts(buildProcMetricChart(
			"Memory — top processes by RSS",
			"RSS (KiB)",
			"rss",
			htmlMem,
			mem,
			func(p *ProcAgg) int64 { return p.RSS() },
		))
	}
	if disk := agg.topProcs(func(p *ProcAgg) int64 { return p.DiskTotal() }, topN); len(disk) > 0 {
		page.AddCharts(buildProcMetricChart(
			"Disk — top processes by reads+writes",
			"disk operations",
			"ops",
			htmlDisk,
			disk,
			func(p *ProcAgg) int64 { return p.DiskTotal() },
		))
	}
	if nets := agg.topProcs(func(p *ProcAgg) int64 { return p.NetTotal() }, topN); len(nets) > 0 {
		page.AddCharts(buildProcMetricChart(
			"Network — top processes by activity",
			"net activity",
			"net",
			htmlNet,
			nets,
			func(p *ProcAgg) int64 { return p.NetTotal() },
		))
	}
	if gpu := agg.topProcs(func(p *ProcAgg) int64 { return p.GPU() }, topN); len(gpu) > 0 {
		page.AddCharts(buildProcMetricChart(
			"GPU — top processes by utilization",
			"GPU %",
			"gpu",
			htmlGpu,
			gpu,
			func(p *ProcAgg) int64 { return p.GPU() },
		))
	}

	if disks := agg.topDevices(agg.disks, agg.diskOrder, topN); len(disks) > 0 {
		page.AddCharts(buildDeviceChart("Disks (DSK) — total I/O", "bytes", htmlDisk, disks))
	}
	if lvms := agg.topDevices(agg.lvms, agg.lvmOrder, topN); len(lvms) > 0 {
		page.AddCharts(buildDeviceChart("Logical volumes (LVM) — total I/O", "bytes", htmlDisk, lvms))
	}
	if mdds := agg.topDevices(agg.mdds, agg.mddOrder, topN); len(mdds) > 0 {
		page.AddCharts(buildDeviceChart("MD RAID (MDD) — total I/O", "bytes", htmlDisk, mdds))
	}
	if nets := agg.topNets(agg.nets, agg.netOrder, topN); len(nets) > 0 {
		page.AddCharts(buildNetChart("Network interfaces (NET)", htmlNet, nets))
	}
	if ifbs := agg.topNets(agg.ifbs, agg.ifbOrder, topN); len(ifbs) > 0 {
		page.AddCharts(buildNetChart("InfiniBand interfaces (IFB)", htmlNet, ifbs))
	}

	page.AddCharts(buildSummaryChart(agg))

	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create output file %q: %w", outputPath, err)
	}
	defer f.Close()
	return page.Render(f)
}

// ── per-process charts ────────────────────────────────────────────────────────

func buildProcCPUChart(procs []*ProcAgg) *charts.Bar {
	// Reverse so the largest bar appears at the top.
	rev := reverseProcs(procs)
	names := make([]string, len(rev))
	sysData := make([]opts.BarData, len(rev))
	usrData := make([]opts.BarData, len(rev))
	for i, p := range rev {
		label := p.Name
		if p.Threads > 1 {
			label += fmt.Sprintf(" ×%d", p.Threads)
		}
		names[i] = label
		sysData[i] = opts.BarData{Value: p.SysTicks, ItemStyle: &opts.ItemStyle{Color: htmlSys}}
		usrData[i] = opts.BarData{Value: p.UsrTicks, ItemStyle: &opts.ItemStyle{Color: htmlUsr}}
	}
	bar := charts.NewBar()
	bar.SetGlobalOptions(
		titleOpts("CPU — top processes by sys+usr ticks",
			fmt.Sprintf("%d processes  ·  ■ sys (amber)  ■ usr (teal)", len(rev))),
		initOpts(len(rev)),
		tooltipAxis(),
		legendData([]string{"sys ticks", "usr ticks"}),
		gridOpts(),
		xAxisOpts("CPU ticks (centiseconds)"),
		yAxisMonoOpts(),
	)
	bar.SetXAxis(names).
		AddSeries("sys ticks", sysData, charts.WithBarChartOpts(opts.BarChart{Stack: "total"})).
		AddSeries("usr ticks", usrData, charts.WithBarChartOpts(opts.BarChart{Stack: "total"}))
	bar.XYReversal()
	return bar
}

func buildProcMetricChart(
	title, axisName, seriesName, color string,
	procs []*ProcAgg,
	score func(*ProcAgg) int64,
) *charts.Bar {
	rev := reverseProcs(procs)
	names := make([]string, len(rev))
	data := make([]opts.BarData, len(rev))
	for i, p := range rev {
		names[i] = p.Name
		data[i] = opts.BarData{Value: score(p), ItemStyle: &opts.ItemStyle{Color: color}}
	}
	bar := charts.NewBar()
	bar.SetGlobalOptions(
		titleOpts(title, fmt.Sprintf("%d processes", len(rev))),
		initOpts(len(rev)),
		tooltipAxis(),
		legendData([]string{seriesName}),
		gridOpts(),
		xAxisOpts(axisName),
		yAxisMonoOpts(),
	)
	bar.SetXAxis(names).AddSeries(seriesName, data)
	bar.XYReversal()
	return bar
}

// ── device charts ─────────────────────────────────────────────────────────────

func buildDeviceChart(title, axisName, color string, devs []*DeviceAgg) *charts.Bar {
	// reverse for top-on-top layout
	out := make([]*DeviceAgg, len(devs))
	for i, d := range devs {
		out[len(devs)-1-i] = d
	}
	names := make([]string, len(out))
	data := make([]opts.BarData, len(out))
	for i, d := range out {
		names[i] = d.Name
		data[i] = opts.BarData{Value: d.IOBytes(), ItemStyle: &opts.ItemStyle{Color: color}}
	}
	bar := charts.NewBar()
	bar.SetGlobalOptions(
		titleOpts(title, fmt.Sprintf("%d devices", len(out))),
		initOpts(len(out)),
		tooltipAxis(),
		legendData([]string{"bytes"}),
		gridOpts(),
		xAxisOpts(axisName),
		yAxisMonoOpts(),
	)
	bar.SetXAxis(names).AddSeries("bytes", data)
	bar.XYReversal()
	return bar
}

func buildNetChart(title, color string, nets []*NetAgg) *charts.Bar {
	out := make([]*NetAgg, len(nets))
	for i, n := range nets {
		out[len(nets)-1-i] = n
	}
	names := make([]string, len(out))
	rxData := make([]opts.BarData, len(out))
	txData := make([]opts.BarData, len(out))
	for i, n := range out {
		names[i] = n.Name
		rxData[i] = opts.BarData{Value: n.RxBytes, ItemStyle: &opts.ItemStyle{Color: color}}
		txData[i] = opts.BarData{Value: n.TxBytes, ItemStyle: &opts.ItemStyle{Color: htmlGpu}}
	}
	bar := charts.NewBar()
	bar.SetGlobalOptions(
		titleOpts(title, fmt.Sprintf("%d interfaces  ·  rx + tx", len(out))),
		initOpts(len(out)),
		tooltipAxis(),
		legendData([]string{"rx bytes", "tx bytes"}),
		gridOpts(),
		xAxisOpts("bytes"),
		yAxisMonoOpts(),
	)
	bar.SetXAxis(names).
		AddSeries("rx bytes", rxData, charts.WithBarChartOpts(opts.BarChart{Stack: "total"})).
		AddSeries("tx bytes", txData, charts.WithBarChartOpts(opts.BarChart{Stack: "total"}))
	bar.XYReversal()
	return bar
}

// ── system summary chart ──────────────────────────────────────────────────────

func buildSummaryChart(agg *Aggregate) *charts.Bar {
	type row struct {
		label string
		value float64
		color string
	}
	var rows []row
	if c := agg.CPU; c != nil {
		rows = append(rows,
			row{"CPU idle %", c.Idle, "#9aa39a"},
			row{"CPU sys %", c.Sys, htmlSys},
			row{"CPU usr %", c.Usr, htmlUsr},
			row{"CPU wait %", c.Wait, htmlDisk},
		)
	}
	if l := agg.CPL; l != nil {
		rows = append(rows,
			row{"load 15m", l.Load15, "#b0b0a8"},
			row{"load 5m", l.Load5, "#a0a098"},
			row{"load 1m", l.Load1, "#909088"},
		)
	}
	if p := agg.PSI; p != nil {
		rows = append(rows,
			row{"PSI io 10s %", p.IOSome10, htmlDisk},
			row{"PSI mem 10s %", p.MemSome10, htmlMem},
			row{"PSI cpu 10s %", p.CPUSome10, htmlSys},
		)
	}
	if m := agg.MEM; m != nil && m.TotalBytes > 0 {
		used := float64(m.UsedBytes()) / float64(m.TotalBytes) * 100
		rows = append(rows, row{"mem used %", used, htmlMem})
	}
	if s := agg.SWP; s != nil && s.TotalBytes > 0 {
		used := float64(s.UsedBytes()) / float64(s.TotalBytes) * 100
		rows = append(rows, row{"swap used %", used, "#c45c5c"})
	}

	if len(rows) == 0 {
		// no system data — render an empty placeholder so the page still parses
		rows = append(rows, row{"snapshots", float64(agg.Snapshots), "#888780"})
	}

	names := make([]string, len(rows))
	data := make([]opts.BarData, len(rows))
	for i, r := range rows {
		names[i] = r.label
		data[i] = opts.BarData{Value: r.value, ItemStyle: &opts.ItemStyle{Color: r.color}}
	}
	bar := charts.NewBar()
	subtitle := fmt.Sprintf("snapshots: %d", agg.Snapshots)
	if agg.NFS != nil {
		role := []string{}
		if agg.NFS.HasClient {
			role = append(role, "client")
		}
		if agg.NFS.HasServer {
			role = append(role, "server")
		}
		subtitle += "  ·  NFS: " + strings.Join(role, "+")
	}
	bar.SetGlobalOptions(
		titleOpts("System summary", subtitle),
		initOpts(len(rows)),
		tooltipAxis(),
		gridOpts(),
		xAxisOpts("value"),
		yAxisMonoOpts(),
	)
	bar.SetXAxis(names).AddSeries("value", data)
	bar.XYReversal()
	return bar
}

// ── shared option helpers ─────────────────────────────────────────────────────

func titleOpts(title, subtitle string) charts.GlobalOpts {
	return charts.WithTitleOpts(opts.Title{Title: title, Subtitle: subtitle})
}

func initOpts(rows int) charts.GlobalOpts {
	h := rows*30 + 140
	if h < 280 {
		h = 280
	}
	return charts.WithInitializationOpts(opts.Initialization{
		Width:  "100%",
		Height: fmt.Sprintf("%dpx", h),
	})
}

func tooltipAxis() charts.GlobalOpts {
	return charts.WithTooltipOpts(opts.Tooltip{Show: true, Trigger: "axis"})
}

func legendData(items []string) charts.GlobalOpts {
	return charts.WithLegendOpts(opts.Legend{Show: true, Data: items})
}

func gridOpts() charts.GlobalOpts {
	return charts.WithGridOpts(opts.Grid{Left: "22%", Right: "6%", Top: "80px", Bottom: "30px"})
}

func xAxisOpts(name string) charts.GlobalOpts {
	return charts.WithXAxisOpts(opts.XAxis{
		Name:         name,
		NameLocation: "middle",
		NameGap:      25,
	})
}

func yAxisMonoOpts() charts.GlobalOpts {
	return charts.WithYAxisOpts(opts.YAxis{
		AxisLabel: &opts.AxisLabel{FontFamily: "monospace", FontSize: "11"},
	})
}

func reverseProcs(procs []*ProcAgg) []*ProcAgg {
	out := make([]*ProcAgg, len(procs))
	for i, p := range procs {
		out[len(procs)-1-i] = p
	}
	return out
}
