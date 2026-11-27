package main

import (
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

// oldRects is the fixed layout shiptop drew before layouts existed (v5.2.0's draw()).
func oldRects(W, H int) map[string]Rect {
	avail := H - 1
	topH := maxi(10, int(math.Round(float64(avail)*.42)))
	nbW := 0
	if W >= 100 {
		nbW = clampi(int(float64(W)*.34), 38, 76)
	}
	m := map[string]Rect{"cpu": {0, 1, W - nbW, topH}}
	if nbW > 0 {
		m["notebook"] = Rect{W - nbW, 1, nbW, topH}
	}
	y0 := 1 + topH
	bh := H - y0
	lw := clampi(int(float64(W)*.44), 40, W-40)
	memH := bh * 52 / 100
	m["mem"] = Rect{0, y0, lw, memH}
	m["net"] = Rect{0, y0 + memH, lw, bh - memH}
	m["proc"] = Rect{lw, y0, W - lw, bh}
	return m
}

func TestDefaultLayoutUnchanged(t *testing.T) {
	for W := 80; W <= 320; W += 3 {
		for H := 24; H <= 90; H += 2 {
			got := layoutOf(defaultTree(W, H), Rect{0, 1, W, H - 1}).Rects
			want := oldRects(W, H)
			if len(got) != len(want) {
				t.Fatalf("%dx%d: %d panels, want %d", W, H, len(got), len(want))
			}
			for id, r := range want {
				if got[id] != r {
					t.Fatalf("%dx%d: %s at %+v, v5.2.0 drew it at %+v", W, H, id, got[id], r)
				}
			}
		}
	}
}

// checkCover: visible tiles stay inside the area, never overlap, and fill it completely.
func checkCover(t *testing.T, pl placed, area Rect, what string) {
	t.Helper()
	owner := make([]string, area.W*area.H)
	for id, r := range pl.Rects {
		if r.W <= 0 || r.H <= 0 || r.X < area.X || r.Y < area.Y || r.X+r.W > area.X+area.W || r.Y+r.H > area.Y+area.H {
			t.Fatalf("%s: %s has rect %+v outside %+v", what, id, r, area)
		}
		for y := r.Y; y < r.Y+r.H; y++ {
			for x := r.X; x < r.X+r.W; x++ {
				i := (y-area.Y)*area.W + x - area.X
				if owner[i] != "" {
					t.Fatalf("%s: %s and %s overlap at %d,%d", what, owner[i], id, x, y)
				}
				owner[i] = id
			}
		}
	}
	for i, o := range owner {
		if o == "" {
			t.Fatalf("%s: cell %d,%d belongs to no panel", what, area.X+i%area.W, area.Y+i/area.W)
		}
	}
}

func randomTree(rng *rand.Rand) *Node {
	ids := []string{}
	for _, d := range panelDefs {
		ids = append(ids, d.ID)
	}
	rng.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
	n := 1 + rng.Intn(len(ids))
	root := &Node{Panel: ids[0]}
	for _, id := range ids[1:n] {
		ls := leavesOf(root)
		sides := []string{"left", "right", "top", "bottom"}
		splitLeaf(ls[rng.Intn(len(ls))], id, sides[rng.Intn(4)], .1+.8*rng.Float64())
	}
	return root
}

func TestLayoutOps(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	sides := []string{"left", "right", "top", "bottom"}
	dirs := []string{"left", "right", "up", "down"}
	for round := 0; round < 400; round++ {
		root := randomTree(rng)
		W, H := 80+rng.Intn(200), 24+rng.Intn(60)
		area := Rect{0, 1, W, H - 1}
		for step := 0; step < 40; step++ {
			pl := layoutOf(root, area)
			ls := leavesOf(root)
			pick := func() string { return ls[rng.Intn(len(ls))].Panel }
			before := len(ls)
			switch rng.Intn(10) {
			case 0:
				swapPanels(root, pick(), pick())
			case 1:
				dockPanel(root, pick(), pick(), sides[rng.Intn(4)])
			case 2:
				dockEdge(root, pick(), sides[rng.Intn(4)])
			case 3:
				if removePanel(root, pick()) {
					before--
				}
			case 4:
				setPanelType(root, pick(), panelDefs[rng.Intn(len(panelDefs))].ID)
				before = len(leavesOf(root)) // a new type keeps the count, a swap too
			case 5:
				turnSplit(root, pick())
			case 6:
				if len(pl.Order) > 0 {
					pl.moveEdge(pl.Order[rng.Intn(len(pl.Order))], dirs[rng.Intn(4)], 1+rng.Intn(9))
				}
			case 7:
				if len(pl.Divs) > 0 {
					d := pl.Divs[rng.Intn(len(pl.Divs))]
					setDivider(d, rng.Intn(300)-50)
				}
			case 8:
				equalize(root, pick())
			case 9:
				for _, d := range panelDefs {
					if !hasPanel(root, d.ID) {
						splitLeaf(ls[rng.Intn(len(ls))], d.ID, sides[rng.Intn(4)], .5)
						before++
						break
					}
				}
			}
			if got := len(leavesOf(root)); got != before {
				t.Fatalf("round %d step %d: %d panels, want %d", round, step, got, before)
			}
			check := cloneTree(root)
			if err := checkTree(check, map[string]bool{}, 0); err != nil {
				t.Fatalf("round %d step %d: tree broken: %v", round, step, err)
			}
			pl = layoutOf(root, area)
			checkCover(t, pl, area, "ops")
			if len(pl.Rects)+len(pl.Hidden) != len(leavesOf(root)) {
				t.Fatalf("round %d: %d shown + %d hidden != %d panels", round, len(pl.Rects), len(pl.Hidden), len(leavesOf(root)))
			}
		}
	}
}

func TestMinSizesRespected(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	for round := 0; round < 2000; round++ {
		root := randomTree(rng)
		W, H := 80+rng.Intn(200), 24+rng.Intn(60)
		pl := layoutOf(root, Rect{0, 1, W, H - 1})
		for _, d := range pl.Divs { // every split that shows both sides gives each side its minimum
			total := d.R.H
			if d.LR {
				total = d.R.W
			}
			minA, minB := minSize(d.N.A, d.LR), minSize(d.N.B, d.LR)
			if total >= minA+minB && (d.A < minA || total-d.A < minB) {
				t.Fatalf("split of %d cells gives %d/%d, minimums %d/%d", total, d.A, total-d.A, minA, minB)
			}
		}
	}
}

func TestMoveEdge(t *testing.T) {
	root := &Node{Split: "lr", Ratio: .5, A: &Node{Panel: "cpu"}, B: &Node{Panel: "proc"}}
	area := Rect{0, 1, 120, 30}
	pl := layoutOf(root, area)
	if !pl.moveEdge("cpu", "right", 10) { // cpu's right edge exists: cpu grows
		t.Fatal("no edge to move")
	}
	if w := layoutOf(root, area).Rects["cpu"].W; w != 70 {
		t.Fatalf("cpu is %d wide, want 70", w)
	}
	pl = layoutOf(root, area)
	pl.moveEdge("proc", "right", 10) // proc has no right edge: its left edge moves right, proc shrinks
	if w := layoutOf(root, area).Rects["proc"].W; w != 40 {
		t.Fatalf("proc is %d wide, want 40", w)
	}
	pl = layoutOf(root, area)
	pl.moveEdge("proc", "right", 50) // stops at proc's minimum width
	if w := layoutOf(root, area).Rects["proc"].W; w != pdef("proc").MinW {
		t.Fatalf("proc is %d wide, want its minimum %d", w, pdef("proc").MinW)
	}
}

func TestNeighbourAndDrop(t *testing.T) {
	L.W, L.H = 140, 38
	pl := layoutOf(defaultTree(140, 38), layoutArea())
	if n := pl.neighbour("mem", "right"); n != "proc" {
		t.Fatalf("right of mem is %q", n)
	}
	if n := pl.neighbour("net", "up"); n != "mem" {
		t.Fatalf("above net is %q", n)
	}
	if n := pl.neighbour("cpu", "up"); n != "" {
		t.Fatalf("above cpu is %q", n)
	}
	r := pl.Rects["proc"]
	cases := []struct {
		x, y int
		want dropT
	}{
		{r.X + r.W/2, r.Y + r.H/2, dropT{"swap", "proc", ""}},
		{r.X + 2, r.Y + r.H/2, dropT{"dock", "proc", "left"}},
		{r.X + r.W - 3, r.Y + r.H/2, dropT{"dock", "proc", "right"}},
		{r.X + r.W/2, r.Y + r.H - 2, dropT{"dock", "proc", "bottom"}},
		{0, 20, dropT{"edge", "", "left"}},
		{139, 20, dropT{"edge", "", "right"}},
		{50, 1, dropT{"edge", "", "top"}},
		{50, 37, dropT{"edge", "", "bottom"}},
	}
	for _, c := range cases {
		if got, ok := dropAt(pl, "net", c.x, c.y); !ok || got != c.want {
			t.Fatalf("drop at %d,%d = %+v %v, want %+v", c.x, c.y, got, ok, c.want)
		}
	}
	if _, ok := dropAt(pl, "proc", r.X+r.W/2, r.Y+r.H/2); ok {
		t.Fatal("dropping a panel on itself should do nothing")
	}
}

func TestDividerDrag(t *testing.T) {
	L.W, L.H = 140, 38
	root := defaultTree(140, 38)
	pl := layoutOf(root, layoutArea())
	mem, proc := pl.Rects["mem"], pl.Rects["proc"]
	d, off, ok := pl.dividerAt(mem.X+mem.W-1, mem.Y+3) // mem's right border column
	if !ok || !d.LR || d.boundary() != proc.X {
		t.Fatalf("no vertical divider between mem and proc: %+v %v", d, ok)
	}
	setDivider(d, 50+off-d.R.X) // drag the border to column 50
	if got := layoutOf(root, layoutArea()).Rects["proc"].X; got != 51 {
		t.Fatalf("proc starts at %d after the drag, want 51", got)
	}
}

func TestSaveLoad(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("AppData", dir)
	if root, note := loadLayout(); root != nil || note != "" {
		t.Fatalf("no file: got %v %q", root, note)
	}
	want := defaultTree(150, 40)
	dockEdge(want, "net", "top")
	splitLeaf(leavesOf(want)[0], "windows", "right", .4)
	path, err := saveLayout(want)
	if err != nil {
		t.Fatal(err)
	}
	got, note := loadLayout()
	if got == nil || note != "" {
		t.Fatalf("load after save: %v %q", got, note)
	}
	a, b := layoutOf(want, Rect{0, 1, 150, 39}), layoutOf(got, Rect{0, 1, 150, 39})
	for id, r := range a.Rects {
		if b.Rects[id] != r {
			t.Fatalf("%s: saved %+v, loaded %+v", id, r, b.Rects[id])
		}
	}
	for _, bad := range []string{
		`{`,
		`{"version":1,"layout":{"panel":"gpu"}}`,
		`{"version":1,"layout":{"split":"lr","ratio":0.5,"a":{"panel":"cpu"},"b":{"panel":"cpu"}}}`,
		`{"version":1,"layout":{"split":"xy","ratio":0.5,"a":{"panel":"cpu"},"b":{"panel":"mem"}}}`,
		`{"version":1,"layout":{"split":"lr","ratio":0.5,"a":{"panel":"cpu"}}}`,
		`{"version":1}`,
	} {
		os.WriteFile(path, []byte(bad), 0o644)
		if root, note := loadLayout(); root != nil || note == "" {
			t.Fatalf("bad file %s: got %v %q", bad, root, note)
		}
	}
	os.WriteFile(path, []byte(`{"version":1,"layout":{"split":"lr","ratio":7,"a":{"panel":"cpu"},"b":{"panel":"mem"}}}`), 0o644)
	if root, _ := loadLayout(); root == nil || root.Ratio != .95 {
		t.Fatalf("ratio out of range should be clamped: %+v", root)
	}
	if _, err := saveLayout(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("reset should remove the file")
	}
	if filepath.Base(filepath.Dir(path)) != "shiptop" {
		t.Fatalf("unexpected path %s", path)
	}
}
