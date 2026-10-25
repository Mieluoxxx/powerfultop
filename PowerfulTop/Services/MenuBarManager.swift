import AppKit
import Observation

/// Bartender-style hiding built on the macOS 27 menu bar: when collapsed, blank spacers push the
/// icons ⌘-dragged left of the separator into the system's own « overflow, without screen capture,
/// click forwarding or any privacy permission. The « reveal shows the blank spacers first, so the
/// chevron is the way back.
@MainActor
@Observable
final class MenuBarManager {
    /// macOS 26 and earlier have no overflow menu to hand hidden icons to.
    static let isSupported = ProcessInfo.processInfo.operatingSystemVersion.majorVersion >= 27
    static let autoHideChoices: [TimeInterval] = [0, 5, 10, 30, 60]

    private static let enabledKey = "menuBarManagerEnabled"
    private static let autoHideDelayKey = "menuBarManagerAutoHideDelay"
    static let autosavePrefix = "PowerfulTop.MenuBarSection."
    /// Lets the item scanner tell this app's own layout items apart from everyone's icons.
    static let separatorIdentifier = autosavePrefix + "separator"
    static let toggleIdentifier = autosavePrefix + "toggle"
    static let spacerIdentifier = autosavePrefix + "spacer"
    /// Apps that register late in a login storm would otherwise be swallowed before they appear.
    private static let launchCollapseDelay: Duration = .milliseconds(2500)

    var isEnabled: Bool {
        didSet {
            guard isEnabled != oldValue else { return }
            UserDefaults.standard.set(isEnabled, forKey: Self.enabledKey)
            isEnabled ? install(collapseAfterLaunch: false) : uninstall()
        }
    }

    /// Seconds before revealed icons hide again; 0 keeps them until the next click.
    var autoHideDelay: TimeInterval {
        didSet {
            UserDefaults.standard.set(autoHideDelay, forKey: Self.autoHideDelayKey)
            scheduleAutoHide()
        }
    }

    private(set) var isCollapsed = false

    @ObservationIgnored var openSettings: (() -> Void)?
    @ObservationIgnored private var toggleItem: NSStatusItem?
    @ObservationIgnored private var separatorItem: NSStatusItem?
    @ObservationIgnored private var spacerItems: [NSStatusItem] = []
    @ObservationIgnored private var launchCollapseTask: Task<Void, Never>?
    @ObservationIgnored private var autoHideTask: Task<Void, Never>?
    @ObservationIgnored private var screenObserver: NSObjectProtocol?
    @ObservationIgnored private var editingCount = 0
    @ObservationIgnored private var collapsedBeforeEditing = false

    init() {
        let defaults = UserDefaults.standard
        isEnabled = defaults.bool(forKey: Self.enabledKey)
        autoHideDelay = defaults.object(forKey: Self.autoHideDelayKey) as? TimeInterval ?? 10
    }

    func start() {
        guard isEnabled else { return }
        install(collapseAfterLaunch: true)
    }

    func shutdown() {
        uninstall()
    }

    func toggle() {
        isCollapsed ? expand() : collapse()
    }

    func collapse() {
        guard toggleItem != nil else { return }
        cancelPendingWork()
        isCollapsed = true
        applyGeometry()
        updateToggle()
    }

    func expand() {
        guard toggleItem != nil else { return }
        cancelPendingWork()
        isCollapsed = false
        applyGeometry()
        updateToggle()
        scheduleAutoHide()
    }

    /// Item frames are only meaningful while expanded, so the layout editor holds the bar open
    /// and puts it back the way it was when the editor goes away.
    func beginEditing() {
        editingCount += 1
        guard editingCount == 1 else { return }
        collapsedBeforeEditing = isCollapsed || launchCollapseTask != nil
        expand()
    }

    func endEditing() {
        guard editingCount > 0 else { return }
        editingCount -= 1
        guard editingCount == 0 else { return }
        collapsedBeforeEditing ? collapse() : scheduleAutoHide()
    }

    // MARK: - Items

    private func install(collapseAfterLaunch: Bool) {
        guard Self.isSupported, toggleItem == nil else { return }
        // macOS 27 keeps positions per autosave name and a name it has never seen lands leftmost,
        // so creating right to left puts the spacers between separator and toggle on first use.
        let toggle = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        toggle.autosaveName = Self.autosavePrefix + "toggle"
        spacerItems = (0..<MenuBarCollapseGeometry.spacerCount).map { index in
            let spacer = NSStatusBar.system.statusItem(withLength: 0)
            spacer.autosaveName = Self.autosavePrefix + "spacer\(index)"
            spacer.button?.setAccessibilityElement(false)
            spacer.button?.setAccessibilityIdentifier(Self.spacerIdentifier)
            // Never hidden: on 27.0.1 an item made visible again re-enters leftmost, left of the
            // icons it should push out. Zero length keeps the slot without taking room.
            spacer.isVisible = true
            return spacer
        }
        let separator = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        separator.autosaveName = Self.autosavePrefix + "separator"
        separator.button?.setAccessibilityLabel(String(localized: "Menu bar separator"))
        separator.button?.setAccessibilityIdentifier(Self.separatorIdentifier)

        if let button = toggle.button {
            button.target = self
            button.action = #selector(toggleClicked)
            button.sendAction(on: [.leftMouseUp, .rightMouseUp])
            button.setAccessibilityIdentifier(Self.toggleIdentifier)
        }
        toggleItem = toggle
        separatorItem = separator
        isCollapsed = false
        applyGeometry()
        updateToggle()

        // One unit length serves every display, so a newly attached narrow one can push it past its cliff.
        screenObserver = NotificationCenter.default.addObserver(
            forName: NSApplication.didChangeScreenParametersNotification, object: nil, queue: .main
        ) { [weak self] _ in
            MainActor.assumeIsolated {
                guard let self, self.isCollapsed else { return }
                self.applyGeometry()
            }
        }

        if collapseAfterLaunch {
            launchCollapseTask = Task { [weak self] in
                guard (try? await Task.sleep(for: Self.launchCollapseDelay)) != nil else { return }
                self?.collapse()
            }
        }
    }

    private func uninstall() {
        cancelPendingWork()
        if let screenObserver {
            NotificationCenter.default.removeObserver(screenObserver)
            self.screenObserver = nil
        }
        for item in [toggleItem, separatorItem].compactMap({ $0 }) + spacerItems {
            NSStatusBar.system.removeStatusItem(item)
        }
        toggleItem = nil
        separatorItem = nil
        spacerItems = []
        isCollapsed = false
    }

    private func applyGeometry() {
        guard let separator = separatorItem else { return }
        if isCollapsed {
            let displays = Self.displays()
            let unit = MenuBarCollapseGeometry.unitLength(displays: displays)
            let active = MenuBarCollapseGeometry.activeSpacers(unit: unit, displays: displays)
            // The span stays on screen while the bar overflows, so the glyph would float mid-bar.
            separator.button?.image = nil
            separator.length = unit
            for (index, spacer) in spacerItems.enumerated() {
                spacer.length = index < active ? unit : 0
            }
        } else {
            for spacer in spacerItems {
                spacer.length = 0
            }
            separator.length = NSStatusItem.variableLength
            separator.button?.image = Self.separatorImage
        }
    }

    private static func displays() -> [MenuBarCollapseGeometry.Display] {
        NSScreen.screens.map { screen in
            MenuBarCollapseGeometry.Display(width: screen.frame.width, statusWidth: screen.auxiliaryTopRightArea?.width)
        }
    }

    private func updateToggle() {
        guard let button = toggleItem?.button else { return }
        let image = NSImage(systemSymbolName: isCollapsed ? "chevron.left" : "chevron.right", accessibilityDescription: nil)?
            .withSymbolConfiguration(.init(pointSize: 13, weight: .semibold))
        image?.isTemplate = true
        button.image = image
        // The chevron is the only cue, so VoiceOver needs the state spelled out.
        button.setAccessibilityLabel(isCollapsed ? String(localized: "Show Hidden Items") : String(localized: "Hide Items"))
    }

    private static let separatorImage: NSImage = {
        let image = NSImage(size: NSSize(width: 8, height: 16), flipped: false) { _ in
            NSColor.black.withAlphaComponent(0.5).setFill()
            NSBezierPath(roundedRect: NSRect(x: 3, y: 1, width: 2, height: 14), xRadius: 1, yRadius: 1).fill()
            return true
        }
        image.isTemplate = true
        return image
    }()

    // MARK: - Interaction

    @objc private func toggleClicked() {
        if NSApp.currentEvent?.type == .rightMouseUp {
            showMenu()
        } else {
            toggle()
        }
    }

    private func showMenu() {
        guard let item = toggleItem else { return }
        let menu = NSMenu()
        addMenuItem(to: menu, title: isCollapsed ? String(localized: "Show Hidden Items") : String(localized: "Hide Items"), action: #selector(toggleFromMenu))
        menu.addItem(.separator())
        addMenuItem(to: menu, title: String(localized: "Menu Bar Manager Settings…"), action: #selector(openSettingsFromMenu))
        addMenuItem(to: menu, title: String(localized: "Turn Off Menu Bar Manager"), action: #selector(turnOffFromMenu))
        // Assigning the menu only for this click keeps left clicks toggling.
        item.menu = menu
        item.button?.performClick(nil)
        item.menu = nil
    }

    private func addMenuItem(to menu: NSMenu, title: String, action: Selector) {
        let entry = NSMenuItem(title: title, action: action, keyEquivalent: "")
        entry.target = self
        menu.addItem(entry)
    }

    @objc private func toggleFromMenu() { toggle() }
    @objc private func openSettingsFromMenu() { openSettings?() }
    @objc private func turnOffFromMenu() { isEnabled = false }

    // MARK: - Auto-hide

    private func scheduleAutoHide() {
        autoHideTask?.cancel()
        autoHideTask = nil
        guard toggleItem != nil, !isCollapsed, autoHideDelay > 0, editingCount == 0 else { return }
        let delay = autoHideDelay
        autoHideTask = Task { [weak self] in
            guard (try? await Task.sleep(for: .seconds(delay))) != nil else { return }
            // Collapsing under an open menu or the pointer would yank away the item being used.
            while Self.isMenuBarInUse {
                guard (try? await Task.sleep(for: .seconds(1))) != nil else { return }
            }
            self?.collapse()
        }
    }

    private func cancelPendingWork() {
        launchCollapseTask?.cancel()
        launchCollapseTask = nil
        autoHideTask?.cancel()
        autoHideTask = nil
    }

    private static var isMenuBarInUse: Bool {
        let mouse = NSEvent.mouseLocation
        let pointerInBar = NSScreen.screens.contains { screen in
            let barHeight = max(screen.frame.maxY - screen.visibleFrame.maxY, NSStatusBar.system.thickness)
            return screen.frame.contains(mouse) && mouse.y >= screen.frame.maxY - barHeight
        }
        if pointerInBar { return true }
        // Window levels and bounds are readable without Screen Recording access.
        let menuLevel = Int(CGWindowLevelForKey(.popUpMenuWindow))
        let windows = CGWindowListCopyWindowInfo([.optionOnScreenOnly, .excludeDesktopElements], kCGNullWindowID) as? [[String: Any]] ?? []
        return windows.contains { ($0[kCGWindowLayer as String] as? Int) == menuLevel }
    }
}
