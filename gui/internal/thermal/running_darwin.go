//go:build darwin

package thermal

import "powerfultop/internal/macos"

// otherFanTool names a running fan tool, "" when none runs.
func otherFanTool() string {
	for _, tool := range conflictingTools {
		if macos.AppRunning(tool.BundleID) {
			return tool.Name
		}
	}
	return ""
}
