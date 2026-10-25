//go:build darwin

package thermal

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClientRequirementPinsThisBuild(t *testing.T) {
	req, err := ClientRequirement()
	if err != nil {
		t.Skip("test binary is not signed:", err)
	}
	if !strings.HasPrefix(req, `cdhash H"`) || len(req) < len(`cdhash H""`)+40 {
		t.Errorf("requirement %q", req)
	}
}

// serve starts a helper on a socket in /tmp (TMPDIR paths overflow sun_path's 104 bytes).
func serve(t *testing.T, s *HelperServer) string {
	dir, err := os.MkdirTemp("/tmp", "pth")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "s")
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go s.Serve(l)
	return path
}

// The real check: the peer's audit token against a code requirement, end to end over a socket.
func TestHelperAcceptsOnlyTheRecordedBuild(t *testing.T) {
	self, err := ClientRequirement()
	if err != nil {
		t.Skip("test binary is not signed:", err)
	}
	smc := newFakeSMC()
	controller := NewFanController(smc, filepath.Join(t.TempDir(), "engaged"))

	client := &HelperClient{socketPath: serve(t, &HelperServer{Controller: controller, Requirement: self})}
	if v, err := client.ProtocolVersion(); err != nil || v != HelperProtocolVersion {
		t.Fatalf("this build must be accepted: %v %v", v, err)
	}

	other := `cdhash H"` + strings.Repeat("00", 20) + `"`
	client = &HelperClient{socketPath: serve(t, &HelperServer{Controller: controller, Requirement: other})}
	if err := client.SetFanTargets([]float64{3000}); err == nil || !strings.Contains(err.Error(), "unauthorized") {
		t.Errorf("another build must be refused: %v", err)
	}
	if len(smc.writes) != 0 {
		t.Error("a refused client must not reach the SMC")
	}
}

func TestHelperRoundTrip(t *testing.T) {
	smc := newFakeSMC()
	controller := NewFanController(smc, filepath.Join(t.TempDir(), "engaged"))
	server := &HelperServer{Controller: controller, Verify: func(*net.UnixConn, string) error { return nil }}
	client := &HelperClient{socketPath: serve(t, server)}

	if err := client.SetFanTargets([]float64{4000, 5000}); err != nil {
		t.Fatal(err)
	}
	if smc.float("F0Tg") != 4000 || !controller.Engaged() {
		t.Error("targets reach the SMC")
	}
	if err := client.SetFanTargets([]float64{-5}); err == nil {
		t.Error("an invalid value is refused")
	}
	if err := client.RestoreAutomatic(time.Second); err != nil || controller.Engaged() {
		t.Errorf("restore: %v", err)
	}
	if (&HelperClient{socketPath: "/tmp/does-not-exist.sock"}).RestoreAutomatic(time.Second) != ErrConnectionFailed {
		t.Error("no helper is a connection failure")
	}
}

func TestHelperStatus(t *testing.T) {
	dir := t.TempDir()
	c := &HelperClient{
		BundledPath:     filepath.Join(dir, "bundled"),
		installedPath:   filepath.Join(dir, "installed"),
		daemonPath:      filepath.Join(dir, "daemon.plist"),
		requirementPath: filepath.Join(dir, "client.req"),
		requirement:     func() (string, error) { return `cdhash H"ab"`, nil },
	}
	write := func(path, s string) { _ = os.WriteFile(path, []byte(s), 0o644) }
	write(c.BundledPath, "v2")

	if c.Status() != HelperNotInstalled {
		t.Error("nothing installed")
	}
	write(c.installedPath, "v2")
	write(c.daemonPath, "plist")
	write(c.requirementPath, `cdhash H"ab"`+"\n")
	if c.Status() != HelperReady {
		t.Error("same bytes and requirement are ready")
	}
	write(c.requirementPath, `cdhash H"cd"`)
	if c.Status() != HelperNeedsUpdate {
		t.Error("a rebuilt app must authorize the helper again")
	}
	write(c.requirementPath, `cdhash H"ab"`)
	write(c.installedPath, "v1")
	if c.Status() != HelperNeedsUpdate {
		t.Error("an older helper binary needs an update")
	}
	write(c.installedPath, "v2")
	c.legacy = []legacyHelper{{DaemonPath: filepath.Join(dir, "legacy.plist")}}
	write(c.legacy[0].DaemonPath, "plist")
	if c.Status() != HelperNeedsUpdate {
		t.Error("a helper of the old bundle ID must be replaced")
	}
}

func TestInstallScript(t *testing.T) {
	c := NewHelperClient("/Applications/Power'fulTop.app/Contents/Resources/bin/PowerfulTopFanHelper")
	script := c.installScript(`cdhash H"ab"`)
	for _, want := range []string{
		`'/Applications/Power'\''fulTop.app/Contents/Resources/bin/PowerfulTopFanHelper'`,
		"chown root:wheel '" + HelperSupportDir + "'",
		"printf '%s' 'cdhash H\"ab\"' > '" + ClientRequirementPath + "'",
		"<key>KeepAlive</key><true/>",
		"launchctl bootstrap system '" + HelperDaemonPath + "'",
		"launchctl bootout system/com.kdolphin.PowerTop.FanHelper",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("install script lacks %q", want)
		}
	}
	if strings.Contains(c.uninstallScript(), "bootstrap") {
		t.Error("uninstall must not start anything")
	}
	if got := appleScriptQuote(`a "b" \c`); got != `"a \"b\" \\c"` {
		t.Errorf("AppleScript quoting %s", got)
	}
	var b bytes.Buffer
	_ = WriteMessage(&b, Request{Op: OpRestore})
	var r Request
	if err := ReadMessage(&b, &r); err != nil || r.Op != OpRestore {
		t.Error("message round trip")
	}
}
