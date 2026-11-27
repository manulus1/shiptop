package main

import (
	"fmt"
	"math"
	"time"
)

// ---------- ⁵ notebook: pixel 3D model with a thermal map ----------

const (
	GX, GZ                  = 32, 22
	TMIN, TMAX, AMB         = 24.0, 62.0, 24.0
	DIST, YC, ZC            = 125.0, 8.5, 3.0
	LX, LY, LZ              = -.447, .744, -.496
	BX0, BX1, BZ0, BZ1, BY1 = -15.5, 15.5, -10.5, 10.5, 1.6
	LS, LT                  = 21.0, .55
	TRANSP                  = 0x80000000
)

var thermStops = []uint32{0x1c1650, 0x3d1680, 0x8a1a86, 0xd0303c, 0xf2661c, 0xfab62c, 0xfff4b8}
var LUT [256]uint32

func init() {
	for i := 0; i < 256; i++ {
		t := float64(i) / 255 * 6
		k := int(t)
		if k > 5 {
			k = 5
		}
		LUT[i] = lerp(thermStops[k], thermStops[k+1], t-float64(k))
	}
}

// thermal camera style auto-span: the palette always covers the current scene
var spanLo, spanHi = 24.0, 40.0

func thermT(T float64) uint32 {
	return LUT[clampi(int(math.Round((T-spanLo)/(spanHi-spanLo)*255)), 0, 255)]
}

type Part struct{ X, Y, Z, VX, VY, VZ, A, L float64 }

type NBState struct {
	N                                     int
	Cores, Off                            []float64
	Tc, Tg, Tv, Ts, Tb, Gl, Fan, Rt, Load float64
	Ram                                   float64
	RealLoad                              bool
	Bat                                   int
	Chg                                   bool
	Ph                                    float64
	Burst                                 int
	Loud                                  bool
	TG                                    [GX * GZ]float64
	Max, Min, Mx, Mz                      float64
	Parts                                 []Part
	CPUName                               string
	MemGB                                 float64
}

var NB = NBState{Tc: 58, Tg: 50, Tv: 56, Ts: 42, Tb: 30, Gl: .4, Fan: 2400, Rt: .5, Load: .3, Bat: -1, Ram: -1}

type View struct {
	Yaw, Pitch, Lid float64
	Therm, Zoom     bool
	Matrix          bool // matrix mode (key m)
	Edit            bool // layout editor (key l)
	Pix             int  // notebook resolution: index into pixModes (key g)
	Hold            time.Time
}

var V = View{Yaw: -.6, Pitch: .46, Lid: 2, Therm: true}

func nbInit() {
	NB.N = clampi(S.Info.Threads, 1, 256)
	NB.CPUName = S.Info.CPU
	NB.MemGB = S.Info.MemGB
	for i := 0; i < NB.N; i++ {
		NB.Cores = append(NB.Cores, .3)
		NB.Off = append(NB.Off, (R()-.5)*.3)
	}
}

func gs(X, Z, x, z, s float64) float64 {
	return math.Exp(-((X-x)*(X-x) + (Z-z)*(Z-z)) / (2 * s * s))
}

func nbHeat() {
	gx, gz := 11.0, 6.8
	if NB.CPUName != "" && containsFold(NB.CPUName, "apple") {
		gx, gz = 17.5, 6.5
	}
	mx, mn := -1.0, 1e9
	for j := 0; j < GZ; j++ {
		for i := 0; i < GX; i++ {
			X, Z := (float64(i)+.5)/GX*31, (float64(j)+.5)/GZ*21
			T := AMB + gs(X, Z, 17.5, 6.5, 3.6)*(NB.Tc-AMB)*.42 + gs(X, Z, gx, gz, 3.8)*(NB.Tg-AMB)*.34 +
				gs(X, Z, 21.5, 4.5, 2.2)*(NB.Tv-AMB)*.3 + gs(X, Z, 24, 11, 2.4)*(NB.Ts-AMB)*.4 + gs(X, Z, 15.5, 16, 7.5)*(NB.Tb-AMB)*.55
			if NB.Chg {
				T += gs(X, Z, 2.2, 5, 1.8) * 8
			}
			if Z < 2.6 && X > 3 && X < 28 {
				T += (NB.Tc - AMB) * .3 * (1 - Z/2.6)
			}
			T += math.Sin(X*1.3+Z*.7+NB.Ph) * .25
			NB.TG[j*GX+i] = T
			if T > mx {
				mx, NB.Mx, NB.Mz = T, X, Z
			}
			if T < mn {
				mn = T
			}
		}
	}
	NB.Max, NB.Min = mx, mn
	lo, hi := mn-.5, math.Max(mx+1.5, mn+12)
	if spanHi <= spanLo+1 {
		spanLo, spanHi = lo, hi
	}
	spanLo += (lo - spanLo) * .3
	spanHi += (hi - spanHi) * .3
}

func tAt(X, Z float64) float64 {
	fx := clamp(X/31*GX-.5, 0, GX-1.001)
	fz := clamp(Z/21*GZ-.5, 0, GZ-1.001)
	i, j := int(fx), int(fz)
	a, b := fx-float64(i), fz-float64(j)
	k := j*GX + i
	T := &NB.TG
	return T[k]*(1-a)*(1-b) + T[k+1]*a*(1-b) + T[k+GX]*(1-a)*b + T[k+GX+1]*a*b
}

// nbStep: real load, RAM and battery drive the model; temperatures are modelled from the load.
func nbStep() {
	if S.CPUOK {
		NB.RealLoad = true
		NB.Load = NB.Load*.4 + S.CPU*.6
		if len(S.Cores) > 0 {
			NB.Cores = append(NB.Cores[:0], S.Cores...)
		}
	} else if !NB.RealLoad {
		NB.Load = clamp(NB.Load+(.3-NB.Load)*.3+(R()-.5)*.1, .02, 1)
		for i := range NB.Cores {
			NB.Off[i] = clamp(NB.Off[i]+(R()-.5)*.05, -.3, .3)
			NB.Cores[i] = clamp(NB.Load+NB.Off[i]+(R()-.5)*.15, .02, 1)
		}
	}
	NB.Ram, NB.Bat, NB.Chg = memUsedFrac(), S.Bat, S.Chg
	if S.Therm.OK { // a real ACPI thermal zone beats the formula
		NB.Tc += (S.Therm.Max - NB.Tc) * .5
	} else {
		NB.Tc += (40+52*NB.Load-NB.Tc)*.25 + (R()-.5)*.3
	}
	if S.GPU.OK { // real GPU load
		NB.Gl = S.GPU.Load
	} else {
		NB.Gl = clamp(.25+NB.Load*.5+NB.Rt/6, 0, 1)
	}
	NB.Tg += (36 + 40*NB.Gl - NB.Tg) * .15
	NB.Tv += (NB.Tc*.85 + 6 - NB.Tv) * .2
	NB.Ts += (38 + 8*NB.Load + (R()-.5)*1 - NB.Ts) * .3
	chg := 0.0
	if NB.Chg {
		chg = 4
	}
	if S.Power.TempC > 0 {
		NB.Tb = S.Power.TempC
	} else {
		NB.Tb += (29 + chg + NB.Load*3 - NB.Tb) * .05
	}
	NB.Fan += (clamp(1300+(NB.Tc-45)*120, 1300, 6400) - NB.Fan) * .25
	NB.Ph = float64(S.Tick) * .7
	nbHeat()
	if NB.Fan > 4600 && !NB.Loud {
		NB.Loud = true
		say(WARN, fmt.Sprintf("notebook under load · modelled fan %.1fk rpm", NB.Fan/1000), false)
		flash("nb")
	} else if NB.Fan < 3600 {
		NB.Loud = false
	}
}

// ---- surface textures: (u,v) on a face -> 0xRRGGBB or TRANSP ----

type texFn func(u, v float64, x, y int) uint32

func al(T float64) float64 { return AMB + (T-AMB)*.8 }

func deck(u, v float64, _, _ int) uint32 {
	T := tAt(u*31, v*21)
	k := 0
	if u > .07 && u < .93 && v > .08 && v < .5 {
		ku, kv := (u-.07)/.86*14, (v-.08)/.42*6
		c, r := int(ku), int(kv)
		fu, fv := ku-float64(c), kv-float64(r)
		sp := r == 5 && c > 3 && c < 10
		if fv < .2 || (fu < .16 && !(sp && c > 4)) {
			k = 2
		} else {
			k = 1
			if !V.Therm && fv < .45 {
				k = 4
			}
		}
	} else if u > .34 && u < .66 && v > .58 && v < .9 {
		k = 3
	}
	if V.Therm {
		switch k {
		case 1:
			return thermT(T)
		case 3:
			return thermT(AMB + (T-AMB)*.9)
		}
		return thermT(al(T))
	}
	if u > .93 && u < .95 && v > .03 && v < .06 {
		return 0x3ee39b
	}
	return []uint32{0x9aa1ab, 0x1b1e24, 0x0e1014, 0xadb4be, 0x2a2f38}[k]
}

func front(u, v float64, _, _ int) uint32 {
	if V.Therm {
		return thermT(al(tAt(u*31, 20.6)))
	}
	if v < .2 {
		return 0xbdc4cd
	}
	if u > .44 && u < .56 && v < .45 {
		return 0x6b717a
	}
	return 0x868d97
}

func back(u, v float64, _, _ int) uint32 {
	vent := v > .25 && v < .8 && ((u > .1 && u < .42) || (u > .58 && u < .9)) && int(u*70)%2 == 0
	if V.Therm {
		if vent {
			return thermT(AMB + (NB.Tc-AMB)*.6)
		}
		return thermT(al(tAt((1-u)*31, .4)))
	}
	if vent {
		return 0x1e2228
	}
	if v < .2 {
		return 0xa9b0ba
	}
	return 0x7f8690
}

func side(left bool) texFn {
	return func(u, v float64, _, _ int) uint32 {
		Z := u * 21
		if !left {
			Z = (1 - u) * 21
		}
		port := v > .3 && v < .7 && ((left && ((Z > 3 && Z < 4.4) || (Z > 5.4 && Z < 6.8) || (Z > 16 && Z < 16.8))) ||
			(!left && ((Z > 4 && Z < 5.4) || (Z > 9 && Z < 11.5))))
		if V.Therm {
			X := .4
			if !left {
				X = 30.6
			}
			p := 0.0
			if port {
				p = 1.5
			}
			return thermT(al(tAt(X, Z)) + p)
		}
		if port {
			return 0x1a1d22
		}
		if v < .2 {
			return 0xb5bcc5
		}
		return 0x868d97
	}
}

var sideL, sideR = side(true), side(false)

func lidIn(u, v float64, _, _ int) uint32 {
	bz := u < .035 || u > .965 || v < .07 || v > .94
	if V.Therm {
		if bz {
			return thermT(29.5 + 2*(1-v))
		}
		return thermT(32.5 + 3.4*(1-v) + .5*math.Sin(u*11+NB.Ph))
	}
	if bz {
		if u > .49 && u < .51 && v > .955 && v < .975 {
			return 0x2e3440
		}
		return 0x111419
	}
	su, sv := (u-.035)/.93, (v-.07)/.87
	if su < .015 || su > .985 || sv < .025 || sv > .975 {
		return 0x3c9b6f
	}
	if sv < .56 {
		a := S.CPUH
		i := len(a) - 40 + int(su*40)
		val := 0.0
		if i >= 0 && i < len(a) {
			val = a[i] / 100
		}
		gv := (sv - .05) / .48
		if gv >= 0 && gv < val {
			return gHeat.At(gv)
		}
		return 0x0a0d12
	}
	if sv > .62 && sv < .94 {
		r, c := int((sv-.62)/.32*5), int(su*26)
		if r == 4 && c < 9 {
			return 0xf2f5f9
		}
		if (c*73+r*151+(c>>2)*31)%97 < 48 {
			return 0x3a4250
		}
	}
	return 0x0a0d12
}

func lidOut(u, v float64, _, _ int) uint32 {
	dx, dy := (u-.3)*31, (v-.62)*21
	d2 := dx*dx + dy*dy
	st1, st2 := d2 < 9, u > .56 && u < .8 && v > .28 && v < .42
	if V.Therm {
		e := 0.0
		if st1 || st2 {
			e = 1.4
		}
		return thermT(28.5 + 4.5*(1-v) + (NB.Tc-AMB)*.06*(1-v) + e)
	}
	if st1 {
		if d2 < 2.2 {
			return 0x07090d
		}
		return 0xff6fae
	}
	if st2 {
		if v > .33 && v < .37 && u > .6 && u < .76 {
			return 0x07090d
		}
		return 0x3fd0e0
	}
	return 0x8b929c
}

func edge(_, _ float64, _, _ int) uint32 {
	if V.Therm {
		return thermT(al(30.5))
	}
	return 0x767d87
}

func floorTex(u, v float64, x, y int) uint32 {
	X, Z := -30+u*60, -30+v*54
	cx, cz := clamp(X, BX0, BX1), clamp(Z, BZ0, BZ1)
	d := math.Hypot(X-cx, Z-cz)
	if V.Therm && d < 7 {
		T := AMB + (tAt(cx+15.5, cz+10.5)-AMB)*.45*math.Exp(-d/2.2)
		if T > AMB+1.2 {
			return thermT(T)
		}
	}
	if !V.Therm && X > -15.2 && X < 17.6 && Z > -10.4 && Z < 12.6 {
		if (x+y)&1 == 1 {
			return 0x161a22
		}
		return TRANSP
	}
	if nbCW < 60 {
		return TRANSP
	}
	r, a := math.Hypot(X, Z+3), math.Atan2(Z+3, X)
	if r > 20.6 && r < 21.5 {
		if math.Abs(a-math.Pi/2) < .14 {
			return 0x7166c8
		}
		return 0x1c2330
	}
	if r > 23.2 && r < 23.8 && int((a+4)*8)%2 == 1 {
		return 0x141922
	}
	return TRANSP
}

// ---- tiny software rasterizer ----

var (
	pw, ph                           int // render buffer in pixels (sub-cells × supersampling)
	px                               []uint32
	zb                               []float32
	idb                              []uint8
	camCY, camSY, camCP, camSP, camF float64
	camOX, camOY                     float64
	fitKey                           [4]float64
	nbCW, nbCH                       int    // the view in terminal cells
	nbSS                             = 1    // supersampling factor (anti-aliasing)
	nbAsp                            = 1.0  // pixel height / pixel width (cells are about twice as tall as wide)
	nbKey                            [3]int // cells and mode of the last render (blit checks it)
	nbHot                            [3]int // cell of the hottest spot, [2] = 1 if it is in view
)

// pixMode: how many sub-pixels one terminal cell shows. More sub-pixels need newer fonts:
// half blocks are everywhere, quadrants almost everywhere, sextants and octants come with
// Cascadia Code 2404+ (Windows Terminal) and modern Linux terminals.
type pixMode struct {
	ID, Name string
	CX, CY   int
	Runes    []rune
}

var pixModes = []pixMode{
	{"half", "1×2", 1, 2, halfRunes[:]},
	{"quad", "2×2", 2, 2, quadRunes[:]},
	{"sext", "2×3", 2, 3, sextRunes[:]},
	{"oct", "2×4", 2, 4, octRunes[:]},
}

// fitCam sizes the camera so the model fits the view at every yaw angle (no pumping while it turns).
func fitCam() {
	key := [4]float64{float64(pw), float64(ph), V.Pitch, nbAsp}
	if key == fitKey {
		return
	}
	fitKey = key
	pts := [][3]float64{}
	for _, x := range []float64{BX0, BX1} {
		for _, z := range []float64{BZ0, BZ1} {
			pts = append(pts, [3]float64{x, 0, z}, [3]float64{x, BY1, z})
		}
		th := 108 * math.Pi / 180
		pts = append(pts, [3]float64{x, BY1 + math.Sin(th)*LS, BZ0 + .2 + math.Cos(th)*LS}, [3]float64{x, BY1 + math.Sin(th)*LS + math.Cos(th)*LT, BZ0 + .2 + math.Cos(th)*LS - math.Sin(th)*LT})
	}
	if nbCW >= 60 {
		for a := 0.0; a < 2*math.Pi; a += math.Pi / 8 {
			pts = append(pts, [3]float64{21.5 * math.Cos(a), 0, 21.5*math.Sin(a) - 3})
		}
	}
	camCP, camSP = math.Cos(V.Pitch), math.Sin(V.Pitch)
	mnx, mxx, mny, mxy := 1e9, -1e9, 1e9, -1e9
	for a := 0.0; a < 2*math.Pi; a += math.Pi / 12 {
		camCY, camSY = math.Cos(a), math.Sin(a)
		for _, p := range pts {
			x, y, z := rot(p[0], p[1]-YC, p[2]+ZC)
			sx, sy := x/(z+DIST), y/(z+DIST)
			mnx, mxx, mny, mxy = math.Min(mnx, sx), math.Max(mxx, sx), math.Min(mny, sy), math.Max(mxy, sy)
		}
	}
	camF = math.Min(float64(pw)*.94/((mxx-mnx)*nbAsp), float64(ph)*.94/(mxy-mny))
	camOX = -(mnx + mxx) / 2 * camF * nbAsp
	camOY = (mny + mxy) / 2 * camF
}

type vtx struct{ x, y, iz, uz, vz float64 }
type pv struct{ x, y, iz, vx, vy, vz float64 }

func rot(x, y, z float64) (float64, float64, float64) {
	x1 := x*camCY + z*camSY
	z1 := x*camSY - z*camCY
	return x1, y*camCP + z1*camSP, -y*camSP + z1*camCP
}

func proj(p [3]float64) pv {
	x, y, z := rot(p[0], p[1]-YC, p[2]+ZC)
	zz := z + DIST
	iz := 1 / zz
	return pv{float64(pw)/2 + camF*nbAsp*x*iz + camOX, float64(ph)/2 - camF*y*iz + camOY, iz, x, y, zz}
}

func pk(c uint32, s float64) uint32 {
	r := math.Min(255, float64((c>>16)&255)*s)
	g := math.Min(255, float64((c>>8)&255)*s)
	b := math.Min(255, float64(c&255)*s)
	return uint32(r)<<16 | uint32(g)<<8 | uint32(b)
}

func tri(a, b, c vtx, fid uint8, tex texFn, sh float64) {
	area := (b.x-a.x)*(c.y-a.y) - (b.y-a.y)*(c.x-a.x)
	if math.Abs(area) < 1e-6 {
		return
	}
	x0 := clampi(int(math.Floor(math.Min(a.x, math.Min(b.x, c.x)))), 0, pw-1)
	x1 := clampi(int(math.Ceil(math.Max(a.x, math.Max(b.x, c.x)))), 0, pw-1)
	y0 := clampi(int(math.Floor(math.Min(a.y, math.Min(b.y, c.y)))), 0, ph-1)
	y1 := clampi(int(math.Ceil(math.Max(a.y, math.Max(b.y, c.y)))), 0, ph-1)
	for y := y0; y <= y1; y++ {
		py := float64(y) + .5
		for x := x0; x <= x1; x++ {
			pxx := float64(x) + .5
			w0 := ((b.x-pxx)*(c.y-py) - (b.y-py)*(c.x-pxx)) / area
			w1 := ((c.x-pxx)*(a.y-py) - (c.y-py)*(a.x-pxx)) / area
			w2 := 1 - w0 - w1
			if w0 < -1e-6 || w1 < -1e-6 || w2 < -1e-6 {
				continue
			}
			iz := w0*a.iz + w1*b.iz + w2*c.iz
			i := y*pw + x
			if float32(iz) <= zb[i] {
				continue
			}
			col := tex((w0*a.uz+w1*b.uz+w2*c.uz)/iz, (w0*a.vz+w1*b.vz+w2*c.vz)/iz, x, y)
			if col == TRANSP {
				continue
			}
			zb[i], idb[i], px[i] = float32(iz), fid, pk(col, sh)
		}
	}
}

var uvs = [4][2]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}}

func quad(P [4][3]float64, n *[3]float64, fid uint8, tex texFn) {
	var q [4]pv
	for i := range P {
		q[i] = proj(P[i])
	}
	sh := 1.0
	if n != nil {
		nx, ny, nz := rot(n[0], n[1], n[2])
		if nx*q[0].vx+ny*q[0].vy+nz*q[0].vz >= 0 {
			return
		}
		l := math.Max(0, nx*LX+ny*LY+nz*LZ)
		if V.Therm {
			sh = .6 + .45*l
		} else {
			sh = .42 + .68*l
		}
		sh = math.Round(sh*8) / 8
	}
	var v [4]vtx
	for i, p := range q {
		v[i] = vtx{p.x, p.y, p.iz, uvs[i][0] * p.iz, uvs[i][1] * p.iz}
	}
	tri(v[0], v[1], v[2], fid, tex, sh)
	tri(v[0], v[2], v[3], fid, tex, sh)
}

func nrm(x, y, z float64) *[3]float64 { return &[3]float64{x, y, z} }

// nbRender draws the laptop for a view of cw×ch terminal cells in the current pixel mode.
func nbRender(cw, ch int) {
	m := pixModes[V.Pix]
	if cw < 2 || ch < 2 {
		nbKey = [3]int{}
		return
	}
	t0 := time.Now()
	nbCW, nbCH = cw, ch
	nbSS = 2 // render at twice the resolution and average: smooth edges
	if cw*m.CX*ch*m.CY*4 > 160000 {
		nbSS = 1 // a very big zoomed view has enough pixels already; keep the CPU cost down
	}
	nbAsp = 2 * float64(m.CX) / float64(m.CY)
	w, h := cw*m.CX*nbSS, ch*m.CY*nbSS
	nbKey = [3]int{cw, ch, V.Pix}
	nbHot = [3]int{}
	if w != pw || h != ph {
		pw, ph = w, h
		px = make([]uint32, w*h)
		zb = make([]float32, w*h)
		idb = make([]uint8, w*h)
	}
	for i := range px {
		px[i], zb[i], idb[i] = TRANSP, 0, 0
	}
	fitCam()
	camCY, camSY, camCP, camSP = math.Cos(V.Yaw), math.Sin(V.Yaw), math.Cos(V.Pitch), math.Sin(V.Pitch)
	quad([4][3]float64{{-30, -.05, -30}, {30, -.05, -30}, {30, -.05, 24}, {-30, -.05, 24}}, nil, 1, floorTex)
	quad([4][3]float64{{BX0, BY1, BZ0}, {BX1, BY1, BZ0}, {BX1, BY1, BZ1}, {BX0, BY1, BZ1}}, nrm(0, 1, 0), 2, deck)
	quad([4][3]float64{{BX0, BY1, BZ1}, {BX1, BY1, BZ1}, {BX1, 0, BZ1}, {BX0, 0, BZ1}}, nrm(0, 0, 1), 3, front)
	quad([4][3]float64{{BX1, BY1, BZ0}, {BX0, BY1, BZ0}, {BX0, 0, BZ0}, {BX1, 0, BZ0}}, nrm(0, 0, -1), 4, back)
	quad([4][3]float64{{BX0, BY1, BZ0}, {BX0, BY1, BZ1}, {BX0, 0, BZ1}, {BX0, 0, BZ0}}, nrm(-1, 0, 0), 5, sideL)
	quad([4][3]float64{{BX1, BY1, BZ1}, {BX1, BY1, BZ0}, {BX1, 0, BZ0}, {BX1, 0, BZ1}}, nrm(1, 0, 0), 6, sideR)
	th := V.Lid * math.Pi / 180
	dy, dz := math.Sin(th), math.Cos(th)
	ny, nz := dz, -dy
	L := func(x, s, t float64) [3]float64 { return [3]float64{x, BY1 + dy*s + ny*t, BZ0 + .2 + dz*s + nz*t} }
	quad([4][3]float64{L(BX0, 0, 0), L(BX1, 0, 0), L(BX1, LS, 0), L(BX0, LS, 0)}, nrm(0, -ny, -nz), 7, lidIn)
	quad([4][3]float64{L(BX1, 0, LT), L(BX0, 0, LT), L(BX0, LS, LT), L(BX1, LS, LT)}, nrm(0, ny, nz), 8, lidOut)
	quad([4][3]float64{L(BX0, LS, 0), L(BX1, LS, 0), L(BX1, LS, LT), L(BX0, LS, LT)}, nrm(0, dy, dz), 9, edge)
	quad([4][3]float64{L(BX0, 0, 0), L(BX0, LS, 0), L(BX0, LS, LT), L(BX0, 0, LT)}, nrm(-1, 0, 0), 10, edge)
	quad([4][3]float64{L(BX1, LS, 0), L(BX1, 0, 0), L(BX1, 0, LT), L(BX1, LS, LT)}, nrm(1, 0, 0), 11, edge)
	quad([4][3]float64{L(BX1, 0, 0), L(BX0, 0, 0), L(BX0, 0, LT), L(BX1, 0, LT)}, nrm(0, -dy, -dz), 12, edge)

	// pixel-art pass: darker silhouette, seams between faces, then an outline around the model;
	// lines are one sub-pixel wide (nbSS render pixels), so they stay crisp at every resolution
	dk := func(c uint32, f float64) uint32 { return pk(c, f) }
	id := func(x, y int) uint8 {
		if x < 0 || y < 0 || x >= pw || y >= ph {
			return 0
		}
		return idb[y*pw+x]
	}
	ss := nbSS
	for y := 0; y < ph; y++ {
		for x := 0; x < pw; x++ {
			i := y*pw + x
			me := idb[i]
			if me < 2 {
				continue
			}
			edge, seam := false, false
			for d := 1; d <= ss; d++ {
				l, r, t, b := id(x-d, y), id(x+d, y), id(x, y-d), id(x, y+d)
				edge = edge || l < 2 || r < 2 || t < 2 || b < 2
				seam = seam || r != me || b != me
			}
			if edge {
				px[i] = dk(px[i], .55)
			} else if seam {
				px[i] = dk(px[i], .78)
			}
		}
	}
	ol := uint32(0x36404a)
	if V.Therm {
		ol = 0x4a2f5a
	}
	for y := 0; y < ph; y++ {
		for x := 0; x < pw; x++ {
			i := y*pw + x
			if idb[i] > 1 {
				continue
			}
			for d := 1; d <= ss; d++ {
				if id(x-d, y) > 1 || id(x+d, y) > 1 || id(x, y-d) > 1 || id(x, y+d) > 1 {
					px[i] = ol
					break
				}
			}
		}
	}
	dot := func(x, y int, c uint32, iz float64) {
		for yy := y; yy < y+ss; yy++ {
			for xx := x; xx < x+ss; xx++ {
				if xx >= 0 && yy >= 0 && xx < pw && yy < ph && (iz == 0 || float32(iz) > zb[yy*pw+xx]) {
					px[yy*pw+xx] = c
				}
			}
		}
	}
	if V.Therm {
		tx := AMB + (NB.Tc-AMB)*.6
		for _, p := range NB.Parts {
			f := 1 - p.A/p.L
			T := AMB + (tx-AMB)*f*f
			if T < AMB+3 {
				continue
			}
			q := proj([3]float64{p.X, p.Y, p.Z})
			dot(int(q.x), int(q.y), thermT(T), q.iz)
		}
		q := proj([3]float64{NB.Mx - 15.5, BY1, NB.Mz - 10.5})
		x, y := int(q.x), int(q.y)
		if x >= 0 && y >= 0 && x < pw && y < ph && idb[y*pw+x] == 2 { // the hottest spot is visible
			nbHot = [3]int{x / (m.CX * ss), y / (m.CY * ss), 1}
		}
	}
	NB.Rt = NB.Rt*.9 + float64(time.Since(t0).Microseconds())/1000*.1
}

func nbFrame(dt float64) {
	if time.Now().After(V.Hold) {
		V.Yaw += dt * .32
	}
	if V.Lid < 108 {
		V.Lid = math.Min(108, V.Lid+dt*70)
	}
	P := NB.Parts[:0]
	for _, p := range NB.Parts {
		p.A += dt
		p.X += p.VX * dt
		p.Y += p.VY * dt
		p.Z += p.VZ * dt
		p.VY += 1.5 * dt
		if p.A < p.L {
			P = append(P, p)
		}
	}
	NB.Parts = P
	if V.Therm {
		k := NB.Fan / 3200 * dt * 30
		for k > 0 {
			if R() < k && len(NB.Parts) < 220 {
				x0 := 4.0
				if R() < .5 {
					x0 = -12
				}
				NB.Parts = append(NB.Parts, Part{X: x0 + R()*8, Y: .5 + R()*.8, Z: BZ0 - .3, VX: (R() - .5) * 1.5, VY: 2.5 + R()*2.5, VZ: -(5 + R()*4), L: .9 + R()})
			}
			k--
		}
	}
}

// nbView draws the laptop into a cw×ch cell area. A render that took long is reused for a few
// frames (the model turns slowly), so a big zoomed view costs at most about a fifth of a core.
var nbCache struct {
	key   [4]int
	at    time.Time
	cells []Cell
	hot   [3]int
}

func nbView(sc *Screen, x0, y0, cw, ch int) {
	if cw < 2 || ch < 2 {
		return
	}
	therm := 0
	if V.Therm {
		therm = 1
	}
	key := [4]int{cw, ch, V.Pix, therm}
	every := time.Duration(math.Min(NB.Rt*4, 150) * float64(time.Millisecond))
	if key != nbCache.key || time.Since(nbCache.at) >= every || len(nbCache.cells) != cw*ch {
		nbRender(cw, ch)
		nbCache.key, nbCache.at = key, time.Now()
		if cap(nbCache.cells) < cw*ch {
			nbCache.cells = make([]Cell, cw*ch)
		}
		nbCache.cells = nbCache.cells[:cw*ch]
		blitCells(nbCache.cells, cw, ch)
		nbCache.hot = nbHot
	}
	for i, c := range nbCache.cells {
		if c.R != 0 {
			sc.PutX(x0+i%cw, y0+i/cw, c.R, c.FG, c.BG, false)
		}
	}
	if h := nbCache.hot; V.Therm && h[2] == 1 && h[0] < cw && h[1] < ch { // mark the hottest spot
		c := nbCache.cells[h[1]*cw+h[0]]
		bg := c.BG
		if c.R != ' ' && c.R != 0 {
			bg = lerp(c.FG, c.BG, .5)
		}
		sc.PutX(x0+h[0], y0+h[1], '╋', 0x120a22, bg, true)
	}
}

// blitPixels draws the last render directly (used where caching does not matter).
func blitPixels(sc *Screen, x0, y0, cw, ch int) {
	if cw < 1 || ch < 1 {
		return
	}
	cells := make([]Cell, cw*ch)
	blitCells(cells, cw, ch)
	for i, c := range cells {
		if c.R != 0 {
			sc.PutX(x0+i%cw, y0+i/cw, c.R, c.FG, c.BG, false)
		}
	}
}

// blitCells turns the rendered picture into terminal cells (R == 0: nothing drawn there). Each cell
// shows its sub-pixels (the average of nbSS×nbSS render pixels each) with the two colours that fit
// them best: a foreground for the sub-pixels drawn by the mosaic character, a background for the rest.
func blitCells(out []Cell, cw, ch int) {
	for i := range out {
		out[i] = Cell{}
	}
	if nbKey != [3]int{cw, ch, V.Pix} || len(px) != pw*ph {
		return
	}
	m := pixModes[V.Pix]
	ss := nbSS
	n := m.CX * m.CY
	var sub [8][3]float64
	bg := [3]float64{float64(BG >> 16 & 255), float64(BG >> 8 & 255), float64(BG & 255)}
	inv := 1 / float64(ss*ss)
	for cy := 0; cy < ch; cy++ {
		for cx := 0; cx < cw; cx++ {
			seen := false
			for k := 0; k < n; k++ {
				var r, g, b float64
				bx, by := (cx*m.CX+k%m.CX)*ss, (cy*m.CY+k/m.CX)*ss
				for yy := by; yy < by+ss; yy++ {
					for xx := bx; xx < bx+ss; xx++ {
						c := px[yy*pw+xx]
						if c == TRANSP { // empty: the panel background shows through
							r, g, b = r+bg[0], g+bg[1], b+bg[2]
							continue
						}
						seen = true
						r, g, b = r+float64(c>>16&255), g+float64(c>>8&255), b+float64(c&255)
					}
				}
				sub[k] = [3]float64{r * inv, g * inv, b * inv}
			}
			if !seen {
				continue
			}
			bits, fg, bgc := split2(sub[:n])
			r := m.Runes[bits]
			if bits == 0 || bits == 1<<n-1 {
				r = ' '
			}
			out[cy*cw+cx] = Cell{r, fg, bgc, false}
		}
	}
}

// split2 divides a cell's sub-pixel colours into two groups (2-means, seeded with the two most
// different colours) and returns which sub-pixels belong to the first group and both mean colours.
// A cell that is one colour (or as good as) comes back as bits 0 with that colour as background.
func split2(c [][3]float64) (bits int, fg, bg uint32) {
	d2 := func(a, b [3]float64) float64 {
		dr, dg, db := a[0]-b[0], a[1]-b[1], a[2]-b[2]
		return dr*dr + dg*dg + db*db
	}
	pack := func(v [3]float64) uint32 {
		return uint32(clamp(v[0]+.5, 0, 255))<<16 | uint32(clamp(v[1]+.5, 0, 255))<<8 | uint32(clamp(v[2]+.5, 0, 255))
	}
	ia, ib, far := 0, 0, -1.0
	for i := range c {
		for j := i + 1; j < len(c); j++ {
			if d := d2(c[i], c[j]); d > far {
				ia, ib, far = i, j, d
			}
		}
	}
	mean := func(sel func(int) bool) ([3]float64, int) {
		var s [3]float64
		k := 0
		for i, v := range c {
			if sel(i) {
				s[0], s[1], s[2] = s[0]+v[0], s[1]+v[1], s[2]+v[2]
				k++
			}
		}
		if k > 0 {
			s[0], s[1], s[2] = s[0]/float64(k), s[1]/float64(k), s[2]/float64(k)
		}
		return s, k
	}
	all, _ := mean(func(int) bool { return true })
	if far < 6*6*3 { // practically one colour
		return 0, pack(all), pack(all)
	}
	a, b := c[ia], c[ib]
	for iter := 0; iter < 3; iter++ {
		bits = 0
		for i, v := range c {
			if d2(v, a) < d2(v, b) {
				bits |= 1 << i
			}
		}
		na, ka := mean(func(i int) bool { return bits&(1<<i) != 0 })
		nb, kb := mean(func(i int) bool { return bits&(1<<i) == 0 })
		if ka == 0 || kb == 0 {
			break
		}
		a, b = na, nb
	}
	if bits == 0 || bits == 1<<len(c)-1 {
		return 0, pack(all), pack(all)
	}
	return bits, pack(a), pack(b)
}
