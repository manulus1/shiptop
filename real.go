package main

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"
)

const N = 360 // samples kept per history

type Proc struct {
	Pid, PPid, Threads int
	Name, Path, User   string
	Mem                uint64
	CPU                float64 // % of total CPU
	GPU                float64 // % of the busiest GPU engine
	Energy             float64 // % share of all CPU+GPU activity (estimate, see energyShares)
	H, EH              []float64
	start              int64
	lastCPU            float64
	asked, known       bool
}

type NetHist struct {
	Key, Name      string
	Rx, Tx         []float64
	RxRate, TxRate float64
	lastRx, lastTx uint64
	baseRx, baseTx uint64
	present        bool
}

type State struct {
	Ms        int
	Paused    bool
	Tick      int
	lastWall  time.Time
	Info      SysInfo
	CPUOK     bool
	CPU       float64   // 0..1
	Cores     []float64 // 0..1
	CPUH      []float64 // percent
	CoreH     [][]float64
	MemH      []float64 // percent used
	Mem       MemStat
	Disks     []Disk
	Nets      []*NetHist
	Iface     int
	IfacePick bool // user chose an adapter
	Procs     map[int]*Proc
	Count     Counts
	Up        float64
	Bat       int
	Chg       bool
	GPU       GPUStat
	Power     PowerStat
	Therm     ThermalStat
	GPUH      []float64
	Sort      string
	Rev       bool
	Sel       int  // selected pid, 0 = nothing selected
	Follow    bool // scroll to the selection on the next frame (set only by user navigation)
	Scroll    int
	Flt       string
	Filtering bool
	Confirm   int // pid waiting for y/n
	Msg, DMsg string
	MsgC      uint32
	DMsgC     uint32
	Flash     map[string]time.Time
}

var S = State{Ms: 1000, Sort: "cpu", Bat: -1, Procs: map[int]*Proc{}, Flash: map[string]time.Time{}}

type detReq struct {
	pid   int
	start int64
}
type detRes struct {
	pid        int
	start      int64
	path, user string
}

// details (path, user) are looked up off the UI thread: account lookups can be slow.
var detReqC = make(chan detReq, 512)
var detResC = make(chan detRes, 512)

func detailWorker() {
	for r := range detReqC {
		p, u := procDetails(r.pid)
		detResC <- detRes{r.pid, r.start, p, u}
	}
}

func applyDetail(d detRes) {
	if p := S.Procs[d.pid]; p != nil && p.start == d.start {
		p.Path, p.User, p.known = d.path, d.user, true
	}
}

func wantDetail(p *Proc) {
	if p.asked {
		return
	}
	select {
	case detReqC <- detReq{p.Pid, p.start}:
		p.asked = true
	default:
	}
}

func update() {
	now := time.Now()
	dt := 0.0
	if !S.lastWall.IsZero() {
		dt = now.Sub(S.lastWall).Seconds()
	}
	S.lastWall = now

	if tot, cores, ok := readCPU(); ok {
		S.CPUOK = true
		S.CPU, S.Cores = clamp(tot, 0, 1), cores
		S.CPUH = pushH(S.CPUH, S.CPU*100, N)
		for len(S.CoreH) < len(cores) {
			S.CoreH = append(S.CoreH, nil)
		}
		for i, c := range cores {
			S.CoreH[i] = pushH(S.CoreH[i], clamp(c, 0, 1)*100, 12)
		}
	}
	S.Mem = readMem()
	if S.Mem.Total > 0 {
		S.MemH = pushH(S.MemH, float64(S.Mem.Total-S.Mem.Avail)/float64(S.Mem.Total)*100, N)
	}
	if S.Tick%5 == 0 {
		S.Disks = readDisks()
	}
	updateNet(readNet(), dt)
	sampleSensors()
	S.GPU = readGPU()
	if S.GPU.OK {
		S.GPUH = pushH(S.GPUH, S.GPU.Load*100, N)
	}
	updateProcs(dt)
	energyShares()
	S.Power = readPower()
	S.Bat, S.Chg = -1, S.Power.Charging
	if S.Power.HasBattery {
		S.Bat = S.Power.Pct
	}
	S.Therm = readThermal()
	S.Up = uptime()
	if _, ok := L.Last.Rects["windows"]; ok {
		refreshWindows()
	}
	nbStep()
	S.Tick++
}

func updateNet(cs []NetCounter, dt float64) {
	for _, h := range S.Nets {
		h.present = false
	}
	for _, c := range cs {
		var h *NetHist
		for _, x := range S.Nets {
			if x.Key == c.Key {
				h = x
			}
		}
		if h == nil {
			h = &NetHist{Key: c.Key, Name: c.Name, lastRx: c.Rx, lastTx: c.Tx, baseRx: c.Rx, baseTx: c.Tx}
			S.Nets = append(S.Nets, h)
		} else if dt > 0 {
			rx, tx := 0.0, 0.0
			if c.Rx >= h.lastRx {
				rx = float64(c.Rx-h.lastRx) / dt
			}
			if c.Tx >= h.lastTx {
				tx = float64(c.Tx-h.lastTx) / dt
			}
			h.RxRate, h.TxRate = rx, tx
			h.Rx = pushH(h.Rx, rx, N)
			h.Tx = pushH(h.Tx, tx, N)
		}
		h.Name, h.lastRx, h.lastTx, h.present = c.Name, c.Rx, c.Tx, true
	}
	// until the user picks one, follow the busiest adapter
	if !S.IfacePick && len(S.Nets) > 0 {
		best, bv := 0, -1.0
		for i, h := range S.Nets {
			v := float64(sub(h.lastRx, h.baseRx)+sub(h.lastTx, h.baseTx)) + float64(h.lastRx)*1e-6 + float64(h.lastTx)*1e-6
			if h.present && v > bv {
				best, bv = i, v
			}
		}
		S.Iface = best
	}
	if S.Iface >= len(S.Nets) {
		S.Iface = 0
	}
}

func updateProcs(dt float64) {
	raws := readProcs()
	if len(raws) == 0 {
		return
	}
	ncpu := float64(runtime.NumCPU())
	seen := make(map[int]bool, len(raws))
	threads := 0
	for _, r := range raws {
		p := S.Procs[r.Pid]
		cpu := 0.0
		if p == nil || p.start != r.Start {
			p = &Proc{Pid: r.Pid, start: r.Start, lastCPU: r.CPUTime}
			S.Procs[r.Pid] = p
		} else if dt > 0 {
			cpu = clamp((r.CPUTime-p.lastCPU)/(dt*ncpu)*100, 0, 100)
		}
		p.lastCPU = r.CPUTime
		p.PPid, p.Threads, p.Name, p.Mem, p.CPU = r.PPid, r.Threads, r.Name, r.Mem, cpu
		p.GPU = 0
		if S.GPU.PerPid != nil {
			p.GPU = S.GPU.PerPid[r.Pid] * 100
		}
		p.H = pushH(p.H, cpu, 120)
		seen[r.Pid] = true
		threads += r.Threads
	}
	for pid := range S.Procs {
		if !seen[pid] {
			delete(S.Procs, pid)
		}
	}
	S.Count = counts(len(raws), threads)
}

// energyShares estimates which processes cause the most work: each process' share of all
// CPU% + GPU% right now. It is an activity share, not a measurement of watts per process
// (Windows does not expose that without admin rights).
func energyShares() {
	tot := 0.0
	for _, p := range S.Procs {
		tot += p.CPU + p.GPU
	}
	for _, p := range S.Procs {
		p.Energy = 0
		if tot > 0 {
			p.Energy = (p.CPU + p.GPU) / tot * 100
		}
		p.EH = pushH(p.EH, p.Energy, 120)
	}
}

func list() []*Proc {
	f := strings.ToLower(S.Flt)
	a := make([]*Proc, 0, len(S.Procs))
	for _, p := range S.Procs {
		if f == "" || strings.Contains(strings.ToLower(fmt.Sprintf("%d %s %s %s", p.Pid, p.Name, p.Path, p.User)), f) {
			a = append(a, p)
		}
	}
	desc := isDesc(S.Sort)
	less := func(x, y *Proc) bool {
		switch S.Sort {
		case "pid":
			return x.Pid < y.Pid
		case "name":
			if !strings.EqualFold(x.Name, y.Name) {
				return strings.ToLower(x.Name) < strings.ToLower(y.Name)
			}
			return x.Pid < y.Pid
		case "user":
			if x.User != y.User {
				return x.User < y.User
			}
			return x.Pid < y.Pid
		case "thr":
			if x.Threads != y.Threads {
				return x.Threads < y.Threads
			}
		case "mem":
			if x.Mem != y.Mem {
				return x.Mem < y.Mem
			}
		case "gpu":
			if x.GPU != y.GPU {
				return x.GPU < y.GPU
			}
		case "energy":
			if x.Energy != y.Energy {
				return x.Energy < y.Energy
			}
		default:
			if x.CPU != y.CPU {
				return x.CPU < y.CPU
			}
			if x.Mem != y.Mem {
				return x.Mem < y.Mem
			}
		}
		return x.Pid > y.Pid
	}
	sort.Slice(a, func(i, j int) bool {
		x, y := a[i], a[j]
		if desc != S.Rev {
			x, y = y, x
		}
		return less(x, y)
	})
	return a
}

var sortKeys = []string{"pid", "name", "thr", "user", "mem", "gpu", "cpu", "energy"}

func isDesc(k string) bool {
	return k == "cpu" || k == "mem" || k == "thr" || k == "gpu" || k == "energy"
}

func cur() *Proc { return S.Procs[S.Sel] }

// move changes the selection; only user navigation makes the list scroll to it.
func move(d int) {
	a := list()
	if len(a) == 0 {
		return
	}
	i := -1
	for k, p := range a {
		if p.Pid == S.Sel {
			i = k
		}
	}
	if i < 0 { // nothing selected yet: the first key press selects the top visible row
		i = clampi(S.Scroll, 0, len(a)-1)
		switch {
		case d > 0:
			d--
		case d < 0:
			d++
		}
	}
	S.Sel = a[clampi(i+d, 0, len(a)-1)].Pid
	S.Follow = true
}

// deselect drops the selection and goes back to the top of the list.
func deselect() {
	S.Sel, S.Scroll, S.Confirm, S.Follow = 0, 0, 0, false
}

// askKill puts the selected process up for confirmation (nothing happens without "y").
func askKill() {
	p := cur()
	if p == nil {
		say(DIM, "select a process first (↑ ↓)", true)
		return
	}
	if why := protectedReason(p); why != "" {
		say(BAD, why, true)
		flash("proc")
		return
	}
	S.Confirm = p.Pid
}

func doKill() {
	pid := S.Confirm
	S.Confirm = 0
	p := S.Procs[pid]
	if p == nil {
		say(WARN, fmt.Sprintf("process %d is already gone", pid), true)
		return
	}
	if err := terminate(pid); err != nil {
		say(BAD, fmt.Sprintf("could not terminate %s (%d): %v", p.Name, pid, err), true)
		flash("proc")
		return
	}
	say(OK, fmt.Sprintf("terminated %s (%d)", p.Name, pid), true)
}

func protectedReason(p *Proc) string {
	if p.Pid == os.Getpid() {
		return "that is shiptop itself, quit with q"
	}
	if isProtected(p) {
		return fmt.Sprintf("refusing to terminate %s (%d): system process", p.Name, p.Pid)
	}
	return ""
}

func memUsedFrac() float64 {
	if S.Mem.Total == 0 {
		return -1
	}
	return float64(S.Mem.Total-S.Mem.Avail) / float64(S.Mem.Total)
}

func hottestCore() (int, float64) {
	bi, bv := -1, -1.0
	for i, c := range S.Cores {
		if c > bv {
			bi, bv = i, c
		}
	}
	return bi, math.Max(0, bv)
}

func sub(a, b uint64) uint64 {
	if a >= b {
		return a - b
	}
	return 0
}
