package main

// Matrix mode (key m): digital rain made from live data.
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
	chars    []rune // one glyph per screen row
}

var mx struct {
	w, h   int
	cols   [][]*drop
	pool   []rune
	poolAt time.Time
}

const mxGlyphs = "0123456789ABCDEFXYZ:.=*+-<>|#$%&"

// mxPool: printable ASCII from the busiest running processes' names plus a few symbols.
func mxPool() []rune {
	if time.Since(mx.poolAt) < 3*time.Second && len(mx.pool) > 0 {
		return mx.pool
	}
	mx.poolAt = time.Now()
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
	mx.pool = pool
	return pool
}

func mxResize(w, h int) {
	mx.w, mx.h = w, h
	mx.cols = make([][]*drop, w)
}

// coreLoadForColumn maps column c to one logical processor (bands across the screen).
func coreLoadForColumn(c int) float64 {
	n := len(S.Cores)
	if n == 0 || mx.w == 0 {
		return S.CPU
	}
	return clamp(S.Cores[clampi(c*n/mx.w, 0, n-1)], 0, 1)
}

func matrixFrame(dt float64) {
	if !V.Matrix || mx.w == 0 {
		return
	}
	pool := mxPool()
	base := 7 + 28*clamp(S.CPU, 0, 1) // rows per second
	for c := 0; c < mx.w; c++ {
		ds := mx.cols[c][:0]
		for _, d := range mx.cols[c] {
			d.y += d.speed * dt
			if int(d.y)-d.n < mx.h {
				if R() < .05 { // flicker a glyph now and then
					d.chars[rand.Intn(len(d.chars))] = pool[rand.Intn(len(pool))]
				}
				ds = append(ds, d)
			}
		}
		mx.cols[c] = ds
		free := len(ds) == 0 || int(ds[len(ds)-1].y)-ds[len(ds)-1].n > 2
		if free && R() < (.10+2.2*coreLoadForColumn(c))*dt {
			d := &drop{y: -R() * 3, speed: base * (.55 + R()*.9), n: 5 + rand.Intn(4+int(20*clamp(S.CPU, 0, 1))), chars: make([]rune, mx.h)}
			for i := range d.chars {
				d.chars[i] = pool[rand.Intn(len(pool))]
			}
			mx.cols[c] = append(mx.cols[c], d)
		}
	}
}

func drawMatrix(sc *Screen) {
	if mx.w != sc.W || mx.h != sc.H {
		mxResize(sc.W, sc.H)
	}
	for c, ds := range mx.cols {
		for _, d := range ds {
			head := int(d.y)
			for k := 0; k < d.n; k++ {
				y := head - k
				if y < 0 || y >= sc.H {
					continue
				}
				fg := lerp(0x45ff8c, 0x06240f, float64(k)/float64(d.n))
				if k == 0 {
					fg = 0xeafff0 // bright head
				}
				sc.PutX(c, y, d.chars[y], fg, BG, k == 0)
			}
		}
	}
	// a small live panel at the bottom
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
	col := uint32(0x2f9e5f)
	sc.Box(x, y, w, 3, col)
	sc.Label(x+2, y, true, col, Seg{"m", HI, false}, Seg{" matrix", TITLE, true})
	sc.LabelR(x+w-2, y+2, false, col, Seg{"m", HI, false}, Seg{"/", DIM, false}, Seg{"Esc", HI, false}, Seg{" back", DIM, false})
	sc.Str(x+2, y+1, trunc(text, w-4), 0x9dffc4, 0)
}
