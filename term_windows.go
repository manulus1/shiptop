//go:build windows

package main

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

var (
	modK32          = syscall.NewLazyDLL("kernel32.dll")
	pGetConsoleMode = modK32.NewProc("GetConsoleMode")
	pSetConsoleMode = modK32.NewProc("SetConsoleMode")
	pGetCSBI        = modK32.NewProc("GetConsoleScreenBufferInfo")
	pSetOutCP       = modK32.NewProc("SetConsoleOutputCP")
	pSetInCP        = modK32.NewProc("SetConsoleCP")
	pGetOutCP       = modK32.NewProc("GetConsoleOutputCP")
	pGetInCP        = modK32.NewProc("GetConsoleCP")
)

var inMode, outMode uint32
var oldOutCP, oldInCP uintptr

type coord struct{ X, Y int16 }
type smallRect struct{ Left, Top, Right, Bottom int16 }
type csbi struct {
	Size, Cursor coord
	Attr         uint16
	Window       smallRect
	MaxSize      coord
}

func hin() uintptr  { return uintptr(os.Stdin.Fd()) }
func hout() uintptr { return uintptr(os.Stdout.Fd()) }

func termInit() error {
	if r, _, _ := pGetConsoleMode.Call(hout(), uintptr(unsafe.Pointer(&outMode))); r == 0 {
		return errors.New("stdout is not a console")
	}
	if r, _, _ := pGetConsoleMode.Call(hin(), uintptr(unsafe.Pointer(&inMode))); r == 0 {
		return errors.New("stdin is not a console")
	}
	oldOutCP, _, _ = pGetOutCP.Call()
	oldInCP, _, _ = pGetInCP.Call()
	pSetOutCP.Call(65001)
	pSetInCP.Call(65001)
	// ENABLE_PROCESSED_OUTPUT | ENABLE_VIRTUAL_TERMINAL_PROCESSING | DISABLE_NEWLINE_AUTO_RETURN
	if r, _, _ := pSetConsoleMode.Call(hout(), uintptr(outMode|0x1|0x4|0x8)); r == 0 {
		if r, _, _ := pSetConsoleMode.Call(hout(), uintptr(outMode|0x1|0x4)); r == 0 {
			return errors.New("this console has no VT support (use Windows Terminal or Windows 10 1809+)")
		}
	}
	// raw input: no line input, no echo, no processed input, no quick edit; VT input + extended flags.
	// ENABLE_MOUSE_INPUT (0x10) with VT input: Windows Terminal and the Windows 11 console deliver
	// clicks, drags and the wheel as SGR sequences once mouse reporting is switched on (mouseOn).
	nm := (inMode &^ (0x1 | 0x2 | 0x4 | 0x40)) | 0x200 | 0x80 | 0x10
	pSetConsoleMode.Call(hin(), uintptr(nm))
	return nil
}

func termRestore() {
	pSetConsoleMode.Call(hin(), uintptr(inMode))
	pSetConsoleMode.Call(hout(), uintptr(outMode))
	if oldOutCP != 0 {
		pSetOutCP.Call(oldOutCP)
	}
	if oldInCP != 0 {
		pSetInCP.Call(oldInCP)
	}
}

func termSize() (int, int) {
	var c csbi
	if r, _, _ := pGetCSBI.Call(hout(), uintptr(unsafe.Pointer(&c))); r == 0 {
		return 120, 30
	}
	return int(c.Window.Right-c.Window.Left) + 1, int(c.Window.Bottom-c.Window.Top) + 1
}
