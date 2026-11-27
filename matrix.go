package main

// Matrix: digital rain made from live data, full screen (key m) or as a panel in the layout.
//   - fall speed follows the total CPU load
//   - each band of columns belongs to one logical processor; its load sets how often drops spawn there
//   - the glyphs are taken from the names of the processes that are running right now

import (
	"math/rand"
	"sort"
	"strings"
	"time"
)

type drop struct {
	y, speed float64
	n        int    // trail length
	chars    []rune // one glyph per row
}

// rain is one rain area; the full screen and the panel each have their own.
type rain struct {
	w, h int
	cols [][]*drop
}

var mxFull, mxPanel rain

var mxPool struct {
	glyphs []rune
	at     time.Time
}

const mxGlyphs = "0123456789ABCDEFXYZ:.=*+-<>|#$%&"

// glyphs: printable ASCII from the busiest running processes' names plus a few symbols.
func glyphs() []rune {
	if time.Since(mxPool.at) < 3*time.Second && len(mxPool.glyphs) > 0 {
		return mxPool.glyphs
	}
	mxPool.at = time.Now()
	procs := make([]*Proc, 0, len(S.Procs))
	for _, p := range S.Procs {
		procs = append(procs, p)
	}
	sort.Slice(procs, func(i, j int) bool { return procs[i].CPU+procs[i].GPU > procs[j].CPU+procs[j].GPU })
	pool := []rune(mxGlyphs)
	for i, p := range procs {
		if i >= 40 {
			break
		}
		for _, r := range strings.ToUpper(strings.TrimSuffix(p.Name, ".exe")) {
			if r > ' ' && r < 0x7f {
				pool = append(pool, r)
			}
		}
	}
	mxPool.glyphs = pool
	return pool
}

func (m *rain) resize(w, h int) {
	m.w, m.h = maxi(0, w), maxi(0, h)
	m.cols = make([][]*drop, m.w)
}

// coreLoad maps column c to one logical processor (bands across the area).
func (m *rain) coreLoad(c int) float64 {
	n := len(S.Cores)
	if n == 0 || m.w == 0 {
		return S.CPU
	}
	return clamp(S.Cores[clampi(c*n/m.w, 0, n-1)], 0, 1)
}

func (m *rain) step(dt float64) {
	if m.w == 0 || m.h == 0 {
		return
	}
	pool := glyphs()
	base := 7 + 28*clamp(S.CPU, 0, 1) // rows per second
	for c := 0; c < m.w; c++ {
		ds := m.cols[c][:0]
		for _, d := range m.cols[c] {
			d.y += d.speed * dt
			if int(d.y)-d.n < m.h {
				if R() < .05 { // flicker a glyph now and then
					d.chars[rand.Intn(len(d.chars))] = pool[rand.Intn(len(pool))]
				}
				ds = append(ds, d)
			}
		}
		m.cols[c] = ds
		free := len(ds) == 0 || int(ds[len(ds)-1].y)-ds[len(ds)-1].n > 2
		if free && R() < (.10+2.2*m.coreLoad(c))*dt {
			d := &drop{y: -R() * 3, speed: base * (.55 + R()*.9), n: 5 + rand.Intn(4+int(20*clamp(S.CPU, 0, 1))), chars: make([]rune, m.h)}
			for i := range d.chars {
				d.chars[i] = pool[rand.Intn(len(pool))]
			}
			m.cols[c] = append(m.cols[c], d)
		}
	}
}

func (m *rain) draw(sc *Screen, x0, y0 int) {
	for c, ds := range m.cols {
		for _, d := range ds {
			head := int(d.y)
			for k := 0; k < d.n; k++ {
				y := head - k
				if y < 0 || y >= m.h {
					continue
				}
				fg := lerp(0x45ff8c, 0x06240f, float64(k)/float64(d.n))
				if k == 0 {
					fg = 0xeafff0 // bright head
				}
				sc.PutX(x0+c, y0+y, d.chars[y], fg, BG, k == 0)
			}
		}
	}
}

// matrixFrame advances whichever rain is on screen.
func matrixFrame(dt float64) {
	if V.Matrix {
		mxFull.step(dt)
		return
	}
	if _, ok := L.Last.Rects["matrix"]; ok {
		mxPanel.step(dt)
	}
}

const CMX = 0x2f9e5f

// drawMatrix is the full-screen mode with a small live panel at the bottom.
func drawMatrix(sc *Screen) {
	if mxFull.w != sc.W || mxFull.h != sc.H {
		mxFull.resize(sc.W, sc.H)
	}
	mxFull.draw(sc, 0, 0)
	gpu := "n/a"
	if S.GPU.OK {
		gpu = pctS(S.GPU.Load * 100)
	}
	ram := "n/a"
	if f := memUsedFrac(); f >= 0 {
		ram = pctS(f * 100)
	}
	parts := []string{"cpu " + pctS(S.CPU*100), "ram " + ram, "gpu " + gpu}
	if S.Power.HasBattery && S.Power.RateOK {
		parts = append(parts, fWatts(S.Power.RateW))
	}
	var top *Proc
	for _, p := range S.Procs {
		if top == nil || p.CPU+p.GPU > top.CPU+top.GPU {
			top = p
		}
	}
	if top != nil {
		parts = append(parts, "busiest "+top.Name)
	}
	text := strings.Join(parts, " · ")
	w := mini(rlen(text)+4, sc.W-2)
	x, y := (sc.W-w)/2, sc.H-4
	if y < 1 || w < 20 {
		return
	}
	for r := y; r < y+3; r++ {
		for c := x; c < x+w; c++ {
			sc.PutX(c, r, ' ', FG, 0x020604, false)
		}
	}
	col := uint32(CMX)
	sc.Box(x, y, w, 3, col)
	sc.Label(x+2, y, true, col, Seg{"m", HI, false}, Seg{" matrix", TITLE, true})
	sc.LabelR(x+w-2, y+2, false, col, Seg{"m", HI, false}, Seg{"/", DIM, false}, Seg{"Esc", HI, false}, Seg{" back", DIM, false})
	sc.Str(x+2, y+1, trunc(text, w-4), 0x9dffc4, 0)
}

// drawMatrixPanel is the rain as a tile in the layout.
func drawMatrixPanel(sc *Screen, x, y, w, h int) {
	col := uint32(CMX)
	iw, ih := w-2, h-2
	if mxPanel.w != iw || mxPanel.h != ih {
		mxPanel.resize(iw, ih)
	}
	mxPanel.draw(sc, x+1, y+1)
	sc.Box(x, y, w, h, col)
	if t := fitSegs([]Seg{{"⁷", HI, false}, {"matrix", TITLE, true}}, w-4, 2); len(t) > 0 {
		sc.Label(x+2, y, true, col, t...)
	}
	cx := sc.LabelR(x+w-2, y+h-1, false, col, Seg{"cpu ", DIM, false}, Seg{pctS(S.CPU * 100), TITLE, false})
	if t := fitSegs([]Seg{{"m ", HI, false}, {"full screen", DIM, false}}, cx-1-(x+2), 0); len(t) == 2 {
		sc.Label(x+2, y+h-1, false, col, t...)
	}
}
