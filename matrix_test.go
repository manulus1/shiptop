package main

import "testing"

// rainFor runs a rain area for a few seconds at a fixed load and returns the number of drops spawned.
func rainFor(load float64) int {
	S.CPU = load
	S.Cores = []float64{load, load, load, load}
	var m rain
	m.resize(120, 40)
	seen := map[*drop]bool{}
	for i := 0; i < 300; i++ { // 10 s at 30 fps
		m.step(1.0 / 30)
		for _, ds := range m.cols {
			for _, d := range ds {
				seen[d] = true
			}
		}
	}
	return len(seen)
}

func TestMatrixFollowsLoad(t *testing.T) {
	idle, busy := rainFor(.03), rainFor(.95)
	t.Logf("drops in 10 s on 120 columns: idle %d, busy %d", idle, busy)
	if busy < 3*idle {
		t.Fatalf("busy CPU should rain much harder: idle %d drops, busy %d drops", idle, busy)
	}
}

func TestMatrixDrawAnySize(t *testing.T) {
	S.CPU, S.Cores = .5, []float64{.2, .9}
	for _, sz := range [][2]int{{1, 1}, {10, 3}, {30, 8}, {80, 24}, {200, 60}, {80, 24}} {
		var sc Screen
		sc.Resize(sz[0], sz[1])
		for i := 0; i < 60; i++ {
			sc.Clear()
			drawMatrix(&sc)
			drawMatrixPanel(&sc, 0, 0, sz[0], sz[1])
			mxFull.step(.1)
			mxPanel.step(.1)
		}
	}
}
