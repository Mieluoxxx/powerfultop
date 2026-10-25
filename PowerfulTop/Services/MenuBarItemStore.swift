import AppKit
import ApplicationServices
import Observation

struct MenuBarItemInfo: Identifiable {
    let id: String
    let name: String
    let bundleID: String?
    /// Distinguishes several items of one app.
    let detail: String?
    let frame: CGRect
    let section: MenuBarSection
    /// Entry in the macOS 27 layout file, when it could be matched unambiguously.
    let layoutKey: String?
    let position: Double?
    /// On screen right now, so a ⌘-drag can reach it.
    let isReachable: Bool

    var canMove: Bool { section != .absent && (layoutKey != nil || isReachable) }
}

/// Every app's menu bar items for the layout editor. Items are listed through Accessibility; their
/// order comes from the macOS 27 layout file when the user has granted it, which also moves them
/// without the pointer. Otherwise they are classified by on-screen frames and moved with the same
/// ⌘-drag a user would do, which only works for items the active bar actually shows.
@MainActor
@Observable
final class MenuBarItemStore {
    private(set) var items: [MenuBarItemInfo] = []
    private(set) var isTrusted = AXIsProcessTrusted()
    /// The bar being read can't fit the separator, so no item can be placed.
    private(set) var isLayoutCrowded = false
    private(set) var hasLayoutAccess = false
    private(set) var isMoving = false
    private(set) var lastError: String?

    @ObservationIgnored private let manager: MenuBarManager
    @ObservationIgnored private var anchors: (separator: CGRect, toggle: CGRect)?
    @ObservationIgnored private var fileAnchors: (separator: Double, toggle: Double)?
    @ObservationIgnored private var isVisible = false
    @ObservationIgnored private var holdsBar = false
    @ObservationIgnored private var pollTask: Task<Void, Never>?
    private static let pollInterval: Duration = .seconds(3)
    /// The bar needs a moment to lay out after expanding or after a drop.
    private static let settleDelay: Duration = .milliseconds(400)

    init(manager: MenuBarManager) {
        self.manager = manager
        MenuBarLayoutFile.restoreAccess()
    }

    /// Called with the editor's on-screen state; polling runs only while someone is looking.
    func setVisible(_ visible: Bool) {
        guard visible != isVisible else { return }
        isVisible = visible
        pollTask?.cancel()
        pollTask = nil
        guard visible else {
            releaseBar()
            return
        }
        pollTask = Task { [weak self] in
            while !Task.isCancelled {
                await self?.refresh()
                guard (try? await Task.sleep(for: Self.pollInterval)) != nil else { return }
            }
        }
    }

    func refresh() async {
        isTrusted = AXIsProcessTrusted()
        let wantsBar = isVisible && isTrusted && manager.isEnabled
        if wantsBar != holdsBar {
            if wantsBar {
                manager.beginEditing()
                holdsBar = true
                try? await Task.sleep(for: Self.settleDelay)
            } else {
                releaseBar()
            }
        }
        guard holdsBar else {
            items = []
            return
        }
        guard !isMoving else { return }
        await scan()
    }

    /// Moves an item into `section`, immediately left of `neighbor` when given, else to the
    /// section's edge next to the chevron.
    func move(_ id: MenuBarItemInfo.ID, to section: MenuBarSection, before neighbor: MenuBarItemInfo.ID? = nil) async {
        guard !isMoving, holdsBar, id != neighbor else { return }
        isMoving = true
        defer { isMoving = false }
        // Positions shift whenever any app's item changes width, so measure right before moving.
        await scan()
        guard let item = items.first(where: { $0.id == id }) else { return }
        let anchor = neighbor.flatMap { neighbor in items.first { $0.id == neighbor && $0.section == section } }
        guard anchor != nil || item.section != section else { return }
        guard item.canMove else {
            lastError = String(localized: "This icon isn't in the menu bar right now, so it can't be moved.")
            return
        }

        if let key = item.layoutKey, let fileAnchors, let positions = MenuBarLayoutFile.positions() {
            let existing = positions.filter { $0.key != key }.map(\.value)
            let target: Double
            if let anchorPosition = anchor?.position {
                target = MenuBarLayout.position(leftOf: anchorPosition, existing: existing)
            } else if section == .hidden {
                target = MenuBarLayout.position(leftOf: fileAnchors.separator, existing: existing)
            } else {
                target = MenuBarLayout.position(rightOf: fileAnchors.toggle, existing: existing)
            }
            if await Task.detached(operation: { MenuBarLayoutFile.setPosition(target, for: key) }).value {
                try? await Task.sleep(for: Self.settleDelay)
                await scan()
                lastError = items.first { $0.id == id }?.section == section ? nil
                    : String(localized: "The icon didn't move. It may be covered by the notch or the « overflow; try again on a wider display.")
                return
            }
        }

        guard item.isReachable, let anchors,
              let target = anchor.flatMap({ $0.isReachable ? MenuBarLayout.dropPoint(leftOf: $0.frame) : nil })
                ?? MenuBarLayout.dropPoint(for: section, separator: anchors.separator, toggle: anchors.toggle) else {
            lastError = String(localized: "The icon didn't move. It may be covered by the notch or the « overflow; try again on a wider display.")
            return
        }
        let start = CGPoint(x: item.frame.midX, y: item.frame.midY)
        await Task.detached { Self.commandDrag(from: start, to: target) }.value
        try? await Task.sleep(for: Self.settleDelay)
        await scan()
        let moved = items.first { $0.id == id }
        let landed = moved?.section == section
            && anchor.map { anchor in (items.first { $0.id == anchor.id }?.frame.midX).map { moved!.frame.midX < $0 } ?? true } ?? true
        lastError = landed ? nil : String(localized: "The icon didn't move. It may be covered by the notch or the « overflow; try again on a wider display.")
    }

    func requestLayoutAccess() {
        hasLayoutAccess = MenuBarLayoutFile.requestAccess()
        if !hasLayoutAccess {
            lastError = String(localized: "PowerfulTop still can't read the menu bar layout file. Pick com.apple.MenuBar.plist in the folder that opens.")
        }
        Task { await refresh() }
    }

    func requestAccessibility() {
        let options = [kAXTrustedCheckOptionPrompt.takeUnretainedValue() as String: true] as CFDictionary
        isTrusted = AXIsProcessTrustedWithOptions(options)
    }

    private func releaseBar() {
        guard holdsBar else { return }
        holdsBar = false
        items = []
        manager.endEditing()
    }

    // MARK: - Scanning

    private struct RawItem: Sendable {
        let pid: pid_t
        let index: Int
        let title: String
        let help: String
        let identifier: String
        let frame: CGRect
    }

    private struct AppRef: Sendable {
        let pid: pid_t
        let name: String
        let bundleID: String?
        /// Every name macOS may file the app's layout keys under.
        let owners: Set<String>
    }

    private func scan() async {
        let apps = NSWorkspace.shared.runningApplications
            .filter { $0.bundleIdentifier != "com.apple.MenuBarAgent" }
            .map { app in
                AppRef(
                    pid: app.processIdentifier,
                    name: app.localizedName ?? app.bundleIdentifier ?? "?",
                    bundleID: app.bundleIdentifier,
                    owners: Set([app.bundleIdentifier, app.localizedName, app.executableURL?.lastPathComponent].compactMap { $0 })
                )
            }
        let raw = await Task.detached { Self.readItems(of: apps.map(\.pid)) }.value
        let overflowButton = await Task.detached { Self.overflowButtonFrame() }.value
        let positions = await Task.detached { MenuBarLayoutFile.positions() }.value
        hasLayoutAccess = positions != nil
        // Items behind « report stacked placeholder frames, so only those right of it are real.
        func isReachable(_ frame: CGRect) -> Bool {
            guard let overflowButton, abs(frame.midY - overflowButton.midY) < overflowButton.height else { return true }
            return frame.midX > overflowButton.maxX
        }

        let ownPID = getpid()
        let own = raw.filter { $0.pid == ownPID }
        guard let separator = own.first(where: { $0.identifier == MenuBarManager.separatorIdentifier })?.frame,
              let toggle = own.first(where: { $0.identifier == MenuBarManager.toggleIdentifier })?.frame else {
            anchors = nil
            items = []
            return
        }
        let framesUsable = isReachable(separator) && isReachable(toggle)
        anchors = framesUsable ? (separator, toggle) : nil

        let ownOwners = apps.first { $0.pid == ownPID }?.owners ?? []
        let parsed = (positions ?? [:]).compactMap { key, value -> (key: String, owner: String, name: String, position: Double)? in
            MenuBarLayout.parseKey(key).map { (key, $0.owner, $0.name, value) }
        }
        let ownBlock = parsed.filter { ownOwners.contains($0.owner) && $0.name.hasPrefix(MenuBarManager.autosavePrefix) }
        if let separatorPosition = ownBlock.first(where: { $0.name == MenuBarManager.separatorIdentifier })?.position,
           let togglePosition = ownBlock.first(where: { $0.name == MenuBarManager.toggleIdentifier })?.position {
            fileAnchors = (separatorPosition, togglePosition)
        } else {
            fileAnchors = nil
        }
        let blockEdge = ownBlock.filter { $0.name != MenuBarManager.toggleIdentifier }.map(\.position).min()

        isLayoutCrowded = fileAnchors == nil && !framesUsable
        guard !isLayoutCrowded else {
            items = []
            return
        }

        let appsByPID = Dictionary(apps.map { ($0.pid, $0) }, uniquingKeysWith: { first, _ in first })
        let ownIdentifiers: Set<String> = [MenuBarManager.separatorIdentifier, MenuBarManager.toggleIdentifier, MenuBarManager.spacerIdentifier]
        let candidates = raw.filter { !ownIdentifiers.contains($0.identifier) && !($0.pid == ownPID && $0.frame.width < 4) }

        var next: [MenuBarItemInfo] = []
        for (pid, live) in Dictionary(grouping: candidates, by: \.pid) {
            guard let app = appsByPID[pid] else { continue }
            let entries = parsed
                .filter { app.owners.contains($0.owner) && !$0.name.hasPrefix("PowerfulTop.") }
                .sorted { $0.position > $1.position }
            // Keys can only be paired with items when the counts agree, and for several items only
            // when their frames give the left-to-right order to pair the positions with.
            var keys: [Int: (key: String, position: Double)] = [:]
            if entries.count == live.count, live.count == 1 || live.allSatisfy({ isReachable($0.frame) }) {
                for (item, entry) in zip(live.sorted { $0.frame.minX < $1.frame.minX }, entries) {
                    keys[item.index] = (entry.key, entry.position)
                }
            }
            for item in live {
                let entry = keys[item.index]
                let onScreen = MenuBarLayout.section(of: item.frame, separator: separator, toggle: toggle)
                let section: MenuBarSection
                if onScreen == .absent {
                    section = .absent
                } else if let entry, let blockEdge, fileAnchors != nil {
                    section = MenuBarLayout.section(position: entry.position, blockEdge: blockEdge)
                } else if isReachable(item.frame) {
                    // With the chevron itself behind «, anything still on screen sits right of it.
                    section = framesUsable ? onScreen : .shown
                } else {
                    // Overflowed with no layout entry: the bar isn't showing it, so it is hidden in practice.
                    section = .hidden
                }
                next.append(MenuBarItemInfo(
                    id: "\(app.bundleID ?? String(pid))#\(item.index)",
                    name: app.name,
                    bundleID: app.bundleID,
                    detail: live.count > 1 ? [item.help, item.title, "\(item.index + 1)"].first { !$0.isEmpty } : nil,
                    frame: item.frame,
                    section: section,
                    layoutKey: entry?.key,
                    position: entry?.position,
                    isReachable: framesUsable && isReachable(item.frame)
                ))
            }
        }
        // Unmatched items borrow a position between their matched on-screen neighbours, so the rows
        // keep the bar's left-to-right order (layout positions grow leftwards).
        let matchedOnScreen = next.filter { $0.position != nil && $0.isReachable }.sorted { $0.frame.minX < $1.frame.minX }
        let sortPositions = Dictionary(next.map { item -> (String, Double?) in
            if let position = item.position { return (item.id, position) }
            guard item.isReachable else { return (item.id, nil) }
            let left = matchedOnScreen.last { $0.frame.minX < item.frame.minX }?.position
            let right = matchedOnScreen.first { $0.frame.minX > item.frame.minX }?.position
            switch (left, right) {
            case let (left?, right?): return (item.id, (left + right) / 2)
            case let (left?, nil): return (item.id, left - 0.5)
            case let (nil, right?): return (item.id, right + 0.5)
            case (nil, nil): return (item.id, nil)
            }
        }, uniquingKeysWith: { first, _ in first })
        items = next.sorted { lhs, rhs in
            switch (sortPositions[lhs.id] ?? nil, sortPositions[rhs.id] ?? nil) {
            case let (left?, right?): return left > right
            case (.some, nil): return true
            case (nil, .some): return false
            case (nil, nil): return lhs.frame.minX < rhs.frame.minX
            }
        }
    }

    /// The system « button: MenuBarAgent's only button among its status groups, present while the bar overflows.
    private nonisolated static func overflowButtonFrame() -> CGRect? {
        guard let agent = NSRunningApplication.runningApplications(withBundleIdentifier: "com.apple.MenuBarAgent").first else { return nil }
        let app = AXUIElementCreateApplication(agent.processIdentifier)
        AXUIElementSetMessagingTimeout(app, 0.25)
        guard let bar: AXUIElement = attribute(app, kAXExtrasMenuBarAttribute),
              let children: [AXUIElement] = attribute(bar, kAXChildrenAttribute),
              let button = children.first(where: { (attribute($0, kAXRoleAttribute) as String?) == kAXButtonRole }) else { return nil }
        return frame(of: button)
    }

    /// AX calls block until the other app answers, so every app is asked in parallel with a short timeout.
    private nonisolated static func readItems(of pids: [pid_t]) -> [RawItem] {
        let lock = NSLock()
        var result: [RawItem] = []
        DispatchQueue.concurrentPerform(iterations: pids.count) { index in
            let pid = pids[index]
            let app = AXUIElementCreateApplication(pid)
            AXUIElementSetMessagingTimeout(app, 0.25)
            guard let bar: AXUIElement = attribute(app, kAXExtrasMenuBarAttribute),
                  let children: [AXUIElement] = attribute(bar, kAXChildrenAttribute) else { return }
            let found = children.enumerated().map { offset, element in
                let title: String = attribute(element, kAXTitleAttribute) ?? ""
                let description: String = attribute(element, kAXDescriptionAttribute) ?? ""
                return RawItem(
                    pid: pid,
                    index: offset,
                    title: title.isEmpty ? description : title,
                    help: attribute(element, kAXHelpAttribute) ?? "",
                    identifier: attribute(element, kAXIdentifierAttribute) ?? "",
                    frame: frame(of: element)
                )
            }
            lock.lock()
            result += found
            lock.unlock()
        }
        return result
    }

    private nonisolated static func attribute<T>(_ element: AXUIElement, _ name: String) -> T? {
        var value: CFTypeRef?
        guard AXUIElementCopyAttributeValue(element, name as CFString, &value) == .success else { return nil }
        return value as? T
    }

    private nonisolated static func frame(of element: AXUIElement) -> CGRect {
        var origin = CGPoint.zero
        var size = CGSize.zero
        if let value: AXValue = attribute(element, kAXPositionAttribute) { AXValueGetValue(value, .cgPoint, &origin) }
        if let value: AXValue = attribute(element, kAXSizeAttribute) { AXValueGetValue(value, .cgSize, &size) }
        return CGRect(origin: origin, size: size)
    }

    // MARK: - Moving

    private nonisolated static func commandDrag(from start: CGPoint, to end: CGPoint) {
        let restore = CGEvent(source: nil)?.location
        func post(_ type: CGEventType, _ point: CGPoint) {
            guard let event = CGEvent(mouseEventSource: nil, mouseType: type, mouseCursorPosition: point, mouseButton: .left) else { return }
            event.flags = .maskCommand
            event.post(tap: .cghidEventTap)
            usleep(15_000)
        }
        post(.mouseMoved, start)
        post(.leftMouseDown, start)
        // The bar only picks an item up after the button has been held for a moment.
        usleep(80_000)
        let steps = 12
        for step in 1...steps {
            let fraction = CGFloat(step) / CGFloat(steps)
            post(.leftMouseDragged, CGPoint(x: start.x + (end.x - start.x) * fraction, y: start.y + (end.y - start.y) * fraction))
        }
        usleep(80_000)
        post(.leftMouseUp, end)
        if let restore {
            CGWarpMouseCursorPosition(restore)
        }
    }
}
