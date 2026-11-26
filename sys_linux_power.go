//go:build linux

package main

// Thermal zones and batteries from sysfs. sysRoot is a variable so tests can point it at a fixture.

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var sysRoot = "/sys"

func init() {
	if v := os.Getenv("SHIPTOP_SYSFS"); v != "" { // test hook: read a fake /sys tree
		sysRoot = v
	}
}

func sampleSensors() {}

func readGPU() GPUStat { return GPUStat{} } // no vendor-neutral GPU load interface on Linux

func sensorStatus() string { return "sysfs" }

const wantSensors = "sysfs"

func sysNum(p string) (float64, bool) {
	v, err := strconv.ParseFloat(strings.TrimSpace(readFile(p)), 64)
	return v, err == nil
}

func readThermal() ThermalStat {
	var ts ThermalStat
	zones, _ := filepath.Glob(filepath.Join(sysRoot, "class/thermal/thermal_zone*"))
	for _, z := range zones {
		v, ok := sysNum(filepath.Join(z, "temp"))
		if !ok {
			continue
		}
		c := v / 1000 // millidegrees Celsius
		if c < 1 || c > 125 {
			continue
		}
		name := strings.TrimSpace(readFile(filepath.Join(z, "type")))
		if name == "" {
			name = filepath.Base(z)
		}
		ts.Zones = append(ts.Zones, Zone{name, c})
		if c > ts.Max {
			ts.Max = c
		}
	}
	ts.OK = len(ts.Zones) > 0
	return ts
}

func readPower() PowerStat {
	ps := PowerStat{Remaining: -1}
	supplies, _ := filepath.Glob(filepath.Join(sysRoot, "class/power_supply/*"))
	var pctSum, nBat int
	for _, s := range supplies {
		typ := strings.TrimSpace(readFile(filepath.Join(s, "type")))
		if typ == "Mains" {
			if v, ok := sysNum(filepath.Join(s, "online")); ok && v == 1 {
				ps.OnAC = true
			}
			continue
		}
		if typ != "Battery" || strings.TrimSpace(readFile(filepath.Join(s, "scope"))) == "Device" {
			continue // skip mice, keyboards, phones ...
		}
		nBat++
		ps.HasBattery = true
		status := strings.TrimSpace(readFile(filepath.Join(s, "status")))
		ps.Charging = ps.Charging || status == "Charging"
		if v, ok := sysNum(filepath.Join(s, "capacity")); ok {
			pctSum += int(v)
		}
		volt, hasVolt := sysNum(filepath.Join(s, "voltage_now")) // µV
		watts, okW := sysNum(filepath.Join(s, "power_now"))      // µW
		if !okW {
			if cur, ok := sysNum(filepath.Join(s, "current_now")); ok && hasVolt { // µA
				watts, okW = cur*volt/1e6, true
			}
		}
		if okW {
			w := watts / 1e6
			if status == "Discharging" {
				w = -w
			} else if status != "Charging" {
				w = 0
			}
			ps.RateW += w
			ps.RateOK = true
		}
		wh := func(energy, charge string) float64 {
			if v, ok := sysNum(filepath.Join(s, energy)); ok { // µWh
				return v / 1e6
			}
			if v, ok := sysNum(filepath.Join(s, charge)); ok && hasVolt { // µAh × V
				return v / 1e6 * volt / 1e6
			}
			return 0
		}
		ps.CapWh += wh("energy_now", "charge_now")
		ps.FullWh += wh("energy_full", "charge_full")
		ps.DesignWh += wh("energy_full_design", "charge_full_design")
		if v, ok := sysNum(filepath.Join(s, "cycle_count")); ok {
			ps.Cycles += int(v)
		}
	}
	if nBat > 0 {
		ps.Pct = pctSum / nBat
	}
	if ps.RateOK && ps.RateW < -0.5 && ps.CapWh > 0 {
		ps.Remaining = ps.CapWh / -ps.RateW * 3600
	}
	return ps
}
