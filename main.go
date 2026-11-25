package main

import (
	"fmt"
	"math"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

const version = "5.0.0"

func main() {
	for _, a := range os.Args[1:] {
		switch a {
		case "-h", "--help", "/?":
			fmt.Print(helpText)
			return
		case "-v", "--version":
			fmt.Println("shiptop", version)
			return
		case "--once":
			once(false)
			return
		case "--selftest":
			once(true)
			return
		}
	}
	if err := termInit(); err != nil {
		fmt.Fprintln(os.Stderr, "shiptop:", err)
		os.Exit(1)
	}
	out := os.Stdout
	out.WriteString("\x1b[?1049h\x1b[?25l\x1b[?7l\x1b[2J\x1b]0;shiptop\x07")
	restore := func() {
		out.WriteString("\x1b[?2026l\x1b[0m\x1b[2J\x1b[H\x1b[?7h\x1b[?25h\x1b[?1049l")
		termRestore()
	}
	defer func() {
		if r := recover(); r != nil {
			restore()
			fmt.Fprintln(os.Stderr, "shiptop crashed:", r)
			os.Stderr.Write(debug.Stack())
			os.Exit(1)
		}
	}()

	S.Info = sysInfo()
	nbInit()
	update() // prime counters; rates need two samples
	say(OK, "reading live system data", false)
	go detailWorker()

	keys := make(chan string, 64)
	go readKeys(keys)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	frame := time.NewTicker(33 * time.Millisecond)
	defer frame.Stop()
	tick := time.NewTimer(500 * time.Millisecond)
	var sc Screen
	last := time.Now()
	for {
		select {
		case k, ok := <-keys:
			if !ok || handleKey(k) {
				restore()
				return
			}
		case <-sig:
			restore()
			return
		case d := <-detResC:
			applyDetail(d)
		case <-tick.C:
			if !S.Paused {
				update()
			}
			tick.Reset(time.Duration(S.Ms) * time.Millisecond)
		case <-frame.C:
			now := time.Now()
			dt := math.Min(.1, now.Sub(last).Seconds())
			last = now
			nbFrame(dt)
			w, h := termSize()
			if w != sc.W || h != sc.H {
				sc.Resize(w, h)
				out.WriteString("\x1b[0m\x1b[2J")
			}
			draw(&sc)
			sc.Flush(out)
		}
	}
}

// once prints one sample of everything as plain text (no TUI) to check the readings.
// With strict=true it exits with status 1 if any reading is missing or implausible (used by CI).
func once(strict bool) {
	S.Info = sysInfo()
	nbInit()
	update()
	time.Sleep(time.Second)
	update()
	i := S.Info
	fmt.Printf("host   %s · %s\ncpu    %s · %d threads · %.1f GiB RAM\n", i.Host, i.OS, i.CPU, i.Threads, i.MemGB)
	fmt.Printf("load   %.1f%% total", S.CPU*100)
	for k, c := range S.Cores {
		fmt.Printf(" C%d %.0f%%", k, c*100)
	}
	m := S.Mem
	fmt.Printf("\nmem    used %s of %s · cache %s · %s %s/%s\n", fBytes(float64(m.Total-m.Avail)), fBytes(float64(m.Total)), fBytes(float64(m.Cache)), swapLabel, fBytes(float64(m.SwapUsed)), fBytes(float64(m.SwapTotal)))
	for _, d := range S.Disks {
		fmt.Printf("disk   %s %s %s · %s free of %s\n", d.Mount, d.Label, d.FS, fBytes(float64(d.Free)), fBytes(float64(d.Total)))
	}
	for _, n := range S.Nets {
		fmt.Printf("net    %s · rx %s tx %s\n", n.Name, fRate(n.RxRate), fRate(n.TxRate))
	}
	fmt.Printf("procs  %d processes · %d threads · %d handles · %s\n", S.Count.Procs, S.Count.Threads, S.Count.Handles, fUptime(S.Up))
	if S.Bat >= 0 {
		fmt.Printf("bat    %d%% charging=%v\n", S.Bat, S.Chg)
	}
	fmt.Printf("source %s\n", procSource)
	a := list()
	for k := 0; k < len(a) && k < 8; k++ {
		p := a[k]
		path, user := procDetails(p.Pid)
		fmt.Printf("  %6d %-22s %5.1f%% %7s thr %-4d %-10s %s\n", p.Pid, trunc(p.Name, 22), p.CPU, fShort(p.Mem), p.Threads, trunc(user, 10), trunc(path, 60))
	}
	if !strict {
		return
	}
	var fails []string
	check := func(ok bool, what string) {
		if !ok {
			fails = append(fails, what)
		}
	}
	check(S.CPUOK && len(S.Cores) > 0, "cpu: no per-core readings")
	check(len(S.Cores) == runtime.NumCPU(), fmt.Sprintf("cpu: %d cores read, runtime reports %d", len(S.Cores), runtime.NumCPU()))
	check(S.Mem.Total > 0 && S.Mem.Avail <= S.Mem.Total, "mem: total/available implausible")
	check(len(S.Disks) > 0, "disks: none found")
	check(len(S.Procs) >= 10, fmt.Sprintf("procs: only %d processes", len(S.Procs)))
	check(S.Up > 0, "uptime: zero")
	check(procSource == wantProcSource, "procs read via "+procSource+", expected "+wantProcSource)
	self := S.Procs[os.Getpid()]
	check(self != nil, "procs: own process missing from the list")
	if self != nil {
		check(containsFold(self.Name, "shiptop"), "procs: own process name is "+self.Name)
		check(self.Mem > 1<<20, "procs: own working set below 1 MiB")
		check(self.Threads >= 1, "procs: own thread count is zero")
		path, user := procDetails(self.Pid)
		check(containsFold(path, "shiptop"), "details: own path is "+path)
		check(user != "", "details: own user is empty")
	}
	if len(fails) > 0 {
		fmt.Println("SELFTEST FAILED:")
		for _, f := range fails {
			fmt.Println("  -", f)
		}
		os.Exit(1)
	}
	fmt.Println("SELFTEST OK")
}

const helpText = `shiptop ` + version + ` · a btop-style monitor with live system data

usage: shiptop.exe           run inside Windows Terminal / PowerShell (bigger window = more detail)
       shiptop.exe --once    print one plain-text sample and exit
       shiptop.exe --selftest   same, but exit 1 if any reading is missing (used by CI)

keys
  ↑ ↓ PgUp PgDn  select process     ← →  sort column      r  reverse sort
  k / Del        terminate the selected process (asks y/n first)
  f              filter (Enter/Esc to finish)            Esc  clear filter
  p / Space      pause                                   + -  slower / faster updates
  b n            previous / next network adapter
  t              thermal / visible light on the notebook model
  z              zoom the notebook (arrows rotate while zoomed)
  q              quit

All numbers are read live from the system. The notebook temperatures are modelled from the
real CPU load (marked ~), because a console app can't read thermal sensors.
`

func readKeys(ch chan<- string) {
	buf := make([]byte, 512)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			close(ch)
			return
		}
		parseKeys(buf[:n], ch)
	}
}

func parseKeys(b []byte, ch chan<- string) {
	for i := 0; i < len(b); {
		c := b[i]
		if c == 0x1b {
			if i+1 < len(b) && (b[i+1] == '[' || b[i+1] == 'O') {
				j := i + 2
				for j < len(b) && !(b[j] >= 0x40 && b[j] <= 0x7e) {
					j++
				}
				if j < len(b) {
					switch string(b[i+2 : j+1]) {
					case "A":
						ch <- "up"
					case "B":
						ch <- "down"
					case "C":
						ch <- "right"
					case "D":
						ch <- "left"
					case "3~":
						ch <- "del"
					case "5~":
						ch <- "pgup"
					case "6~":
						ch <- "pgdn"
					case "H", "1~":
						ch <- "home"
					case "F", "4~":
						ch <- "end"
					}
					i = j + 1
					continue
				}
			}
			ch <- "esc"
			i++
			continue
		}
		switch {
		case c == 3:
			ch <- "ctrl-c"
		case c == 13 || c == 10:
			ch <- "enter"
		case c == 127 || c == 8:
			ch <- "bs"
		case c < 32:
		default:
			r, sz := utf8.DecodeRune(b[i:])
			ch <- string(r)
			i += sz
			continue
		}
		i++
	}
}

func handleKey(k string) bool {
	if k == "ctrl-c" {
		return true
	}
	if S.Confirm != 0 { // waiting for y/n: only "y" terminates
		if k == "y" || k == "Y" {
			doKill()
		} else {
			S.Confirm = 0
			say(DIM, "terminate cancelled", true)
		}
		return false
	}
	if S.Filtering {
		switch k {
		case "enter", "esc":
			S.Filtering = false
		case "bs":
			if r := []rune(S.Flt); len(r) > 0 {
				S.Flt = string(r[:len(r)-1])
			}
		default:
			if rlen(k) == 1 && k >= " " && rlen(S.Flt) < 30 {
				S.Flt += k
			}
		}
		return false
	}
	hold := func() { V.Hold = time.Now().Add(3 * time.Second) }
	switch k {
	case "q", "Q":
		return true
	case "esc":
		if V.Zoom {
			V.Zoom = false
		} else {
			S.Flt = ""
		}
	case "up":
		if V.Zoom {
			V.Pitch = clamp(V.Pitch-.08, .12, .95)
			hold()
		} else {
			move(-1)
		}
	case "down":
		if V.Zoom {
			V.Pitch = clamp(V.Pitch+.08, .12, .95)
			hold()
		} else {
			move(1)
		}
	case "pgup":
		move(-10)
	case "pgdn":
		move(10)
	case "home":
		move(-1 << 20)
	case "end":
		move(1 << 20)
	case "left", "right":
		d := 1
		if k == "left" {
			d = -1
		}
		if V.Zoom {
			V.Yaw += float64(d) * .18
			hold()
			break
		}
		i := 0
		for n, s := range sortKeys {
			if s == S.Sort {
				i = n
			}
		}
		S.Sort = sortKeys[(i+d+len(sortKeys))%len(sortKeys)]
	case "k", "K", "del":
		askKill()
	case "f", "/":
		S.Filtering = true
	case "p", " ":
		S.Paused = !S.Paused
	case "+", "=":
		S.Ms = clampi(S.Ms+250, 250, 5000)
	case "-", "_":
		S.Ms = clampi(S.Ms-250, 250, 5000)
	case "r":
		S.Rev = !S.Rev
	case "b", "n":
		if len(S.Nets) > 0 {
			d := 1
			if k == "b" {
				d = -1
			}
			S.Iface = (S.Iface + d + len(S.Nets)) % len(S.Nets)
			S.IfacePick = true
			flash("net")
		}
	case "t":
		V.Therm = !V.Therm
		flash("nb")
	case "z", "5":
		V.Zoom = !V.Zoom
	}
	return false
}

func boxCol(base uint32, key string) uint32 {
	if time.Now().Before(S.Flash[key]) {
		return lerp(base, 0xffffff, .45)
	}
	return base
}

func draw(sc *Screen) {
	sc.Clear()
	W, H := sc.W, sc.H
	if W < 80 || H < 24 {
		m := fmt.Sprintf("shiptop needs at least 80×24 · this window is %d×%d", W, H)
		sc.Str(maxi(0, (W-rlen(m))/2), H/2, m, WARN, W)
		sc.Str(maxi(0, (W-22)/2), H/2+1, "make the window bigger", DIM, W)
		return
	}
	drawHeader(sc)
	if V.Zoom {
		drawNB(sc, 0, 1, W, H-1)
		return
	}
	avail := H - 1
	topH := maxi(10, int(math.Round(float64(avail)*.42)))
	nbW := 0
	if W >= 100 {
		nbW = clampi(int(float64(W)*.34), 38, 76)
	}
	drawCPU(sc, 0, 1, W-nbW, topH)
	if nbW > 0 {
		drawNB(sc, W-nbW, 1, nbW, topH)
	}
	y0 := 1 + topH
	bh := H - y0
	lw := clampi(int(float64(W)*.44), 40, W-40)
	memH := bh * 52 / 100
	drawMem(sc, 0, y0, lw, memH)
	drawNet(sc, 0, y0+memH, lw, bh-memH)
	drawProc(sc, lw, y0, W-lw, bh)
}

func drawHeader(sc *Screen) {
	x := 1
	for i, r := range "shiptop" {
		sc.PutX(x+i, 0, r, gHeat.At(float64(i)/6), BG, true)
	}
	x = sc.Str(x+8, 0, "v"+version, DIM, 0) + 2
	host := S.Info.Host
	if sc.W >= 110 && S.Info.OS != "" {
		host += " · " + S.Info.OS
	}
	x = sc.Str(x, 0, trunc(host, sc.W/2), FG, 0)
	avail := sc.W - 1 - (x + 3)
	if avail > 6 && S.Msg != "" {
		sc.RStr(sc.W-1, 0, trunc("● "+S.Msg, avail), S.MsgC)
	}
}

func drawCPU(sc *Screen, x, y, w, h int) {
	col := boxCol(CCPU, "cpu")
	sc.Box(x, y, w, h, col)
	sc.Label(x+2, y, true, col, Seg{"¹", HI, false}, Seg{"cpu", TITLE, true})
	if w >= 60 {
		sc.Label(x+w/2-5, y, true, col, Seg{time.Now().Format("15:04:05"), TITLE, false})
	}
	ms := Seg{fmt.Sprintf(" %dms ", S.Ms), TITLE, false}
	if S.Paused {
		ms = Seg{" paused ", WARN, true}
	}
	sc.LabelR(x+w-2, y, true, col, Seg{"-", HI, false}, ms, Seg{"+", HI, false})
	ux := sc.LabelR(x+w-2, y+h-1, false, col, Seg{fUptime(S.Up), TITLE, false})
	cnt := fmt.Sprintf("procs %d · threads %s", S.Count.Procs, fCount(S.Count.Threads))
	if S.Count.Handles > 0 {
		cnt += " · handles " + fCount(S.Count.Handles)
	}
	if room := ux - (x + 2) - 3; room > 12 {
		sc.Label(x+2, y+h-1, false, col, Seg{trunc(cnt, room), DIM, false})
	}

	ix, iy, iw, ih := x+1, y+1, w-2, h-2
	n := len(S.Cores)
	if n == 0 {
		n = S.Info.Threads
	}
	subInner := ih - 2
	header := subInner >= 11
	rows := subInner
	if header {
		rows -= 2
	}
	rows = maxi(1, rows)
	ew := 13
	if n > 100 {
		ew = 14
	}
	cols := (n + rows - 1) / rows
	maxCols := maxi(1, (iw*55/100-4+2)/(ew+2))
	if cols > maxCols {
		cols = maxCols
	}
	show := mini(n, cols*rows)
	subW := cols*ew + (cols-1)*2 + 4
	if header && subW < 36 {
		subW = 36
	}
	gw := iw - subW - 1
	gh1 := int(math.Round(float64(ih) * .62))
	sc.Graph(ix, iy, gw, gh1, S.CPUH, 100, gHeat, false)
	sc.Graph(ix, iy+gh1, gw, ih-gh1, S.MemH, 100, gCyan, true)
	sc.Str(ix, iy, fmt.Sprintf("cpu %s ", pctS(S.CPU*100)), TITLE, gw)
	if S.Mem.Total > 0 {
		sc.Str(ix, iy+ih-1, fmt.Sprintf("ram %s · %s of %s ", pctS(last(S.MemH)), fBytes(float64(S.Mem.Total-S.Mem.Avail)), fBytes(float64(S.Mem.Total))), 0x7fb2ff, gw)
	}

	sx := ix + gw + 1
	scol := lerp(col, BG, .3)
	sc.Box(sx, iy, subW, ih, scol)
	sc.Label(sx+2, iy, true, scol, Seg{trunc(S.Info.CPU, subW-6), TITLE, false})
	if show < n {
		sc.LabelR(sx+subW-2, iy+ih-1, false, scol, Seg{fmt.Sprintf("+%d more", n-show), DIM, false})
	}
	if hi, hv := hottestCore(); hi >= 0 && subW >= 30 {
		sc.Label(sx+2, iy+ih-1, false, scol, Seg{"peak ", DIM, false}, Seg{fmt.Sprintf("C%d %s", hi, pctS(hv*100)), TITLE, false})
	}
	ry := iy + 1
	if header {
		mw := subW - 4 - 6 - 5
		sc.Str(sx+2, ry, "total", TITLE, 0)
		sc.Meter(sx+8, ry, mw, S.CPU, gHeat)
		sc.RStr(sx+subW-2, ry, pctS(S.CPU*100), FG)
		sc.Str(sx+2, ry+1, fmt.Sprintf("%d logical processors", S.Info.Threads), DIM, subW-4)
		ry += 2
	}
	for i := 0; i < show; i++ {
		c, rr := i/rows, i%rows
		ex, ey := sx+2+c*(ew+2), ry+rr
		v := 0.0
		if i < len(S.Cores) {
			v = S.Cores[i] * 100
		}
		var hst []float64
		if i < len(S.CoreH) {
			hst = S.CoreH[i]
		}
		lbl := fmt.Sprintf("C%02d", i)
		sc.Str(ex, ey, lbl, DIM, 0)
		sc.Spark(ex+rlen(lbl)+1, ey, hst, 5, 100, 100, gHeat)
		sc.RStr(ex+ew, ey, pctS(v), FG)
	}
}

func drawNB(sc *Screen, x, y, w, h int) {
	col := boxCol(CNB, "nb")
	sc.Box(x, y, w, h, col)
	sc.Label(x+2, y, true, col, Seg{"⁵", HI, false}, Seg{"notebook", TITLE, true})
	mode := "thermal"
	if !V.Therm {
		mode = "visible"
	}
	rx := sc.LabelR(x+w-2, y, true, col, Seg{"t ", HI, false}, Seg{mode, TITLE, false})
	zt := "zoom"
	if V.Zoom {
		zt = "back"
	}
	if w >= 44 {
		sc.LabelR(rx-1, y, true, col, Seg{"z ", HI, false}, Seg{zt, TITLE, false})
	}
	bx := sc.LabelR(x+w-2, y+h-1, false, col, Seg{"model ", DIM, false}, Seg{fmt.Sprintf("~%.1f°", NB.Max), TITLE, false})
	spec := fmt.Sprintf("%s · %d threads · %.0f GB", NB.CPUName, NB.N, NB.MemGB)
	if room := bx - (x + 2) - 3; room > 8 {
		sc.Label(x+2, y+h-1, false, col, Seg{trunc(spec, room), DIM, false})
	}
	ix, iy, iw, ih := x+1, y+1, w-2, h-2
	rl := 2
	if ih >= 12 {
		rl = 3
	}
	vh := ih - rl
	if vh < 3 {
		return
	}
	vw := iw - 5
	nbRender(vw, vh*2)
	blitPixels(sc, ix, iy)
	if V.Zoom {
		sc.Str(ix+1, iy+vh-1, "← → ↑ ↓ rotate · t thermal · z back", DIM, vw-2)
	}
	lx := ix + iw - 4
	sc.Str(lx, iy, fmt.Sprintf("%.0f°", spanHi), DIM, 0)
	for r := 1; r < vh-1; r++ {
		t := 1 - float64(r-1)/math.Max(1, float64(vh-3))
		c := LUT[clampi(int(t*255), 0, 255)]
		if !V.Therm {
			c = lerp(c, BG, .7)
		}
		sc.Put(lx+1, iy+r, '█', c)
	}
	sc.Str(lx, iy+vh-1, fmt.Sprintf("%.0f°", spanLo), DIM, 0)

	ram, bat := "n/a", "ac"
	if NB.Ram >= 0 {
		ram = pctS(NB.Ram * 100)
	}
	if NB.Bat >= 0 {
		bat = fmt.Sprintf("%d%%", NB.Bat)
		if NB.Chg {
			bat += "+"
		}
	}
	items := []struct {
		l, v string
		c    uint32
	}{
		{"load", pctS(NB.Load * 100), gHeat.At(NB.Load)},
		{"ram", ram, FG},
		{"bat", bat, FG},
		{"cpu", fmt.Sprintf("~%.0f°C", NB.Tc), gHeat.At((NB.Tc - 35) / 60)},
		{"gpu", fmt.Sprintf("~%.0f°C", NB.Tg), gHeat.At((NB.Tg - 35) / 60)},
		{"fan", fmt.Sprintf("~%.1fk", NB.Fan/1000), FG},
	}
	cw := iw / 3
	ry := iy + vh
	for i, it := range items {
		cx, cy := ix+1+(i%3)*cw, ry+i/3
		sc.Str(cx, cy, it.l, DIM, 0)
		sc.RStr(cx+cw-2, cy, it.v, it.c)
	}
	if rl == 3 {
		ty := ry + 2
		tx := sc.Str(ix+1, ty, "threads ", DIM, 0)
		fit := ix + iw - 1 - tx
		n := len(NB.Cores)
		shown := n
		if n > fit {
			shown = maxi(0, fit-5)
		}
		for i := 0; i < shown; i++ {
			sc.Put(tx+i, ty, '■', LUT[clampi(int(NB.Cores[i]*255), 0, 255)])
		}
		if shown < n {
			sc.Str(tx+shown+1, ty, fmt.Sprintf("+%d", n-shown), DIM, 0)
		}
	}
}

func drawMem(sc *Screen, x, y, w, h int) {
	col := boxCol(CMEM, "mem")
	sc.Box(x, y, w, h, col)
	sc.Label(x+2, y, true, col, Seg{"²", HI, false}, Seg{"mem", TITLE, true})
	m := S.Mem
	sc.LabelR(x+w-2, y, true, col, Seg{"Σ " + fBytes(float64(m.Total)), TITLE, false})
	ix, iy, iw, ih := x+1, y+1, w-2, h-2
	split := iw >= 44
	lw := iw - 1
	if split {
		lw = iw * 44 / 100
	}
	frac := func(a, b uint64) float64 {
		if b == 0 {
			return 0
		}
		return float64(a) / float64(b) * 100
	}
	items := []struct {
		l string
		v float64
		g Grad
	}{
		{"used", frac(sub(m.Total, m.Avail), m.Total), gHeat},
		{"available", frac(m.Avail, m.Total), gCyan},
		{"cached", frac(m.Cache, m.Total), gViolet},
		{swapLabel, frac(m.SwapUsed, m.SwapTotal), gRed},
	}
	for i := 0; i < mini(4, ih); i++ {
		it := items[i]
		sc.Str(ix+1, iy+i, it.l, FG, 9)
		mw := maxi(3, lw-16)
		sc.Meter(ix+11, iy+i, mw, it.v/100, it.g)
		sc.RStr(ix+lw, iy+i, pctS(it.v), FG)
	}
	if ih >= 5 {
		sc.Str(ix+1, iy+4, fmt.Sprintf("%s %s / %s", swapLabel, fBytes(float64(m.SwapUsed)), fBytes(float64(m.SwapTotal))), 0xe3c95a, lw-1)
	}
	if ih > 5 {
		sc.Graph(ix+1, iy+5, lw-1, ih-5, S.MemH, 100, gMem, false)
	}
	if !split {
		return
	}
	dx := ix + lw + 1
	for r := 0; r < ih; r++ {
		sc.Put(dx, iy+r, '│', FAINT)
	}
	hx := dx + 2
	hw := ix + iw - 1 - hx
	sc.Bold(hx, iy, "disks", TITLE, 0)
	if len(S.Disks) == 0 {
		sc.Str(hx, iy+1, "no local drives found", DIM, hw)
		return
	}
	two := ih-1 >= 2*len(S.Disks)
	szw := 0
	if hw >= 40 {
		szw = 9
	}
	r := 1
	for _, d := range S.Disks {
		if r >= ih {
			break
		}
		name := d.Mount
		if d.Label != "" {
			name += " " + d.Label
		}
		used := frac(sub(d.Total, d.Free), d.Total)
		sc.Str(hx, iy+r, trunc(name, 14), TITLE, 14)
		mw := maxi(2, hw-15-5-szw)
		sc.Meter(hx+15, iy+r, mw, used/100, gHeat)
		sc.RStr(hx+15+mw+5, iy+r, pctS(used), FG)
		if szw > 0 {
			sc.RStr(hx+hw, iy+r, fShort(d.Total), DIM)
		}
		r++
		if two && r < ih {
			sc.Str(hx+2, iy+r, fmt.Sprintf("%s free of %s · %s", fBytes(float64(d.Free)), fBytes(float64(d.Total)), d.FS), DIM, hw-2)
			r++
		}
	}
}

func drawNet(sc *Screen, x, y, w, h int) {
	col := boxCol(CNET, "net")
	sc.Box(x, y, w, h, col)
	sc.Label(x+2, y, true, col, Seg{"³", HI, false}, Seg{"net", TITLE, true})
	ix, iy, iw, ih := x+1, y+1, w-2, h-2
	if len(S.Nets) == 0 {
		sc.Str(ix+1, iy, "no network adapters found", DIM, iw-2)
		return
	}
	f := S.Nets[S.Iface]
	sc.LabelR(x+w-2, y, true, col, Seg{"‹b ", HI, false}, Seg{trunc(f.Name, w/2-8), TITLE, true}, Seg{" n›", HI, false})
	totRx, totTx := sub(f.lastRx, f.baseRx), sub(f.lastTx, f.baseTx)
	sc.Label(x+2, y+h-1, false, col, Seg{trunc(fmt.Sprintf("since start ▼ %s ▲ %s", fBytes(float64(totRx)), fBytes(float64(totTx))), w-6), DIM, false})
	sw := 0
	if iw >= 46 {
		sw = 23
	}
	gw := iw
	if sw > 0 {
		gw = iw - sw - 1
	}
	gh1 := (ih + 1) / 2
	sc.Graph(ix, iy, gw, gh1, f.Rx, math.Max(1024, maxOf(f.Rx, gw*2))*1.12, gRx, false)
	sc.Graph(ix, iy+gh1, gw, ih-gh1, f.Tx, math.Max(1024, maxOf(f.Tx, gw*2))*1.12, gTx, true)
	sc.Str(ix, iy, "▼ "+fRate(f.RxRate)+" ", 0xd79ae8, gw)
	sc.Str(ix, iy+ih-1, "▲ "+fRate(f.TxRate)+" ", 0x6fdbe8, gw)
	if sw == 0 {
		return
	}
	sx := ix + gw + 1
	scol := lerp(col, BG, .3)
	sc.Box(sx, iy, sw, ih, scol)
	sc.Label(sx+2, iy, true, scol, Seg{fmt.Sprintf("%d/%d", S.Iface+1, len(S.Nets)), TITLE, false})
	lines := []struct {
		l, v string
		c    uint32
	}{
		{"▼ rx", fRate(f.RxRate), 0xd79ae8},
		{"▲ tx", fRate(f.TxRate), 0x6fdbe8},
		{"top ▼", fRate(maxOf(f.Rx, len(f.Rx))), DIM},
		{"top ▲", fRate(maxOf(f.Tx, len(f.Tx))), DIM},
		{"Σ ▼", fBytes(float64(totRx)), DIM},
		{"Σ ▲", fBytes(float64(totTx)), DIM},
	}
	for i := 0; i < mini(len(lines), ih-2); i++ {
		ln := lines[i]
		sc.Str(sx+2, iy+1+i, ln.l, ln.c, 0)
		sc.RStr(sx+sw-2, iy+1+i, trunc(ln.v, sw-4-rlen(ln.l)-1), FG)
	}
}

type pcol struct {
	key, title string
	w          int
	right      bool
}

func drawProc(sc *Screen, x, y, w, h int) {
	col := boxCol(CPROC, "proc")
	sc.Box(x, y, w, h, col)
	sc.Label(x+2, y, true, col, Seg{"⁴", HI, false}, Seg{"proc", TITLE, true})
	revC := uint32(DIM)
	if S.Rev {
		revC = TITLE
	}
	rx := sc.LabelR(x+w-2, y, true, col, Seg{"r ", HI, false}, Seg{"reverse", revC, false})
	fl := []Seg{{"f ", HI, false}, {"filter", DIM, false}}
	if S.Filtering || S.Flt != "" {
		cursor := ""
		if S.Filtering && time.Now().UnixMilli()/500%2 == 0 {
			cursor = "▏"
		}
		fl = append(fl, Seg{": " + S.Flt + cursor, TITLE, true})
	}
	sc.LabelR(rx-1, y, true, col, fl...)

	a := list()
	idx := -1
	for i, p := range a {
		if p.Pid == S.Sel {
			idx = i
		}
	}
	if idx < 0 && len(a) > 0 {
		idx = mini(S.Si, len(a)-1)
		S.Sel = a[idx].Pid
	}
	if idx >= 0 {
		S.Si = idx
	}
	cx := sc.LabelR(x+w-2, y+h-1, false, col, Seg{fmt.Sprintf("%d/%d", len(a), len(S.Procs)), TITLE, false})
	keys := "↑↓ select  ←→ sort  k terminate  f filter  p pause  t thermal  z zoom  q quit"
	if room := cx - (x + 2) - 3; room > 10 {
		sc.Label(x+2, y+h-1, false, col, Seg{trunc(keys, room), DIM, false})
	}

	ix, iy, iw, ih := x+1, y+1, w-2, h-2
	dh := 5
	if ih < 14 {
		dh = 4
	}
	dcol := lerp(col, BG, .35)
	sc.Box(ix, iy, iw, dh, dcol)
	if p := cur(); p != nil {
		wantDetail(p)
		sc.Label(ix+2, iy, true, dcol, Seg{fmt.Sprintf("%d ", p.Pid), DIM, false}, Seg{trunc(p.Name, iw/2), TITLE, true})
		sc.LabelR(ix+iw-2, iy, true, dcol, Seg{"cpu ", DIM, false}, Seg{fmt.Sprintf("%.1f%%", p.CPU), TITLE, false})
		gwid := 0
		if iw >= 56 {
			gwid = iw * 28 / 100
		}
		tw := iw - 4 - gwid
		if gwid > 0 {
			tw--
		}
		user := p.User
		if user == "" {
			user = "—"
		}
		state := "idle"
		if p.CPU >= .05 {
			state = "active"
		}
		meta := fmt.Sprintf("%s · %s · thr %d · mem %s · ppid %d", state, user, p.Threads, fBytes(float64(p.Mem)), p.PPid)
		path := p.Path
		if path == "" {
			if p.known {
				path = "(path not accessible without admin rights)"
			} else {
				path = "…"
			}
		}
		ly := iy + 1
		sc.Str(ix+2, ly, meta, FG, tw)
		ly++
		if dh == 5 {
			sc.Str(ix+2, ly, path, DIM, tw)
			ly++
		}
		ax := ix + 2
		if S.Confirm == p.Pid {
			ax = sc.Bold(ax, ly, trunc(fmt.Sprintf("terminate %s (%d)? ", p.Name, p.Pid), tw-18), WARN, 0)
			ax = sc.Str(ax, ly, "[", DIM, 0)
			ax = sc.Str(ax, ly, "y", HI, 0)
			ax = sc.Str(ax, ly, " yes] [", DIM, 0)
			ax = sc.Str(ax, ly, "n", HI, 0)
			sc.Str(ax, ly, " no]", DIM, 0)
		} else {
			ax = sc.Str(ax, ly, "[", DIM, 0)
			ax = sc.Str(ax, ly, "k", HI, 0)
			ax = sc.Str(ax, ly, " terminate", TITLE, 0)
			ax = sc.Str(ax, ly, "]", DIM, 0) + 2
			if S.DMsg != "" && ix+2+tw-ax > 6 {
				sc.Str(ax, ly, trunc("» "+S.DMsg, ix+2+tw-ax), S.DMsgC, 0)
			}
		}
		if gwid > 0 {
			gx := ix + iw - 1 - gwid
			sc.Graph(gx, iy+1, gwid, dh-2, p.H, math.Max(5, maxOf(p.H, len(p.H)))*1.1, gHeat, false)
		}
	} else {
		sc.Str(ix+2, iy+1, "no process selected", DIM, 0)
	}

	inner := iw - 2
	cols := []pcol{{"pid", "Pid", 7, true}, {"name", "Program", 16, false}, {"path", "Path", 0, false}, {"thr", "Thr", 5, true}, {"user", "User", 10, false}, {"mem", "MemB", 6, true}, {"cpu", "Cpu%", 5, true}, {"spark", "", 5, false}}
	fixed := 0
	for _, c := range cols {
		fixed += c.w
	}
	pathW := inner - fixed - (len(cols) - 1)
	var use []pcol
	for _, c := range cols {
		switch {
		case c.key == "path" && pathW < 12:
			continue
		case (c.key == "thr" || c.key == "spark") && inner < 56:
			continue
		case c.key == "path":
			c.w = pathW
		}
		use = append(use, c)
	}
	if pathW < 12 {
		fx := 0
		for _, c := range use {
			fx += c.w
		}
		extra := inner - fx - (len(use) - 1)
		for i := range use {
			if use[i].key == "name" {
				use[i].w += clampi(extra, 0, 14)
			}
		}
	}
	hy := iy + dh
	cx0 := ix + 1
	px := cx0
	for _, c := range use {
		t := c.title
		fg := uint32(TITLE)
		if c.key == S.Sort {
			fg = HI
			desc := c.key == "cpu" || c.key == "mem" || c.key == "thr"
			if desc != S.Rev {
				t += "▾"
			} else {
				t += "▴"
			}
		}
		if c.right {
			sc.Bold(px+c.w-rlen(t), hy, t, fg, 0)
		} else {
			sc.Bold(px, hy, t, fg, c.w)
		}
		px += c.w + 1
	}
	nrows := ih - dh - 1
	if nrows <= 0 {
		return
	}
	if len(a) == 0 {
		msg := "reading processes…"
		if S.Flt != "" {
			msg = "no process matches “" + S.Flt + "”"
		}
		sc.Str(cx0, hy+1, msg, DIM, inner)
		return
	}
	if idx < S.Scroll {
		S.Scroll = idx
	}
	if idx >= S.Scroll+nrows {
		S.Scroll = idx - nrows + 1
	}
	S.Scroll = clampi(S.Scroll, 0, maxi(0, len(a)-nrows))
	for r := 0; r < nrows && S.Scroll+r < len(a); r++ {
		p := a[S.Scroll+r]
		wantDetail(p)
		ry := hy + 1 + r
		fg := uint32(FG)
		sel := p.Pid == S.Sel
		if sel {
			sc.SetBG(ix, ry, iw, SEL)
			fg = TITLE
		}
		px := cx0
		for _, c := range use {
			var v string
			cf := fg
			switch c.key {
			case "pid":
				v = fmt.Sprint(p.Pid)
			case "name":
				v = p.Name
			case "path":
				v = p.Path
				if !sel {
					cf = DIM
				}
			case "thr":
				v = fmt.Sprint(p.Threads)
			case "user":
				v = p.User
				if !sel {
					cf = DIM
				}
			case "mem":
				v = fShort(p.Mem)
			case "cpu":
				v = fmt.Sprintf("%.1f", p.CPU)
				cf = gHeat.At(p.CPU / 40)
				if p.CPU < .05 && !sel {
					cf = DIM
				}
			case "spark":
				sc.Spark(px, ry, p.H, 5, math.Max(5, maxOf(p.H, 5)), 50, gHeat)
				px += c.w + 1
				continue
			}
			v = trunc(strings.TrimSpace(v), c.w)
			if c.right {
				sc.Str(px+c.w-rlen(v), ry, v, cf, 0)
			} else {
				sc.Str(px, ry, v, cf, 0)
			}
			px += c.w + 1
		}
	}
}
