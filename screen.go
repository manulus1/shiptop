package main

import (
	"io"
	"math"
	"strconv"
	"unicode/utf8"
)

// Colours (0xRRGGBB)
const (
	BG     = 0x07090d
	FG     = 0xc3ccd8
	DIM    = 0x667084
	FAINT  = 0x222833
	TITLE  = 0xf2f5f9
	HI     = 0xff5c6c
	SEL    = 0x4a1f2a
	OK     = 0x3ee39b
	WARN   = 0xe8d356
	BAD    = 0xff4b5c
	CCPU   = 0x3c9b6f
	CMEM   = 0xa3923d
	CNET   = 0x7166c8
	CPROC  = 0xb4544f
	CNB    = 0xc0703a
	METER0 = 0x262c37
)

type Grad [3]uint32

var (
	gHeat   = Grad{0x3ee39b, 0xe8d356, 0xff4b5c}
	gShield = Grad{0x24d0ff, 0x4f7bff, 0xb46bff}
	gRx     = Grad{0x6f63d6, 0xc76fe0, 0xff6fae}
	gTx     = Grad{0x1fae96, 0x3fd0e0, 0xb4f6ff}
	gMem    = Grad{0x8d7f35, 0xe3c95a, 0xfff0b8}
	gCyan   = Grad{0x24d0ff, 0x3a9bff, 0x4f7bff}
	gViolet = Grad{0x8f6bff, 0xb873e8, 0xe07ad0}
	gRed    = Grad{0xff9a5a, 0xff7259, 0xff4b5c}
)

func lerp(a, b uint32, u float64) uint32 {
	u = clamp(u, 0, 1)
	var o uint32
	for sh := 0; sh <= 16; sh += 8 {
		x := float64((a >> sh) & 255)
		y := float64((b >> sh) & 255)
		o |= uint32(math.Round(x+(y-x)*u)) << sh
	}
	return o
}

func (g Grad) At(t float64) uint32 {
	t = clamp(t, 0, 1)
	if t < .5 {
		return lerp(g[0], g[1], t*2)
	}
	return lerp(g[1], g[2], t*2-1)
}

type Cell struct {
	R      rune
	FG, BG uint32
	B      bool
}

type Screen struct {
	W, H int
	C, P []Cell
	buf  []byte
}

func (s *Screen) Resize(w, h int) {
	s.W, s.H = w, h
	s.C = make([]Cell, w*h)
	s.P = nil
}

func (s *Screen) Clear() {
	for i := range s.C {
		s.C[i] = Cell{' ', FG, BG, false}
	}
}

func (s *Screen) Put(x, y int, r rune, fg uint32) {
	if x < 0 || y < 0 || x >= s.W || y >= s.H {
		return
	}
	c := &s.C[y*s.W+x]
	c.R, c.FG, c.B = r, fg, false
}

func (s *Screen) PutX(x, y int, r rune, fg, bg uint32, bold bool) {
	if x < 0 || y < 0 || x >= s.W || y >= s.H {
		return
	}
	s.C[y*s.W+x] = Cell{r, fg, bg, bold}
}

func (s *Screen) SetBG(x, y, w int, bg uint32) {
	for i := 0; i < w; i++ {
		if x+i >= 0 && x+i < s.W && y >= 0 && y < s.H {
			s.C[y*s.W+x+i].BG = bg
		}
	}
}

// Str writes text (keeping the cell background), at most max cells (0 = no limit); returns the next x.
func (s *Screen) Str(x, y int, str string, fg uint32, max int) int {
	n := 0
	for _, r := range str {
		if max > 0 && n >= max {
			break
		}
		s.Put(x+n, y, r, fg)
		n++
	}
	return x + n
}

func (s *Screen) Bold(x, y int, str string, fg uint32, max int) int {
	n := 0
	for _, r := range str {
		if max > 0 && n >= max {
			break
		}
		if x+n >= 0 && x+n < s.W && y >= 0 && y < s.H {
			c := &s.C[y*s.W+x+n]
			c.R, c.FG, c.B = r, fg, true
		}
		n++
	}
	return x + n
}

// RStr right-aligns text so it ends at x (exclusive).
func (s *Screen) RStr(x, y int, str string, fg uint32) {
	s.Str(x-utf8.RuneCountInString(str), y, str, fg, 0)
}

func rlen(s string) int { return utf8.RuneCountInString(s) }

func trunc(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if rlen(s) <= n {
		return s
	}
	r := []rune(s)
	if n == 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
}

func padL(s string, n int) string {
	for rlen(s) < n {
		s = " " + s
	}
	return s
}

func (s *Screen) Box(x, y, w, h int, col uint32) {
	if w < 2 || h < 2 {
		return
	}
	for i := 1; i < w-1; i++ {
		s.Put(x+i, y, '─', col)
		s.Put(x+i, y+h-1, '─', col)
	}
	for j := 1; j < h-1; j++ {
		s.Put(x, y+j, '│', col)
		s.Put(x+w-1, y+j, '│', col)
	}
	s.Put(x, y, '╭', col)
	s.Put(x+w-1, y, '╮', col)
	s.Put(x, y+h-1, '╰', col)
	s.Put(x+w-1, y+h-1, '╯', col)
}

type Seg struct {
	T string
	C uint32
	B bool
}

func segLen(segs []Seg) int {
	n := 2
	for _, s := range segs {
		n += rlen(s.T)
	}
	return n
}

// Label draws a btop style title on a border: ┐title┌ on top, ┘title└ at the bottom.
func (s *Screen) Label(x, y int, top bool, bcol uint32, segs ...Seg) int {
	l, r := '┐', '┌'
	if !top {
		l, r = '┘', '└'
	}
	s.Put(x, y, l, bcol)
	n := 1
	for _, sg := range segs {
		for _, ru := range sg.T {
			if sg.B {
				s.Bold(x+n, y, string(ru), sg.C, 0)
			} else {
				s.Put(x+n, y, ru, sg.C)
			}
			n++
		}
	}
	s.Put(x+n, y, r, bcol)
	return n + 1
}

// LabelR draws a label that ends at xEnd (exclusive).
func (s *Screen) LabelR(xEnd, y int, top bool, bcol uint32, segs ...Seg) int {
	w := segLen(segs)
	s.Label(xEnd-w, y, top, bcol, segs...)
	return xEnd - w
}

var leftBit = [4]rune{0x01, 0x02, 0x04, 0x40}
var rightBit = [4]rune{0x08, 0x10, 0x20, 0x80}

// Graph draws a braille graph: two samples per cell column, four dot rows per cell, coloured by height.
func (s *Screen) Graph(x, y, w, h int, data []float64, max float64, g Grad, inv bool) {
	if w <= 0 || h <= 0 {
		return
	}
	cols, dots := w*2, h*4
	hs := make([]int, cols)
	for c := 0; c < cols; c++ {
		k := len(data) - cols + c
		if k < 0 {
			continue
		}
		v := data[k] / max
		if v <= 0 {
			continue
		}
		d := int(math.Round(clamp(v, 0, 1) * float64(dots)))
		if d < 1 {
			d = 1
		}
		hs[c] = d
	}
	for cy := 0; cy < h; cy++ {
		t := (float64(h-1-cy) + .5) / float64(h)
		if inv {
			t = (float64(cy) + .5) / float64(h)
		}
		fg := g.At(t)
		for cx := 0; cx < w; cx++ {
			var bits rune
			for r := 0; r < 4; r++ {
				idx := (h-1-cy)*4 + (3 - r)
				if inv {
					idx = cy*4 + r
				}
				if hs[cx*2] > idx {
					bits |= leftBit[r]
				}
				if hs[cx*2+1] > idx {
					bits |= rightBit[r]
				}
			}
			if bits != 0 {
				s.Put(x+cx, y+cy, 0x2800+bits, fg)
			}
		}
	}
}

func (s *Screen) Meter(x, y, w int, v float64, g Grad) {
	for i := 0; i < w; i++ {
		fg := uint32(METER0)
		if float64(i)+.5 < v*float64(w) {
			fg = g.At(float64(i) / math.Max(1, float64(w-1)))
		}
		s.Put(x+i, y, '■', fg)
	}
}

var blocks = []rune("▁▂▃▄▅▆▇█")

func (s *Screen) Spark(x, y int, h []float64, n int, max, cmax float64, g Grad) {
	for i := 0; i < n; i++ {
		k := len(h) - n + i
		v := 0.0
		if k >= 0 {
			v = h[k]
		}
		b := int(math.Round(clamp(v/max, 0, 1) * 7))
		s.Put(x+i, y, blocks[b], g.At(v/cmax))
	}
}

func appendRGB(b []byte, pre string, c uint32) []byte {
	b = append(b, pre...)
	b = strconv.AppendInt(b, int64((c>>16)&255), 10)
	b = append(b, ';')
	b = strconv.AppendInt(b, int64((c>>8)&255), 10)
	b = append(b, ';')
	b = strconv.AppendInt(b, int64(c&255), 10)
	return b
}

// Flush writes only the cells that changed since the last frame.
func (s *Screen) Flush(w io.Writer) {
	b := s.buf[:0]
	b = append(b, "\x1b[?2026h"...)
	full := len(s.P) != len(s.C)
	if full {
		s.P = make([]Cell, len(s.C))
	}
	var lf, lb uint32 = 0xffffffff, 0xffffffff
	lbold := false
	cx, cy := -1, -1
	for y := 0; y < s.H; y++ {
		for x := 0; x < s.W; x++ {
			i := y*s.W + x
			c := s.C[i]
			if !full && c == s.P[i] {
				continue
			}
			if cx != x || cy != y {
				b = append(b, "\x1b["...)
				b = strconv.AppendInt(b, int64(y+1), 10)
				b = append(b, ';')
				b = strconv.AppendInt(b, int64(x+1), 10)
				b = append(b, 'H')
			}
			if c.FG != lf || c.BG != lb || c.B != lbold {
				b = append(b, "\x1b[0"...)
				if c.B {
					b = append(b, ";1"...)
				}
				b = appendRGB(b, ";38;2;", c.FG)
				b = appendRGB(b, ";48;2;", c.BG)
				b = append(b, 'm')
				lf, lb, lbold = c.FG, c.BG, c.B
			}
			b = utf8.AppendRune(b, c.R)
			cx, cy = x+1, y
			s.P[i] = c
		}
	}
	b = append(b, "\x1b[?2026l"...)
	w.Write(b)
	s.buf = b
}
