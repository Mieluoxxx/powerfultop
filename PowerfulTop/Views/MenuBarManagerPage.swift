import AppKit
import SwiftUI

struct MenuBarManagerPage: View {
    let manager: MenuBarManager
    @State private var store: MenuBarItemStore

    init(manager: MenuBarManager) {
        self.manager = manager
        _store = State(initialValue: MenuBarItemStore(manager: manager))
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            DashCard(title: String(localized: "Hidden Items"), icon: "eye.slash", tint: .indigo) {
                Toggle(String(localized: "Enable Menu Bar Manager"), isOn: Binding(
                    get: { manager.isEnabled },
                    set: { manager.isEnabled = $0 }
                ))
                .font(.system(size: 12))
                .disabled(!MenuBarManager.isSupported)

                if !MenuBarManager.isSupported {
                    StatusNote(icon: "exclamationmark.triangle.fill", tint: .orange, text: String(localized: "Requires macOS 27 or later."))
                } else if manager.isEnabled {
                    CardRow(
                        label: String(localized: "Status"),
                        value: manager.isCollapsed ? String(localized: "Icons Hidden") : String(localized: "Icons Shown"),
                        emphasized: true
                    )
                    HStack(spacing: 12) {
                        Text(String(localized: "Hide Again After"))
                            .font(.system(size: 12))
                            .foregroundStyle(.secondary)
                        Picker("", selection: Binding(
                            get: { manager.autoHideDelay },
                            set: { manager.autoHideDelay = $0 }
                        )) {
                            ForEach(MenuBarManager.autoHideChoices, id: \.self) { delay in
                                Text(Self.autoHideLabel(delay)).tag(delay)
                            }
                        }
                        .labelsHidden()
                        .fixedSize()
                        Spacer()
                        Button(manager.isCollapsed ? String(localized: "Show Hidden Items") : String(localized: "Hide Items")) {
                            manager.toggle()
                        }
                    }
                }
            }

            if MenuBarManager.isSupported {
                MenuBarLayoutCard(manager: manager, store: store)
            }

            DashCard(title: String(localized: "How It Works"), icon: "hand.draw", tint: .blue) {
                step(1, String(localized: "Hold ⌘ and drag menu bar icons to the left of the │ separator to choose which ones hide."))
                step(2, String(localized: "Click the chevron to hide or show them. The macOS « button can't reveal them, because it shows the blank space that pushes them out first."))
                step(3, String(localized: "Right-click the chevron for more options."))
                CardNote(String(localized: "Uses the macOS 27 menu bar itself, so no Screen Recording or Accessibility access is needed. Apple's own icons are switched on and off in System Settings."))
                HStack {
                    Button(String(localized: "Open System Settings…")) {
                        NSWorkspace.shared.open(controlCenterSettingsURL)
                    }
                    Spacer()
                }
            }
        }
        .frame(maxWidth: 560, alignment: .leading)
        .background(WindowVisibilityReader { store.setVisible($0) })
        .onChange(of: manager.isEnabled) {
            Task { await store.refresh() }
        }
    }

    private func step(_ number: Int, _ text: String) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: 8) {
            Text("\(number)")
                .font(.system(size: 10, weight: .bold, design: .rounded))
                .foregroundStyle(.white)
                .frame(width: 16, height: 16)
                .background(Color.blue, in: Circle())
            Text(text)
                .font(.system(size: 12))
                .fixedSize(horizontal: false, vertical: true)
        }
    }

    private static func autoHideLabel(_ delay: TimeInterval) -> String {
        guard delay > 0 else { return String(localized: "Never") }
        let formatter = DateComponentsFormatter()
        formatter.allowedUnits = [.minute, .second]
        formatter.unitsStyle = .full
        return formatter.string(from: delay) ?? "\(Int(delay))s"
    }
}

private let controlCenterSettingsURL = URL(string: "x-apple.systempreferences:com.apple.ControlCenter-Settings.extension")!

private struct MenuBarLayoutCard: View {
    let manager: MenuBarManager
    let store: MenuBarItemStore

    var body: some View {
        DashCard(title: String(localized: "Menu Bar Layout"), icon: "menubar.dock.rectangle", tint: .purple) {
            if !manager.isEnabled {
                CardNote(String(localized: "Turn on the menu bar manager to arrange icons here."))
            } else if !store.isTrusted {
                StatusNote(icon: "lock.fill", tint: .orange, text: String(localized: "Arranging icons here needs Accessibility access."))
                HStack {
                    Button(String(localized: "Grant Accessibility Access…")) { store.requestAccessibility() }
                    Spacer()
                }
                CardNote(String(localized: "macOS has no API for other apps' menu bar icons, so PowerfulTop reads them through Accessibility and moves them with the same ⌘-drag you would do."))
            } else {
                layoutAccessRow
                if store.isLayoutCrowded {
                    StatusNote(icon: "rectangle.compress.vertical", tint: .orange, text: String(localized: "The active menu bar is too crowded to show the separator."))
                    CardNote(String(localized: "Click an empty spot in the menu bar of a wider display, or hide a few icons with ⌘-drag first."))
                } else {
                    layout
                }
            }
        }
    }

    @ViewBuilder
    private var layoutAccessRow: some View {
        if store.hasLayoutAccess {
            StatusNote(icon: "checkmark.seal.fill", tint: .green, text: String(localized: "Using the menu bar layout file: icons move without the pointer, even behind the notch."))
        } else {
            HStack(spacing: 8) {
                StatusNote(icon: "doc.badge.gearshape", tint: .blue, text: String(localized: "Allow the menu bar layout file for reliable sorting and pointer-free moves."))
                Spacer()
                Button(String(localized: "Allow…")) { store.requestLayoutAccess() }
            }
        }
    }

    @ViewBuilder
    private var layout: some View {
        LayoutBar(
            title: String(localized: "Shown Icons"),
            subtitle: String(localized: "Always in the menu bar"),
            items: store.items.filter { $0.section == .shown },
            section: .shown,
            store: store
        )
        LayoutBar(
            title: String(localized: "Hidden Icons"),
            subtitle: String(localized: "Revealed with the chevron"),
            items: store.items.filter { $0.section == .hidden },
            section: .hidden,
            store: store
        )
        let absent = store.items.filter { $0.section == .absent }
        if !absent.isEmpty {
            LayoutBar(
                title: String(localized: "Not in Menu Bar"),
                subtitle: String(localized: "Usually switched off in System Settings"),
                items: absent,
                section: .absent,
                store: store
            )
        }

        HStack(spacing: 8) {
            if store.isMoving {
                ProgressView().controlSize(.small)
                Text(String(localized: "Moving…"))
                    .font(.system(size: 11))
                    .foregroundStyle(.secondary)
            } else if let error = store.lastError {
                StatusNote(icon: "exclamationmark.triangle.fill", tint: .orange, text: error)
            }
            Spacer()
        }
        CardNote(String(localized: "Drag an icon onto another to put it just left of that one, or onto an empty spot to move it to that row. Right-click an icon for more. The pointer jumps for a moment while an icon moves, and the menu bar stays expanded while this page is open."))
    }
}

/// One section drawn like a strip of the menu bar, icons in their real left-to-right order (after Ice and Thaw).
private struct LayoutBar: View {
    let title: String
    let subtitle: String
    let items: [MenuBarItemInfo]
    let section: MenuBarSection
    let store: MenuBarItemStore

    @State private var isTargeted = false

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(spacing: 6) {
                Text(title)
                    .font(.system(size: 12, weight: .semibold))
                Text(subtitle)
                    .font(.system(size: 11))
                    .foregroundStyle(.tertiary)
            }
            HStack(spacing: 4) {
                ForEach(items) { item in
                    ItemTile(item: item, store: store)
                }
                if items.isEmpty {
                    Text(section == .absent ? "" : String(localized: "Drop icons here"))
                        .font(.system(size: 11))
                        .foregroundStyle(.white.opacity(0.4))
                        .frame(maxWidth: .infinity)
                }
                Spacer(minLength: 0)
            }
            .padding(.horizontal, 8)
            .frame(maxWidth: .infinity, minHeight: 34, alignment: .leading)
            .background(Color.black.opacity(section == .absent ? 0.35 : 0.72), in: RoundedRectangle(cornerRadius: 9, style: .continuous))
            .overlay(
                RoundedRectangle(cornerRadius: 9, style: .continuous)
                    .strokeBorder(isTargeted ? Color.accentColor : Color.white.opacity(0.08), lineWidth: isTargeted ? 2 : 1)
            )
            .dropDestination(for: String.self) { ids, _ in
                guard section != .absent, let id = ids.first else { return false }
                Task { await store.move(id, to: section) }
                return true
            } isTargeted: { isTargeted = $0 && section != .absent }
        }
    }
}

private struct ItemTile: View {
    let item: MenuBarItemInfo
    let store: MenuBarItemStore

    @State private var isHovered = false
    @State private var isTargeted = false

    private var isMovable: Bool { item.canMove }

    private var title: String {
        item.detail.map { "\(item.name) · \($0)" } ?? item.name
    }

    var body: some View {
        Image(nsImage: Self.appIcon(for: item))
            .resizable()
            .frame(width: 20, height: 20)
            .padding(4)
            .background(Color.white.opacity(isHovered ? 0.16 : 0), in: RoundedRectangle(cornerRadius: 6, style: .continuous))
            .overlay(alignment: .leading) {
                // Shows where the dragged icon will land: just left of this one.
                if isTargeted {
                    Capsule().fill(Color.accentColor).frame(width: 3, height: 22).offset(x: -3)
                }
            }
            .opacity(isMovable ? 1 : 0.45)
            .contentShape(Rectangle())
            .onHover { isHovered = $0 }
            .help(isMovable || item.section == .absent ? title : title + "\n" + String(localized: "Behind the « button right now, so it can't be moved."))
            .draggable(item.id) {
                Image(nsImage: Self.appIcon(for: item)).resizable().frame(width: 20, height: 20)
            }
            .dropDestination(for: String.self) { ids, _ in
                guard isMovable, let id = ids.first, id != item.id else { return false }
                Task { await store.move(id, to: item.section, before: item.id) }
                return true
            } isTargeted: { isTargeted = $0 && isMovable }
            .contextMenu {
                Text(title)
                if isMovable {
                    Button(item.section == .shown ? String(localized: "Move to Hidden") : String(localized: "Move to Shown")) {
                        Task { await store.move(item.id, to: item.section == .shown ? .hidden : .shown) }
                    }
                }
            }
    }

    private static func appIcon(for item: MenuBarItemInfo) -> NSImage {
        if let bundleID = item.bundleID, let url = NSWorkspace.shared.urlForApplication(withBundleIdentifier: bundleID) {
            return NSWorkspace.shared.icon(forFile: url.path)
        }
        return NSWorkspace.shared.icon(for: .application)
    }
}

/// Reports whether the hosting window is on screen; `onDisappear` misses a window being ordered out.
private struct WindowVisibilityReader: NSViewRepresentable {
    let onChange: (Bool) -> Void

    func makeNSView(context: Context) -> VisibilityView {
        let view = VisibilityView()
        view.onChange = onChange
        return view
    }

    func updateNSView(_ view: VisibilityView, context: Context) {
        view.onChange = onChange
    }

    static func dismantleNSView(_ view: VisibilityView, coordinator: ()) {
        view.onChange?(false)
        view.onChange = nil
    }

    final class VisibilityView: NSView {
        var onChange: ((Bool) -> Void)?
        private var observer: NSObjectProtocol?

        override func viewDidMoveToWindow() {
            super.viewDidMoveToWindow()
            if let observer {
                NotificationCenter.default.removeObserver(observer)
                self.observer = nil
            }
            if let window {
                observer = NotificationCenter.default.addObserver(
                    forName: NSWindow.didChangeOcclusionStateNotification, object: window, queue: .main
                ) { [weak self] _ in
                    MainActor.assumeIsolated { self?.report() }
                }
            }
            report()
        }

        private func report() {
            onChange?(window?.occlusionState.contains(.visible) == true)
        }
    }
}
