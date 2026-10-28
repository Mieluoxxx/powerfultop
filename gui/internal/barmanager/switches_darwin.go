//go:build darwin

package barmanager

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/egoist/mygo"

	"powerfultop/internal/lang"
	"powerfultop/internal/macos"
)

const (
	allowListKey         = "trackedApplications"
	allowListBookmarkKey = "menuBarAllowListBookmark"
)

// allowListPath is Control Center's list behind System Settings › Menu Bar › Allow in the Menu Bar.
// Its container is protected; the user picking the file lets this app at that one file. A write
// through CFPreferences by absolute path goes through cfprefsd, which tells MenuBarAgent at once;
// UserDefaults by suite name would need Apple's app group entitlement and reach another file.
var allowListPath = filepath.Join(os.Getenv("HOME"), "Library/Group Containers/group.com.apple.controlcenter/Library/Preferences/group.com.apple.controlcenter.plist")

func (d *darwinItems) allowList() []any {
	prefs := macos.DomainPreferences(allowListPath)
	// MenuBarAgent adds entries while PowerfulTop runs; writing back a stale copy would drop them,
	// and it would take those apps for new ones.
	prefs.Synchronize()
	data, ok := prefs.Data(allowListKey)
	if !ok {
		return nil
	}
	value, err := macos.DecodePlistData(data)
	if err != nil {
		return nil
	}
	list, ok := value.([]any)
	if !ok {
		return nil
	}
	return list
}

func (d *darwinItems) setAllowList(list []any) bool {
	data, err := macos.EncodeBinaryPlist(list)
	if err != nil {
		return false
	}
	return macos.DomainPreferences(allowListPath).Set(allowListKey, data) == nil
}

func (d *darwinItems) requestAllowListAccess() bool {
	granted := d.requestFileAccess(allowListPath, allowListBookmarkKey, lang.L(
		"Choose group.com.apple.controlcenter.plist so PowerfulTop can switch menu bar icons on and off for the tray.",
		"请选择 group.com.apple.controlcenter.plist，PowerfulTop 才能为托盘开关菜单栏图标。"))
	return granted && d.allowList() != nil
}

func (d *darwinItems) checkbox(key string) (int, bool) {
	v, ok := macos.CurrentHostPreferences(controlCenterDomain).Value(key)
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	}
	return 0, false
}

func (d *darwinItems) setCheckbox(key string, w checkboxWrite) {
	var value any
	if !w.remove {
		value = int64(w.value)
	}
	_ = macos.CurrentHostPreferences(controlCenterDomain).Set(key, value)
}

func (d *darwinItems) flag(f flagSwitch) (bool, bool) {
	var v any
	var ok bool
	if f.currentHost {
		v, ok = macos.CurrentHostPreferences(f.domain).Value(f.key)
	} else {
		prefs := macos.DomainPreferences(f.domain)
		prefs.Synchronize()
		v, ok = prefs.Value(f.key)
	}
	if !ok {
		return false, false
	}
	return macos.Bool(map[string]any{"v": v}, "v")
}

func (d *darwinItems) setFlag(f flagSwitch, value bool) {
	if f.currentHost {
		_ = macos.CurrentHostPreferences(f.domain).Set(f.key, value)
	} else {
		_ = macos.DomainPreferences(f.domain).Set(f.key, value)
	}
}

// setLoginItem stops or starts Weather's menu helper: launchd restarts the process of a running
// job, so the job goes first. Login starts it again, and the tray switches it off again.
func (d *darwinItems) setLoginItem(l loginItem, running bool) {
	if running {
		if _, ok := d.runningApp(l.label); !ok {
			_ = mygo.Shell.OpenPath(l.app)
		}
		return
	}
	_ = exec.Command("/bin/launchctl", "bootout", "gui/"+strconv.Itoa(os.Getuid())+"/"+l.label).Run()
	// One the tray launched itself is not launchd's.
	if app, ok := d.runningApp(l.label); ok {
		_ = syscall.Kill(int(app.PID), syscall.SIGTERM)
	}
}

func (d *darwinItems) launch(path string) { _ = mygo.Shell.OpenPath(path) }

// restart relies on launchd starting the process again at once, as it does Control Center and
// the text input menu; measured on macOS 27.0.1, the switch then takes within a third of a second.
func (d *darwinItems) restart(bundleID string) {
	if app, ok := d.runningApp(bundleID); ok {
		_ = syscall.Kill(int(app.PID), syscall.SIGTERM)
	}
}
