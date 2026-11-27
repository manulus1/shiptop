package main

import (
	"fmt"
	"strings"
)

// Layout editor (key l). Works on a copy of the layout; Enter saves it, Esc throws it away.
//   mouse: drag a panel onto another to swap, onto another's edge to dock it there, onto the
//          screen edge for a full column/row; drag a border to resize; wheel changes the panel
//   keys:  Tab select · arrows/hjkl swap · Shift+arrows/HJKL move an edge · Ctrl+arrows to the
//          screen edge · Space next panel type · a add · x remove · s turn · = equal · r reset

var E struct {
	Tree    *Node
	Default bool // still the built-in layout (saving goes back to it and removes the file)
	Sel     string
	Drag    *edrag
}

type edrag struct {
	div          bool
	d            divider
	off          int
	src          string
	x0, y0, x, y int
	moved        bool
}

type dropT struct{ kind, dst, side string } // kind: swap, dock, edge

func enterEditor() {
	E.Tree = cloneTree(currentTree(L.W, L.H))
	E.Default = L.Root == nil
	E.Drag = nil
	V.Edit, V.Zoom = true, false
	pl := editPlaced()
	E.Sel = focused()
	if _, ok := pl.Rects[E.Sel]; !ok && len(pl.Order) > 0 {
		E.Sel = pl.Order[0]
	}
}

func editPlaced() placed { return layoutOf(E.Tree, layoutArea()) }

func changed() { E.Default = false }

func editSave() {
	V.Edit, E.Drag = false, nil
	var root *Node
	if !E.Default {
		root = E.Tree
	}
	L.Root = root
	path, err := saveLayout(root)
	switch {
	case err != nil:
		say(WARN, "layout in use, but not saved: "+err.Error(), false)
	case root == nil:
		say(OK, "built-in layout restored", false)
	default:
		say(OK, "layout saved to "+path, false)
	}
	if hasPanel(currentTree(L.W, L.H), "windows") {
		refreshWindows()
	}
}

var dirKeys = map[string]string{"up": "up", "down": "down", "left": "left", "right": "right", "k": "up", "j": "down", "h": "left", "l": "right"}
var sideOf = map[string]string{"up": "top", "down": "bottom", "left": "left", "right": "right"}

func editKey(k string) {
	pl := editPlaced()
	switch {
	case k == "esc":
		V.Edit, E.Drag = false, nil
		say(DIM, "layout unchanged", false)
	case k == "enter":
		editSave()
	case k == "tab" || k == "shift-tab":
		if len(pl.Order) > 0 {
			i := 0
			for n, id := range pl.Order {
				if id == E.Sel {
					i = n
				}
			}
			d := 1
			if k == "shift-tab" {
				d = -1
			}
			E.Sel = pl.Order[(i+d+len(pl.Order))%len(pl.Order)]
		}
	case dirKeys[k] != "":
		if n := pl.neighbour(E.Sel, dirKeys[k]); n != "" && swapPanels(E.Tree, E.Sel, n) {
			changed()
		}
	case strings.HasPrefix(k, "shift-") && dirKeys[k[6:]] != "" || len(k) == 1 && strings.Contains("HJKL", k):
		dir := dirKeys[strings.ToLower(strings.TrimPrefix(k, "shift-"))]
		step := 1
		if dir == "left" || dir == "right" {
			step = 2
		}
		if pl.moveEdge(E.Sel, dir, step) {
			changed()
		}
	case strings.HasPrefix(k, "ctrl-") && dirKeys[k[5:]] != "":
		if dockEdge(E.Tree, E.Sel, sideOf[k[5:]]) {
			changed()
		}
	case k == " ":
		cycleType(1)
	case k == "a" || k == "A":
		addPanel(pl)
	case k == "x" || k == "X" || k == "del":
		next := pl.neighbour(E.Sel, "left")
		if removePanel(E.Tree, E.Sel) {
			changed()
			if next == "" {
				next = editPlaced().Order[0]
			}
			E.Sel = next
		} else {
			say(DIM, "the last panel can't be removed", false)
		}
	case k == "s" || k == "S":
		if turnSplit(E.Tree, E.Sel) {
			changed()
		}
	case k == "=":
		if equalize(E.Tree, E.Sel) {
			changed()
		}
	case k == "r" || k == "R":
		E.Tree, E.Default = defaultTree(L.W, L.H), true
		if p := editPlaced(); len(p.Order) > 0 {
			E.Sel = p.Order[0]
		}
	case k == "q" || k == "Q":
		say(DIM, "Enter saves the layout, Esc cancels", false)
	}
}

// cycleType shows the next (d=1) or previous panel type in the selected tile.
func cycleType(d int) {
	n := len(panelDefs)
	i := 0
	for k, p := range panelDefs {
		if p.ID == E.Sel {
			i = k
		}
	}
	next := panelDefs[(i+d+n)%n].ID
	if setPanelType(E.Tree, E.Sel, next) {
		E.Sel = next
		changed()
		if next == "windows" {
			refreshWindows()
		}
	}
}

// addPanel puts the first panel that is not on screen next to the selected one.
func addPanel(pl placed) {
	add := ""
	for _, d := range panelDefs {
		if !hasPanel(E.Tree, d.ID) {
			add = d.ID
			break
		}
	}
	if add == "" {
		say(DIM, "every panel is already in the layout", false)
		return
	}
	leaf, _ := findLeaf(E.Tree, E.Sel)
	r, ok := pl.Rects[E.Sel]
	if leaf == nil || !ok {
		return
	}
	a, b := pdef(E.Sel), pdef(add)
	side := "right" // split along the side that has room for both, the longer one if both do
	fitsLR, fitsTB := r.W >= a.MinW+b.MinW, r.H >= a.MinH+b.MinH
	if fitsTB && (!fitsLR || r.H*2 > r.W) {
		side = "bottom"
	}
	splitLeaf(leaf, add, side, .5)
	E.Sel = add
	changed()
	if add == "windows" {
		refreshWindows()
	}
}

func editMouse(m Mouse) {
	pl := editPlaced()
	switch {
	case m.WheelUp || m.WheelDown:
		if id := pl.at(m.X, m.Y); id != "" {
			E.Sel = id
			if m.WheelUp {
				cycleType(-1)
			} else {
				cycleType(1)
			}
		}
	case m.Press && m.Button == 0:
		if d, off, ok := pl.dividerAt(m.X, m.Y); ok {
			E.Drag = &edrag{div: true, d: d, off: off}
			return
		}
		if id := pl.at(m.X, m.Y); id != "" {
			E.Sel = id
			E.Drag = &edrag{src: id, x0: m.X, y0: m.Y, x: m.X, y: m.Y}
		}
	case m.Drag && E.Drag != nil:
		g := E.Drag
		if g.div {
			pos, origin := m.Y, g.d.R.Y
			if g.d.LR {
				pos, origin = m.X, g.d.R.X
			}
			setDivider(g.d, pos+g.off-origin)
			changed()
			return
		}
		g.x, g.y = m.X, m.Y
		if g.x != g.x0 || g.y != g.y0 {
			g.moved = true
		}
	case m.Release && E.Drag != nil:
		g := E.Drag
		E.Drag = nil
		if g.div || !g.moved {
			return
		}
		t, ok := dropAt(pl, g.src, m.X, m.Y)
		if !ok {
			return
		}
		done := false
		switch t.kind {
		case "swap":
			done = swapPanels(E.Tree, g.src, t.dst)
		case "dock":
			done = dockPanel(E.Tree, g.src, t.dst, t.side)
		case "edge":
			done = dockEdge(E.Tree, g.src, t.side)
		}
		if done {
			E.Sel = g.src
			changed()
		}
	}
}

// dropAt says what releasing a dragged panel at (x, y) does: on the outermost row or column it goes
// to that screen edge, near another panel's edge it docks there, in another panel's middle they swap.
func dropAt(pl placed, src string, x, y int) (dropT, bool) {
	switch {
	case x <= 0:
		return dropT{"edge", "", "left"}, true
	case x >= L.W-1:
		return dropT{"edge", "", "right"}, true
	case y <= 1:
		return dropT{"edge", "", "top"}, true
	case y >= L.H-1:
		return dropT{"edge", "", "bottom"}, true
	}
	id := pl.at(x, y)
	if id == "" || id == src {
		return dropT{}, false
	}
	r := pl.Rects[id]
	fx := (float64(x-r.X) + .5) / float64(r.W)
	fy := (float64(y-r.Y) + .5) / float64(r.H)
	side, best := "", .25
	for _, c := range []struct {
		s string
		d float64
	}{{"left", fx}, {"right", 1 - fx}, {"top", fy}, {"bottom", 1 - fy}} {
		if c.d < best {
			side, best = c.s, c.d
		}
	}
	if side == "" {
		return dropT{"swap", id, ""}, true
	}
	return dropT{"dock", id, side}, true
}

const (
	edFrame = 0x5d6b85
	edSel   = 0xffd166
	edDrop  = 0x3ee39b
	edTint  = 0x163526
	edTag   = 0x10141b
)

func drawEditor(sc *Screen) {
	W, H := sc.W, sc.H
	pl := editPlaced()
	L.Last = pl
	drawPanels(sc, pl)
	for i := W; i < len(sc.C); i++ { // dim the live content so the frames stand out
		c := &sc.C[i]
		c.FG, c.BG, c.B = lerp(c.FG, BG, .62), lerp(c.BG, BG, .6), false
	}
	g := E.Drag
	var drop dropT
	dropOK := false
	if g != nil && !g.div && g.moved {
		drop, dropOK = dropAt(pl, g.src, g.x, g.y)
	}
	if dropOK { // preview where the panel goes
		var r Rect
		switch drop.kind {
		case "edge":
			r = Rect{0, 1, W, H - 1}
			r = sideRect(r, drop.side, .3)
		case "dock":
			r = sideRect(pl.Rects[drop.dst], drop.side, .5)
		case "swap":
			r = pl.Rects[drop.dst]
		}
		for y := r.Y; y < r.Y+r.H; y++ {
			sc.SetBG(r.X, y, r.W, edTint)
		}
	}
	for _, id := range pl.Order {
		r := pl.Rects[id]
		switch {
		case dropOK && drop.kind == "swap" && id == drop.dst:
			sc.Box2(r.X, r.Y, r.W, r.H, edDrop)
		case id == E.Sel:
			sc.Box2(r.X, r.Y, r.W, r.H, edSel)
		default:
			sc.Box(r.X, r.Y, r.W, r.H, edFrame)
		}
		lines := []string{fmt.Sprintf("%d×%d", r.W, r.H)}
		if id == E.Sel {
			lines = append(lines, "←↑↓→ swap · Shift+←↑↓→ size", "Ctrl+←↑↓→ to screen edge", "Space/wheel: other panel")
		}
		drawTag(sc, r, pdef(id).Name, lines, id == E.Sel)
	}
	if g != nil && g.div { // the border being dragged
		d := g.d
		b := d.boundary()
		for i := 0; i < 2; i++ {
			if d.LR {
				for y := d.R.Y; y < d.R.Y+d.R.H; y++ {
					sc.PutX(b-1+i, y, '┃', edSel, BG, true)
				}
			} else {
				for x := d.R.X; x < d.R.X+d.R.W; x++ {
					sc.PutX(x, b-1+i, '━', edSel, BG, true)
				}
			}
		}
	}
	if g != nil && !g.div && g.moved { // the panel under the pointer
		t := " " + pdef(g.src).Name + " "
		sc.Fill(clampi(g.x-rlen(t)/2, 0, maxi(0, W-rlen(t))), g.y, t, BG, edSel, true)
	}
	// the bar on top
	for x := 0; x < W; x++ {
		sc.PutX(x, 0, ' ', FG, 0x1a1f29, false)
	}
	x := sc.Fill(0, 0, " layout editor ", BG, edSel, true) + 1
	right := " Enter save · Esc cancel "
	msg := "drag a panel to move it · drag a border to resize · a add · x remove · s turn · = equal · r reset"
	switch {
	case dropOK && drop.kind == "swap":
		msg = fmt.Sprintf("release: swap %s and %s", g.src, drop.dst)
	case dropOK && drop.kind == "dock":
		where := map[string]string{"left": "left of", "right": "right of", "top": "above", "bottom": "below"}[drop.side]
		msg = fmt.Sprintf("release: put %s %s %s", g.src, where, drop.dst)
	case dropOK && drop.kind == "edge":
		msg = fmt.Sprintf("release: %s along the %s edge of the screen", g.src, drop.side)
	case g != nil && !g.div && g.moved:
		msg = "drop on a panel to swap · on its edge to dock · on the screen edge for a full row or column"
	case len(pl.Hidden) > 0:
		msg = "no room for " + strings.Join(pl.Hidden, ", ") + ": make the window bigger or other panels smaller · " + msg
	case E.Default:
		msg = "built-in layout · " + msg
	}
	sc.Str(x, 0, trunc(msg, W-x-rlen(right)-1), FG, 0)
	sc.Fill(W-rlen(right), 0, right, TITLE, 0x1a1f29, false)
}

// sideRect is the part of r on one side, taking frac of it.
func sideRect(r Rect, side string, frac float64) Rect {
	switch side {
	case "left":
		r.W = maxi(1, int(float64(r.W)*frac))
	case "right":
		w := maxi(1, int(float64(r.W)*frac))
		r.X, r.W = r.X+r.W-w, w
	case "top":
		r.H = maxi(1, int(float64(r.H)*frac))
	case "bottom":
		h := maxi(1, int(float64(r.H)*frac))
		r.Y, r.H = r.Y+r.H-h, h
	}
	return r
}

// drawTag writes the panel name (and a few lines) on a small plate in the middle of its tile.
func drawTag(sc *Screen, r Rect, name string, lines []string, sel bool) {
	maxW := r.W - 4
	if maxW < 3 || r.H < 3 {
		return
	}
	rows := mini(1+len(lines), r.H-2)
	w := rlen(name)
	for _, l := range lines[:rows-1] {
		w = maxi(w, rlen(l))
	}
	w = mini(w+2, maxW)
	x0, y0 := r.X+(r.W-w)/2, r.Y+(r.H-rows)/2
	fg := uint32(FG)
	if sel {
		fg = edSel
	}
	for y := y0; y < y0+rows; y++ {
		for x := x0; x < x0+w; x++ {
			sc.PutX(x, y, ' ', FG, edTag, false)
		}
	}
	n := trunc(name, w-2)
	sc.Bold(x0+(w-rlen(n))/2, y0, n, fg, 0)
	for i, l := range lines[:rows-1] {
		l = trunc(l, w-2)
		c := uint32(DIM)
		if i == 0 {
			c = FG
		}
		sc.Str(x0+(w-rlen(l))/2, y0+1+i, l, c, 0)
	}
}
