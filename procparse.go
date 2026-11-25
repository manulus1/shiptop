package main

// Parsing of the Windows SystemProcessInformation buffer. Kept platform-neutral so the
// test in procparse_test.go can run anywhere; offsets are verified in wincheck/check.c.

import (
	"encoding/binary"
	"unicode/utf16"
	"unsafe"
)

func le16(b []byte, o int) uint16 { return binary.LittleEndian.Uint16(b[o:]) }
func le32(b []byte, o int) uint32 { return binary.LittleEndian.Uint32(b[o:]) }
func le64(b []byte, o int) uint64 { return binary.LittleEndian.Uint64(b[o:]) }

// abuf returns an 8-byte aligned buffer for Windows structs.
func abuf(n int) []byte {
	w := make([]uint64, (n+7)/8)
	return unsafe.Slice((*byte)(unsafe.Pointer(&w[0])), len(w)*8)
}

func ptr(b []byte) uintptr { return uintptr(unsafe.Pointer(&b[0])) }

func parseProcs(b []byte, n int) ([]ProcRaw, bool) {
	if n <= 0 || n > len(b) {
		return nil, false
	}
	base := uintptr(unsafe.Pointer(&b[0]))
	out := make([]ProcRaw, 0, 400)
	off := 0
	for guard := 0; guard < 1<<16; guard++ {
		if off < 0 || off+152 > n {
			return nil, false
		}
		e := b[off:]
		next := int(le32(e, 0))
		pid := int(le64(e, 80))
		threads := int(le32(e, 4))
		if threads < 0 || threads > 1<<20 {
			return nil, false
		}
		if pid != 0 { // pid 0 is the idle process
			name := ""
			if ln, p := int(le16(e, 56)), uintptr(le64(e, 64)); ln > 0 && ln%2 == 0 && p >= base && p+uintptr(ln) <= base+uintptr(n) {
				s := int(p - base)
				u := make([]uint16, ln/2)
				for i := range u {
					u[i] = le16(b, s+2*i)
				}
				name = string(utf16.Decode(u))
			}
			if name == "" && pid == 4 {
				name = "System"
			}
			out = append(out, ProcRaw{
				Pid: pid, PPid: int(le64(e, 88)), Threads: threads, Name: name,
				Mem:     le64(e, 144),
				CPUTime: float64(le64(e, 40)+le64(e, 48)) / 1e7, // 100 ns units
				Start:   int64(le64(e, 32)),
			})
		}
		if next == 0 {
			break
		}
		off += next
	}
	return out, len(out) > 0
}
