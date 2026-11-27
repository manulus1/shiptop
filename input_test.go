package main

import (
	"reflect"
	"testing"
)

func keysOf(chunks ...string) []string {
	ch := make(chan string, 64)
	var rest []byte
	for _, c := range chunks {
		rest = parseKeys(append(rest, c...), ch)
	}
	close(ch)
	var out []string
	for k := range ch {
		out = append(out, k)
	}
	return out
}

func TestParseKeys(t *testing.T) {
	cases := []struct {
		in   []string
		want []string
	}{
		{[]string{"q"}, []string{"q"}},
		{[]string{"\x1b[A\x1b[B\x1b[C\x1b[D"}, []string{"up", "down", "right", "left"}},
		{[]string{"\x1b[1;2C\x1b[1;5A\x1b[1;3D\x1b[Z\t"}, []string{"shift-right", "ctrl-up", "alt-left", "shift-tab", "tab"}},
		{[]string{"\x1b[<0;12;5M\x1b[<0;12;5m"}, []string{"mouse 0 11 4 M", "mouse 0 11 4 m"}},
		{[]string{"\x1b[<32;1", "0;3M"}, []string{"mouse 32 9 2 M"}}, // split across two reads
		{[]string{"\xc3", "\xa4x"}, []string{"ä", "x"}},
		{[]string{"\x1b", "q"}, []string{"esc", "q"}},
		{[]string{"\x1b[3~\x1b[5~\x1b[6~\x1b[H\x1b[F\r"}, []string{"del", "pgup", "pgdn", "home", "end", "enter"}},
	}
	for _, c := range cases {
		if got := keysOf(c.in...); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q: got %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseMouse(t *testing.T) {
	cases := []struct {
		seq  string
		want Mouse
	}{
		{"\x1b[<0;5;7M", Mouse{X: 4, Y: 6, Press: true}},
		{"\x1b[<0;5;7m", Mouse{X: 4, Y: 6, Release: true}},
		{"\x1b[<32;6;7M", Mouse{X: 5, Y: 6, Drag: true}},
		{"\x1b[<35;6;7M", Mouse{X: 5, Y: 6, Button: 3}}, // plain motion: nothing
		{"\x1b[<64;2;2M", Mouse{X: 1, Y: 1, WheelUp: true}},
		{"\x1b[<65;2;2M", Mouse{X: 1, Y: 1, WheelDown: true, Button: 1}},
		{"\x1b[<2;3;3M", Mouse{X: 2, Y: 2, Press: true, Button: 2}},
		{"\x1b[<20;3;3M", Mouse{X: 2, Y: 2, Press: true, Shift: true, Ctrl: true}},
	}
	for _, c := range cases {
		ks := keysOf(c.seq)
		if len(ks) != 1 {
			t.Fatalf("%q: keys %q", c.seq, ks)
		}
		m, ok := parseMouse(ks[0])
		if !ok || m != c.want {
			t.Errorf("%q: got %+v, want %+v", c.seq, m, c.want)
		}
	}
	if _, ok := parseMouse("m"); ok {
		t.Fatal("the m key is not a mouse event")
	}
}
