package main

import (
	"math/rand"
	"os"
	"strings"
	"testing"
)

func fakeState() {
	S.Info = SysInfo{Host: "test", OS: "test os", CPU: "Test CPU 9000", Threads: 8, MemGB: 16}
	if NB.N == 0 {
		nbInit()
	}
	S.CPU, S.Cores = .4, []float64{.1, .9, .3, .5, .2, .8, .6, .4}
	S.Mem = MemStat{Total: 16 << 30, Avail: 9 << 30, Cache: 2 << 30, SwapTotal: 4 << 30, SwapUsed: 1 << 30}
	S.Procs = map[int]*Proc{}
	for i := 1; i <= 60; i++ {
		S.Procs[i] = &Proc{Pid: i, PPid: 1, Threads: i % 9, Name: strings.Repeat("proc", i%5+1) + ".exe", Mem: uint64(i) << 20, CPU: float64(i%7) * 1.5, GPU: float64(i % 3), Energy: 100.0 / 60, H: []float64{1, 5, 3}}
	}
	S.Procs[61] = &Proc{Pid: 61, Name: "weird\nname\twith wide 漢字 and \u0301 marks", Path: "C:\\x\ny"}
	S.Nets = []*NetHist{{Name: "Wi-Fi", Rx: []float64{1, 2000, 300}, Tx: []float64{5, 6}, RxRate: 2000, TxRate: 6}}
	S.Disks = []Disk{{Mount: "C:", Label: "Windows", FS: "NTFS", Total: 500 << 30, Free: 100 << 30}}
	S.GPU = GPUStat{OK: true, Name: "Test GPU", Load: .3}
	S.Power = PowerStat{HasBattery: true, Pct: 55, RateOK: true, RateW: -12.5, Remaining: 7200}
	WS.List = []Win{
		{H: 0x10, Title: "Inbox – Mail", PID: 3},
		{H: 0x20, Title: strings.Repeat("a very long window title ", 12), PID: 7, Min: true},
		{H: 0x30, Title: "window of a process shiptop can't see", PID: 99999},
		{H: 0x40, Title: "tabs\tand\nnewlines 漢字", PID: 61},
	}
	WS.Err = ""
}

// Every panel, at every size, stays inside its tile and does not panic.
func TestPanelsStayInTheirTile(t *testing.T) {
	fakeState()
	rng := rand.New(rand.NewSource(11))
	var sc Screen
	sc.Resize(240, 80)
	for _, d := range panelDefs {
		for round := 0; round < 300; round++ {
			w, h := 1+rng.Intn(120), 1+rng.Intn(50)
			x, y := rng.Intn(240-w+1), rng.Intn(80-h+1)
			if round%3 == 0 {
				WS.Sel, S.Sel = 0x20, 61
			} else {
				WS.Sel, S.Sel = 0, 0
			}
			S.Sort = []string{"cpu", "energy", "gpu", "name"}[round%4]
			V.Pix, V.Therm = round%len(pixModes), round%2 == 0
			for i := range sc.C {
				sc.C[i] = Cell{'#', 1, 2, false}
			}
			sc.Clip(x, y, w, h)
			d.Draw(&sc, x, y, w, h)
			sc.NoClip()
			for cy := 0; cy < sc.H; cy++ {
				for cx := 0; cx < sc.W; cx++ {
					if (Rect{x, y, w, h}).has(cx, cy) {
						continue
					}
					if c := sc.C[cy*sc.W+cx]; c != (Cell{'#', 1, 2, false}) {
						t.Fatalf("%s at %d,%d %dx%d drew %q outside its tile at %d,%d", d.ID, x, y, w, h, c.R, cx, cy)
					}
				}
			}
		}
	}
}

// The whole UI (normal, editor, zoom, matrix) draws any random layout at any size.
func TestDrawAnyLayout(t *testing.T) {
	fakeState()
	rng := rand.New(rand.NewSource(5))
	for round := 0; round < 300; round++ {
		var sc Screen
		sc.Resize(60+rng.Intn(220), 18+rng.Intn(70))
		L.Root = randomTree(rng)
		V.Edit, V.Zoom, V.Matrix = false, false, false
		switch round % 5 {
		case 1:
			L.W, L.H = sc.W, sc.H
			enterEditor()
			E.Drag = &edrag{src: E.Sel, x: rng.Intn(sc.W), y: rng.Intn(sc.H), moved: true}
		case 2:
			V.Zoom = true
		case 3:
			V.Matrix = true
		}
		draw(&sc)
		matrixFrame(.05)
		draw(&sc)
	}
	L.Root, V.Edit, V.Zoom, V.Matrix, E.Drag = nil, false, false, false, nil
}

// TestWindowsPanelLook writes a rendering of the windows panel for a visual check
// (go test -run WindowsPanelLook with SHIPTOP_DUMP=file).
func TestWindowsPanelLook(t *testing.T) {
	out := os.Getenv("SHIPTOP_DUMP")
	if out == "" {
		t.Skip("set SHIPTOP_DUMP to write the rendering")
	}
	fakeState()
	WS.Sel = 0x10
	var sc Screen
	sc.Resize(100, 9)
	sc.Clear()
	L.Last = placed{Rects: map[string]Rect{"windows": {0, 0, 100, 9}, "proc": {}}}
	L.Focus = "windows"
	drawWindows(&sc, 0, 0, 100, 9)
	var b strings.Builder
	for y := 0; y < sc.H; y++ {
		for x := 0; x < sc.W; x++ {
			b.WriteRune(sc.C[y*sc.W+x].R)
		}
		b.WriteByte('\n')
	}
	os.WriteFile(out, []byte(b.String()), 0o644)
}
