//go:build linux

package main

// Real system data on Linux from /proc and /sys (also used to test the UI on real data).

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

const swapLabel = "swap"
const wantProcSource = "procfs"

var procSource = "procfs"

const clkTck = 100.0 // USER_HZ, 100 on all mainstream Linux builds

func readFile(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return string(b)
}

func sysInfo() SysInfo {
	info := SysInfo{Threads: runtime.NumCPU(), OS: "Linux"}
	for _, l := range strings.Split(readFile("/proc/cpuinfo"), "\n") {
		if strings.HasPrefix(l, "model name") {
			if i := strings.Index(l, ":"); i >= 0 {
				info.CPU = cleanCPU(l[i+1:])
			}
			break
		}
	}
	if info.CPU == "" {
		info.CPU = "CPU (" + runtime.GOARCH + ")"
	}
	for _, l := range strings.Split(readFile("/etc/os-release"), "\n") {
		if strings.HasPrefix(l, "PRETTY_NAME=") {
			info.OS = strings.Trim(strings.TrimPrefix(l, "PRETTY_NAME="), `"`)
		}
	}
	info.Host, _ = os.Hostname()
	info.MemGB = float64(readMem().Total) / (1 << 30)
	return info
}

var cpuPrev [][2]uint64

func readCPU() (float64, []float64, bool) {
	var cur [][2]uint64 // idle, total per core; index 0 = all cpus
	for _, l := range strings.Split(readFile("/proc/stat"), "\n") {
		if !strings.HasPrefix(l, "cpu") {
			continue
		}
		f := strings.Fields(l)
		var total, idle uint64
		for i, x := range f[1:] {
			v, _ := strconv.ParseUint(x, 10, 64)
			if i >= 8 { // guest time is already counted in user/nice
				break
			}
			total += v
			if i == 3 || i == 4 {
				idle += v
			}
		}
		cur = append(cur, [2]uint64{idle, total})
	}
	prev := cpuPrev
	cpuPrev = cur
	if len(cur) < 2 || len(prev) != len(cur) {
		return 0, nil, false
	}
	frac := func(i int) float64 {
		dt := sub(cur[i][1], prev[i][1])
		di := sub(cur[i][0], prev[i][0])
		if dt == 0 || di > dt {
			return 0
		}
		return 1 - float64(di)/float64(dt)
	}
	cores := make([]float64, len(cur)-1)
	for i := range cores {
		cores[i] = frac(i + 1)
	}
	return frac(0), cores, true
}

func readMem() MemStat {
	var m MemStat
	kv := map[string]uint64{}
	for _, l := range strings.Split(readFile("/proc/meminfo"), "\n") {
		f := strings.Fields(l)
		if len(f) >= 2 {
			v, _ := strconv.ParseUint(f[1], 10, 64)
			kv[strings.TrimSuffix(f[0], ":")] = v * 1024
		}
	}
	m.Total, m.Avail, m.Cache = kv["MemTotal"], kv["MemAvailable"], kv["Cached"]
	m.SwapTotal = kv["SwapTotal"]
	m.SwapUsed = sub(kv["SwapTotal"], kv["SwapFree"])
	return m
}

func counts(procs, threads int) Counts { return Counts{procs, threads, 0} }

func readProcs() []ProcRaw {
	ents, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	page := uint64(os.Getpagesize())
	out := make([]ProcRaw, 0, len(ents))
	for _, e := range ents {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		s := readFile("/proc/" + e.Name() + "/stat")
		i, j := strings.IndexByte(s, '('), strings.LastIndexByte(s, ')')
		if i < 0 || j < i || j+2 > len(s) {
			continue
		}
		f := strings.Fields(s[j+2:]) // f[0] is field 3 (state)
		if len(f) < 22 {
			continue
		}
		num := func(k int) uint64 { v, _ := strconv.ParseUint(f[k], 10, 64); return v }
		out = append(out, ProcRaw{
			Pid: pid, PPid: int(num(1)), Name: s[i+1 : j], Threads: int(num(17)),
			CPUTime: float64(num(11)+num(12)) / clkTck,
			Start:   int64(num(19)),
			Mem:     num(21) * page,
		})
	}
	return out
}

var uidNames map[string]string

func procDetails(pid int) (path, user string) {
	p := "/proc/" + strconv.Itoa(pid)
	path = strings.TrimSpace(strings.ReplaceAll(readFile(p+"/cmdline"), "\x00", " "))
	if uidNames == nil {
		uidNames = map[string]string{}
		for _, l := range strings.Split(readFile("/etc/passwd"), "\n") {
			if f := strings.Split(l, ":"); len(f) > 2 {
				uidNames[f[2]] = f[0]
			}
		}
	}
	for _, l := range strings.Split(readFile(p+"/status"), "\n") {
		if strings.HasPrefix(l, "Uid:") {
			if f := strings.Fields(l); len(f) > 1 {
				user = uidNames[f[1]]
				if user == "" {
					user = f[1]
				}
			}
		}
	}
	return
}

func terminate(pid int) error { return syscall.Kill(pid, syscall.SIGTERM) }

func isProtected(p *Proc) bool { return p.Pid <= 2 || p.PPid == 2 }

var diskFS = map[string]bool{"ext2": true, "ext3": true, "ext4": true, "xfs": true, "btrfs": true, "vfat": true, "exfat": true,
	"ntfs": true, "ntfs3": true, "fuseblk": true, "f2fs": true, "zfs": true, "overlay": true}

func readDisks() []Disk {
	var out []Disk
	seen := map[string]bool{}
	for _, l := range strings.Split(readFile("/proc/mounts"), "\n") {
		f := strings.Fields(l)
		if len(f) < 3 || !diskFS[f[2]] || seen[f[1]] {
			continue
		}
		seen[f[1]] = true
		var st syscall.Statfs_t
		if syscall.Statfs(f[1], &st) != nil || st.Blocks == 0 {
			continue
		}
		bs := uint64(st.Bsize)
		out = append(out, Disk{Mount: f[1], Label: "", FS: f[2], Total: st.Blocks * bs, Free: st.Bavail * bs})
		if len(out) >= 8 {
			break
		}
	}
	return out
}

func readNet() []NetCounter {
	var out []NetCounter
	for _, l := range strings.Split(readFile("/proc/net/dev"), "\n") {
		i := strings.IndexByte(l, ':')
		if i < 0 {
			continue
		}
		name := strings.TrimSpace(l[:i])
		f := strings.Fields(l[i+1:])
		if name == "lo" || len(f) < 9 {
			continue
		}
		rx, _ := strconv.ParseUint(f[0], 10, 64)
		tx, _ := strconv.ParseUint(f[8], 10, 64)
		out = append(out, NetCounter{Key: name, Name: name, Rx: rx, Tx: tx})
	}
	return out
}

func uptime() float64 {
	f := strings.Fields(readFile("/proc/uptime"))
	if len(f) == 0 {
		return 0
	}
	v, _ := strconv.ParseFloat(f[0], 64)
	return v
}
