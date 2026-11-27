//go:build linux

package main

import "errors"

var errNoWinList = errors.New("the window list needs Windows")

var readWindows = func() ([]Win, error) { return nil, errNoWinList }

func raiseWindow(h uintptr) (string, error) { return "", errNoWinList }
