package main

import (
	"fmt"
	"time"
)

// L is the layout in use and what the last frame put where (for mouse hits and keyboard moves).
var L struct {
	Root  *Node // saved custom layout; nil = the built-in one, which adapts to the window size
	Last  placed
	W, H  int
	Focus string // list panel that gets the arrow keys: "proc" or "windows"
}

func currentTree(w, h int) *Node {
	if L.Root != nil {
		return L.Root
	}
	return defaultTree(w, h)
}

func layoutArea() Rect { return Rect{0, 1, L.W, L.H - 1} }

func draw(sc *Screen) {
	sc.Clear()
	W, H := sc.W, sc.H
	L.W, L.H = W, H
	hitProc, hitWin = rowHit{}, rowHit{}
	L.Last = placed{Rects: map[string]Rect{}}
	if W < 80 || H < 24 {
		m := fmt.Sprintf("shiptop needs at least 80×24 · this window is %d×%d", W, H)
		sc.Str(maxi(0, (W-rlen(m))/2), H/2, m, WARN, W)
		sc.Str(maxi(0, (W-22)/2), H/2+1, "make the window bigger", DIM, W)
		return
	}
	if V.Matrix {
		drawMatrix(sc)
		return
	}
	if V.Edit {
		drawEditor(sc)
		return
	}
	drawHeader(sc)
	if V.Zoom {
		L.Last = layoutOf(&Node{Panel: "notebook"}, layoutArea())
	} else {
		L.Last = layoutOf(currentTree(W, H), layoutArea())
	}
	drawPanels(sc, L.Last)
}

func drawPanels(sc *Screen, pl placed) {
	for _, id := range pl.Order {
		r := pl.Rects[id]
		if d := pdef(id); d != nil {
			sc.Clip(r.X, r.Y, r.W, r.H)
			d.Draw(sc, r.X, r.Y, r.W, r.H)
		}
	}
	sc.NoClip()
}

// visibleLists are the list panels on screen, in catalog order.
func visibleLists() []string {
	var out []string
	for _, d := range panelDefs {
		if _, ok := L.Last.Rects[d.ID]; ok && d.List {
			out = append(out, d.ID)
		}
	}
	return out
}

// focused is the list panel that gets the arrow keys.
func focused() string {
	ls := visibleLists()
	for _, id := range ls {
		if id == L.Focus {
			return id
		}
	}
	if len(ls) > 0 {
		return ls[0]
	}
	return ""
}

func cycleFocus(d int) {
	ls := visibleLists()
	if len(ls) < 2 {
		return
	}
	i := 0
	for k, id := range ls {
		if id == focused() {
			i = k
		}
	}
	L.Focus = ls[(i+d+len(ls))%len(ls)]
	flash(L.Focus)
}

func boxCol(base uint32, key string) uint32 {
	if time.Now().Before(S.Flash[key]) {
		return lerp(base, 0xffffff, .45)
	}
	if (key == "proc" || key == "windows") && focused() == key && len(visibleLists()) > 1 {
		return lerp(base, 0xffffff, .28) // the focused list stands out when there are two
	}
	return base
}

var lastClick struct {
	t    time.Time
	x, y int
}

// handleMouse: click selects a row (and focuses its list), double click on a window brings it to
// the front, the wheel scrolls lists and turns the notebook.
func handleMouse(m Mouse) {
	if V.Edit {
		editMouse(m)
		return
	}
	if V.Matrix {
		return
	}
	id := L.Last.at(m.X, m.Y)
	switch {
	case m.WheelUp || m.WheelDown:
		d := 3
		if m.WheelUp {
			d = -3
		}
		switch id {
		case "proc":
			S.Scroll = maxi(0, S.Scroll+d)
		case "windows":
			WS.Scroll = maxi(0, WS.Scroll+d)
		case "notebook":
			V.Yaw += float64(d) * .06
			V.Hold = time.Now().Add(3 * time.Second)
		}
	case m.Press && m.Button == 0:
		dbl := time.Since(lastClick.t) < 450*time.Millisecond && lastClick.x == m.X && lastClick.y == m.Y
		lastClick.t, lastClick.x, lastClick.y = time.Now(), m.X, m.Y
		if d := pdef(id); d != nil && d.List {
			L.Focus = id
		}
		if pid, ok := hitProc.at(m.X, m.Y); ok {
			if S.Sel != int(pid) {
				S.Confirm = 0
			}
			S.Sel, S.Follow = int(pid), true
		}
		if h, ok := hitWin.at(m.X, m.Y); ok {
			WS.Sel = h
			if dbl {
				raiseSelected()
			}
		}
	}
}
