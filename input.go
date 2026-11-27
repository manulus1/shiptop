package main

import (
	"os"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Keyboard and mouse input. Keys arrive as names ("up", "shift-left", "tab", "q"); mouse reports
// (SGR encoding, enabled with ?1000h ?1002h ?1006h) arrive as "mouse <code> <x> <y> <M|m>" with
// 0-based cell coordinates and are decoded by parseMouse.

const mouseOn = "\x1b[?1000h\x1b[?1002h\x1b[?1006h"
const mouseOff = "\x1b[?1006l\x1b[?1002l\x1b[?1000l\x1b[?1003l"

func readKeys(ch chan<- string) {
	buf := make([]byte, 1024)
	var rest []byte
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			close(ch)
			return
		}
		rest = parseKeys(append(rest, buf[:n]...), ch)
		if len(rest) > 64 { // not a sequence we will ever finish: drop it
			rest = nil
		}
	}
}

var arrowName = map[byte]string{'A': "up", 'B': "down", 'C': "right", 'D': "left", 'H': "home", 'F': "end"}

// parseKeys turns raw input into key names. An escape sequence cut off at the end of the
// buffer is returned so the caller can complete it with the next read.
func parseKeys(b []byte, ch chan<- string) []byte {
	for i := 0; i < len(b); {
		c := b[i]
		if c == 0x1b {
			if i+1 < len(b) && (b[i+1] == '[' || b[i+1] == 'O') {
				j := i + 2
				for j < len(b) && !(b[j] >= 0x40 && b[j] <= 0x7e) {
					j++
				}
				if j >= len(b) {
					return append([]byte(nil), b[i:]...) // incomplete: wait for the rest
				}
				if k := csiKey(string(b[i+2 : j+1])); k != "" {
					ch <- k
				}
				i = j + 1
				continue
			}
			ch <- "esc"
			i++
			continue
		}
		switch {
		case c == 3:
			ch <- "ctrl-c"
		case c == 9:
			ch <- "tab"
		case c == 13 || c == 10:
			ch <- "enter"
		case c == 127 || c == 8:
			ch <- "bs"
		case c < 32:
		default:
			if !utf8.FullRune(b[i:]) {
				return append([]byte(nil), b[i:]...)
			}
			r, sz := utf8.DecodeRune(b[i:])
			ch <- string(r)
			i += sz
			continue
		}
		i++
	}
	return nil
}

// csiKey names the key for the body of a CSI/SS3 sequence (everything after "ESC [").
func csiKey(seq string) string {
	if strings.HasPrefix(seq, "<") { // SGR mouse: <code;x;y then M (press/motion) or m (release)
		f := strings.Split(seq[1:len(seq)-1], ";")
		if len(f) != 3 {
			return ""
		}
		code, e1 := strconv.Atoi(f[0])
		x, e2 := strconv.Atoi(f[1])
		y, e3 := strconv.Atoi(f[2])
		if e1 != nil || e2 != nil || e3 != nil {
			return ""
		}
		return "mouse " + strconv.Itoa(code) + " " + strconv.Itoa(x-1) + " " + strconv.Itoa(y-1) + " " + seq[len(seq)-1:]
	}
	switch seq {
	case "Z":
		return "shift-tab"
	case "3~":
		return "del"
	case "5~":
		return "pgup"
	case "6~":
		return "pgdn"
	case "1~":
		return "home"
	case "4~":
		return "end"
	}
	last := seq[len(seq)-1]
	name, ok := arrowName[last]
	if !ok {
		return ""
	}
	body := seq[:len(seq)-1]
	if body == "" {
		return name
	}
	// "1;2A" = shift+up, 3 = alt, 5 = ctrl (xterm modifier = 1 + shift(1) + alt(2) + ctrl(4))
	if i := strings.LastIndexByte(body, ';'); i >= 0 {
		m, err := strconv.Atoi(body[i+1:])
		if err != nil || m < 2 {
			return name
		}
		m--
		switch {
		case m&4 != 0:
			return "ctrl-" + name
		case m&2 != 0:
			return "alt-" + name
		case m&1 != 0:
			return "shift-" + name
		}
	}
	return name
}

type Mouse struct {
	X, Y                 int
	Press, Release, Drag bool
	WheelUp, WheelDown   bool
	Button               int // 0 left, 1 middle, 2 right
	Shift, Ctrl          bool
}

// parseMouse decodes a "mouse …" key; ok is false for anything else.
func parseMouse(k string) (m Mouse, ok bool) {
	f := strings.Fields(k)
	if len(f) != 5 || f[0] != "mouse" {
		return m, false
	}
	code, _ := strconv.Atoi(f[1])
	m.X, _ = strconv.Atoi(f[2])
	m.Y, _ = strconv.Atoi(f[3])
	m.Shift, m.Ctrl = code&4 != 0, code&16 != 0
	m.Button = code & 3
	switch {
	case code&64 != 0:
		m.WheelUp, m.WheelDown = code&1 == 0, code&1 == 1
		if code&2 != 0 { // horizontal wheel: ignore
			m.WheelUp, m.WheelDown = false, false
		}
	case code&32 != 0:
		m.Drag = m.Button != 3 // motion with a button held (3 = no button: plain hover)
	case f[4] == "m":
		m.Release = true
	default:
		m.Press = true
	}
	return m, true
}
