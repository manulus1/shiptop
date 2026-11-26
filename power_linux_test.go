//go:build linux

package main

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	for p, v := range files {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(v+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLinuxSensorsFromSysfs(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"class/thermal/thermal_zone0/type":            "x86_pkg_temp",
		"class/thermal/thermal_zone0/temp":            "61500",
		"class/thermal/thermal_zone1/type":            "acpitz",
		"class/thermal/thermal_zone1/temp":            "47000",
		"class/thermal/thermal_zone2/type":            "broken",
		"class/thermal/thermal_zone2/temp":            "-273000",
		"class/power_supply/AC/type":                  "Mains",
		"class/power_supply/AC/online":                "0",
		"class/power_supply/BAT0/type":                "Battery",
		"class/power_supply/BAT0/status":              "Discharging",
		"class/power_supply/BAT0/capacity":            "81",
		"class/power_supply/BAT0/power_now":           "14250000",
		"class/power_supply/BAT0/energy_now":          "41230000",
		"class/power_supply/BAT0/energy_full":         "54210000",
		"class/power_supply/BAT0/energy_full_design":  "60000000",
		"class/power_supply/BAT0/cycle_count":         "312",
		"class/power_supply/hidpp_battery_0/type":     "Battery",
		"class/power_supply/hidpp_battery_0/scope":    "Device",
		"class/power_supply/hidpp_battery_0/capacity": "5",
	})
	old := sysRoot
	sysRoot = root
	defer func() { sysRoot = old }()

	th := readThermal()
	if !th.OK || len(th.Zones) != 2 || th.Max != 61.5 {
		t.Errorf("thermal %+v", th)
	}
	p := readPower()
	if !p.HasBattery || p.Pct != 81 || p.OnAC || p.Charging || !p.RateOK {
		t.Fatalf("power %+v", p)
	}
	if math.Abs(p.RateW+14.25) > 1e-9 || math.Abs(p.CapWh-41.23) > 1e-9 || math.Abs(p.FullWh-54.21) > 1e-9 || math.Abs(p.DesignWh-60) > 1e-9 || p.Cycles != 312 {
		t.Errorf("power values %+v", p)
	}
	if want := 41.23 / 14.25 * 3600; math.Abs(p.Remaining-want) > 1e-6 {
		t.Errorf("remaining %.1f want %.1f", p.Remaining, want)
	}

	// charge_* / current_now batteries (no energy_* files), charging
	root2 := t.TempDir()
	writeTree(t, root2, map[string]string{
		"class/power_supply/BAT1/type":        "Battery",
		"class/power_supply/BAT1/status":      "Charging",
		"class/power_supply/BAT1/capacity":    "40",
		"class/power_supply/BAT1/current_now": "2000000",
		"class/power_supply/BAT1/voltage_now": "12000000",
		"class/power_supply/BAT1/charge_now":  "1600000",
		"class/power_supply/BAT1/charge_full": "4000000",
	})
	sysRoot = root2
	p = readPower()
	if !p.Charging || !p.RateOK || math.Abs(p.RateW-24) > 1e-9 || math.Abs(p.CapWh-19.2) > 1e-9 || math.Abs(p.FullWh-48) > 1e-9 || p.Remaining >= 0 {
		t.Errorf("charging battery %+v", p)
	}

	sysRoot = t.TempDir() // a desktop: nothing at all
	if p := readPower(); p.HasBattery || p.RateOK {
		t.Errorf("desktop %+v", p)
	}
	if th := readThermal(); th.OK {
		t.Errorf("desktop thermal %+v", th)
	}
}
