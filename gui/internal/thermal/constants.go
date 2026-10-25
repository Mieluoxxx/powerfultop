package thermal

import "time"

// Helper identity and paths. The Go helper uses names of its own so it never collides with the
// Swift app's installed helper while both builds exist; the cut-over switches them here only.
const (
	HelperLabel           = "io.github.mieluoxxx.PowerfulTop.mygo.FanHelper"
	HelperInstalledPath   = "/Library/PrivilegedHelperTools/" + HelperLabel
	HelperDaemonPath      = "/Library/LaunchDaemons/" + HelperLabel + ".plist"
	HelperSupportDir      = "/Library/Application Support/PowerfulTop.mygo"
	ClientRequirementPath = HelperSupportDir + "/fan-helper-client.req"
	// Present while our helper holds the fans; survives a helper crash so the next start can restore.
	EngagedMarkerPath = HelperSupportDir + "/fan-helper.engaged"
	// Root-owned directory, world-connectable socket: every connection is checked against the recorded requirement.
	HelperSocketPath = HelperSupportDir + "/fan-helper.sock"

	// BundledHelperName is the helper's path inside the app's resources (Contents/Resources).
	BundledHelperName = "bin/PowerfulTopFanHelper"

	HelperProtocolVersion = 1
	// Ftst handoff took 10–12 s on an M4 Max under macOS 27, longer than the 3–6 s reported elsewhere.
	EngageTimeout = 20 * time.Second
	// The helper hands fans back to macOS if the app stops checking in for this long.
	HeartbeatTimeout = 15 * time.Second
	// The helper hands fans back to macOS at this temperature regardless of the requested mode.
	FailsafeCelsius = 100.0
)

// legacyHelper is a helper installed by builds published under the upstream bundle ID.
type legacyHelper struct {
	Label, InstalledPath, DaemonPath, SupportDir, EngagedMarkerPath string
}

// LegacyHelpers are removed on install; their engaged marker moves to ours so the new helper
// restores automatic control as soon as it starts. The cut-over adds the Swift app's helper here.
var LegacyHelpers = []legacyHelper{{
	Label:             "com.kdolphin.PowerTop.FanHelper",
	InstalledPath:     "/Library/PrivilegedHelperTools/com.kdolphin.PowerTop.FanHelper",
	DaemonPath:        "/Library/LaunchDaemons/com.kdolphin.PowerTop.FanHelper.plist",
	SupportDir:        "/Library/Application Support/PowerTop",
	EngagedMarkerPath: "/Library/Application Support/PowerTop/fan-helper.engaged",
}}

// conflictingBundleIDs drive fans themselves; pre-rename builds run their own helper and would fight over the fans.
var conflictingBundleIDs = []string{"com.crystalidea.macsfancontrol", "com.kdolphin.PowerTop"}
