package main

import (
	"fmt"
	"strings"
	"time"
)

// The windows panel lists the open top-level windows (the ones Alt+Tab shows, most recently used
// first) with the live numbers of the process that owns each one. Enter or a double click brings
// the selected window to the front. Reading windows is Windows-only (winlist_windows.go).

type Win struct {
	H     uintptr
	Title string
	Class string
	PID   int
	Min   bool // minimized
}

var WS struct {
	List    []Win
	Err     string
	Sel     uintptr // selected window, 0 = none
	Scroll  int
	Follow  bool
	Raising bool
}

type raiseRes struct {
	h     uintptr
	title string
	err   error
}

var raiseResC = make(chan raiseRes, 4)

// rowHit remembers where a list was drawn so clicks can be mapped to rows.
type rowHit struct {
	R    Rect
	Y0   int
	Keys []uintptr
}

func (h rowHit) at(x, y int) (uintptr, bool) {
	if !h.R.has(x, y) {
		return 0, false
	}
	i := y - h.Y0
	if i < 0 || i >= len(h.Keys) {
		return 0, false
	}
	return h.Keys[i], true
}

var hitProc, hitWin rowHit

func refreshWindows() {
	l, err := readWindows()
	if err != nil {
		WS.List, WS.Err = nil, err.Error()
		return
	}
	WS.List, WS.Err = l, ""
	if WS.Sel != 0 && winIndex(WS.Sel) < 0 { // the selected window was closed
		WS.Sel = 0
	}
}

func winIndex(h uintptr) int {
	for i, w := range WS.List {
		if w.H == h {
			return i
		}
	}
	return -1
}

// winMove moves the window selection; like the process list, the first key selects the top visible row.
func winMove(d int) {
	if len(WS.List) == 0 {
		return
	}
	i := winIndex(WS.Sel)
	if i < 0 {
		i = clampi(WS.Scroll, 0, len(WS.List)-1)
		switch {
		case d > 0:
			d--
		case d < 0:
			d++
		}
	}
	WS.Sel = WS.List[clampi(i+d, 0, len(WS.List)-1)].H
	WS.Follow = true
}

func raiseSelected() {
	i := winIndex(WS.Sel)
	if i < 0 {
		say(DIM, "select a window first (↑ ↓)", false)
		return
	}
	if WS.Raising {
		return
	}
	w := WS.List[i]
	WS.Raising = true
	go func() {
		_, err := raiseWindow(w.H)
		raiseResC <- raiseRes{w.H, w.Title, err}
	}()
}

func applyRaise(r raiseRes) {
	WS.Raising = false
	t := "“" + trunc(r.title, 40) + "”"
	if r.err != nil {
		say(WARN, "could not bring "+t+" to the front: "+r.err.Error(), false)
		flash("windows")
		return
	}
	say(OK, "brought "+t+" to the front", false)
	refreshWindows()
}

const CWIN = 0x3f86b0

func drawWindows(sc *Screen, x, y, w, h int) {
	col := boxCol(CWIN, "windows")
	sc.Box(x, y, w, h, col)
	cnt := sc.LabelR(x+w-2, y, true, col, Seg{fmt.Sprintf("%d", len(WS.List)), TITLE, false}, Seg{" open", DIM, false})
	if t := fitSegs([]Seg{{"⁶", HI, false}, {"windows", TITLE, true}}, cnt-1-(x+2), 2); len(t) > 0 {
		sc.Label(x+2, y, true, col, t...)
	}
	hint := []Seg{{"↑↓", HI, false}, {" select  ", DIM, false}, {"Enter", HI, false}, {" bring to front", DIM, false}}
	if len(visibleLists()) > 1 {
		hint = append(hint, Seg{"  Tab", HI, false}, Seg{" focus", DIM, false})
	}
	if t := fitSegs(hint, w-4, 0); len(t) > 0 {
		sc.Label(x+2, y+h-1, false, col, t...)
	}
	ix, iy, iw, ih := x+1, y+1, w-2, h-2
	hitWin = rowHit{R: Rect{ix, iy, iw, ih}}
	if WS.Err != "" {
		sc.Str(ix+1, iy, WS.Err, DIM, iw-2)
		return
	}
	if len(WS.List) == 0 {
		sc.Str(ix+1, iy, "no windows found", DIM, iw-2)
		return
	}
	inner := iw - 2
	use := []pcol{{"name", "Program", 14, false}, {"pid", "Pid", 6, true}, {"mem", "MemB", 6, true}}
	if S.GPU.OK {
		use = append(use, pcol{"gpu", "Gpu%", 5, true})
	}
	use = append(use, pcol{"cpu", "Cpu%", 5, true}, pcol{"energy", "Enrg%", 6, true})
	width := func(cs []pcol) int {
		n := 0
		for _, c := range cs {
			n += c.w + 1
		}
		return n
	}
	for _, k := range []string{"energy", "gpu", "pid", "mem", "name"} { // dropped while the title gets too narrow
		if inner-width(use) >= 18 {
			break
		}
		var keep []pcol
		for _, c := range use {
			if c.key != k {
				keep = append(keep, c)
			}
		}
		use = keep
	}
	tw := maxi(1, inner-width(use))
	cx0 := ix + 1
	sc.Bold(cx0, iy, "Window", TITLE, tw)
	px := cx0 + tw + 1
	for _, c := range use {
		if c.right {
			sc.Bold(px+c.w-rlen(c.title), iy, c.title, TITLE, 0)
		} else {
			sc.Bold(px, iy, c.title, TITLE, c.w)
		}
		px += c.w + 1
	}
	nrows := ih - 1
	if nrows <= 0 {
		return
	}
	idx := winIndex(WS.Sel)
	if WS.Follow && idx >= 0 {
		if idx < WS.Scroll {
			WS.Scroll = idx
		}
		if idx >= WS.Scroll+nrows {
			WS.Scroll = idx - nrows + 1
		}
	}
	WS.Follow = false
	WS.Scroll = clampi(WS.Scroll, 0, maxi(0, len(WS.List)-nrows))
	hitWin.Y0 = iy + 1
	for r := 0; r < nrows && WS.Scroll+r < len(WS.List); r++ {
		wn := WS.List[WS.Scroll+r]
		hitWin.Keys = append(hitWin.Keys, wn.H)
		ry := iy + 1 + r
		sel := wn.H == WS.Sel
		fg := uint32(FG)
		if wn.Min {
			fg = DIM
		}
		if sel {
			sc.SetBG(ix, ry, iw, SEL)
			fg = TITLE
		}
		sc.Str(cx0, ry, trunc(strings.TrimSpace(wn.Title), tw), fg, 0)
		p := S.Procs[wn.PID]
		px := cx0 + tw + 1
		for _, c := range use {
			v, cf := "?", uint32(DIM)
			if p != nil {
				cf = fg
				switch c.key {
				case "name":
					v = strings.TrimSuffix(p.Name, ".exe")
					if !sel {
						cf = DIM
					}
				case "pid":
					v = fmt.Sprint(p.Pid)
				case "mem":
					v = fShort(p.Mem)
				case "gpu":
					v, cf = fmt.Sprintf("%.1f", p.GPU), heatOrDim(p.GPU, sel)
				case "cpu":
					v, cf = fmt.Sprintf("%.1f", p.CPU), heatOrDim(p.CPU, sel)
				case "energy":
					v, cf = fmt.Sprintf("%.1f", p.Energy), heatOrDim(p.Energy, sel)
				}
			} else if c.key == "pid" && wn.PID > 0 {
				v = fmt.Sprint(wn.PID)
			}
			v = trunc(v, c.w)
			if c.right {
				sc.Str(px+c.w-rlen(v), ry, v, cf, 0)
			} else {
				sc.Str(px, ry, v, cf, 0)
			}
			px += c.w + 1
		}
	}
	if idx >= 0 && (idx < WS.Scroll || idx >= WS.Scroll+nrows) {
		arrow := "▼ "
		if idx < WS.Scroll {
			arrow = "▲ "
		}
		sc.LabelR(x+w-2, y+h-1, false, col, Seg{arrow, HI, false}, Seg{"selection", DIM, false})
	}
}

func heatOrDim(v float64, sel bool) uint32 {
	if v < .05 && !sel {
		return DIM
	}
	return gHeat.At(v / 40)
}

// listWindows prints the window list (shiptop --windows), for checking what the panel will show.
func listWindows() int {
	S.Info = sysInfo()
	update()
	time.Sleep(time.Second)
	update()
	l, err := readWindows()
	if err != nil {
		fmt.Println("windows:", err)
		return 1
	}
	for _, w := range l {
		name, cpu := "?", 0.0
		if p := S.Procs[w.PID]; p != nil {
			name, cpu = p.Name, p.CPU
		}
		state := ""
		if w.Min {
			state = " (minimized)"
		}
		fmt.Printf("%#010x pid %-6d %-22s %5.1f%%  %s%s\n", w.H, w.PID, trunc(name, 22), cpu, trunc(w.Title, 70), state)
	}
	fmt.Printf("%d windows\n", len(l))
	return 0
}

// raiseByTitle brings the first window whose title contains text to the front (shiptop --raise).
func raiseByTitle(text string) int {
	l, err := readWindows()
	if err != nil {
		fmt.Println("windows:", err)
		return 1
	}
	for _, w := range l {
		if containsFold(w.Title, text) {
			how, err := raiseWindow(w.H)
			if err != nil {
				fmt.Printf("could not bring %q to the front: %v\n", w.Title, err)
				return 1
			}
			fmt.Printf("brought %q to the front (%s)\n", w.Title, how)
			return 0
		}
	}
	fmt.Printf("no window title contains %q\n", text)
	return 1
}
