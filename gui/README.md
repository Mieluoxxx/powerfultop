# PowerfulTop (Go)

The app, written in Go and drawn with [MyGo](https://github.com/egoist/mygo)'s native UI. Build and
test it from the repository root with `bash build.sh` and `bash test.sh`, or from here:

```sh
go tool mygo dev     # development build with live reload
go test ./...        # views and parsers, without a window
```

## Layout

- `main.go`, `tray.go`, `details.go`, `context.go`: the app — monitors, the menu bar item and its
  popover, the details window, and what connects them. The views are put together from
  [CrychicUI](https://github.com/ELY-Open-Source/CrychicUI) components on MyGo's native UI.
- `internal/macos`: macOS APIs MyGo doesn't wrap, through purego (no cgo): the IOKit registry,
  CF ↔ Go conversion over XML property lists, cfprefsd preferences, and the Objective-C
  helpers (strings, data, autorelease pools, typed `objc_msgSend`) every package shares.
- `internal/power`, `internal/thermal`, `internal/system`, `internal/display`: the monitors, each
  with `Start`/`Stop`/`Snapshot`/`OnChange`; `internal/notify` runs every `OnChange`.
- `internal/menubar`: the menu bar label, rendered to a template PNG since trays can't show
  attributed titles.
- `internal/barmanager`: the macOS 27 menu bar manager and its layout editor.
- `internal/view`: the dashboard components and every page.
- `internal/lang`: `L(en, zh)`, through which all text goes; it imports nothing of the app, so
  every package can use it (pages call it as `view.L`).
- `cmd/fanhelper`: the root fan helper, built by `scripts/build-helper.sh` into `resources/`.

## Notes

- Preferences use the Swift app's domain and keys, so settings carry over.
- macOS 27 silently keeps a status item off the menu bar when another app registered under the
  same bundle identifier has a different signature; run a development copy under a different
  `identifier` in `mygo.json` while an installed build exists.
- MyGo quits when the last window closes, so the app registers `App.OnWindowAllClosed`.
- MenuBarAgent doesn't always notice writes to its layout file; the layout editor checks the bar
  after each move and restarts the agent when the write didn't take.
- The README's screenshots are rendered from these views, with one Mac's readings, so they can be
  regenerated without a window or a charger to plug in:
  `README_SCREENSHOTS=$PWD/../screenshot go test ./internal/view -run TestReadmeScreenshots`.
  `README_LOCALE=zh-Hans` renders them in Chinese, and the Menu Bar Manager shot takes app logos
  as `<app>.png` from `README_ICONS=<dir>`.
