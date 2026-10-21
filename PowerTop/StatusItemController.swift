import AppKit
import SwiftUI

/// Menu bar extra implemented as NSStatusItem + NSPopover (system chrome, intrinsic size).
@MainActor
final class StatusItemController: NSObject, NSPopoverDelegate, NSWindowDelegate {
    private let monitor: PowerMonitor
    private let thermal: ThermalMonitor
    private let statusItem: NSStatusItem
    private let popover = NSPopover()
    private var detailWindow: NSWindow?
    /// Skip redundant status-item layout; rewriting length/title every 2s shifts the item.
    private var renderedLabel: MenuBarLabel?
    /// Accessory apps often don't resign key, so `.transient` misses outside clicks on macOS 15/26.
    private var localClickMonitor: Any?
    private var globalClickMonitor: Any?

    init(monitor: PowerMonitor, thermal: ThermalMonitor) {
        self.monitor = monitor
        self.thermal = thermal
        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        super.init()
        configureStatusItem()
        configurePopover()
        startObservingMonitor()
        updateStatusItem()
    }

    func invalidate() {
        stopDismissMonitors()
        popover.performClose(nil)
        detailWindow?.delegate = nil
        detailWindow?.close()
        detailWindow = nil
        NSStatusBar.system.removeStatusItem(statusItem)
    }

    private func configureStatusItem() {
        guard let button = statusItem.button else { return }
        button.imagePosition = .imageLeading
        button.font = NSFont.monospacedDigitSystemFont(ofSize: 12, weight: .medium)
        // Mouse-down + deferred show: a mouse-up on a transient popover is otherwise
        // treated as an outside click and the panel flashes closed immediately.
        button.sendAction(on: [.leftMouseDown])
        button.target = self
        button.action = #selector(statusItemClicked)
    }

    private func configurePopover() {
        popover.behavior = .transient
        popover.animates = true
        popover.delegate = self
        let root = StatusPopoverContent(
            monitor: monitor,
            thermal: thermal,
            openDetails: { [weak self] in self?.openDetails() },
            onMeasuredSize: { [weak self] size in
                self?.applyPopoverSize(size)
            }
        )
        let hosting = NSHostingController(rootView: root)
        hosting.sizingOptions = [.preferredContentSize, .intrinsicContentSize]
        popover.contentViewController = hosting
        popover.contentSize = NSSize(width: PopoverView.preferredWidth, height: 420)
    }

    private func startObservingMonitor() {
        observeMonitor()
    }

    private func observeMonitor() {
        withObservationTracking {
            _ = monitor.menuBarDisplayStyle
            _ = monitor.isDataAvailable
            _ = monitor.currentData.menuBarPowerText
            _ = monitor.currentData.effectiveIsOnAC
            _ = monitor.currentData.batteryPercent
        } onChange: { [weak self] in
            Task { @MainActor in
                self?.updateStatusItem()
                self?.observeMonitor()
            }
        }
    }

    private func updateStatusItem() {
        let label = MenuBarLabel(
            style: monitor.menuBarDisplayStyle,
            isDataAvailable: monitor.isDataAvailable,
            data: monitor.currentData
        )
        guard label != renderedLabel else { return }
        renderedLabel = label

        guard let button = statusItem.button else { return }
        let image = label.symbolName.flatMap { NSImage(systemSymbolName: $0, accessibilityDescription: nil) }
        image?.isTemplate = true
        button.image = image
        if image == nil {
            button.imagePosition = .noImage
        } else {
            button.imagePosition = label.title.isEmpty ? .imageOnly : .imageLeading
        }
        button.title = label.title
    }

    @objc private func statusItemClicked() {
        if popover.isShown {
            popover.performClose(nil)
        } else {
            DispatchQueue.main.async { [weak self] in
                self?.showPopover()
            }
        }
    }

    private func showPopover() {
        guard let button = statusItem.button else { return }
        if let hosting = popover.contentViewController {
            let fitted = hosting.view.fittingSize
            if fitted.width > 1, fitted.height > 1 {
                applyPopoverSize(fitted)
            }
        }
        // Do not make the popover key: that emphasizes the status item text
        // and it can stick when activating a window on another display.
        popover.show(relativeTo: button.bounds, of: button, preferredEdge: .minY)
        startDismissMonitors()
    }

    private func startDismissMonitors() {
        stopDismissMonitors()
        let mask: NSEvent.EventTypeMask = [.leftMouseDown, .rightMouseDown]
        localClickMonitor = NSEvent.addLocalMonitorForEvents(matching: mask) { [weak self] event in
            Task { @MainActor in
                self?.dismissPopoverIfClickIsOutside()
            }
            return event
        }
        globalClickMonitor = NSEvent.addGlobalMonitorForEvents(matching: mask) { [weak self] _ in
            Task { @MainActor in
                self?.dismissPopoverIfClickIsOutside()
            }
        }
    }

    private func stopDismissMonitors() {
        if let localClickMonitor {
            NSEvent.removeMonitor(localClickMonitor)
            self.localClickMonitor = nil
        }
        if let globalClickMonitor {
            NSEvent.removeMonitor(globalClickMonitor)
            self.globalClickMonitor = nil
        }
    }

    private func dismissPopoverIfClickIsOutside() {
        guard popover.isShown else { return }
        let point = NSEvent.mouseLocation
        if let popoverWindow = popover.contentViewController?.view.window,
           popoverWindow.frame.contains(point) {
            return
        }
        if let button = statusItem.button, let buttonWindow = button.window {
            let buttonRect = buttonWindow.convertToScreen(button.convert(button.bounds, to: nil))
            if buttonRect.contains(point) {
                return
            }
        }
        popover.performClose(nil)
    }

    private func applyPopoverSize(_ size: CGSize) {
        let width = max(size.width, PopoverView.preferredWidth)
        let height = max(size.height, 80)
        let next = NSSize(width: width.rounded(), height: height.rounded())
        guard abs(popover.contentSize.width - next.width) > 0.5
            || abs(popover.contentSize.height - next.height) > 0.5 else { return }
        popover.contentSize = next
    }

    func openDetails() {
        popover.performClose(nil)
        NSApp.activate(ignoringOtherApps: true)
        if detailWindow == nil {
            let hosting = NSHostingController(rootView: DetailWindowView(monitor: monitor, thermal: thermal))
            let window = NSWindow(contentViewController: hosting)
            window.title = String(localized: "PowerTop Details")
            window.styleMask = [.titled, .closable, .miniaturizable, .resizable]
            window.setContentSize(NSSize(width: DetailWindowView.preferredWidth, height: DetailWindowView.preferredHeight))
            window.contentMinSize = NSSize(width: DetailWindowView.minimumWidth, height: 480)
            window.center()
            window.isReleasedWhenClosed = false
            window.delegate = self
            detailWindow = window
        }
        detailWindow?.makeKeyAndOrderFront(nil)
    }

    func windowShouldClose(_ sender: NSWindow) -> Bool {
        sender.orderOut(nil)
        return false
    }

    func popoverDidClose(_ notification: Notification) {
        stopDismissMonitors()
        statusItem.button?.highlight(false)
    }
}

private struct PopoverMeasuredSizeKey: PreferenceKey {
    static var defaultValue: CGSize = .zero
    static func reduce(value: inout CGSize, nextValue: () -> CGSize) {
        value = nextValue()
    }
}

private struct StatusPopoverContent: View {
    let monitor: PowerMonitor
    let thermal: ThermalMonitor
    let openDetails: () -> Void
    let onMeasuredSize: (CGSize) -> Void

    var body: some View {
        PopoverView(monitor: monitor, thermal: thermal, openDetails: openDetails)
            .background {
                GeometryReader { proxy in
                    Color.clear.preference(key: PopoverMeasuredSizeKey.self, value: proxy.size)
                }
            }
            .onPreferenceChange(PopoverMeasuredSizeKey.self) { size in
                guard size.width > 1, size.height > 1 else { return }
                onMeasuredSize(size)
            }
    }
}
