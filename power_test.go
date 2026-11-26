package main

import (
	"encoding/binary"
	"math"
	"testing"
	"unicode/utf16"
	"unsafe"
)

// buildPdh lays out PDH_FMT_COUNTERVALUE_ITEM_W entries like PDH does: items first, strings after.
func buildPdh(items []pdhItem) ([]byte, int) {
	b := abuf(4096)
	base := uintptr(unsafe.Pointer(&b[0]))
	str := len(items) * 24
	for i, it := range items {
		o := i * 24
		u := utf16.Encode([]rune(it.Name))
		binary.LittleEndian.PutUint64(b[o:], uint64(base)+uint64(str))
		for k, c := range u {
			binary.LittleEndian.PutUint16(b[str+2*k:], c)
		}
		str += 2*len(u) + 2 // NUL terminator
		binary.LittleEndian.PutUint32(b[o+8:], it.Status)
		binary.LittleEndian.PutUint64(b[o+16:], math.Float64bits(it.Value))
	}
	return b, str
}

func TestParsePdhItems(t *testing.T) {
	in := []pdhItem{
		{"pid_1234_luid_0x00000000_0x0000D1F2_phys_0_eng_0_engtype_3D", 0, 37.5},
		{`\_TZ.THM0`, 1, 3231.5},
		{"Ünïcødé", 0xC0000BC6, 0},
	}
	b, size := buildPdh(in)
	got := parsePdhItems(b, size, len(in))
	if len(got) != len(in) {
		t.Fatalf("got %d items", len(got))
	}
	for i := range in {
		if got[i] != in[i] {
			t.Errorf("item %d: got %+v want %+v", i, got[i], in[i])
		}
	}
	binary.LittleEndian.PutUint64(b[0:], 0xdeadbeef) // name pointer outside the buffer
	if got := parsePdhItems(b, size, len(in)); got[0].Name != "" {
		t.Errorf("followed an out-of-buffer pointer: %q", got[0].Name)
	}
	if parsePdhItems(b, 40, 3) != nil {
		t.Errorf("accepted a buffer too small for its item count")
	}
}

func TestParseGPUEngine(t *testing.T) {
	pid, a, e, ok := parseGPUEngine("pid_1234_luid_0x00000000_0x0000D1F2_phys_0_eng_0_engtype_3D")
	if !ok || pid != 1234 || a != "luid_0x00000000_0x0000D1F2" || e != "3D" {
		t.Errorf("got %d %q %q %v", pid, a, e, ok)
	}
	if _, _, e, ok := parseGPUEngine("pid_8_luid_0x0_0x1_phys_0_eng_3_engtype_VideoDecode"); !ok || e != "VideoDecode" {
		t.Errorf("engtype %q ok=%v", e, ok)
	}
	for _, bad := range []string{"", "_Total", "pid_x_luid_0_phys_0_engtype_3D", "pid_12_phys_0_eng_0"} {
		if _, _, _, ok := parseGPUEngine(bad); ok {
			t.Errorf("accepted %q", bad)
		}
	}
}

func TestAggregateGPU(t *testing.T) {
	A, B := "luid_0x0_0xA", "luid_0x0_0xB"
	n := func(pid, ad, eng string) string { return "pid_" + pid + "_" + ad + "_phys_0_eng_0_engtype_" + eng }
	items := []pdhItem{
		{n("10", A, "3D"), 0, 30},
		{n("20", A, "3D"), 0, 20},          // 3D on adapter A: 50 in total
		{n("10", A, "Copy"), 1, 15},        // copy engine on A: 15
		{n("30", B, "VideoDecode"), 0, 70}, // adapter B decode: 70, the busiest engine overall
		{n("40", A, "3D"), 0xC0000BC6, 99}, // invalid sample: ignored
		{"_Total", 0, 100},                 // not an engine instance: ignored
	}
	load, per, ok := aggregateGPU(items)
	if !ok || math.Abs(load-.70) > 1e-9 {
		t.Fatalf("load %.3f ok %v", load, ok)
	}
	want := map[int]float64{10: .30, 20: .20, 30: .70}
	for pid, w := range want {
		if math.Abs(per[pid]-w) > 1e-9 {
			t.Errorf("pid %d: %.3f want %.3f", pid, per[pid], w)
		}
	}
	if _, has := per[40]; has {
		t.Errorf("invalid sample counted")
	}
	if _, _, ok := aggregateGPU(nil); ok {
		t.Errorf("no items must mean no GPU data")
	}
}

func TestBatteryStructs(t *testing.T) {
	st := abuf(16)
	putLE32(st, 0, battDischarging)
	putLE32(st, 4, 41230)
	putLE32(st, 8, 11870)
	rate := int32(-14250) // -14.25 W
	putLE32(st, 12, uint32(rate))
	s, ok := parseBattStatus(st)
	if !ok || s.PowerState != battDischarging || s.Capacity != 41230 || s.Voltage != 11870 || s.Rate != -14250 {
		t.Errorf("status %+v", s)
	}
	in := abuf(36)
	putLE32(in, 0, 0x80000000)
	putLE32(in, 12, 60000)
	putLE32(in, 16, 54210)
	putLE32(in, 32, 312)
	i, ok := parseBattInfo(in)
	if !ok || i.Design != 60000 || i.Full != 54210 || i.Cycles != 312 {
		t.Errorf("info %+v", i)
	}
	if _, ok := parseBattStatus(st[:12]); ok {
		t.Errorf("short status accepted")
	}
}

func TestFormat(t *testing.T) {
	for in, want := range map[float64]string{-14.25: "-14.2W", 30.07: "+30.1W", 0.01: "0.0W"} {
		if got := fWatts(in); got != want {
			t.Errorf("fWatts(%v) = %q want %q", in, got, want)
		}
	}
	if got := fDur(2*3600 + 41*60 + 30); got != "2:41" {
		t.Errorf("fDur = %q", got)
	}
}

func TestEnergyShares(t *testing.T) {
	S.Procs = map[int]*Proc{1: {Pid: 1, CPU: 30, GPU: 0}, 2: {Pid: 2, CPU: 5, GPU: 45}, 3: {Pid: 3}}
	energyShares()
	if e := S.Procs[1].Energy; math.Abs(e-37.5) > 1e-9 {
		t.Errorf("pid 1 share %.2f", e)
	}
	if e := S.Procs[2].Energy; math.Abs(e-62.5) > 1e-9 {
		t.Errorf("pid 2 share %.2f", e)
	}
	if S.Procs[3].Energy != 0 {
		t.Errorf("idle process has a share")
	}
	S.Procs = map[int]*Proc{}
}
