import AppKit

/// The one file where macOS 27 keeps the menu bar order: each item's preferred distance from the
/// right edge, keyed "status:<owner>::<autosave name>". Reading it doesn't depend on which display's
/// bar is active or what overflows, and writing it through cfprefsd moves an item at once without
/// touching the pointer (the approach Thaw 3 takes). It lives in an Apple group container, so macOS
/// denies access until the user picks the file once.
enum MenuBarLayoutFile {
    static let url = FileManager.default.homeDirectoryForCurrentUser
        .appendingPathComponent("Library/Group Containers/com.apple.MenuBar/Library/Preferences/com.apple.MenuBar.plist")

    private static let positionsKey = "TrailingItemPreferredPositions" as CFString
    private static let bookmarkKey = "menuBarLayoutFileBookmark"
    /// cfprefsd accepts an absolute plist path as the domain.
    private static var domain: CFString { url.path as CFString }

    static func positions() -> [String: Double]? {
        CFPreferencesAppSynchronize(domain)
        if let raw = CFPreferencesCopyAppValue(positionsKey, domain) as? [String: NSNumber] {
            return raw.mapValues(\.doubleValue)
        }
        // Without cfprefsd's cooperation the file itself may still be readable after the grant.
        guard let data = try? Data(contentsOf: url),
              let plist = try? PropertyListSerialization.propertyList(from: data, format: nil) as? [String: Any],
              let raw = plist[positionsKey as String] as? [String: NSNumber] else { return nil }
        return raw.mapValues(\.doubleValue)
    }

    /// Writes through cfprefsd so MenuBarAgent sees the change; a direct file write would be
    /// overwritten from cfprefsd's cache and never reach the bar.
    @discardableResult
    static func setPosition(_ position: Double, for key: String) -> Bool {
        guard var all = positions() else { return false }
        all[key] = position
        CFPreferencesSetAppValue(positionsKey, all as CFDictionary, domain)
        guard CFPreferencesAppSynchronize(domain) else { return false }
        return positions()?[key] == position
    }

    // MARK: - Access

    /// Asks the user to pick the file; their choice is what lets this app past the container's protection.
    @MainActor
    static func requestAccess() -> Bool {
        let panel = NSOpenPanel()
        panel.directoryURL = url.deletingLastPathComponent()
        panel.canChooseFiles = true
        panel.canChooseDirectories = false
        panel.allowsMultipleSelection = false
        panel.showsHiddenFiles = true
        panel.message = String(localized: "Choose com.apple.MenuBar.plist so PowerfulTop can arrange the menu bar without moving the pointer.")
        panel.prompt = String(localized: "Allow")
        NSApp.activate(ignoringOtherApps: true)
        guard panel.runModal() == .OK, let picked = panel.url,
              picked.standardizedFileURL.path == url.standardizedFileURL.path else { return false }
        _ = picked.startAccessingSecurityScopedResource()
        if let bookmark = (try? picked.bookmarkData(options: .withSecurityScope)) ?? (try? picked.bookmarkData()) {
            UserDefaults.standard.set(bookmark, forKey: bookmarkKey)
        }
        return positions() != nil
    }

    /// Re-opens the grant saved by `requestAccess` on a previous launch.
    static func restoreAccess() {
        guard let bookmark = UserDefaults.standard.data(forKey: bookmarkKey) else { return }
        var stale = false
        let resolved = (try? URL(resolvingBookmarkData: bookmark, options: .withSecurityScope, bookmarkDataIsStale: &stale))
            ?? (try? URL(resolvingBookmarkData: bookmark, options: [], bookmarkDataIsStale: &stale))
        _ = resolved?.startAccessingSecurityScopedResource()
    }
}
