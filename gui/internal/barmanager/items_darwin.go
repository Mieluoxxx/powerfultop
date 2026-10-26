//go:build darwin

package barmanager

import (
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"

	"powerfultop/internal/macos"
)

// NewItemStore lists items for the layout editor; ownStatusItem is the autosave name of the app's
// own status item. It re-opens a layout-file grant saved on an earlier launch.
func NewItemStore(ownStatusItem string) *ItemStore {
	p := &darwinItems{onMain: mygo.RunOnMain}
	p.restoreLayoutAccess()
	return newItemStore(p, majorVersion() >= 27, ownStatusItem)
}

func (d *darwinItems) ownPID() int32 { return int32(os.Getpid()) }

func majorVersion() int {
	version, err := syscall.Sysctl("kern.osproductversion")
	if err != nil {
		return 0
	}
	major, _ := strconv.Atoi(strings.SplitN(version, ".", 2)[0])
	return major
}

// tr picks the text for the language the app shows; this package can't use view.L, which imports it.
func tr(en, zh string) string {
	if strings.HasPrefix(i18n.Locale(), "zh") {
		return zh
	}
	return en
}

const (
	positionsKey     = "TrailingItemPreferredPositions"
	bookmarkKey      = "menuBarLayoutFileBookmark"
	axMessageTimeout = 0.25
	// Every app is asked at once, but a hung app's timeout shouldn't hold up the rest for long.
	axParallelism = 16
)

// layoutPath is the one file where macOS 27 keeps the menu bar order: each item's preferred
// distance from the right edge. Writing it through cfprefsd moves an item at once (the approach
// Thaw 3 takes); it sits in an Apple group container, so macOS denies access until the user picks it.
var layoutPath = filepath.Join(os.Getenv("HOME"), "Library/Group Containers/com.apple.MenuBar/Library/Preferences/com.apple.MenuBar.plist")

var (
	axOnce sync.Once

	axIsProcessTrusted             func() bool
	axIsProcessTrustedWithOptions  func(options uintptr) bool
	axUIElementCreateApplication   func(pid int32) uintptr
	axUIElementCopyAttributeValue  func(element, attribute uintptr, value *uintptr) int32
	axUIElementSetMessagingTimeout func(element uintptr, seconds float32) int32
	axValueGetValue                func(value uintptr, typ int32, out unsafe.Pointer) bool

	axAttributes = map[string]uintptr{}
)

func loadAX() {
	axOnce.Do(func() {
		loadCG()
		lib, err := purego.Dlopen("/System/Library/Frameworks/ApplicationServices.framework/ApplicationServices", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			panic(err)
		}
		purego.RegisterLibFunc(&axIsProcessTrusted, lib, "AXIsProcessTrusted")
		purego.RegisterLibFunc(&axIsProcessTrustedWithOptions, lib, "AXIsProcessTrustedWithOptions")
		purego.RegisterLibFunc(&axUIElementCreateApplication, lib, "AXUIElementCreateApplication")
		purego.RegisterLibFunc(&axUIElementCopyAttributeValue, lib, "AXUIElementCopyAttributeValue")
		purego.RegisterLibFunc(&axUIElementSetMessagingTimeout, lib, "AXUIElementSetMessagingTimeout")
		purego.RegisterLibFunc(&axValueGetValue, lib, "AXValueGetValue")
		// Created once and kept: the CFStrings name attributes on every scan.
		for _, name := range []string{"AXExtrasMenuBar", "AXChildren", "AXPosition", "AXSize", "AXTitle", "AXDescription", "AXHelp", "AXIdentifier", "AXRole"} {
			axAttributes[name] = macos.CFString(name)
		}
	})
}

type darwinItems struct {
	onMain func(func())
}

func (d *darwinItems) trusted() bool {
	loadAX()
	return axIsProcessTrusted()
}

func (d *darwinItems) requestTrust() bool {
	loadAX()
	options, err := macos.ToCF(map[string]any{"AXTrustedCheckOptionPrompt": true})
	if err != nil {
		return axIsProcessTrusted()
	}
	defer cfRelease(options)
	return axIsProcessTrustedWithOptions(options)
}

func (d *darwinItems) runningApps() []appRef {
	var apps []appRef
	d.onMain(func() {
		withPool(func() {
			list := send(send(class("NSWorkspace"), "sharedWorkspace"), "runningApplications")
			n := objc.Send[uint](list, sel("count"))
			for i := range n {
				app := send(list, "objectAtIndex:", i)
				ref := appRef{
					PID:      objc.Send[int32](app, sel("processIdentifier")),
					Name:     goString(send(app, "localizedName")),
					BundleID: goString(send(app, "bundleIdentifier")),
					Path:     goString(send(send(app, "bundleURL"), "path")),
				}
				executable := goString(send(send(app, "executableURL"), "lastPathComponent"))
				for _, owner := range []string{ref.BundleID, ref.Name, executable} {
					if owner != "" {
						ref.Owners = append(ref.Owners, owner)
					}
				}
				if ref.Name == "" {
					ref.Name = firstNonEmpty(ref.BundleID, executable, "?")
				}
				apps = append(apps, ref)
			}
		})
	})
	return apps
}

func copyAttribute(element uintptr, name string) uintptr {
	var value uintptr
	if axUIElementCopyAttributeValue(element, axAttributes[name], &value) != 0 {
		return 0
	}
	return value
}

func stringAttribute(element uintptr, name string) string {
	ref := copyAttribute(element, name)
	if ref == 0 {
		return ""
	}
	defer cfRelease(ref)
	value, _ := macos.FromCF(ref)
	s, _ := value.(string)
	return s
}

func frameOf(element uintptr) Rect {
	const pointType, sizeType = 1, 2 // kAXValueCGPointType, kAXValueCGSizeType
	var origin nsPoint
	var size nsSize
	if ref := copyAttribute(element, "AXPosition"); ref != 0 {
		axValueGetValue(ref, pointType, unsafe.Pointer(&origin))
		cfRelease(ref)
	}
	if ref := copyAttribute(element, "AXSize"); ref != 0 {
		axValueGetValue(ref, sizeType, unsafe.Pointer(&size))
		cfRelease(ref)
	}
	return Rect{origin.X, origin.Y, size.W, size.H}
}

// children calls fn with each item of the app's extras menu bar.
func extrasChildren(pid int32, fn func(index int, element uintptr)) {
	app := axUIElementCreateApplication(pid)
	if app == 0 {
		return
	}
	defer cfRelease(app)
	axUIElementSetMessagingTimeout(app, axMessageTimeout)
	bar := copyAttribute(app, "AXExtrasMenuBar")
	if bar == 0 {
		return
	}
	defer cfRelease(bar)
	children := copyAttribute(bar, "AXChildren")
	if children == 0 {
		return
	}
	defer cfRelease(children)
	for i := range cfArrayGetCount(children) {
		fn(i, cfArrayGetValueAtIndex(children, i))
	}
}

// readItems asks every app in parallel, since an AX call blocks until the other app answers.
func (d *darwinItems) readItems(pids []int32) []rawItem {
	loadAX()
	var (
		mu     sync.Mutex
		result []rawItem
		wg     sync.WaitGroup
		slots  = make(chan struct{}, axParallelism)
	)
	for _, pid := range pids {
		wg.Add(1)
		slots <- struct{}{}
		go func(pid int32) {
			defer wg.Done()
			defer func() { <-slots }()
			var found []rawItem
			extrasChildren(pid, func(index int, element uintptr) {
				title := stringAttribute(element, "AXTitle")
				if title == "" {
					title = stringAttribute(element, "AXDescription")
				}
				found = append(found, rawItem{
					PID:        pid,
					Index:      index,
					Title:      title,
					Help:       stringAttribute(element, "AXHelp"),
					Identifier: stringAttribute(element, "AXIdentifier"),
					Frame:      frameOf(element),
				})
			})
			mu.Lock()
			result = append(result, found...)
			mu.Unlock()
		}(pid)
	}
	wg.Wait()
	return result
}

// overflowButton finds the system « button: MenuBarAgent's only button among its status groups,
// present while the bar overflows.
// barAnchors reads MenuBarAgent's items: its AXButton is the « button, and most of its items (Control
// Center, the clock) sit on the bar, so the row they share is the bar's.
func (d *darwinItems) barAnchors(apps []appRef) barAnchors {
	loadAX()
	var anchors barAnchors
	for _, app := range apps {
		if app.BundleID != menuBarAgentID {
			continue
		}
		rows := map[int][]Rect{}
		extrasChildren(app.PID, func(_ int, element uintptr) {
			f := frameOf(element)
			if anchors.overflow == nil && stringAttribute(element, "AXRole") == "AXButton" {
				anchors.overflow = &f
			}
			rows[int(math.Round(f.MidY()))] = append(rows[int(math.Round(f.MidY()))], f)
		})
		for _, frames := range rows {
			if anchors.row == nil || len(frames) > len(rows[int(math.Round(anchors.row.MidY()))]) {
				anchors.row = &frames[0]
			}
		}
	}
	return anchors
}

func (d *darwinItems) commandDrag(start, end Point) { commandDrag(start, end) }

func (d *darwinItems) appIcon(path string) []byte {
	png, err := mygo.Shell.FileIcon(path, 64)
	if err != nil {
		return nil
	}
	return png
}

// MARK: - Layout file

func (d *darwinItems) positions() map[string]float64 {
	if value, ok := macos.DomainPreferences(layoutPath).Value(positionsKey); ok {
		return toPositions(value)
	}
	// Without cfprefsd's cooperation the file itself may still be readable after the grant.
	data, err := os.ReadFile(layoutPath)
	if err != nil {
		return nil
	}
	value, ok := plistFromBytes(data)
	if !ok {
		return nil
	}
	dict, _ := value.(map[string]any)
	return toPositions(dict[positionsKey])
}

func toPositions(value any) map[string]float64 {
	dict, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]float64, len(dict))
	for key, v := range dict {
		switch n := v.(type) {
		case float64:
			out[key] = n
		case int64:
			out[key] = float64(n)
		}
	}
	return out
}

// setPosition writes through cfprefsd so MenuBarAgent sees the change; a direct file write would be
// overwritten from cfprefsd's cache and never reach the bar.
func (d *darwinItems) setPosition(key string, value float64) bool {
	all := d.positions()
	if all == nil {
		return false
	}
	dict := make(map[string]any, len(all)+1)
	for k, v := range all {
		dict[k] = v
	}
	dict[key] = value
	if err := macos.DomainPreferences(layoutPath).Set(positionsKey, dict); err != nil {
		return false
	}
	return d.positions()[key] == value
}

// reloadMenuBar restarts MenuBarAgent, which launchd starts again at once, and waits until the new
// one has laid the bar out from the layout file.
func (d *darwinItems) reloadMenuBar() {
	old := d.menuBarAgentPID()
	if old == 0 || syscall.Kill(int(old), syscall.SIGTERM) != nil {
		return
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		if pid := d.menuBarAgentPID(); pid != 0 && pid != old {
			break
		}
	}
	// Status items register with the new agent a moment after it starts.
	time.Sleep(time.Second)
}

func (d *darwinItems) menuBarAgentPID() int32 {
	for _, app := range d.runningApps() {
		if app.BundleID == menuBarAgentID {
			return app.PID
		}
	}
	return 0
}

// requestLayoutAccess asks the user to pick the file; their choice is what lets this app past the
// container's protection. Main thread only: it runs a modal panel.
func (d *darwinItems) requestLayoutAccess() bool {
	granted := false
	withPool(func() {
		panel := send(class("NSOpenPanel"), "openPanel")
		send(panel, "setDirectoryURL:", send(class("NSURL"), "fileURLWithPath:", nsString(filepath.Dir(layoutPath))))
		msgBool(panel, sel("setCanChooseFiles:"), true)
		msgBool(panel, sel("setCanChooseDirectories:"), false)
		msgBool(panel, sel("setAllowsMultipleSelection:"), false)
		msgBool(panel, sel("setShowsHiddenFiles:"), true)
		send(panel, "setMessage:", nsString(tr(
			"Choose com.apple.MenuBar.plist so PowerfulTop can arrange the menu bar without moving the pointer.",
			"请选择 com.apple.MenuBar.plist，PowerfulTop 就能在不移动鼠标的情况下整理菜单栏。")))
		send(panel, "setPrompt:", nsString(tr("Allow", "允许")))
		msgBool(send(class("NSApplication"), "sharedApplication"), sel("activateIgnoringOtherApps:"), true)
		const modalResponseOK = 1
		if objc.Send[int](panel, sel("runModal")) != modalResponseOK {
			return
		}
		url := send(panel, "URL")
		if filepath.Clean(goString(send(url, "path"))) != filepath.Clean(layoutPath) {
			return
		}
		send(url, "startAccessingSecurityScopedResource")
		const withSecurityScope = 1 << 11 // NSURLBookmarkCreationWithSecurityScope
		bookmark := send(url, "bookmarkDataWithOptions:includingResourceValuesForKeys:relativeToURL:error:", uint64(withSecurityScope), objc.ID(0), objc.ID(0), objc.ID(0))
		if bookmark == 0 {
			bookmark = send(url, "bookmarkDataWithOptions:includingResourceValuesForKeys:relativeToURL:error:", uint64(0), objc.ID(0), objc.ID(0), objc.ID(0))
		}
		if b := dataBytes(bookmark); len(b) > 0 {
			_ = macos.AppPreferences().Set(bookmarkKey, b)
		}
		granted = true
	})
	return granted && d.positions() != nil
}

// restoreLayoutAccess re-opens the grant saved by requestLayoutAccess on a previous launch.
func (d *darwinItems) restoreLayoutAccess() {
	bookmark, ok := macos.AppPreferences().Data(bookmarkKey)
	if !ok {
		return
	}
	withPool(func() {
		const withSecurityScope = 1 << 10 // NSURLBookmarkResolutionWithSecurityScope
		var stale bool
		resolve := func(options uint64) objc.ID {
			return send(class("NSURL"), "URLByResolvingBookmarkData:options:relativeToURL:bookmarkDataIsStale:error:",
				nsData(bookmark), options, objc.ID(0), unsafe.Pointer(&stale), objc.ID(0))
		}
		url := resolve(withSecurityScope)
		if url == 0 {
			url = resolve(0)
		}
		if url != 0 {
			send(url, "startAccessingSecurityScopedResource")
		}
	})
}
