<h1 align="center">shiptop</h1>

<p align="center">
  <b>A btop-style system monitor for Windows Terminal, with a spinning pixel-3D thermal model of your laptop.</b><br>
  One 2&nbsp;MB <code>.exe</code>. No install, no admin, no dependencies.
</p>

<p align="center">
  <a href="https://github.com/manulus1/shiptop/actions/workflows/ci.yml"><img src="https://github.com/manulus1/shiptop/actions/workflows/ci.yml/badge.svg" alt="ci"></a>
  <a href="https://github.com/manulus1/shiptop/releases/latest"><img src="https://img.shields.io/github/v/release/manulus1/shiptop?color=3c9b6f" alt="release"></a>
  <img src="https://img.shields.io/badge/platform-Windows%20%7C%20Linux-7166c8" alt="platform">
  <img src="https://img.shields.io/badge/deps-zero-c0703a" alt="zero dependencies">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-b4544f" alt="MIT"></a>
</p>

<p align="center"><img src="docs/demo.gif" alt="shiptop showing live CPU, memory, disks, network and processes" width="100%"></p>

<p align="center"><sub>Recorded with real data on a small 2-core Linux VM under a synthetic load. On your laptop it looks busier.</sub></p>

## Why

Task Manager is fine. But your terminal deserves a cockpit: braille graphs for every core, live network
throughput, a sortable process list you drive from the keyboard, and a tiny 3D laptop that turns hot
where your CPU is working.

## Features

- **cpu**: total load graph and every logical processor with its own sparkline, uptime, process, thread and handle counts
- **mem**: used / available / cached, plus commit charge vs. commit limit
- **disks**: every local drive with used %, free space and file system
- **net**: live download / upload rates per adapter, peaks and totals since start (`b` / `n` switches adapter)
- **proc**: all processes with CPU %, GPU %, working set, threads, parent, user and path; sort, filter, terminate
- **energy view** (`e`): which processes cause the most CPU + GPU activity, i.e. what drains your battery,
  next to the real battery draw in watts and the time left
- **notebook**: a pixel-art 3D laptop rendered right in the terminal with half-block characters, coloured
  like a thermal camera by your **real** CPU and GPU load. Shows GPU load, battery draw in watts, time left,
  battery health and cycle count, and real temperatures where your laptop exposes them.
  Press `z` to zoom, arrows to rotate, `t` for visible light, `g` for the resolution: up to 2×4 pixels per
  character with the octant characters of Cascadia Code (Windows Terminal's font), anti-aliased.

<p align="center"><img src="docs/notebook.gif" alt="rotating 3D thermal laptop" width="60%"></p>

- **windows**: every open window (what Alt+Tab shows, most recently used first) with the CPU, RAM, GPU and
  energy of the process behind it; for Store apps that is the app itself, not ApplicationFrameHost.
  `Enter` or a double click brings the selected window to the front, restoring it if it is minimized.
- **matrix** (`m` for full screen, or as a panel): digital rain made from your live data. It falls faster the busier
  the CPU is, each band of columns belongs to one logical processor (a busy core rains harder), and the glyphs
  are taken from the names of the processes running right now.
- **your layout** (`l`): move, resize and swap panels, add the windows or matrix panel, take out what you
  don't need. Drag panels and borders with the mouse or use the keys; the layout is saved and loaded next time.

<p align="center"><img src="docs/layout.gif" alt="layout editor: dragging the net panel to the screen edge, adding the matrix panel, resizing" width="80%"></p>

<p align="center"><img src="docs/matrix.gif" alt="matrix mode: digital rain driven by live CPU load" width="60%"></p>

## Install

**Windows 10 (1809+) or 11.** Windows Terminal recommended: the classic console may show boxes instead of braille dots.

1. Download `shiptop-windows-amd64.exe` (or `-arm64` for Snapdragon laptops) from the [latest release](https://github.com/manulus1/shiptop/releases/latest).
2. Run it in Windows Terminal or PowerShell: `.\shiptop-windows-amd64.exe`

The binary is not code-signed yet, so Windows may warn about an unknown publisher: choose *More info → Run anyway*,
or run `Unblock-File .\shiptop-windows-amd64.exe` first. Checksums are in `SHA256SUMS.txt` on each release.

With Go installed: `go install github.com/manulus1/shiptop@latest`

**Linux** works too (data from `/proc`): grab `shiptop-linux-amd64` / `-arm64` from the release.

Just want to look? There is a [browser demo](https://manulus1.github.io/shiptop/) with simulated starship data, where the project started.

## Keys

| key | action |
|---|---|
| `↑` `↓` `PgUp` `PgDn` `Home` `End` | select a process (nothing is selected at start, so the busiest processes stay on screen) |
| `Esc` | deselect and jump back to the top; press again to clear the filter |
| `←` `→` | change sort column · `r` reverse |
| `e` | energy view: sort by share of CPU + GPU activity, with the battery draw on top |
| `k` / `Del` | terminate the selected process: asks `y/n`, only `y` ends it |
| `f` | filter by name, pid, path or user (`Enter`/`Esc` to finish) |
| `p` / `Space` | pause · `+` `-` update interval (250 ms to 5 s) |
| `b` `n` | previous / next network adapter |
| `t` | thermal / visible light on the notebook |
| `g` | notebook resolution: 1×2 (half blocks, any font), 2×2 (quadrants), 2×3 (sextants), 2×4 (octants). Sextants and octants need Cascadia Code 2404 or newer (Windows Terminal updates it) or a recent Linux terminal; if you see boxes, press `g` until it looks right. The choice is saved. |
| `z` | zoom the notebook (arrows rotate it) |
| `m` | matrix mode: digital rain made from live data (`m` / `Esc` back) |
| `Tab` | switch the arrow keys between the process list and the window list |
| `Enter` | bring the selected window to the front (window list) |
| `l` | layout editor (see below) |
| `q` | quit |

**Mouse** (Windows Terminal, the default terminal on Windows 11, and Linux terminals): click a row to select
it, double-click a window to bring it to the front, the wheel scrolls lists and turns the notebook.
Hold `Shift` to select text with the mouse as usual.

**Layout editor** (`l`): changes are only kept when you press `Enter`; `Esc` throws them away.

| mouse / key | action |
| --- | --- |
| drag a panel onto another | swap them |
| drag a panel onto another panel's edge | put it there, splitting that panel |
| drag a panel onto the screen edge | full-height column or full-width row on that side |
| drag a border | resize |
| wheel over a panel · `Space` | show a different panel there |
| `Tab` | select the next panel |
| `←` `↑` `↓` `→` or `h` `j` `k` `l` | swap with the neighbour |
| `Shift`+arrows or `H` `J` `K` `L` | move an edge of the selected panel |
| `Ctrl`+arrows | move the panel to that screen edge |
| `a` · `x` | add a panel · remove the selected one |
| `s` · `=` | side by side ↔ stacked · equal sizes |
| `r` | back to the built-in layout |

The layout is saved to `%AppData%\shiptop\layout.json` (`~/.config/shiptop/layout.json` on Linux).
`shiptop --reset-layout` deletes it.

`shiptop --once` prints a single plain-text sample, handy to compare with Task Manager.
`shiptop --windows` lists the windows the windows panel shows, `shiptop --raise TEXT` brings the first
window whose title contains `TEXT` to the front.

## What is real and what is modelled

Read live from Windows, no admin rights or extra drivers needed:

- cpu, memory, disks, network and processes; process CPU % is the share of total CPU time, like Task Manager
- **GPU load** overall and per process, from the same performance counters Task Manager uses
  (busiest engine of the busiest adapter)
- **battery**: charge or discharge rate in watts, time left, capacity, health (full vs. design capacity),
  cycle count and battery temperature if the battery reports them, straight from the battery driver
- **temperatures** from ACPI thermal zones, if your laptop exposes them (shown as `zone`). These are
  motherboard or package sensors chosen by the laptop maker, not individual CPU cores.

Estimated, and marked as such:

- **energy %** in the energy view is each process' share of all current CPU % + GPU % activity. It shows who
  keeps the machine busy, it is not a per-process wattage (Windows does not expose that to normal users).
- **modelled temperatures** (marked `~`) are used only when no thermal zone is available. The model is simple:
  CPU 40 °C at idle rising linearly to 92 °C at full load, GPU 36 °C plus 40 °C × GPU load, battery
  29 °C plus up to 3 °C (4 °C more while charging). It is a plausible ballpark for a typical laptop and can
  be 10 to 30 °C off for yours.
- the **heat map** on the 3D laptop is always a model: real load and temperatures go in, but how heat spreads
  across the chassis is drawn from typical laptop layouts.

Per-core CPU temperatures on Intel and AMD need a kernel driver on Windows; shiptop deliberately does not
install one.

Terminating is guarded: shiptop always asks first, and refuses system processes such as System, csrss,
lsass or svchost, as well as itself.

## How it works

- Pure Go standard library, about 6,800 lines, no cgo and no third-party modules.
- Windows data comes from the same sources Task Manager uses: `NtQuerySystemInformation` for processes and
  per-core times, `GetPerformanceInfo`, `GlobalMemoryStatusEx`, `GetDiskFreeSpaceExW`, `GetIfEntry`.
- The screen is a cell buffer flushed as 24-bit ANSI with diffing, so only changed cells are written.
- The layout is a binary split tree: each split gives a share of its space to one side, within the panels'
  minimum sizes. Every panel is drawn inside a clip rectangle, so it can never paint over its neighbours.
- The window list uses `EnumWindows` with the same filter as Alt+Tab (visible, not cloaked, no tool
  windows or owned popups). Windows only lets the program you last typed into change the foreground
  window, and that is the terminal, so to bring a window to the front shiptop briefly attaches to the
  input of the current foreground window, as task switchers do.
- The laptop is a tiny software rasterizer: z-buffer, perspective-correct texture lookup, flat lighting,
  a pixel-art outline pass, and a heat field sampled on a 32×22 grid. It renders at twice the resolution
  and averages (anti-aliasing), then each character cell gets the two colours that fit its 2×4 (or 2×3, 2×2,
  1×2) sub-pixels best and the mosaic character for that pattern. The character tables are generated from
  the Unicode character names ([`tools/mosaic_gen.py`](tools/mosaic_gen.py)).

### How it is verified

Windows APIs are easy to get subtly wrong, so:

- every struct offset, size and constant the Windows code relies on (processes, memory, performance
  counters, battery driver IOCTLs, console) is checked at compile time against the Windows SDK headers in
  [`tools/wincheck`](tools/wincheck) (run in CI with mingw-w64),
- DLLs that are not KnownDLLs (`pdh.dll`, `setupapi.dll`) are loaded by their full System32 path,
- the parsers for the process list, performance counter arrays, GPU engine names and battery structs are
  unit-tested against buffers laid out like the real ones, including Unicode names, out-of-bounds pointers
  and truncated data; the Linux battery and thermal readers are tested against fake sysfs trees,
- CI runs `shiptop --selftest` on a real **Windows** runner and on Linux: it reads every metric and fails if
  any is missing or implausible. On Windows it also opens Notepad and checks that the window list finds it.
- the layout code is tested on random layouts and random sequences of moves, swaps, resizes and removals:
  panels always cover the screen without overlapping and keep their minimum sizes; every panel is drawn at
  random sizes to check it never paints outside its tile; and the built-in layout is checked to be identical,
  cell for cell, to the fixed layout of earlier versions at over 1,800 window sizes.

## Build

```
go build -o shiptop.exe .          # on Windows
GOOS=windows GOARCH=amd64 go build # cross-compile from anywhere
go test ./...
```

## Roadmap

- 64-bit network counters and friendly adapter names (`GetIfTable2`)
- disk read / write rates
- process tree view and themes
- winget and Scoop packages, signed binaries

Ideas and pull requests are welcome, see [CONTRIBUTING.md](CONTRIBUTING.md).

## Credits

The look is a love letter to [btop++](https://github.com/aristocratos/btop) by aristocratos. shiptop shares no
code with btop and is not affiliated with it. If you want the full btop experience on Windows, try
[btop4win](https://github.com/aristocratos/btop4win).

## License

[MIT](LICENSE)
