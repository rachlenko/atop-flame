package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// procLineRe matches per-process labels (PRC/PRM/PRD/PRN/PRG):
//   LABEL host epoch date time interval pid (name) state <rest...>
var procLineRe = regexp.MustCompile(
	`^(\S+)\s+\S+\s+\d+\s+\S+\s+\S+\s+\d+\s+(\d+)\s+\(([^)]*)\)\s+(\S+)\s*(.*)$`,
)

// sysLineRe matches non-process labels:
//   LABEL host epoch date time interval <rest...>
var sysLineRe = regexp.MustCompile(
	`^(\S+)\s+\S+\s+\d+\s+\S+\s+\S+\s+\d+\s*(.*)$`,
)

// procLabels enumerates labels emitted one row per process.
var procLabels = map[string]struct{}{
	"PRC": {}, "PRM": {}, "PRD": {}, "PRN": {}, "PRG": {},
}

// parseInput consumes an atop -P style stream and returns an aggregate.
// Unrecognized or malformed lines are silently skipped.
func parseInput(r io.Reader) *Aggregate {
	agg := newAggregate()
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)

	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		if line == "SEP" || strings.HasPrefix(line, "SEP ") {
			agg.Snapshots++
			continue
		}

		// Per-process labels need the (name) capture group.
		if m := procLineRe.FindStringSubmatch(line); m != nil {
			if _, ok := procLabels[m[1]]; ok {
				name := m[3]
				state := m[4]
				rest := strings.Fields(m[5])
				parseProcLine(agg, m[1], name, state, rest)
				continue
			}
		}

		// System / device labels.
		if m := sysLineRe.FindStringSubmatch(line); m != nil {
			rest := strings.Fields(m[2])
			parseSysLine(agg, m[1], rest)
			continue
		}
	}

	if err := sc.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: input read error: %v\n", err)
	}

	// SEP only marks separators between snapshots; the first snapshot has no
	// leading SEP, so bump the count up by one when we saw any data.
	if agg.HasAny() && agg.Snapshots == 0 {
		agg.Snapshots = 1
	}
	return agg
}

// parseProcLine dispatches per-process records.
//
// rest holds whitespace-tokenized fields AFTER `state`, in atop's column order.
func parseProcLine(agg *Aggregate, label, name, state string, rest []string) {
	p := agg.process(name)
	if stateRank(state) > stateRank(p.State) {
		p.State = state
	}

	switch label {
	case "PRC":
		// rest: nproc, sys, usr, nice, prio, ...
		if len(rest) < 3 {
			return
		}
		sys, _ := strconv.Atoi(rest[1])
		usr, _ := strconv.Atoi(rest[2])
		p.SysTicks += sys
		p.UsrTicks += usr
		p.Threads++
		p.HasPRC = true

	case "PRM":
		// rest: pagesize, vsize(KiB), rsize(KiB), pshared, vgrow, rgrow,
		//       minorfaults, majorfaults, swap(KiB), ...
		if len(rest) < 3 {
			return
		}
		vsz, _ := strconv.Atoi(rest[1])
		rss, _ := strconv.Atoi(rest[2])
		if rss > p.RSSKiB {
			p.RSSKiB = rss
		}
		if vsz > p.VSizeKiB {
			p.VSizeKiB = vsz
		}
		// swap is around index 8 in current atop releases; tolerate missing.
		if len(rest) >= 9 {
			if v, err := strconv.Atoi(rest[8]); err == nil && v > p.SwapKiB {
				p.SwapKiB = v
			}
		}
		p.HasPRM = true

	case "PRD":
		// rest: container_flag, account_flag, reads, rsect, writes, wsect, ...
		if len(rest) < 6 {
			return
		}
		reads, errR := strconv.Atoi(rest[2])
		writes, errW := strconv.Atoi(rest[4])
		if errR == nil {
			p.DiskReads += reads
		}
		if errW == nil {
			p.DiskWrites += writes
		}
		p.HasPRD = true

	case "PRN":
		// kernel-patched per-process net stats; layout varies. Sum every
		// numeric field as a coarse "network activity" score.
		sum := 0
		for _, f := range rest {
			if v, err := strconv.Atoi(f); err == nil {
				sum += v
			}
		}
		p.NetActivity += sum
		p.HasPRN = true

	case "PRG":
		// per-process GPU utilization. Take the largest plausible % seen.
		for _, f := range rest {
			if v, err := strconv.Atoi(f); err == nil && v > p.GpuPct && v <= 100 {
				p.GpuPct = v
			}
		}
		p.HasPRG = true
	}
}

// parseSysLine dispatches non-process records.
func parseSysLine(agg *Aggregate, label string, f []string) {
	atoi := func(i int) int {
		if i < 0 || i >= len(f) {
			return 0
		}
		v, _ := strconv.Atoi(f[i])
		return v
	}
	atoi64 := func(i int) int64 {
		if i < 0 || i >= len(f) {
			return 0
		}
		v, _ := strconv.ParseInt(f[i], 10, 64)
		return v
	}
	atof := func(i int) float64 {
		if i < 0 || i >= len(f) {
			return 0
		}
		v, _ := strconv.ParseFloat(f[i], 64)
		return v
	}

	switch label {
	case "CPU":
		// 0=tps 1=nprocs 2=sys 3=usr 4=nice 5=idle 6=iowait 7=irq 8=softirq ...
		if len(f) < 8 {
			return
		}
		tps := atoi(0)
		sys, usr := atoi(2), atoi(3)
		nice, idle, iow, irq := atoi(4), atoi(5), atoi(6), atoi(7)
		sirq := 0
		if len(f) >= 9 {
			sirq = atoi(8)
		}
		total := sys + usr + nice + idle + iow + irq + sirq
		if total <= 0 {
			return
		}
		agg.CPU = &CPUStat{
			Ticks:  tps,
			NumCPU: atoi(1),
			Sys:    pct(sys+irq+sirq, total),
			Usr:    pct(usr+nice, total),
			Wait:   pct(iow, total),
			Idle:   pct(idle, total),
			Core:   -1,
		}

	case "cpu":
		// Per-core CPU. Field layout matches CPU but with the core index at
		// position 1 (after tps). Different atop versions diverge here; be
		// defensive and only emit a record if the math holds.
		if len(f) < 9 {
			return
		}
		tps := atoi(0)
		core := atoi(1)
		sys, usr := atoi(3), atoi(4)
		nice, idle, iow, irq := atoi(5), atoi(6), atoi(7), atoi(8)
		sirq := 0
		if len(f) >= 10 {
			sirq = atoi(9)
		}
		total := sys + usr + nice + idle + iow + irq + sirq
		if total <= 0 {
			return
		}
		c := &CPUStat{
			Ticks: tps,
			Sys:   pct(sys+irq+sirq, total),
			Usr:   pct(usr+nice, total),
			Wait:  pct(iow, total),
			Idle:  pct(idle, total),
			Core:  core,
		}
		// keep the latest snapshot's per-core readings (slice indexed by core id)
		for len(agg.PerCore) <= core {
			agg.PerCore = append(agg.PerCore, nil)
		}
		agg.PerCore[core] = c

	case "CPL":
		if len(f) < 6 {
			return
		}
		agg.CPL = &CPLStat{
			NumCPU:      atoi(0),
			Load1:       atof(1),
			Load5:       atof(2),
			Load15:      atof(3),
			CtxSwitches: atoi64(4),
			Interrupts:  atoi64(5),
		}

	case "MEM":
		// 0=pagesize 1=total 2=free 3=cache 4=buffer 5=slab (all in pages)
		if len(f) < 3 {
			return
		}
		ps := atoi64(0)
		m := &MemStat{
			TotalBytes: atoi64(1) * ps,
			FreeBytes:  atoi64(2) * ps,
		}
		if len(f) >= 4 {
			m.CacheBytes = atoi64(3) * ps
		}
		if len(f) >= 5 {
			m.BufferBytes = atoi64(4) * ps
		}
		if len(f) >= 6 {
			m.SlabBytes = atoi64(5) * ps
		}
		agg.MEM = m

	case "SWP":
		if len(f) < 3 {
			return
		}
		ps := atoi64(0)
		s := &SwpStat{
			TotalBytes: atoi64(1) * ps,
			FreeBytes:  atoi64(2) * ps,
		}
		if len(f) >= 5 {
			s.CommittedBytes = atoi64(4) * ps
		}
		agg.SWP = s

	case "PAG":
		// 0=pagesize, then counters in pages: scans, steals, stalls, swapins, swapouts, ...
		if len(f) < 1 {
			return
		}
		p := &PagStat{Pagesize: atoi(0)}
		if len(f) >= 4 {
			p.PageStalls = atoi64(3)
		}
		if len(f) >= 7 {
			p.SwapIn = atoi64(5)
			p.SwapOut = atoi64(6)
		}
		agg.PAG = p

	case "PSI":
		// 0=y|n enable indicator; then quadruples of (avg10 avg60 avg300 total)
		// for cpu_some, cpu_full, mem_some, mem_full, io_some, io_full.
		if len(f) < 1 || f[0] != "y" {
			return
		}
		psi := &PsiStat{}
		if len(f) >= 2 {
			psi.CPUSome10 = atof(1)
		}
		if len(f) >= 10 {
			psi.MemSome10 = atof(9)
		}
		if len(f) >= 18 {
			psi.IOSome10 = atof(17)
		}
		agg.PSI = psi

	case "DSK", "LVM", "MDD":
		// 0=name 1=ms_busy 2=reads 3=rsect 4=writes 5=wsect
		if len(f) < 6 {
			return
		}
		var d *DeviceAgg
		switch label {
		case "DSK":
			d = agg.disk(f[0])
		case "LVM":
			d = agg.lvm(f[0])
		case "MDD":
			d = agg.mdd(f[0])
		}
		d.MsBusy += atoi64(1)
		d.Reads += atoi64(2)
		d.ReadSectors += atoi64(3)
		d.Writes += atoi64(4)
		d.WriteSectors += atoi64(5)

	case "NET":
		// "NET upper ..." vs "NET <iface> rx_pkt rx_byte tx_pkt tx_byte ..."
		if len(f) < 1 {
			return
		}
		// Skip the aggregated "upper"/"network" summary lines — they aren't an
		// interface and would crowd the per-iface chart.
		if f[0] == "upper" || f[0] == "network" {
			return
		}
		if len(f) < 5 {
			return
		}
		n := agg.net(f[0])
		n.RxPackets += atoi64(1)
		n.RxBytes += atoi64(2)
		n.TxPackets += atoi64(3)
		n.TxBytes += atoi64(4)

	case "IFB":
		if len(f) < 5 {
			return
		}
		n := agg.ifb(f[0])
		n.RxPackets += atoi64(1)
		n.RxBytes += atoi64(2)
		n.TxPackets += atoi64(3)
		n.TxBytes += atoi64(4)

	case "NFS", "NFSC", "NFSS":
		if agg.NFS == nil {
			agg.NFS = &NfsStat{}
		}
		if label == "NFSC" || label == "NFS" {
			agg.NFS.HasClient = true
		}
		if label == "NFSS" {
			agg.NFS.HasServer = true
		}

	case "GPU":
		if len(f) < 1 {
			return
		}
		g := agg.gpu(f[0])
		for _, x := range f[1:] {
			if v, err := strconv.Atoi(x); err == nil && v > g.UtilPct && v <= 100 {
				g.UtilPct = v
			}
		}
	}
}

func pct(part, total int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(part) / float64(total) * 100
}
