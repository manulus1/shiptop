package main

import (
	"math/rand"
	"testing"
)

func TestMosaicTables(t *testing.T) {
	for _, m := range pixModes {
		if len(m.Runes) != 1<<(m.CX*m.CY) {
			t.Fatalf("%s: %d characters for %d sub-pixels", m.ID, len(m.Runes), m.CX*m.CY)
		}
		seen := map[rune]bool{}
		for p, r := range m.Runes {
			if seen[r] {
				t.Fatalf("%s: %q used twice", m.ID, r)
			}
			seen[r] = true
			if cellRune(r) != r {
				t.Fatalf("%s: pattern %d character %U would be replaced on screen", m.ID, p, r)
			}
		}
		if m.Runes[0] != ' ' || m.Runes[len(m.Runes)-1] != '█' {
			t.Fatalf("%s: empty and full must be space and full block", m.ID)
		}
	}
	// spot checks against the plain block elements
	checks := []struct {
		r    []rune
		bits int
		want rune
	}{
		{halfRunes[:], 1, '▀'}, {halfRunes[:], 2, '▄'},
		{quadRunes[:], 0b0001, '▘'}, {quadRunes[:], 0b1001, '▚'}, {quadRunes[:], 0b0101, '▌'},
		{sextRunes[:], 0b010101, '▌'}, {sextRunes[:], 0b101010, '▐'}, {sextRunes[:], 1, 0x1FB00},
		{octRunes[:], 0x0F, '▀'}, {octRunes[:], 0xF0, '▄'}, {octRunes[:], 0x55, '▌'}, {octRunes[:], 0xAA, '▐'},
		{octRunes[:], 0x05, '▘'}, {octRunes[:], 0xC0, '▂'}, {octRunes[:], 0x04, 0x1CD00}, // BLOCK OCTANT-3
	}
	for _, c := range checks {
		if c.r[c.bits] != c.want {
			t.Errorf("pattern %b is %U, want %U", c.bits, c.r[c.bits], c.want)
		}
	}
	for p := 1; p < 63; p++ { // the well-known sextant numbering
		if p != 21 && p != 42 {
			want := rune(0x1FB00 + p - 1)
			if p > 21 {
				want--
			}
			if p > 42 {
				want--
			}
			if sextRunes[p] != want {
				t.Fatalf("sextant %d is %U, want %U", p, sextRunes[p], want)
			}
		}
	}
}

func TestSplit2(t *testing.T) {
	red, blue := [3]float64{220, 30, 30}, [3]float64{20, 40, 200}
	c := [][3]float64{red, blue, blue, red, red, red, blue, blue}
	bits, fg, bg := split2(c)
	got := map[int]bool{}
	for i := range c {
		got[i] = bits&(1<<i) != 0
	}
	for i := range c {
		if got[i] != got[0] && c[i] == c[0] || got[i] == got[0] && c[i] != c[0] {
			t.Fatalf("bits %08b do not separate the two colours", bits)
		}
	}
	if (fg != 0xdc1e1e || bg != 0x1428c8) && (fg != 0x1428c8 || bg != 0xdc1e1e) {
		t.Fatalf("colours %06x %06x", fg, bg)
	}
	one := [][3]float64{{10, 10, 10}, {11, 10, 10}, {10, 12, 10}, {10, 10, 9}}
	if bits, fg, bg := split2(one); bits != 0 || fg != bg {
		t.Fatalf("one colour should give an empty pattern: %b %06x %06x", bits, fg, bg)
	}
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 2000; i++ { // the split never does worse than painting the cell one colour
		n := []int{2, 4, 6, 8}[rng.Intn(4)]
		c := make([][3]float64, n)
		for k := range c {
			c[k] = [3]float64{rng.Float64() * 255, rng.Float64() * 255, rng.Float64() * 255}
		}
		bits, fg, bg := split2(c)
		unpack := func(u uint32) [3]float64 {
			return [3]float64{float64(u >> 16 & 255), float64(u >> 8 & 255), float64(u & 255)}
		}
		f, b := unpack(fg), unpack(bg)
		var errSplit, errOne float64
		var mean [3]float64
		for _, v := range c {
			for j := range mean {
				mean[j] += v[j] / float64(n)
			}
		}
		for k, v := range c {
			ref := b
			if bits&(1<<k) != 0 {
				ref = f
			}
			for j := 0; j < 3; j++ {
				errSplit += (v[j] - ref[j]) * (v[j] - ref[j])
				errOne += (v[j] - mean[j]) * (v[j] - mean[j])
			}
		}
		if errSplit > errOne+3*float64(n) {
			t.Fatalf("split error %.0f is worse than one colour %.0f", errSplit, errOne)
		}
	}
}
