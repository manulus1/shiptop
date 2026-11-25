package main

import (
	"fmt"
	"math"
	"math/rand"
	"regexp"
	"strings"
	"time"
)

// ---- data the platform collectors deliver (sys_windows.go / sys_linux.go) ----

type SysInfo struct {
	CPU, Host, OS string
	Threads       int
	MemGB         float64
}

type MemStat struct {
	Total, Avail, Cache uint64
	SwapUsed, SwapTotal uint64 // Windows: committed / commit limit, Linux: swap
}

type Disk struct {
	Mount, Label, FS string
	Total, Free      uint64
}

// NetCounter holds cumulative byte counters for one adapter.
type NetCounter struct {
	Key, Name string
	Rx, Tx    uint64
}

type ProcRaw struct {
	Pid, PPid, Threads int
	Name               string
	Mem                uint64  // resident / working set, bytes
	CPUTime            float64 // total cpu seconds used so far
	Start              int64   // start-time stamp, detects pid reuse
}

type Counts struct{ Procs, Threads, Handles int }

// ---- small helpers ----

func R() float64 { return rand.Float64() }

func clamp(v, a, b float64) float64 {
	if v < a {
		return a
	}
	if v > b {
		return b
	}
	return v
}

func clampi(v, a, b int) int {
	if v < a {
		return a
	}
	if v > b {
		return b
	}
	return v
}

func maxi(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func mini(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func pushH(a []float64, v float64, n int) []float64 {
	if len(a) >= n {
		copy(a, a[1:])
		a[len(a)-1] = v
		return a
	}
	return append(a, v)
}

func last(a []float64) float64 {
	if len(a) == 0 {
		return 0
	}
	return a[len(a)-1]
}

func maxOf(a []float64, n int) float64 {
	m := 1e-9
	for i := len(a) - n; i < len(a); i++ {
		if i >= 0 && a[i] > m {
			m = a[i]
		}
	}
	return m
}

var reNoise = regexp.MustCompile(`\((R|TM|r|tm)\)|\s+CPU\b|\s*@.*$|\s+w/.*$|\s+with\s+Radeon.*$|\s+\d+-Core Processor.*$`)

func cleanCPU(s string) string {
	s = reNoise.ReplaceAllString(strings.TrimSpace(s), "")
	return strings.Join(strings.Fields(s), " ")
}

func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}

// fBytes: binary units, e.g. "9.81 GiB"
func fBytes(b float64) string {
	u := []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"}
	i := 0
	for b >= 1024 && i < len(u)-1 {
		b /= 1024
		i++
	}
	switch {
	case i == 0:
		return fmt.Sprintf("%.0f B", b)
	case b >= 100:
		return fmt.Sprintf("%.0f %s", b, u[i])
	case b >= 10:
		return fmt.Sprintf("%.1f %s", b, u[i])
	}
	return fmt.Sprintf("%.2f %s", b, u[i])
}

// fShort: compact column form, e.g. "312M", "1.4G"
func fShort(b uint64) string {
	f := float64(b)
	u := []string{"B", "K", "M", "G", "T"}
	i := 0
	for f >= 1024 && i < len(u)-1 {
		f /= 1024
		i++
	}
	if i == 0 || f >= 100 {
		return fmt.Sprintf("%.0f%s", f, u[i])
	}
	return fmt.Sprintf("%.1f%s", f, u[i])
}

func fRate(bps float64) string { return fBytes(bps) + "/s" }

func fCount(n int) string {
	switch {
	case n >= 1e6:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1e4:
		return fmt.Sprintf("%.0fk", float64(n)/1e3)
	case n >= 1e3:
		return fmt.Sprintf("%.1fk", float64(n)/1e3)
	}
	return fmt.Sprint(n)
}

func fUptime(sec float64) string {
	s := int(sec)
	d := s / 86400
	if d > 0 {
		return fmt.Sprintf("up %dd %02d:%02d:%02d", d, s%86400/3600, s%3600/60, s%60)
	}
	return fmt.Sprintf("up %02d:%02d:%02d", s/3600, s%3600/60, s%60)
}

func pctS(v float64) string { return fmt.Sprintf("%d%%", int(math.Round(v))) }

func say(c uint32, m string, local bool) {
	S.Msg, S.MsgC = m, c
	if local {
		S.DMsg, S.DMsgC = m, c
	}
}

func flash(k string) { S.Flash[k] = time.Now().Add(900 * time.Millisecond) }
