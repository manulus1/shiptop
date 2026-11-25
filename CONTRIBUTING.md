# Contributing

Thanks for helping! A few ground rules keep shiptop small and trustworthy:

- **Standard library only.** No third-party Go modules, no cgo.
- **Real data stays real.** Anything modelled or estimated must be marked as such in the UI (like the `~` temperatures).
- **Windows struct layouts are proven, not guessed.** If you read a new Windows struct, add `_Static_assert`s
  for every offset you use to `tools/wincheck/check.c` and parse it from a byte buffer at those offsets.
- Run `go vet ./...` and `go test ./...` for both `GOOS=windows` and `linux` before opening a pull request.
  CI runs `shiptop --selftest` on a real Windows machine for every push.
- Bug reports: please include the output of `shiptop --once`, your Windows version and terminal.
