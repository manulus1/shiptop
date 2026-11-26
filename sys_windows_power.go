//go:build windows

package main

// GPU load (performance counters), ACPI thermal zones (performance counters) and the battery
// (battery class driver). No admin rights or extra drivers needed. Every DLL that is not a
// KnownDLL is loaded by its full System32 path so a stray DLL next to the exe is never picked up.
// Layouts and constants are checked in tools/wincheck/power.c.

import (
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var pGetSystemDirectory = modK32.NewProc("GetSystemDirectoryW")

func sys32(dll string) string {
	buf := make([]uint16, 260)
	dir := `C:\Windows\System32`
	if n, _, _ := pGetSystemDirectory.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf))); n > 0 && int(n) < len(buf) {
		dir = syscall.UTF16ToString(buf[:n])
	}
	return filepath.Join(dir, dll)
}

var (
	modPdh                       = syscall.NewLazyDLL(sys32("pdh.dll"))
	pPdhOpenQuery                = modPdh.NewProc("PdhOpenQueryW")
	pPdhAddEnglishCounter        = modPdh.NewProc("PdhAddEnglishCounterW")
	pPdhCollectQueryData         = modPdh.NewProc("PdhCollectQueryData")
	pPdhGetFormattedCounterArray = modPdh.NewProc("PdhGetFormattedCounterArrayW")

	modSetupapi                      = syscall.NewLazyDLL(sys32("setupapi.dll"))
	pSetupDiGetClassDevs             = modSetupapi.NewProc("SetupDiGetClassDevsW")
	pSetupDiEnumDeviceInterfaces     = modSetupapi.NewProc("SetupDiEnumDeviceInterfaces")
	pSetupDiGetDeviceInterfaceDetail = modSetupapi.NewProc("SetupDiGetDeviceInterfaceDetailW")
	pSetupDiDestroyDeviceInfoList    = modSetupapi.NewProc("SetupDiDestroyDeviceInfoList")
)

const (
	pdhFmtDouble   = 0x00000200
	pdhFmtNoCap100 = 0x00008000
	pdhMoreData    = 0x800007D2

	digcfPresent         = 0x2
	digcfDeviceInterface = 0x10

	ioctlBatteryQueryTag         = 0x294040
	ioctlBatteryQueryInformation = 0x294044
	ioctlBatteryQueryStatus      = 0x29404C
	battLevelInformation         = 0
	battLevelTemperature         = 2
)

var guidDevclassBattery = syscall.GUID{Data1: 0x72631e54, Data2: 0x78a4, Data3: 0x11d0, Data4: [8]byte{0xbc, 0xf7, 0x00, 0xaa, 0x00, 0xb7, 0xb3, 0x2a}}

// ---- performance counters ----

var pdh struct {
	tried, ok bool
	q         uintptr
	gpu, tz   uintptr
	tzHP      bool
}

func pdhAdd(path string) uintptr {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0
	}
	var c uintptr
	if r, _, _ := pPdhAddEnglishCounter.Call(pdh.q, uintptr(unsafe.Pointer(p)), 0, uintptr(unsafe.Pointer(&c))); r != 0 {
		return 0
	}
	return c
}

func pdhInit() {
	pdh.tried = true
	if modPdh.Load() != nil {
		return
	}
	for _, p := range []*syscall.LazyProc{pPdhOpenQuery, pPdhAddEnglishCounter, pPdhCollectQueryData, pPdhGetFormattedCounterArray} {
		if p.Find() != nil {
			return
		}
	}
	if r, _, _ := pPdhOpenQuery.Call(0, 0, uintptr(unsafe.Pointer(&pdh.q))); r != 0 {
		return
	}
	pdh.ok = true
	// English names work on every UI language (German Windows included)
	pdh.gpu = pdhAdd(`\GPU Engine(*)\Utilization Percentage`)
	if c := pdhAdd(`\Thermal Zone Information(*)\High Precision Temperature`); c != 0 {
		pdh.tz, pdh.tzHP = c, true
	} else {
		pdh.tz = pdhAdd(`\Thermal Zone Information(*)\Temperature`)
	}
	pPdhCollectQueryData.Call(pdh.q) // rate counters need a first sample
}

// sampleSensors takes one performance counter sample per refresh.
func sampleSensors() {
	if !pdh.tried {
		pdhInit()
	}
	if pdh.ok {
		pPdhCollectQueryData.Call(pdh.q)
	}
}

func pdhArray(c uintptr) []pdhItem {
	if c == 0 {
		return nil
	}
	var size, count uint32
	for try := 0; try < 3; try++ {
		var b []byte
		var p uintptr
		if size > 0 {
			b = abuf(int(size))
			p = ptr(b)
		}
		r, _, _ := pPdhGetFormattedCounterArray.Call(c, pdhFmtDouble|pdhFmtNoCap100, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), p)
		switch {
		case r == 0 && b != nil:
			return parsePdhItems(b, int(size), int(count))
		case uint32(r) == pdhMoreData && size > 0 && size < 64<<20:
			continue // (more) instances appeared: retry with the size PDH asked for
		default:
			return nil
		}
	}
	return nil
}

var gpuName string
var gpuNameDone bool

// adapter names from the display class key, skipping Microsoft's virtual adapters
func readGPUName() string {
	const cls = `SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}\`
	var names, virtual []string
	for _, sub := range []string{"0000", "0001", "0002", "0003"} {
		switch n := regString(cls+sub, "DriverDesc"); {
		case n == "":
		case strings.HasPrefix(n, "Microsoft "):
			virtual = append(virtual, n)
		default:
			names = append(names, n)
		}
	}
	if len(names) == 0 { // only Microsoft's basic/remote adapters, e.g. in a VM
		names = virtual
	}
	return strings.Join(names, " + ")
}

func readGPU() GPUStat {
	if !gpuNameDone {
		gpuName, gpuNameDone = readGPUName(), true
	}
	if !pdh.ok {
		return GPUStat{Name: gpuName}
	}
	load, per, ok := aggregateGPU(pdhArray(pdh.gpu))
	return GPUStat{OK: ok, Load: load, PerPid: per, Name: gpuName}
}

func readThermal() ThermalStat {
	var ts ThermalStat
	if !pdh.ok {
		return ts
	}
	for _, it := range pdhArray(pdh.tz) {
		if it.Status > 1 {
			continue
		}
		c := it.Value - 273.15 // Kelvin
		if pdh.tzHP {
			c = it.Value/10 - 273.15 // tenths of Kelvin
		}
		if c < 1 || c > 125 {
			continue
		}
		ts.Zones = append(ts.Zones, Zone{strings.TrimPrefix(it.Name, `\_TZ.`), c})
		if c > ts.Max {
			ts.Max = c
		}
	}
	ts.OK = len(ts.Zones) > 0
	return ts
}

func sensorStatus() string {
	if !pdh.tried {
		pdhInit()
	}
	if !pdh.ok {
		return "performance counters unavailable"
	}
	s := "performance counters ok"
	if pdh.gpu == 0 {
		s += " · no GPU Engine counter"
	}
	if pdh.tz == 0 {
		s += " · no Thermal Zone counter"
	}
	return s
}

const wantSensors = "performance counters ok"

// ---- battery ----

type battery struct {
	h    syscall.Handle
	tag  uint32
	info battInfo
}

var batts []battery
var battScan time.Time

func batteryPaths() []string {
	if modSetupapi.Load() != nil {
		return nil
	}
	for _, p := range []*syscall.LazyProc{pSetupDiGetClassDevs, pSetupDiEnumDeviceInterfaces, pSetupDiGetDeviceInterfaceDetail, pSetupDiDestroyDeviceInfoList} {
		if p.Find() != nil {
			return nil
		}
	}
	hdev, _, _ := pSetupDiGetClassDevs.Call(uintptr(unsafe.Pointer(&guidDevclassBattery)), 0, 0, digcfPresent|digcfDeviceInterface)
	if hdev == 0 || hdev == ^uintptr(0) {
		return nil
	}
	defer pSetupDiDestroyDeviceInfoList.Call(hdev)
	var paths []string
	for i := 0; i < 8; i++ {
		did := abuf(32)
		putLE32(did, 0, 32) // cbSize = sizeof(SP_DEVICE_INTERFACE_DATA)
		if r, _, _ := pSetupDiEnumDeviceInterfaces.Call(hdev, 0, uintptr(unsafe.Pointer(&guidDevclassBattery)), uintptr(i), ptr(did)); r == 0 {
			break
		}
		var need uint32
		pSetupDiGetDeviceInterfaceDetail.Call(hdev, ptr(did), 0, 0, uintptr(unsafe.Pointer(&need)), 0)
		if need < 8 || need > 8192 {
			continue
		}
		det := abuf(int(need))
		putLE32(det, 0, 8) // cbSize = sizeof(SP_DEVICE_INTERFACE_DETAIL_DATA_W) on 64-bit Windows
		if r, _, _ := pSetupDiGetDeviceInterfaceDetail.Call(hdev, ptr(did), ptr(det), uintptr(need), 0, 0); r == 0 {
			continue
		}
		u := make([]uint16, (int(need)-4)/2)
		for k := range u {
			u[k] = le16(det, 4+2*k)
		}
		if p := syscall.UTF16ToString(u); p != "" {
			paths = append(paths, p)
		}
	}
	return paths
}

func battQueryInfo(h syscall.Handle, tag, level uint32, out []byte) bool {
	in := abuf(12)
	putLE32(in, 0, tag)
	putLE32(in, 4, level) // AtRate stays 0
	var n uint32
	return syscall.DeviceIoControl(h, ioctlBatteryQueryInformation, &in[0], 12, &out[0], uint32(len(out)), &n, nil) == nil && n > 0
}

func openBattery(path string) (battery, bool) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return battery{}, false
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ|syscall.GENERIC_WRITE, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return battery{}, false
	}
	var wait, tag, n uint32
	if err := syscall.DeviceIoControl(h, ioctlBatteryQueryTag, (*byte)(unsafe.Pointer(&wait)), 4, (*byte)(unsafe.Pointer(&tag)), 4, &n, nil); err != nil || tag == 0 {
		syscall.CloseHandle(h)
		return battery{}, false
	}
	b := battery{h: h, tag: tag}
	info := abuf(36)
	if battQueryInfo(h, tag, battLevelInformation, info) {
		b.info, _ = parseBattInfo(info)
	}
	return b, true
}

func closeBatteries() {
	for _, b := range batts {
		syscall.CloseHandle(b.h)
	}
	batts = nil
}

func readPower() PowerStat {
	ps := PowerStat{Remaining: -1}
	sp := abuf(12)
	if r, _, _ := pGetSystemPowerStatus.Call(ptr(sp)); r != 0 {
		ac, flag, pct, life := sp[0], sp[1], sp[2], le32(sp, 4)
		ps.OnAC = ac == 1
		if flag != 128 && flag != 255 && pct <= 100 {
			ps.HasBattery, ps.Pct = true, int(pct)
		}
		if life != 0xFFFFFFFF {
			ps.Remaining = float64(life) // Windows' own estimate, in seconds
		}
	}
	if !ps.HasBattery {
		return ps
	}
	if len(batts) == 0 && time.Since(battScan) > 30*time.Second {
		battScan = time.Now()
		for _, p := range batteryPaths() {
			if b, ok := openBattery(p); ok {
				batts = append(batts, b)
			}
		}
	}
	ps.RateOK = len(batts) > 0
	broken := false
	for _, b := range batts {
		in := abuf(20)
		putLE32(in, 0, b.tag) // Timeout, PowerState, LowCapacity, HighCapacity stay 0: answer now
		out := abuf(16)
		var n uint32
		if syscall.DeviceIoControl(b.h, ioctlBatteryQueryStatus, &in[0], 20, &out[0], 16, &n, nil) != nil || n < 16 {
			broken = true
			continue
		}
		st, _ := parseBattStatus(out)
		ps.Charging = ps.Charging || st.PowerState&battCharging != 0
		relative := b.info.Caps&battCapRelative != 0
		if relative || uint32(st.Rate) == battUnknownRate {
			ps.RateOK = false
		} else {
			ps.RateW += float64(st.Rate) / 1000
		}
		if !relative {
			if st.Capacity != battUnknownCapacity {
				ps.CapWh += float64(st.Capacity) / 1000
			}
			ps.FullWh += float64(b.info.Full) / 1000
			ps.DesignWh += float64(b.info.Design) / 1000
		}
		ps.Cycles += int(b.info.Cycles)
		t := abuf(8)
		if battQueryInfo(b.h, b.tag, battLevelTemperature, t) {
			if c := float64(le32(t, 0))/10 - 273.15; c > 0 && c < 90 {
				ps.TempC = c
			}
		}
	}
	if broken {
		ps.RateOK = false
		closeBatteries() // the battery may have been removed or re-enumerated: rescan later
	}
	if ps.Remaining < 0 && ps.RateOK && ps.RateW < -0.5 && ps.CapWh > 0 {
		ps.Remaining = ps.CapWh / -ps.RateW * 3600
	}
	return ps
}
