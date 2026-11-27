//go:build windows

package main

import (
	"errors"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

var (
	modU32                    = syscall.NewLazyDLL("user32.dll") // a KnownDLL: always the System32 copy
	pEnumWindows              = modU32.NewProc("EnumWindows")
	pEnumChildWindows         = modU32.NewProc("EnumChildWindows")
	pGetWindowTextW           = modU32.NewProc("GetWindowTextW")
	pGetClassNameW            = modU32.NewProc("GetClassNameW")
	pIsWindow                 = modU32.NewProc("IsWindow")
	pIsWindowVisible          = modU32.NewProc("IsWindowVisible")
	pIsIconic                 = modU32.NewProc("IsIconic")
	pGetWindowLongPtrW        = modU32.NewProc("GetWindowLongPtrW")
	pGetWindow                = modU32.NewProc("GetWindow")
	pGetWindowThreadProcessId = modU32.NewProc("GetWindowThreadProcessId")
	pGetForegroundWindow      = modU32.NewProc("GetForegroundWindow")
	pSetForegroundWindow      = modU32.NewProc("SetForegroundWindow")
	pShowWindowAsync          = modU32.NewProc("ShowWindowAsync")
	pBringWindowToTop         = modU32.NewProc("BringWindowToTop")
	pAttachThreadInput        = modU32.NewProc("AttachThreadInput")
	pSwitchToThisWindow       = modU32.NewProc("SwitchToThisWindow")
	pPeekMessageW             = modU32.NewProc("PeekMessageW")
	pGetCurrentThreadId       = modK32.NewProc("GetCurrentThreadId")
	modDwm                    = syscall.NewLazyDLL(sys32("dwmapi.dll")) // not a KnownDLL: load by full path
	pDwmGetWindowAttribute    = modDwm.NewProc("DwmGetWindowAttribute")
)

const (
	gwlExStyle      = ^uintptr(19) // GWL_EXSTYLE (-20)
	gwOwner         = 4            // GW_OWNER
	wsExToolWindow  = 0x00000080
	wsExAppWindow   = 0x00040000
	wsExNoActivate  = 0x08000000
	dwmwaCloaked    = 14
	swRestore       = 9
	uwpFrameClass   = "ApplicationFrameWindow"
	uwpCoreWinClass = "Windows.UI.Core.CoreWindow"
)

// EnumWindows/EnumChildWindows call back into Go; the callbacks are created once (Windows limits
// how many Go callbacks a process may create) and only used from one goroutine at a time.
var (
	enumFound  []uintptr
	enumCB     = syscall.NewCallback(func(h, _ uintptr) uintptr { enumFound = append(enumFound, h); return 1 })
	childFound []uintptr
	childCB    = syscall.NewCallback(func(h, _ uintptr) uintptr { childFound = append(childFound, h); return 1 })
)

func call(p *syscall.LazyProc, a ...uintptr) uintptr {
	r, _, _ := p.Call(a...)
	return r
}

func winText(h uintptr) string {
	// GetWindowText reads another process's caption without sending it a message,
	// so a window that hangs can't make shiptop hang.
	buf := make([]uint16, 512)
	n := call(pGetWindowTextW, h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf[:n])
}

func winClass(h uintptr) string {
	buf := make([]uint16, 256)
	n := call(pGetClassNameW, h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf[:n])
}

func winPID(h uintptr) int {
	var pid uint32
	call(pGetWindowThreadProcessId, h, uintptr(unsafe.Pointer(&pid)))
	return int(pid)
}

// cloaked windows are hidden by the shell (suspended Store apps, windows on other virtual desktops).
func cloaked(h uintptr) bool {
	if modDwm.Load() != nil || pDwmGetWindowAttribute.Find() != nil {
		return false
	}
	var v uint32
	r := call(pDwmGetWindowAttribute, h, dwmwaCloaked, uintptr(unsafe.Pointer(&v)), 4)
	return r == 0 && v != 0
}

// uwpPID: a Store app's window is a frame owned by ApplicationFrameHost.exe; the app's own
// process owns the CoreWindow inside it.
func uwpPID(h uintptr, host int) int {
	childFound = childFound[:0]
	call(pEnumChildWindows, h, childCB, 0)
	for _, c := range childFound {
		if winClass(c) == uwpCoreWinClass {
			if pid := winPID(c); pid != 0 && pid != host {
				return pid
			}
		}
	}
	return 0
}

// readWindows lists the windows Alt+Tab would show, in z-order (most recently used first).
var readWindows = func() ([]Win, error) {
	enumFound = enumFound[:0]
	if r, _, err := pEnumWindows.Call(enumCB, 0); r == 0 {
		return nil, err
	}
	var out []Win
	for _, h := range enumFound {
		if call(pIsWindowVisible, h) == 0 {
			continue
		}
		ex := call(pGetWindowLongPtrW, h, gwlExStyle)
		if ex&wsExAppWindow == 0 && (ex&(wsExToolWindow|wsExNoActivate) != 0 || call(pGetWindow, h, gwOwner) != 0) {
			continue // tool windows, popups and owned dialogs are not shown by Alt+Tab
		}
		title := winText(h)
		if title == "" || cloaked(h) {
			continue
		}
		class := winClass(h)
		if class == "Progman" || class == "WorkerW" { // the desktop
			continue
		}
		pid := winPID(h)
		if class == uwpFrameClass {
			if p := uwpPID(h, pid); p != 0 {
				pid = p
			}
		}
		out = append(out, Win{H: h, Title: title, Class: class, PID: pid, Min: call(pIsIconic, h) != 0})
	}
	return out, nil
}

// raiseWindow restores the window if it is minimized and makes it the foreground window.
// Windows only lets the process that received the last input change the foreground window,
// and that is the terminal, not shiptop; so when the plain call is refused, shiptop briefly
// attaches to the input of the current foreground thread (what task switchers do).
func raiseWindow(h uintptr) (string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if call(pIsWindow, h) == 0 {
		return "", errors.New("the window was closed")
	}
	if call(pIsIconic, h) != 0 {
		call(pShowWindowAsync, h, swRestore)
	}
	isFront := func() bool {
		for i := 0; i < 10; i++ {
			if call(pGetForegroundWindow) == h {
				return true
			}
			time.Sleep(30 * time.Millisecond)
		}
		return false
	}
	call(pSetForegroundWindow, h)
	if isFront() {
		return "SetForegroundWindow", nil
	}
	var msg [64]byte
	call(pPeekMessageW, uintptr(unsafe.Pointer(&msg[0])), 0, 0, 0, 0) // PM_NOREMOVE: gives this thread a message queue
	me := call(pGetCurrentThreadId)
	if fg := call(pGetForegroundWindow); fg != 0 {
		if ft := call(pGetWindowThreadProcessId, fg, 0); ft != 0 && ft != me && call(pAttachThreadInput, me, ft, 1) != 0 {
			call(pBringWindowToTop, h)
			call(pSetForegroundWindow, h)
			call(pAttachThreadInput, me, ft, 0)
		}
	}
	if isFront() {
		return "attached input", nil
	}
	call(pSwitchToThisWindow, h, 1)
	if isFront() {
		return "SwitchToThisWindow", nil
	}
	return "", errors.New("Windows kept the current window in front")
}
