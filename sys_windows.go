//go:build windows

package main

// Real system data on Windows. Every hard-coded offset/size/constant below is checked
// against the Windows SDK headers (mingw-w64) in wincheck/check.c.

import (
	"encoding/binary"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var (
	modNt                      = syscall.NewLazyDLL("ntdll.dll")
	pNtQuerySystemInformation  = modNt.NewProc("NtQuerySystemInformation")
	pGlobalMemoryStatusEx      = modK32.NewProc("GlobalMemoryStatusEx")
	pGetPerformanceInfo        = modK32.NewProc("K32GetPerformanceInfo")
	pGetProcessMemoryInfo      = modK32.NewProc("K32GetProcessMemoryInfo")
	pGetSystemPowerStatus      = modK32.NewProc("GetSystemPowerStatus")
	pGetTickCount64            = modK32.NewProc("GetTickCount64")
	pGetLogicalDriveStrings    = modK32.NewProc("GetLogicalDriveStringsW")
	pGetDriveType              = modK32.NewProc("GetDriveTypeW")
	pGetDiskFreeSpaceEx        = modK32.NewProc("GetDiskFreeSpaceExW")
	pGetVolumeInformation      = modK32.NewProc("GetVolumeInformationW")
	pSetErrorMode              = modK32.NewProc("SetErrorMode")
	pQueryFullProcessImageName = modK32.NewProc("QueryFullProcessImageNameW")
)

const (
	sysProcessInformation    = 5
	sysProcessorPerformance  = 8
	statusInfoLengthMismatch = 0xC0000004
	processQueryLimited      = 0x1000
	processTerminate         = 0x0001
	tokenQuery               = 0x0008
	driveRemovable           = 2
	driveFixed               = 3
)

func regString(path, name string) string {
	var h syscall.Handle
	p, _ := syscall.UTF16PtrFromString(path)
	if err := syscall.RegOpenKeyEx(syscall.HKEY_LOCAL_MACHINE, p, 0, syscall.KEY_READ, &h); err != nil {
		return ""
	}
	defer syscall.RegCloseKey(h)
	n, _ := syscall.UTF16PtrFromString(name)
	buf := make([]uint16, 256)
	size := uint32(len(buf) * 2)
	var typ uint32
	if err := syscall.RegQueryValueEx(h, n, nil, &typ, (*byte)(unsafe.Pointer(&buf[0])), &size); err != nil {
		return ""
	}
	if typ != syscall.REG_SZ && typ != syscall.REG_EXPAND_SZ {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

func windowsName() string {
	k := `SOFTWARE\Microsoft\Windows NT\CurrentVersion`
	name := regString(k, "ProductName")
	build := regString(k, "CurrentBuildNumber")
	if b, err := strconv.Atoi(build); err == nil && b >= 22000 {
		name = strings.Replace(name, "Windows 10", "Windows 11", 1) // ProductName still says 10 on 11
	}
	if v := regString(k, "DisplayVersion"); v != "" {
		name += " " + v
	}
	if name == "" {
		name = "Windows"
	}
	return name
}

type memRaw struct{ total, avail uint64 }

func memStatus() (memRaw, bool) {
	b := abuf(64)
	binary.LittleEndian.PutUint32(b, 64) // dwLength
	if r, _, _ := pGlobalMemoryStatusEx.Call(ptr(b)); r == 0 {
		return memRaw{}, false
	}
	return memRaw{le64(b, 8), le64(b, 16)}, true
}

type perfRaw struct {
	commit, limit, cache    uint64
	handles, procs, threads int
}

func perfInfo() (perfRaw, bool) {
	b := abuf(104)
	binary.LittleEndian.PutUint32(b, 104) // cb
	if r, _, _ := pGetPerformanceInfo.Call(ptr(b), 104); r == 0 {
		return perfRaw{}, false
	}
	ps := le64(b, 80) // PageSize; the counters below are in pages
	return perfRaw{le64(b, 8) * ps, le64(b, 16) * ps, le64(b, 48) * ps, int(le32(b, 88)), int(le32(b, 92)), int(le32(b, 96))}, true
}

var lastPerf perfRaw

func sysInfo() SysInfo {
	pSetErrorMode.Call(0x0001 | 0x8000) // no "insert a disk" popups while probing drives
	info := SysInfo{Threads: runtime.NumCPU(), OS: windowsName()}
	info.CPU = cleanCPU(regString(`HARDWARE\DESCRIPTION\System\CentralProcessor\0`, "ProcessorNameString"))
	if info.CPU == "" {
		info.CPU = "CPU (" + runtime.GOARCH + ")"
	}
	if m, ok := memStatus(); ok {
		info.MemGB = float64(m.total) / (1 << 30)
	}
	info.Host, _ = os.Hostname()
	return info
}

var cpuPrev [][3]uint64

// readCPU: per logical processor busy fraction from idle/kernel/user times (kernel includes idle).
func readCPU() (float64, []float64, bool) {
	b := abuf(48 * 256)
	var ret uint32
	st, _, _ := pNtQuerySystemInformation.Call(sysProcessorPerformance, ptr(b), uintptr(len(b)), uintptr(unsafe.Pointer(&ret)))
	if st != 0 || ret < 48 || ret > uint32(len(b)) {
		return 0, nil, false
	}
	n := int(ret) / 48
	cur := make([][3]uint64, n)
	for i := range cur {
		o := i * 48
		cur[i] = [3]uint64{le64(b, o), le64(b, o+8), le64(b, o+16)}
	}
	prev := cpuPrev
	cpuPrev = cur
	if len(prev) != n {
		return 0, nil, false
	}
	cores := make([]float64, n)
	var ti, tt uint64
	for i := range cur {
		di := sub(cur[i][0], prev[i][0])
		d := sub(cur[i][1], prev[i][1]) + sub(cur[i][2], prev[i][2])
		if d > 0 && di <= d {
			cores[i] = 1 - float64(di)/float64(d)
		}
		ti += di
		tt += d
	}
	tot := 0.0
	if tt > 0 && ti <= tt {
		tot = 1 - float64(ti)/float64(tt)
	}
	return tot, cores, true
}

func readMem() MemStat {
	var m MemStat
	if r, ok := memStatus(); ok {
		m.Total, m.Avail = r.total, r.avail
	}
	if p, ok := perfInfo(); ok {
		m.Cache, m.SwapUsed, m.SwapTotal = p.cache, p.commit, p.limit
		lastPerf = p
	}
	return m
}

func counts(procs, threads int) Counts {
	if lastPerf.procs > 0 {
		return Counts{lastPerf.procs, lastPerf.threads, lastPerf.handles}
	}
	return Counts{procs, threads, 0}
}

var procBuf []byte

// readProcs uses SystemProcessInformation (what Task Manager uses): all processes,
// including ones a normal user may not open. Falls back to Toolhelp if parsing fails.
func readProcs() []ProcRaw {
	if procBuf == nil {
		procBuf = abuf(1 << 20)
	}
	for try := 0; try < 6; try++ {
		var ret uint32
		st, _, _ := pNtQuerySystemInformation.Call(sysProcessInformation, ptr(procBuf), uintptr(len(procBuf)), uintptr(unsafe.Pointer(&ret)))
		if st == 0 {
			if out, ok := parseProcs(procBuf, int(ret)); ok {
				procSource = wantProcSource
				return out
			}
			break
		}
		if uint32(st) != statusInfoLengthMismatch {
			break
		}
		n := int(ret) + 256<<10
		if n <= len(procBuf) {
			n = len(procBuf) * 2
		}
		if n > 256<<20 {
			break
		}
		procBuf = abuf(n)
	}
	procSource = "Toolhelp32 (fallback)"
	return toolhelpProcs()
}

func ft(f syscall.Filetime) uint64 { return uint64(f.HighDateTime)<<32 | uint64(f.LowDateTime) }

func toolhelpProcs() []ProcRaw {
	h, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer syscall.CloseHandle(h)
	var pe syscall.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	var out []ProcRaw
	for err = syscall.Process32First(h, &pe); err == nil; err = syscall.Process32Next(h, &pe) {
		if pe.ProcessID == 0 {
			continue
		}
		r := ProcRaw{Pid: int(pe.ProcessID), PPid: int(pe.ParentProcessID), Threads: int(pe.Threads), Name: syscall.UTF16ToString(pe.ExeFile[:])}
		if ph, e := syscall.OpenProcess(processQueryLimited, false, pe.ProcessID); e == nil {
			var c, x, k, u syscall.Filetime
			if syscall.GetProcessTimes(ph, &c, &x, &k, &u) == nil {
				r.CPUTime = float64(ft(k)+ft(u)) / 1e7
				r.Start = int64(ft(c))
			}
			mb := abuf(72)
			binary.LittleEndian.PutUint32(mb, 72)
			if ok, _, _ := pGetProcessMemoryInfo.Call(uintptr(ph), ptr(mb), 72); ok != 0 {
				r.Mem = le64(mb, 16)
			}
			syscall.CloseHandle(ph)
		}
		out = append(out, r)
	}
	return out
}

var sidNames = map[string]string{}

// procDetails runs on the detail worker goroutine only (sidNames is not shared).
func procDetails(pid int) (path, user string) {
	if pid == 4 {
		return "", "SYSTEM"
	}
	h, err := syscall.OpenProcess(processQueryLimited, false, uint32(pid))
	if err != nil {
		return "", ""
	}
	defer syscall.CloseHandle(h)
	buf := make([]uint16, 1024)
	n := uint32(len(buf))
	if r, _, _ := pQueryFullProcessImageName.Call(uintptr(h), 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n))); r != 0 && n <= uint32(len(buf)) {
		path = string(utf16.Decode(buf[:n]))
	}
	var tok syscall.Token
	if syscall.OpenProcessToken(h, tokenQuery, &tok) == nil {
		if tu, err := tok.GetTokenUser(); err == nil {
			if sid, err := tu.User.Sid.String(); err == nil {
				if v, ok := sidNames[sid]; ok {
					user = v
				} else {
					if acc, _, _, err := tu.User.Sid.LookupAccount(""); err == nil {
						user = acc
					}
					sidNames[sid] = user
				}
			}
		}
		tok.Close()
	}
	return
}

func terminate(pid int) error {
	h, err := syscall.OpenProcess(processTerminate, false, uint32(pid))
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(h)
	return syscall.TerminateProcess(h, 1)
}

var protectedNames = map[string]bool{
	"system": true, "registry": true, "secure system": true, "memory compression": true,
	"smss.exe": true, "csrss.exe": true, "wininit.exe": true, "winlogon.exe": true, "services.exe": true,
	"lsass.exe": true, "lsaiso.exe": true, "svchost.exe": true, "dwm.exe": true, "fontdrvhost.exe": true,
	"msmpeng.exe": true, "sgrmbroker.exe": true,
}

func isProtected(p *Proc) bool { return p.Pid <= 4 || protectedNames[strings.ToLower(p.Name)] }

func readDisks() []Disk {
	buf := make([]uint16, 512)
	n, _, _ := pGetLogicalDriveStrings.Call(uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])))
	if n == 0 || int(n) > len(buf) {
		return nil
	}
	var out []Disk
	start := 0
	for i := 0; i < int(n); i++ {
		if buf[i] != 0 {
			continue
		}
		root := string(utf16.Decode(buf[start:i]))
		start = i + 1
		if root == "" {
			continue
		}
		rp, err := syscall.UTF16PtrFromString(root)
		if err != nil {
			continue
		}
		t, _, _ := pGetDriveType.Call(uintptr(unsafe.Pointer(rp)))
		if t != driveFixed && t != driveRemovable {
			continue // skip network, optical and RAM drives: they can stall
		}
		var free, total, totalFree uint64
		if r, _, _ := pGetDiskFreeSpaceEx.Call(uintptr(unsafe.Pointer(rp)), uintptr(unsafe.Pointer(&free)), uintptr(unsafe.Pointer(&total)), uintptr(unsafe.Pointer(&totalFree))); r == 0 || total == 0 {
			continue
		}
		label := make([]uint16, 261)
		fs := make([]uint16, 261)
		pGetVolumeInformation.Call(uintptr(unsafe.Pointer(rp)), uintptr(unsafe.Pointer(&label[0])), uintptr(len(label)), 0, 0, 0, uintptr(unsafe.Pointer(&fs[0])), uintptr(len(fs)))
		out = append(out, Disk{Mount: strings.TrimSuffix(root, `\`), Label: syscall.UTF16ToString(label), FS: syscall.UTF16ToString(fs), Total: total, Free: totalFree})
	}
	return out
}

type ifAcc struct {
	lastIn, lastOut uint32
	in, out         uint64
	init            bool
}

var ifState = map[uint32]*ifAcc{}

func cstr(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

// readNet: IPv4 adapters (GetAdaptersInfo) with their interface byte counters (GetIfEntry).
// The counters are 32-bit, so they are accumulated into 64-bit totals across wraps.
func readNet() []NetCounter {
	l := uint32(16 << 10)
	b := abuf(int(l))
	err := syscall.GetAdaptersInfo((*syscall.IpAdapterInfo)(unsafe.Pointer(&b[0])), &l)
	if err == syscall.ERROR_BUFFER_OVERFLOW {
		b = abuf(int(l))
		err = syscall.GetAdaptersInfo((*syscall.IpAdapterInfo)(unsafe.Pointer(&b[0])), &l)
	}
	if err != nil {
		return nil
	}
	var out []NetCounter
	for ai := (*syscall.IpAdapterInfo)(unsafe.Pointer(&b[0])); ai != nil; ai = ai.Next {
		row := syscall.MibIfRow{Index: ai.Index}
		if syscall.GetIfEntry(&row) != nil {
			continue
		}
		st := ifState[ai.Index]
		if st == nil {
			st = &ifAcc{}
			ifState[ai.Index] = st
		}
		if st.init {
			st.in += uint64(row.InOctets - st.lastIn) // uint32 arithmetic handles wrap-around
			st.out += uint64(row.OutOctets - st.lastOut)
		} else {
			st.in, st.out, st.init = uint64(row.InOctets), uint64(row.OutOctets), true
		}
		st.lastIn, st.lastOut = row.InOctets, row.OutOctets
		out = append(out, NetCounter{Key: fmt.Sprint(ai.Index), Name: cstr(ai.Description[:]), Rx: st.in, Tx: st.out})
	}
	return out
}

func uptime() float64 {
	r, _, _ := pGetTickCount64.Call()
	return float64(r) / 1000
}

// platform labels
const swapLabel = "commit"
const wantProcSource = "NtQuerySystemInformation"

var procSource = "none"
