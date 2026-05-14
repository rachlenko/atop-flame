package main

import "sort"

// Aggregate accumulates parsed atop -P data across all snapshots in the stream.
// Per-process and per-device metrics are merged by name; system metrics are
// kept as the most recently observed snapshot value.
type Aggregate struct {
	procs     map[string]*ProcAgg
	procOrder []string

	disks     map[string]*DeviceAgg
	diskOrder []string
	lvms      map[string]*DeviceAgg
	lvmOrder  []string
	mdds      map[string]*DeviceAgg
	mddOrder  []string

	nets     map[string]*NetAgg
	netOrder []string
	ifbs     map[string]*NetAgg
	ifbOrder []string

	gpus     map[string]*GpuAgg
	gpuOrder []string

	CPU     *CPUStat
	PerCore []*CPUStat // from 'cpu' lines, indexed by core id
	CPL     *CPLStat
	MEM     *MemStat
	SWP     *SwpStat
	PAG     *PagStat
	PSI     *PsiStat
	NFS     *NfsStat

	Snapshots int // counted via SEP separator lines
}

func newAggregate() *Aggregate {
	return &Aggregate{
		procs: map[string]*ProcAgg{},
		disks: map[string]*DeviceAgg{},
		lvms:  map[string]*DeviceAgg{},
		mdds:  map[string]*DeviceAgg{},
		nets:  map[string]*NetAgg{},
		ifbs:  map[string]*NetAgg{},
		gpus:  map[string]*GpuAgg{},
	}
}

// HasAny reports whether the aggregate received any recognized data.
func (a *Aggregate) HasAny() bool {
	return len(a.procs) > 0 || len(a.disks) > 0 || len(a.lvms) > 0 ||
		len(a.mdds) > 0 || len(a.nets) > 0 || len(a.ifbs) > 0 || len(a.gpus) > 0 ||
		a.CPU != nil || a.CPL != nil || a.MEM != nil || a.SWP != nil ||
		a.PAG != nil || a.PSI != nil || a.NFS != nil || len(a.PerCore) > 0
}

func (a *Aggregate) process(name string) *ProcAgg {
	p, ok := a.procs[name]
	if !ok {
		p = &ProcAgg{Name: name}
		a.procs[name] = p
		a.procOrder = append(a.procOrder, name)
	}
	return p
}

func (a *Aggregate) disk(name string) *DeviceAgg {
	d, ok := a.disks[name]
	if !ok {
		d = &DeviceAgg{Name: name}
		a.disks[name] = d
		a.diskOrder = append(a.diskOrder, name)
	}
	return d
}

func (a *Aggregate) lvm(name string) *DeviceAgg {
	d, ok := a.lvms[name]
	if !ok {
		d = &DeviceAgg{Name: name}
		a.lvms[name] = d
		a.lvmOrder = append(a.lvmOrder, name)
	}
	return d
}

func (a *Aggregate) mdd(name string) *DeviceAgg {
	d, ok := a.mdds[name]
	if !ok {
		d = &DeviceAgg{Name: name}
		a.mdds[name] = d
		a.mddOrder = append(a.mddOrder, name)
	}
	return d
}

func (a *Aggregate) net(name string) *NetAgg {
	n, ok := a.nets[name]
	if !ok {
		n = &NetAgg{Name: name}
		a.nets[name] = n
		a.netOrder = append(a.netOrder, name)
	}
	return n
}

func (a *Aggregate) ifb(name string) *NetAgg {
	n, ok := a.ifbs[name]
	if !ok {
		n = &NetAgg{Name: name}
		a.ifbs[name] = n
		a.ifbOrder = append(a.ifbOrder, name)
	}
	return n
}

func (a *Aggregate) gpu(name string) *GpuAgg {
	g, ok := a.gpus[name]
	if !ok {
		g = &GpuAgg{Name: name}
		a.gpus[name] = g
		a.gpuOrder = append(a.gpuOrder, name)
	}
	return g
}

// topProcs returns the top-N processes ranked descending by score, filtering
// out entries with zero score.
func (a *Aggregate) topProcs(score func(*ProcAgg) int64, n int) []*ProcAgg {
	out := make([]*ProcAgg, 0, len(a.procOrder))
	for _, name := range a.procOrder {
		p := a.procs[name]
		if score(p) > 0 {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return score(out[i]) > score(out[j]) })
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

func (a *Aggregate) topDevices(devs map[string]*DeviceAgg, order []string, n int) []*DeviceAgg {
	out := make([]*DeviceAgg, 0, len(order))
	for _, name := range order {
		d := devs[name]
		if d.IOTotal() > 0 || d.MsBusy > 0 {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].IOTotal() > out[j].IOTotal() })
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

func (a *Aggregate) topNets(nets map[string]*NetAgg, order []string, n int) []*NetAgg {
	out := make([]*NetAgg, 0, len(order))
	for _, name := range order {
		x := nets[name]
		if x.TotalBytes() > 0 {
			out = append(out, x)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TotalBytes() > out[j].TotalBytes() })
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

// ── per-process ───────────────────────────────────────────────────────────────

type ProcAgg struct {
	Name  string
	State string

	// PRC — summed across threads
	SysTicks int
	UsrTicks int
	Threads  int
	HasPRC   bool

	// PRM — peak observed (threads share memory; sum would over-count)
	RSSKiB   int
	VSizeKiB int
	SwapKiB  int
	HasPRM   bool

	// PRD — summed across snapshots
	DiskReads  int
	DiskWrites int
	HasPRD     bool

	// PRN — summed across snapshots
	NetActivity int
	HasPRN      bool

	// PRG — peak utilization observed
	GpuPct int
	HasPRG bool
}

func (p *ProcAgg) CPUTotal() int64  { return int64(p.SysTicks + p.UsrTicks) }
func (p *ProcAgg) DiskTotal() int64 { return int64(p.DiskReads + p.DiskWrites) }
func (p *ProcAgg) RSS() int64       { return int64(p.RSSKiB) }
func (p *ProcAgg) NetTotal() int64  { return int64(p.NetActivity) }
func (p *ProcAgg) GPU() int64       { return int64(p.GpuPct) }

// ── per-device ────────────────────────────────────────────────────────────────

type DeviceAgg struct {
	Name         string
	MsBusy       int64
	Reads        int64
	ReadSectors  int64
	Writes       int64
	WriteSectors int64
}

// IOTotal returns total sectors moved (read + write).
func (d *DeviceAgg) IOTotal() int64 { return d.ReadSectors + d.WriteSectors }

// IOBytes returns total bytes moved, assuming the conventional 512-byte sector.
func (d *DeviceAgg) IOBytes() int64 { return d.IOTotal() * 512 }

type NetAgg struct {
	Name      string
	RxPackets int64
	RxBytes   int64
	TxPackets int64
	TxBytes   int64
}

func (n *NetAgg) TotalBytes() int64   { return n.RxBytes + n.TxBytes }
func (n *NetAgg) TotalPackets() int64 { return n.RxPackets + n.TxPackets }

type GpuAgg struct {
	Name    string
	UtilPct int
	MemKiB  int
}

// ── system singletons ─────────────────────────────────────────────────────────

type CPUStat struct {
	Sys, Usr, Wait, Idle float64 // percentages, sum to ~100
	Ticks                int     // jiffies per second (HZ)
	NumCPU               int
	Core                 int // -1 for total, 0..N for per-core
}

type CPLStat struct {
	NumCPU              int
	Load1, Load5, Load15 float64
	CtxSwitches         int64
	Interrupts          int64
}

type MemStat struct {
	TotalBytes  int64
	FreeBytes   int64
	CacheBytes  int64
	BufferBytes int64
	SlabBytes   int64
}

func (m *MemStat) UsedBytes() int64 {
	used := m.TotalBytes - m.FreeBytes - m.CacheBytes - m.BufferBytes - m.SlabBytes
	if used < 0 {
		return 0
	}
	return used
}

type SwpStat struct {
	TotalBytes     int64
	FreeBytes      int64
	CommittedBytes int64
}

func (s *SwpStat) UsedBytes() int64 {
	used := s.TotalBytes - s.FreeBytes
	if used < 0 {
		return 0
	}
	return used
}

type PagStat struct {
	Pagesize   int
	SwapIn     int64
	SwapOut    int64
	PageStalls int64
}

type PsiStat struct {
	CPUSome10 float64
	MemSome10 float64
	IOSome10  float64
}

type NfsStat struct {
	HasClient bool
	HasServer bool
}

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
