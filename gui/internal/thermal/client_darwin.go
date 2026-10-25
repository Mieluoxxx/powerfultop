//go:build darwin

package thermal

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"
)

// HelperStatus is whether the installed helper serves this build.
type HelperStatus int

const (
	HelperNotInstalled HelperStatus = iota
	// HelperNeedsUpdate: the installed helper or its recorded client requirement no longer matches this build.
	HelperNeedsUpdate
	HelperReady
)

var (
	ErrMissingHelper    = errors.New("the fan helper is missing from the app bundle")
	ErrUnsignedApp      = errors.New("PowerfulTop must be code-signed to control fans")
	ErrCancelled        = errors.New("authorization was cancelled")
	ErrConnectionFailed = errors.New("could not connect to the fan helper")
)

// HelperClient installs and talks to the root fan helper. The app itself never writes to the SMC.
type HelperClient struct {
	// BundledPath is the helper inside the app's resources.
	BundledPath string

	installedPath, daemonPath, requirementPath, socketPath, supportDir, markerPath string
	legacy                                                                         []legacyHelper
	requirement                                                                    func() (string, error)
	runAdministrator                                                               func(script string) error
}

// NewHelperClient addresses the installed helper; bundledPath is where this build ships it.
func NewHelperClient(bundledPath string) *HelperClient {
	return &HelperClient{
		BundledPath:      bundledPath,
		installedPath:    HelperInstalledPath,
		daemonPath:       HelperDaemonPath,
		requirementPath:  ClientRequirementPath,
		socketPath:       HelperSocketPath,
		supportDir:       HelperSupportDir,
		markerPath:       EngagedMarkerPath,
		legacy:           LegacyHelpers,
		requirement:      ClientRequirement,
		runAdministrator: runAsAdministrator,
	}
}

// Status compares the installed helper and its recorded requirement with this build's.
func (c *HelperClient) Status() HelperStatus {
	// A helper left by an older bundle ID would otherwise stay registered as a root daemon forever.
	for _, l := range c.legacy {
		if exists(l.DaemonPath) {
			return HelperNeedsUpdate
		}
	}
	if !exists(c.installedPath) || !exists(c.daemonPath) {
		return HelperNotInstalled
	}
	bundled, errB := os.ReadFile(c.BundledPath)
	installed, errI := os.ReadFile(c.installedPath)
	recorded, errR := os.ReadFile(c.requirementPath)
	requirement, errQ := c.requirement()
	if errB != nil || errI != nil || errR != nil || errQ != nil || !bytes.Equal(bundled, installed) ||
		strings.TrimSpace(string(recorded)) != requirement {
		return HelperNeedsUpdate
	}
	return HelperReady
}

// Install asks for an administrator password once, then registers the helper as a launchd daemon.
func (c *HelperClient) Install() error {
	if !exists(c.BundledPath) {
		return ErrMissingHelper
	}
	requirement, err := c.requirement()
	if err != nil || requirement == "" {
		return ErrUnsignedApp
	}
	return c.runAdministrator(c.installScript(requirement))
}

// Uninstall removes the helper and anything an older build left behind.
func (c *HelperClient) Uninstall() error {
	return c.runAdministrator(c.uninstallScript())
}

func (c *HelperClient) daemonPlist() string {
	// RunAtLoad and KeepAlive: the helper owns its socket, so it must be up before the app connects.
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>` + HelperLabel + `</string>
<key>Program</key><string>` + c.installedPath + `</string>
<key>RunAtLoad</key><true/>
<key>KeepAlive</key><true/>
</dict></plist>`
}

func (c *HelperClient) installScript(requirement string) string {
	q := shellQuote
	return strings.Join([]string{
		"set -e",
		"launchctl bootout system/" + HelperLabel + " 2>/dev/null || true",
		"mkdir -p /Library/PrivilegedHelperTools " + q(c.supportDir),
		// Root-owned and closed to the user: the socket and the requirement live here.
		"chown root:wheel " + q(c.supportDir),
		"chmod 755 " + q(c.supportDir),
		c.legacyCleanupScript(),
		"cp " + q(c.BundledPath) + " " + q(c.installedPath),
		"chown root:wheel " + q(c.installedPath),
		"chmod 755 " + q(c.installedPath),
		"printf '%s' " + q(requirement) + " > " + q(c.requirementPath),
		"chown root:wheel " + q(c.requirementPath),
		"chmod 644 " + q(c.requirementPath),
		"printf '%s' " + q(c.daemonPlist()) + " > " + q(c.daemonPath),
		"chown root:wheel " + q(c.daemonPath),
		"chmod 644 " + q(c.daemonPath),
		"launchctl bootstrap system " + q(c.daemonPath),
	}, "\n")
}

func (c *HelperClient) uninstallScript() string {
	q := shellQuote
	return strings.Join([]string{
		"launchctl bootout system/" + HelperLabel + " 2>/dev/null || true",
		"rm -f " + q(c.daemonPath) + " " + q(c.installedPath) + " " + q(c.requirementPath),
		c.legacyCleanupScript(),
	}, "\n")
}

// legacyCleanupScript removes older helpers. The upstream helper has no SIGTERM handler, so if it
// held the fans when unloaded they stay manual; moving its marker makes ours restore at start.
func (c *HelperClient) legacyCleanupScript() string {
	q := shellQuote
	var lines []string
	for _, l := range c.legacy {
		lines = append(lines,
			"if [ -e "+q(l.EngagedMarkerPath)+" ]; then mkdir -p "+q(c.supportDir)+"; touch "+q(c.markerPath)+"; fi",
			"launchctl bootout system/"+l.Label+" 2>/dev/null || true",
			"rm -f "+q(l.DaemonPath)+" "+q(l.InstalledPath),
			"rm -rf "+q(l.SupportDir),
		)
	}
	return strings.Join(lines, "\n")
}

// SetFanTargets sends RPMs per fan index; the helper clamps them and unlocks manual mode if needed.
func (c *HelperClient) SetFanTargets(rpms []float64) error {
	_, err := c.call(Request{Op: OpSetTargets, RPMs: rpms}, requestDeadline)
	return err
}

// RestoreAutomatic hands the fans back to macOS, waiting at most timeout.
func (c *HelperClient) RestoreAutomatic(timeout time.Duration) error {
	_, err := c.call(Request{Op: OpRestore}, timeout)
	return err
}

// ProtocolVersion asks the helper which protocol it speaks.
func (c *HelperClient) ProtocolVersion() (int, error) {
	resp, err := c.call(Request{Op: OpVersion}, 3*time.Second)
	return resp.Version, err
}

func (c *HelperClient) call(req Request, timeout time.Duration) (Response, error) {
	conn, err := net.DialTimeout("unix", c.socketPath, min(timeout, 2*time.Second))
	if err != nil {
		return Response{}, ErrConnectionFailed
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	if err := WriteMessage(conn, req); err != nil {
		return Response{}, ErrConnectionFailed
	}
	var resp Response
	if err := ReadMessage(conn, &resp); err != nil {
		return Response{}, ErrConnectionFailed
	}
	if resp.Error != "" {
		return resp, errors.New(resp.Error)
	}
	return resp, nil
}

// runAsAdministrator runs a shell script through AppleScript's administrator prompt. osascript keeps
// AppleScript off the app's main thread, which NSAppleScript would need.
func runAsAdministrator(script string) error {
	source := "do shell script " + appleScriptQuote(script) + " with administrator privileges"
	out, err := exec.Command("/usr/bin/osascript", "-e", source).CombinedOutput()
	if err == nil {
		return nil
	}
	message := strings.TrimSpace(string(out))
	if strings.Contains(message, "(-128)") {
		return ErrCancelled
	}
	if message == "" {
		message = err.Error()
	}
	return fmt.Errorf("%s", message)
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func appleScriptQuote(s string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
