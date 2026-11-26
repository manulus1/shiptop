package main

import "testing"

// rain runs the matrix simulation for a few seconds at a fixed load and returns the number of drops spawned.
func rain(load float64) int {
	S.CPU = load
	S.Cores = []float64{load, load, load, load}
	V.Matrix = true
	mxResize(120, 40)
	seen := map[*drop]bool{}
	for i := 0; i < 300; i++ { // 10 s at 30 fps
		matrixFrame(1.0 / 30)
		for _, ds := range mx.cols {
			for _, d := range ds {
				seen[d] = true
			}
		}
	}
	n := len(seen)
	return n
}

func TestMatrixFollowsLoad(t *testing.T) {
	idle, busy := rain(.03), rain(.95)
	t.Logf("drops in 10 s on 120 columns: idle %d, busy %d", idle, busy)
	if busy < 3*idle {
		t.Fatalf("busy CPU should rain much harder: idle %d drops, busy %d drops", idle, busy)
	}
}

func TestMatrixDrawAnySize(t *testing.T) {
	S.CPU, S.Cores = .5, []float64{.2, .9}
	V.Matrix = true
	for _, sz := range [][2]int{{1, 1}, {10, 3}, {30, 8}, {80, 24}, {200, 60}, {80, 24}} {
		var sc Screen
		sc.Resize(sz[0], sz[1])
		for i := 0; i < 60; i++ {
			drawMatrix(&sc)
			matrixFrame(.1)
		}
	}
}
