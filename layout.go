package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
)

// Panel layout: a binary split tree. A leaf holds one panel; an inner node splits its rectangle
// left|right ("lr") or top/bottom ("tb") and gives Ratio of it to the first child. Every tile is
// drawn inside its own clip rectangle, so a panel can never paint over its neighbours.

type Node struct {
	Panel string  `json:"panel,omitempty"`
	Split string  `json:"split,omitempty"` // "lr": A left of B · "tb": A above B
	Ratio float64 `json:"ratio,omitempty"` // share of the first child, 0..1
	A     *Node   `json:"a,omitempty"`
	B     *Node   `json:"b,omitempty"`
}

type Rect struct{ X, Y, W, H int }

func (r Rect) has(x, y int) bool { return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H }

type panelDef struct {
	ID, Name   string
	MinW, MinH int
	Prio       int // when a split is too small for both sides, the side with the higher priority stays
	Draw       func(sc *Screen, x, y, w, h int)
	List       bool // has a selectable list: Tab and clicks move the keyboard focus between these
}

var panelDefs []panelDef

func init() {
	panelDefs = []panelDef{
		{"cpu", "cpu", 40, 8, 6, drawCPU, false},
		{"mem", "mem", 30, 5, 4, drawMem, false},
		{"net", "net", 30, 5, 3, drawNet, false},
		{"proc", "proc", 36, 6, 7, drawProc, true},
		{"notebook", "notebook", 30, 8, 2, drawNB, false},
		{"windows", "windows", 40, 5, 5, drawWindows, true},
		{"matrix", "matrix", 12, 4, 1, drawMatrixPanel, false},
	}
}

func pdef(id string) *panelDef {
	for i := range panelDefs {
		if panelDefs[i].ID == id {
			return &panelDefs[i]
		}
	}
	return nil
}

// defaultTree is the built-in layout for a w×h screen: cpu and notebook on top, mem and net left,
// proc right. The ratios reproduce the sizes shiptop has always used; below 100 columns the
// notebook is left out so the cpu panel keeps its room.
func defaultTree(w, h int) *Node {
	avail := h - 1
	topH := maxi(10, int(math.Round(float64(avail)*.42)))
	lw := clampi(int(float64(w)*.44), 40, w-40)
	bh := avail - topH
	memH := bh * 52 / 100
	ratio := func(a, total int, dflt float64) float64 {
		if total <= 0 || a <= 0 || a >= total {
			return dflt
		}
		return float64(a) / float64(total)
	}
	top := &Node{Panel: "cpu"}
	if w >= 100 {
		nbW := clampi(int(float64(w)*.34), 38, 76)
		top = &Node{Split: "lr", Ratio: ratio(w-nbW, w, .66), A: &Node{Panel: "cpu"}, B: &Node{Panel: "notebook"}}
	}
	left := &Node{Split: "tb", Ratio: ratio(memH, bh, .52), A: &Node{Panel: "mem"}, B: &Node{Panel: "net"}}
	bottom := &Node{Split: "lr", Ratio: ratio(lw, w, .44), A: left, B: &Node{Panel: "proc"}}
	return &Node{Split: "tb", Ratio: ratio(topH, avail, .42), A: top, B: bottom}
}

// ---- geometry ----

type divider struct {
	N  *Node
	R  Rect // the split node's rectangle
	A  int  // size of the first child along the split axis
	LR bool
}

// boundary is the first cell of the second child (the divider lies between boundary-1 and boundary).
func (d divider) boundary() int {
	if d.LR {
		return d.R.X + d.A
	}
	return d.R.Y + d.A
}

type placed struct {
	Rects  map[string]Rect
	Order  []string // visible panels in reading order
	Divs   []divider
	Hidden []string // panels that got no room
}

func layoutOf(root *Node, r Rect) placed {
	p := placed{Rects: map[string]Rect{}}
	p.place(root, r)
	for id := range p.Rects {
		p.Order = append(p.Order, id)
	}
	sort.Slice(p.Order, func(i, j int) bool {
		a, b := p.Rects[p.Order[i]], p.Rects[p.Order[j]]
		if a.Y != b.Y {
			return a.Y < b.Y
		}
		return a.X < b.X
	})
	return p
}

func (p *placed) place(n *Node, r Rect) {
	if n == nil {
		return
	}
	if r.W <= 0 || r.H <= 0 {
		for _, l := range leavesOf(n) {
			p.Hidden = append(p.Hidden, l.Panel)
		}
		return
	}
	if n.Panel != "" {
		p.Rects[n.Panel] = r
		return
	}
	lr := n.Split == "lr"
	total := r.H
	if lr {
		total = r.W
	}
	a := splitAt(n, total)
	ra, rb := r, r
	if lr {
		ra.W, rb.X, rb.W = a, r.X+a, r.W-a
	} else {
		ra.H, rb.Y, rb.H = a, r.Y+a, r.H-a
	}
	if a > 0 && a < total {
		p.Divs = append(p.Divs, divider{n, r, a, lr})
	}
	p.place(n.A, ra)
	p.place(n.B, rb)
}

// splitAt is how many cells the first child of n gets out of total.
func splitAt(n *Node, total int) int {
	lr := n.Split == "lr"
	minA, minB := minSize(n.A, lr), minSize(n.B, lr)
	if total >= minA+minB {
		return clampi(int(math.Round(n.Ratio*float64(total))), minA, total-minB)
	}
	if prio(n.A) >= prio(n.B) { // not enough room for both: the more important side takes all
		return total
	}
	return 0
}

func minSize(n *Node, lr bool) int {
	if n == nil {
		return 0
	}
	if n.Panel != "" {
		d := pdef(n.Panel)
		if d == nil {
			return 1
		}
		if lr {
			return d.MinW
		}
		return d.MinH
	}
	a, b := minSize(n.A, lr), minSize(n.B, lr)
	if (n.Split == "lr") == lr {
		return a + b
	}
	return maxi(a, b)
}

func prio(n *Node) int {
	best := 0
	for _, l := range leavesOf(n) {
		if d := pdef(l.Panel); d != nil && d.Prio > best {
			best = d.Prio
		}
	}
	return best
}

func (p placed) at(x, y int) string {
	for _, id := range p.Order {
		if p.Rects[id].has(x, y) {
			return id
		}
	}
	return ""
}

// dividerAt finds the split border under (x, y): both border columns (or rows) of the two tiles
// count. off is where inside the border the pointer is, so a drag keeps the border under it.
func (p placed) dividerAt(x, y int) (divider, int, bool) {
	for i := len(p.Divs) - 1; i >= 0; i-- { // innermost splits were appended last
		d := p.Divs[i]
		b := d.boundary()
		if d.LR && (x == b-1 || x == b) && y >= d.R.Y && y < d.R.Y+d.R.H {
			return d, b - x, true
		}
		if !d.LR && (y == b-1 || y == b) && x >= d.R.X && x < d.R.X+d.R.W {
			return d, b - y, true
		}
	}
	return divider{}, 0, false
}

// setDivider moves a split so that its first child gets a cells (kept within the panels' minimum sizes).
func setDivider(d divider, a int) {
	total := d.R.H
	if d.LR {
		total = d.R.W
	}
	if total <= 0 {
		return
	}
	minA, minB := minSize(d.N.A, d.LR), minSize(d.N.B, d.LR)
	if total >= minA+minB {
		a = clampi(a, minA, total-minB)
	} else {
		a = clampi(a, 1, total-1)
	}
	d.N.Ratio = float64(a) / float64(total)
}

// edgeDivider finds the divider that forms the given edge ("left", "right", "top", "bottom") of a tile.
func (p placed) edgeDivider(id, edge string) (divider, bool) {
	r, ok := p.Rects[id]
	if !ok {
		return divider{}, false
	}
	for _, d := range p.Divs {
		b := d.boundary()
		switch {
		case d.LR && (edge == "right" && b == r.X+r.W || edge == "left" && b == r.X) && r.Y >= d.R.Y && r.Y+r.H <= d.R.Y+d.R.H:
			return d, true
		case !d.LR && (edge == "bottom" && b == r.Y+r.H || edge == "top" && b == r.Y) && r.X >= d.R.X && r.X+r.W <= d.R.X+d.R.W:
			return d, true
		}
	}
	return divider{}, false
}

// moveEdge moves one edge of a tile by step cells in direction dir: the edge on that side if there
// is one (the tile grows), otherwise the opposite edge (the tile shrinks), like tmux resize-pane.
func (p placed) moveEdge(id, dir string, step int) bool {
	opposite := map[string]string{"left": "right", "right": "left", "up": "bottom", "down": "top"}
	edge := map[string]string{"left": "left", "right": "right", "up": "top", "down": "bottom"}[dir]
	d, ok := p.edgeDivider(id, edge)
	if !ok {
		if d, ok = p.edgeDivider(id, opposite[dir]); !ok {
			return false
		}
	}
	if dir == "left" || dir == "up" {
		step = -step
	}
	setDivider(d, d.A+step)
	return true
}

// neighbour is the tile next to id in direction dir (the one sharing the longest border).
func (p placed) neighbour(id, dir string) string {
	r, ok := p.Rects[id]
	if !ok {
		return ""
	}
	best, bestScore := "", math.Inf(-1)
	for _, o := range p.Order {
		if o == id {
			continue
		}
		q := p.Rects[o]
		var gap, overlap int
		switch dir {
		case "right":
			gap, overlap = q.X-(r.X+r.W), span(r.Y, r.H, q.Y, q.H)
		case "left":
			gap, overlap = r.X-(q.X+q.W), span(r.Y, r.H, q.Y, q.H)
		case "down":
			gap, overlap = q.Y-(r.Y+r.H), span(r.X, r.W, q.X, q.W)
		case "up":
			gap, overlap = r.Y-(q.Y+q.H), span(r.X, r.W, q.X, q.W)
		}
		if gap < 0 || overlap <= 0 {
			continue
		}
		if score := float64(overlap) - 1000*float64(gap); score > bestScore {
			best, bestScore = o, score
		}
	}
	return best
}

func span(a, al, b, bl int) int { return mini(a+al, b+bl) - maxi(a, b) }

// ---- tree operations (all work in place on a tree the caller owns) ----

func cloneTree(n *Node) *Node {
	if n == nil {
		return nil
	}
	c := *n
	c.A, c.B = cloneTree(n.A), cloneTree(n.B)
	return &c
}

func leavesOf(n *Node) []*Node {
	if n == nil {
		return nil
	}
	if n.Panel != "" {
		return []*Node{n}
	}
	return append(leavesOf(n.A), leavesOf(n.B)...)
}

func findLeaf(n *Node, id string) (leaf, parent *Node) {
	if n == nil {
		return nil, nil
	}
	if n.Panel == id {
		return n, nil
	}
	for _, c := range []*Node{n.A, n.B} {
		if c == nil {
			continue
		}
		if c.Panel == id {
			return c, n
		}
		if l, p := findLeaf(c, id); l != nil {
			return l, p
		}
	}
	return nil, nil
}

func hasPanel(root *Node, id string) bool { l, _ := findLeaf(root, id); return l != nil }

// removePanel takes a panel out; its sibling takes over the space. The last panel can't be removed.
func removePanel(root *Node, id string) bool {
	leaf, parent := findLeaf(root, id)
	if leaf == nil || parent == nil {
		return false
	}
	sib := parent.A
	if sib == leaf {
		sib = parent.B
	}
	*parent = *sib
	return true
}

// splitLeaf turns a leaf into a split holding its panel and the new one on the given side;
// share is the part of the space the new panel gets.
func splitLeaf(leaf *Node, add, side string, share float64) {
	old, nw := &Node{Panel: leaf.Panel}, &Node{Panel: add}
	switch side {
	case "left":
		*leaf = Node{Split: "lr", Ratio: share, A: nw, B: old}
	case "right":
		*leaf = Node{Split: "lr", Ratio: 1 - share, A: old, B: nw}
	case "top":
		*leaf = Node{Split: "tb", Ratio: share, A: nw, B: old}
	default:
		*leaf = Node{Split: "tb", Ratio: 1 - share, A: old, B: nw}
	}
}

// dockPanel moves src next to dst, on the given side of it, splitting dst's tile.
func dockPanel(root *Node, src, dst, side string) bool {
	if src == dst || !hasPanel(root, dst) || !removePanel(root, src) {
		return false
	}
	leaf, _ := findLeaf(root, dst)
	splitLeaf(leaf, src, side, .5)
	return true
}

// dockEdge moves src to one edge of the whole screen as a full-height column or full-width row.
func dockEdge(root *Node, src, side string) bool {
	if root.Panel != "" || !removePanel(root, src) {
		return false
	}
	rest := *root
	nw := &Node{Panel: src}
	switch side {
	case "left":
		*root = Node{Split: "lr", Ratio: .3, A: nw, B: &rest}
	case "right":
		*root = Node{Split: "lr", Ratio: .7, A: &rest, B: nw}
	case "top":
		*root = Node{Split: "tb", Ratio: .3, A: nw, B: &rest}
	default:
		*root = Node{Split: "tb", Ratio: .7, A: &rest, B: nw}
	}
	return true
}

func swapPanels(root *Node, a, b string) bool {
	la, _ := findLeaf(root, a)
	lb, _ := findLeaf(root, b)
	if la == nil || lb == nil || la == lb {
		return false
	}
	la.Panel, lb.Panel = lb.Panel, la.Panel
	return true
}

// setPanelType shows typ in id's tile; if typ is already on screen the two panels swap places.
func setPanelType(root *Node, id, typ string) bool {
	if id == typ || pdef(typ) == nil {
		return false
	}
	if hasPanel(root, typ) {
		return swapPanels(root, id, typ)
	}
	leaf, _ := findLeaf(root, id)
	if leaf == nil {
		return false
	}
	leaf.Panel = typ
	return true
}

// turnSplit flips the split that holds id between side by side and stacked.
func turnSplit(root *Node, id string) bool {
	_, parent := findLeaf(root, id)
	if parent == nil {
		return false
	}
	if parent.Split == "lr" {
		parent.Split = "tb"
	} else {
		parent.Split = "lr"
	}
	return true
}

func equalize(root *Node, id string) bool {
	_, parent := findLeaf(root, id)
	if parent == nil {
		return false
	}
	parent.Ratio = .5
	return true
}

// ---- saving ----

type layoutFile struct {
	Version int   `json:"version"`
	Layout  *Node `json:"layout"`
}

// layoutPath: %AppData%\shiptop\layout.json on Windows, ~/.config/shiptop/layout.json on Linux.
func layoutPath() (string, error) {
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "shiptop", "layout.json"), nil
}

// checkTree makes sure a tree read from disk is something shiptop can draw; ratios are clamped.
func checkTree(n *Node, seen map[string]bool, depth int) error {
	if n == nil {
		return errors.New("empty node")
	}
	if depth > 24 {
		return errors.New("nested too deep")
	}
	if n.Panel != "" {
		if pdef(n.Panel) == nil {
			return fmt.Errorf("unknown panel %q", n.Panel)
		}
		if seen[n.Panel] {
			return fmt.Errorf("panel %q appears twice", n.Panel)
		}
		if n.A != nil || n.B != nil || n.Split != "" {
			return fmt.Errorf("panel %q has children", n.Panel)
		}
		seen[n.Panel] = true
		return nil
	}
	if n.Split != "lr" && n.Split != "tb" {
		return fmt.Errorf("unknown split %q", n.Split)
	}
	if math.IsNaN(n.Ratio) || math.IsInf(n.Ratio, 0) {
		n.Ratio = .5
	}
	n.Ratio = clamp(n.Ratio, .05, .95)
	if err := checkTree(n.A, seen, depth+1); err != nil {
		return err
	}
	return checkTree(n.B, seen, depth+1)
}

// loadLayout returns the saved layout, or nil for the built-in one (with a note if the file was bad).
func loadLayout() (*Node, string) {
	path, err := layoutPath()
	if err != nil {
		return nil, ""
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ""
	}
	if err != nil {
		return nil, "could not read " + path + ": " + err.Error()
	}
	var f layoutFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, "ignoring broken layout file " + path
	}
	if err := checkTree(f.Layout, map[string]bool{}, 0); err != nil {
		return nil, "ignoring layout file " + path + ": " + err.Error()
	}
	return f.Layout, ""
}

// saveLayout writes the layout (nil = go back to the built-in one, the file is removed).
func saveLayout(root *Node) (string, error) {
	path, err := layoutPath()
	if err != nil {
		return "", err
	}
	if root == nil {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return path, err
		}
		return path, nil
	}
	b, err := json.MarshalIndent(layoutFile{1, root}, "", "  ")
	if err != nil {
		return path, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return path, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return path, err
	}
	return path, os.Rename(tmp, path)
}
