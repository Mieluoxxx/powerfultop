import AppKit
import SwiftUI

struct DetailWindowView: View {
    static let preferredWidth: CGFloat = 1000
    static let preferredHeight: CGFloat = 720
    static let minimumWidth: CGFloat = 760
    static let pageKey = "dashboardPage"

    enum Page: String, CaseIterable, Identifiable {
        case overview, system, cooling, battery, menuBar, menuBarManager, settings
        var id: String { rawValue }

        var title: String {
            switch self {
            case .overview: return String(localized: "Overview")
            case .system: return String(localized: "System")
            case .cooling: return String(localized: "Cooling")
            case .battery: return String(localized: "Battery")
            case .menuBar: return String(localized: "Menu Bar")
            case .menuBarManager: return String(localized: "Menu Bar Manager")
            case .settings: return String(localized: "Settings")
            }
        }

        var icon: String {
            switch self {
            case .overview: return "square.grid.2x2"
            case .system: return "cpu"
            case .cooling: return "fan"
            case .battery: return "battery.75percent"
            case .menuBar: return "menubar.rectangle"
            case .menuBarManager: return "menubar.arrow.up.rectangle"
            case .settings: return "gearshape"
            }
        }
    }

    let monitor: PowerMonitor
    let thermal: ThermalMonitor
    let system: SystemMonitor
    let history: HistoryRecorder
    let menuBarManager: MenuBarManager
    @AppStorage(DetailWindowView.pageKey) private var page: Page = .overview

    var body: some View {
        HStack(spacing: 0) {
            sidebar
            Divider()
            Group {
                if monitor.isDataAvailable {
                    GeometryReader { proxy in
                        ScrollView {
                            content(width: proxy.size.width)
                                .padding(24)
                                .frame(maxWidth: .infinity, alignment: .topLeading)
                        }
                    }
                } else {
                    unavailableState
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
        }
        .frame(minWidth: Self.minimumWidth, maxWidth: .infinity, minHeight: 520, maxHeight: .infinity, alignment: .top)
        .background(.windowBackground)
        .focusEffectDisabled()
        .onAppear {
            DispatchQueue.main.async {
                NSApp.keyWindow?.makeFirstResponder(nil)
            }
        }
    }

    @ViewBuilder
    private func content(width: CGFloat) -> some View {
        VStack(alignment: .leading, spacing: 16) {
            Text(page.title)
                .font(.system(size: 22, weight: .bold))
            switch page {
            case .overview:
                OverviewPage(monitor: monitor, thermal: thermal, system: system, history: history)
            case .system:
                SystemPage(system: system, thermal: thermal, history: history, columnCount: width >= 760 ? 2 : 1)
            case .cooling:
                CoolingPage(thermal: thermal, history: history)
            case .battery:
                BatteryPage(monitor: monitor, columnCount: width >= 760 ? 2 : 1)
            case .menuBar:
                MenuBarPage(monitor: monitor, thermal: thermal, system: system)
            case .menuBarManager:
                MenuBarManagerPage(manager: menuBarManager)
            case .settings:
                SettingsPage(monitor: monitor, thermal: thermal)
            }
        }
    }

    private var sidebar: some View {
        VStack(alignment: .leading, spacing: 4) {
            ForEach(Page.allCases) { item in
                Button {
                    page = item
                } label: {
                    HStack(spacing: 6) {
                        Label(item.title, systemImage: item.icon)
                        Spacer(minLength: 0)
                        if needsAttention(item) {
                            Image(systemName: "exclamationmark.triangle.fill")
                                .foregroundStyle(.orange)
                                .help(String(localized: "The fan helper needs to be authorized again."))
                        }
                    }
                    .font(.system(size: 13, weight: page == item ? .semibold : .regular))
                    .foregroundStyle(page == item ? Color.accentColor : Color.primary)
                    .padding(.horizontal, 12)
                    .padding(.vertical, 8)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(
                        page == item ? Color.accentColor.opacity(0.14) : Color.clear,
                        in: RoundedRectangle(cornerRadius: 9, style: .continuous)
                    )
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
            }
            Spacer()
            Text(String(format: String(localized: "Updated at %@"), monitor.currentData.timestamp.formatted(date: .omitted, time: .standard)))
                .font(.system(size: 10))
                .foregroundStyle(.tertiary)
                .monospacedDigit()
                .padding(.horizontal, 12)
        }
        .padding(12)
        .padding(.top, 8)
        .frame(width: 190)
        .frame(maxHeight: .infinity, alignment: .top)
        .background(Color.primary.opacity(0.025))
    }

    /// Cooling hosts the fan controls and the update button, so a stale helper is flagged there.
    private func needsAttention(_ item: Page) -> Bool {
        item == .cooling && thermal.helperStatus == .needsUpdate
    }

    private var unavailableState: some View {
        VStack(spacing: 12) {
            Image(systemName: "exclamationmark.triangle.fill")
                .font(.largeTitle)
                .foregroundStyle(.orange)
            Text(String(localized: "Built-in battery not detected. PowerfulTop requires a MacBook."))
                .font(.system(size: 13))
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, 40)
    }
}
