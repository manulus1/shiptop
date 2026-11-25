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
- **proc**: all processes with CPU %, working set, threads, parent, user and path; sort, filter, terminate
- **notebook**: a pixel-art 3D laptop rendered right in the terminal with half-block characters, coloured
  like a thermal camera by your **real** CPU load. Press `z` to zoom, arrows to rotate, `t` for visible light.

<p align="center"><img src="docs/notebook.gif" alt="rotating 3D thermal laptop" width="60%"></p>

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
| `k` / `Del` | terminate the selected process: asks `y/n`, only `y` ends it |
| `f` | filter by name, pid, path or user (`Enter`/`Esc` to finish) |
| `p` / `Space` | pause · `+` `-` update interval (250 ms to 5 s) |
| `b` `n` | previous / next network adapter |
| `t` | thermal / visible light on the notebook |
| `z` | zoom the notebook (arrows rotate it) |
| `q` | quit |

`shiptop --once` prints a single plain-text sample, handy to compare with Task Manager.

## What is real and what is modelled

Everything in the cpu, mem, disks, net and proc boxes is read live from the OS. Process CPU % is the share of
total CPU time, like Task Manager. The notebook box shows real load, RAM, battery, CPU name and thread count.
**Its temperatures and fan speed are modelled from the real load** and marked with `~`, because a console
program cannot read thermal sensors on Windows without a kernel driver.

Terminating is guarded: shiptop always asks first, and refuses system processes such as System, csrss,
lsass or svchost, as well as itself.

## How it works

- Pure Go standard library, about 3,400 lines, no cgo and no third-party modules.
- Windows data comes from the same sources Task Manager uses: `NtQuerySystemInformation` for processes and
  per-core times, `GetPerformanceInfo`, `GlobalMemoryStatusEx`, `GetDiskFreeSpaceExW`, `GetIfEntry`.
- The screen is a cell buffer flushed as 24-bit ANSI with diffing, so only changed cells are written.
- The laptop is a tiny software rasterizer: z-buffer, perspective-correct texture lookup, flat lighting,
  a pixel-art outline pass, and a heat field sampled on a 32×22 grid.

### How it is verified

Windows APIs are easy to get subtly wrong, so:

- every struct offset, size and constant the Windows code relies on is checked at compile time against the
  Windows SDK headers in [`tools/wincheck`](tools/wincheck) (run in CI with mingw-w64),
- the process-list parser is unit-tested against a buffer laid out like the real one, including Unicode names,
  out-of-bounds pointers and truncated data,
- CI runs `shiptop --selftest` on a real **Windows** runner and on Linux: it reads every metric and fails if
  any is missing or implausible.

## Build

```
go build -o shiptop.exe .          # on Windows
GOOS=windows GOARCH=amd64 go build # cross-compile from anywhere
go test ./...
```

## Roadmap

- 64-bit network counters and friendly adapter names (`GetIfTable2`)
- real GPU load and laptop power draw in watts
- disk read / write rates
- process tree view, mouse support, saved settings and themes
- winget and Scoop packages, signed binaries

Ideas and pull requests are welcome, see [CONTRIBUTING.md](CONTRIBUTING.md).

## Credits

The look is a love letter to [btop++](https://github.com/aristocratos/btop) by aristocratos. shiptop shares no
code with btop and is not affiliated with it. If you want the full btop experience on Windows, try
[btop4win](https://github.com/aristocratos/btop4win).

## License

[MIT](LICENSE)
