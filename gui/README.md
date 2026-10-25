# PowerfulTop on MyGo

The Go port of PowerfulTop, drawn with [MyGo](https://github.com/egoist/mygo)'s native UI. It
replaces the Swift app in `../PowerfulTop` stage by stage; until the last stage both live in the
repository and run side by side.

```sh
go tool mygo dev     # development build with live reload
go test ./...        # views and parsers, without a window
go tool mygo build   # build/darwin-arm64/PowerfulTop.app and a .dmg
```

## Stages

1. **Skeleton** (done): an accessory app whose menu bar item shows live system power and opens a
   popover of native UI. IOKit is read through purego (`internal/macos`), so no cgo.
2. **Power data**: the rest of PowerData — telemetry details, cell topology, charge limit, time
   estimates — and the popover at parity with the Swift one.
3. **Details window**: sidebar and pages, charts with `ui/chart`.
4. **Thermal and fans**: SMC reads; the root fan helper stays the Swift binary for now, bundled
   under Resources and installed as today.
5. **Menu bar manager**: own NSStatusItems through purego/objc for the separator and spacers, and
   the macOS 27 layout file.
6. **Cut-over**: switch to the shipping bundle identifier and remove the Swift app.

## Notes

- The bundle identifier is `io.github.mieluoxxx.PowerfulTop.mygo` until the cut-over. macOS 27
  silently keeps a status item off the menu bar when another app registered under the same
  identifier has a different signature, which the Swift build does on a development Mac.
- MyGo quits when the last window closes, so the app registers `App.OnWindowAllClosed`: the
  popover is its only window.
- The popover closes through `internal/macos.ClickMonitor`: since macOS 14 an accessory app isn't
  activated when its popover opens, so AppKit's transient dismissal never fires.
