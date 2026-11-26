package main

// GPU, power and temperature data, and the platform-neutral parsers behind it
// (kept here so power_test.go can test them on any OS).

import (
	"encoding/binary"
	"math"
	"strconv"
	"strings"
	"unicode/utf16"
	"unsafe"
)

type GPUStat struct {
	OK     bool
	Load   float64         // 0..1: busiest engine of the busiest adapter, like Task Manager
	PerPid map[int]float64 // 0..1 per process, same rule
	Name   string
}

type PowerStat struct {
	HasBattery     bool
	Pct            int
	OnAC, Charging bool
	RateOK         bool
	RateW          float64 // > 0 charging, < 0 discharging (= what the laptop draws from the battery)
	Remaining      float64 // seconds, < 0 unknown
	CapWh, FullWh  float64
	DesignWh       float64
	Cycles         int
	TempC          float64 // battery temperature if the battery reports one, else 0
}

type Zone struct {
	Name string
	C    float64
}

type ThermalStat struct {
	OK    bool
	Zones []Zone
	Max   float64
}

type pdhItem struct {
	Name   string
	Status uint32
	Value  float64
}

// parsePdhItems reads count PDH_FMT_COUNTERVALUE_ITEM_W entries (24 bytes each: name pointer at 0,
// CStatus at 8, double at 16) from buf. Name pointers are only followed when they point inside buf,
// where PDH places the strings after the items.
func parsePdhItems(buf []byte, size, count int) []pdhItem {
	if count <= 0 || size > len(buf) || count*24 > size {
		return nil
	}
	base := uintptr(unsafe.Pointer(&buf[0]))
	out := make([]pdhItem, 0, count)
	for i := 0; i < count; i++ {
		o := i * 24
		name := ""
		if p := uintptr(le64(buf, o)); p >= base && p < base+uintptr(size) {
			var u []uint16
			for k := int(p - base); k+1 < size; k += 2 {
				c := le16(buf, k)
				if c == 0 {
					break
				}
				u = append(u, c)
			}
			name = string(utf16.Decode(u))
		}
		out = append(out, pdhItem{name, le32(buf, o+8), math.Float64frombits(le64(buf, o+16))})
	}
	return out
}

// parseGPUEngine splits a "GPU Engine" instance name such as
// pid_1234_luid_0x00000000_0x0000D1F2_phys_0_eng_0_engtype_3D
func parseGPUEngine(name string) (pid int, adapter, engtype string, ok bool) {
	if !strings.HasPrefix(name, "pid_") {
		return
	}
	i := strings.IndexByte(name[4:], '_')
	if i < 0 {
		return
	}
	pid, err := strconv.Atoi(name[4 : 4+i])
	if err != nil {
		return
	}
	li, pi, ei := strings.Index(name, "luid_"), strings.Index(name, "_phys_"), strings.Index(name, "engtype_")
	if li < 0 || pi < li || ei < pi {
		return
	}
	return pid, name[li:pi], name[ei+8:], true
}

// aggregateGPU sums every engine type per adapter across processes; the GPU load is the busiest
// engine type (Task Manager's rule). Per process it is the busiest engine type that process uses.
func aggregateGPU(items []pdhItem) (load float64, perPid map[int]float64, ok bool) {
	type key struct{ a, e string }
	eng := map[key]float64{}
	pp := map[int]map[key]float64{}
	n := 0
	for _, it := range items {
		if it.Status > 1 { // not PDH_CSTATUS_VALID_DATA / NEW_DATA
			continue
		}
		pid, a, e, good := parseGPUEngine(it.Name)
		if !good {
			continue
		}
		n++
		v := clamp(it.Value, 0, 100)
		k := key{a, e}
		eng[k] += v
		if pp[pid] == nil {
			pp[pid] = map[key]float64{}
		}
		pp[pid][k] += v
	}
	if n == 0 {
		return 0, nil, false
	}
	for _, v := range eng {
		load = math.Max(load, v)
	}
	perPid = make(map[int]float64, len(pp))
	for pid, m := range pp {
		mx := 0.0
		for _, v := range m {
			mx = math.Max(mx, v)
		}
		perPid[pid] = clamp(mx, 0, 100) / 100
	}
	return clamp(load, 0, 100) / 100, perPid, true
}

// battery class driver structs (layouts checked in tools/wincheck/power.c)

type battStatus struct {
	PowerState, Capacity, Voltage uint32
	Rate                          int32 // mW
}

func parseBattStatus(b []byte) (battStatus, bool) {
	if len(b) < 16 {
		return battStatus{}, false
	}
	return battStatus{le32(b, 0), le32(b, 4), le32(b, 8), int32(le32(b, 12))}, true
}

type battInfo struct{ Caps, Design, Full, Cycles uint32 }

func parseBattInfo(b []byte) (battInfo, bool) {
	if len(b) < 36 {
		return battInfo{}, false
	}
	return battInfo{le32(b, 0), le32(b, 12), le32(b, 16), le32(b, 32)}, true
}

func putLE32(b []byte, o int, v uint32) { binary.LittleEndian.PutUint32(b[o:], v) }

const (
	battUnknownRate     = 0x80000000
	battUnknownCapacity = 0xFFFFFFFF
	battCapRelative     = 0x40000000
	battCharging        = 0x4
	battDischarging     = 0x2
)

// fWatts formats a signed battery rate: "-14.2W" while discharging, "+30.1W" while charging.
func fWatts(w float64) string {
	if math.Abs(w) < 0.05 {
		return "0.0W"
	}
	s := strconv.FormatFloat(w, 'f', 1, 64) + "W"
	if w > 0 {
		s = "+" + s
	}
	return s
}

func fDur(sec float64) string {
	if sec < 0 {
		return "?"
	}
	m := int(sec / 60)
	return strconv.Itoa(m/60) + ":" + pad2(m%60)
}

func pad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}
