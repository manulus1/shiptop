//go:build linux

package main

import (
	"os"
	"syscall"
	"unsafe"
)

var oldT syscall.Termios

func ioctl(fd, req uintptr, arg unsafe.Pointer) syscall.Errno {
	_, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(arg))
	return e
}

func termInit() error {
	if e := ioctl(os.Stdin.Fd(), syscall.TCGETS, unsafe.Pointer(&oldT)); e != 0 {
		return e
	}
	t := oldT
	t.Iflag &^= syscall.IGNBRK | syscall.BRKINT | syscall.PARMRK | syscall.ISTRIP | syscall.INLCR | syscall.IGNCR | syscall.ICRNL | syscall.IXON
	t.Lflag &^= syscall.ECHO | syscall.ECHONL | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	t.Cflag &^= syscall.CSIZE | syscall.PARENB
	t.Cflag |= syscall.CS8
	t.Cc[syscall.VMIN], t.Cc[syscall.VTIME] = 1, 0
	if e := ioctl(os.Stdin.Fd(), syscall.TCSETS, unsafe.Pointer(&t)); e != 0 {
		return e
	}
	return nil
}

func termRestore() { ioctl(os.Stdin.Fd(), syscall.TCSETS, unsafe.Pointer(&oldT)) }

func termSize() (int, int) {
	var ws struct{ Row, Col, X, Y uint16 }
	if e := ioctl(os.Stdout.Fd(), syscall.TIOCGWINSZ, unsafe.Pointer(&ws)); e != 0 || ws.Col == 0 {
		return 120, 30
	}
	return int(ws.Col), int(ws.Row)
}
