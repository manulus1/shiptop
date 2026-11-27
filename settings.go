package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

// settings.json (next to layout.json) keeps small preferences between runs.
type settings struct {
	Pixels string `json:"pixels,omitempty"` // notebook pixel mode: half, quad, sext, oct
}

var pixNote = map[string]string{
	"half": "(half blocks, works with every font)",
	"quad": "(quadrants)",
	"sext": "(sextants, needs Cascadia Code 2404+ or a recent Linux terminal)",
	"oct":  "(octants, needs Cascadia Code 2404+ or a recent Linux terminal)",
}

func settingsPath() (string, error) {
	p, err := layoutPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(p), "settings.json"), nil
}

// defaultPix: octants in Windows Terminal (its font Cascadia Code has them), half blocks in the
// classic Windows console (its fonts don't), quadrants elsewhere (nearly every font has those).
func defaultPix() int {
	id := "quad"
	if runtime.GOOS == "windows" {
		id = "half"
		if os.Getenv("WT_SESSION") != "" {
			id = "oct"
		}
	}
	return pixIndex(id)
}

func pixIndex(id string) int {
	for i, m := range pixModes {
		if m.ID == id {
			return i
		}
	}
	return -1
}

func loadSettings() {
	V.Pix = defaultPix()
	path, err := settingsPath()
	if err != nil {
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var s settings
	if json.Unmarshal(b, &s) == nil {
		if i := pixIndex(s.Pixels); i >= 0 {
			V.Pix = i
		}
	}
}

func saveSettings() error {
	path, err := settingsPath()
	if err != nil {
		return err
	}
	b, _ := json.MarshalIndent(settings{Pixels: pixModes[V.Pix].ID}, "", "  ")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return errors.Join(err, os.Remove(tmp))
	}
	return nil
}
