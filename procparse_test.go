package main

import (
	"encoding/binary"
	"testing"
	"unicode/utf16"
	"unsafe"
)

// builds a buffer laid out exactly like SYSTEM_PROCESS_INFORMATION entries on x64
func TestParseProcs(t *testing.T) {
	b := abuf(4096)
	base := uintptr(unsafe.Pointer(&b[0]))
	type ent struct {
		pid, ppid, thr uint64
		user, kern, ws uint64
		create         uint64
		name           string
	}
	ents := []ent{
		{0, 0, 8, 0, 0, 8192, 0, ""}, // idle process: skipped
		{4, 0, 250, 1e7, 3e7, 140 << 10, 111, ""},
		{1234, 900, 37, 2e7, 5e6, 512 << 20, 222, "chrome.exe"},
		{5678, 1234, 3, 0, 1, 1 << 20, 333, "Ünïcødé.exe"},
	}
	off, nameOff := 0, 2048
	for i, e := range ents {
		x := b[off:]
		next := uint32(256)
		if i == len(ents)-1 {
			next = 0
		}
		binary.LittleEndian.PutUint32(x[0:], next)
		binary.LittleEndian.PutUint32(x[4:], uint32(e.thr))
		binary.LittleEndian.PutUint64(x[32:], e.create)
		binary.LittleEndian.PutUint64(x[40:], e.user)
		binary.LittleEndian.PutUint64(x[48:], e.kern)
		if e.name != "" {
			u := utf16.Encode([]rune(e.name))
			for k, c := range u {
				binary.LittleEndian.PutUint16(b[nameOff+2*k:], c)
			}
			binary.LittleEndian.PutUint16(x[56:], uint16(2*len(u)))
			binary.LittleEndian.PutUint16(x[58:], uint16(2*len(u)))
			binary.LittleEndian.PutUint64(x[64:], uint64(base)+uint64(nameOff))
			nameOff += 2*len(u) + 8
		}
		binary.LittleEndian.PutUint64(x[80:], e.pid)
		binary.LittleEndian.PutUint64(x[88:], e.ppid)
		binary.LittleEndian.PutUint64(x[144:], e.ws)
		off += 256
	}
	got, ok := parseProcs(b, 3000)
	if !ok || len(got) != 3 {
		t.Fatalf("ok=%v len=%d", ok, len(got))
	}
	want := []ProcRaw{
		{Pid: 4, PPid: 0, Threads: 250, Name: "System", Mem: 140 << 10, CPUTime: 4, Start: 111},
		{Pid: 1234, PPid: 900, Threads: 37, Name: "chrome.exe", Mem: 512 << 20, CPUTime: 2.5, Start: 222},
		{Pid: 5678, PPid: 1234, Threads: 3, Name: "Ünïcødé.exe", Mem: 1 << 20, CPUTime: 1e-7, Start: 333},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d:\n got  %+v\n want %+v", i, got[i], want[i])
		}
	}
	// a name pointer outside the buffer must be ignored, not followed
	binary.LittleEndian.PutUint64(b[512+64:], 0xdeadbeef)
	got, _ = parseProcs(b, 3000)
	if got[1].Name != "" {
		t.Errorf("out-of-buffer name pointer was followed: %q", got[1].Name)
	}
	// truncated buffer must fail cleanly
	if _, ok := parseProcs(b, 300); ok {
		t.Errorf("truncated buffer accepted")
	}
}

func TestAbufAligned(t *testing.T) {
	for _, n := range []int{12, 64, 104, 1 << 20} {
		if uintptr(unsafe.Pointer(&abuf(n)[0]))%8 != 0 {
			t.Fatalf("abuf(%d) not 8-byte aligned", n)
		}
	}
}

func TestCellRune(t *testing.T) {
	cases := map[rune]rune{'a': 'a', '\n': ' ', '\t': ' ', 0x7f: ' ', 0x85: ' ', 'é': 'é', '·': '·', '…': '…',
		'⣿': '⣿', '▀': '▀', '█': '█', '╭': '╭', '¹': '¹', '°': '°', '中': '?', '😀': '?', 0x301: '?', 0x200B: '?', 'ｱ': 'ｱ', 'Ａ': '?'}
	for in, want := range cases {
		if got := cellRune(in); got != want {
			t.Errorf("cellRune(%U) = %q, want %q", in, got, want)
		}
	}
	var sc Screen
	sc.Resize(20, 1)
	sc.Clear()
	end := sc.Str(0, 0, "a\nb中c", FG, 0)
	if end != 5 {
		t.Fatalf("Str advanced to %d, want 5", end)
	}
	got := ""
	for _, c := range sc.C[:5] {
		got += string(c.R)
	}
	if got != "a b?c" {
		t.Errorf("row = %q", got)
	}
}
