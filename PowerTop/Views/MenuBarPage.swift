import AppKit
import SwiftUI

/// AlDente-style editor: selected items preview live values; the catalog toggles items on and off.
struct MenuBarPage: View {
    let monitor: PowerMonitor
    let thermal: ThermalMonitor
    let system: SystemMonitor

    private var configuration: MenuBarConfiguration { monitor.menuBarConfiguration }
    private var context: MenuBarContext { makeMenuBarContext(monitor: monitor, thermal: thermal, system: system) }

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            selectedSection
            catalogSection
            tuningSection
        }
        .frame(maxWidth: 760, alignment: .leading)
    }

    /// Each style chip shows how that style would draw the current level.
    private func preview(_ style: MenuBarIconStyle) -> NSImage? {
        guard style != .hidden else { return nil }
        var sample = configuration
        sample.iconStyle = style
        return MenuBarLabel(configuration: sample, context: context).icon.flatMap(MenuBarIconRenderer.image(for:))
    }

    private func update(_ change: (inout MenuBarConfiguration) -> Void) {
        var next = monitor.menuBarConfiguration
        change(&next)
        monitor.menuBarConfiguration = next
    }

    // MARK: - Selected

    private var selectedSection: some View {
        DashCard(title: String(localized: "Selected Items"), icon: "menubar.rectangle", tint: .blue) {
            FlowLayout(spacing: 8) {
                if configuration.iconStyle != .hidden, let icon = MenuBarLabel(configuration: configuration, context: context).icon {
                    Chip(symbol: nil, image: MenuBarIconRenderer.image(for: icon), text: nil, filled: true)
                }
                ForEach(configuration.items) { item in
                    Chip(
                        symbol: item.symbol,
                        text: MenuBarLabel.segment(for: item, context: context)?.text ?? item.title,
                        filled: true,
                        onRemove: { update { $0.items.removeAll { $0 == item } } }
                    )
                    .help(item.title)
                    .draggable(item.rawValue)
                    .dropDestination(for: String.self) { dropped, _ in
                        guard let raw = dropped.first, let moved = MenuBarItem(rawValue: raw), moved != item else { return false }
                        update { configuration in
                            configuration.items.removeAll { $0 == moved }
                            let target = configuration.items.firstIndex(of: item) ?? configuration.items.endIndex
                            configuration.items.insert(moved, at: target)
                        }
                        return true
                    }
                }
            }
            if configuration.items.isEmpty && configuration.iconStyle == .hidden {
                CardNote(String(localized: "Nothing selected; a small icon stays in the menu bar so PowerTop can still be opened."))
            } else {
                CardNote(String(localized: "Drag items to reorder. Readings that don't apply right now, such as adapter values on battery, are hidden."))
            }
        }
    }

    // MARK: - Catalog

    private var catalogSection: some View {
        DashCard(title: String(localized: "Item Catalog"), icon: "books.vertical", tint: .purple) {
            CatalogGroup(title: String(localized: "Main Icon"), subtitle: String(localized: "(choose one)")) {
                ForEach(MenuBarIconStyle.allCases) { style in
                    Button {
                        update { $0.iconStyle = style }
                    } label: {
                        Chip(symbol: style == .hidden ? style.symbol : nil, image: preview(style), text: style.title, filled: configuration.iconStyle == style)
                    }
                    .buttonStyle(.plain)
                }
            }
            ForEach(MenuBarItemGroup.allCases) { group in
                CatalogGroup(title: group.title) {
                    ForEach(MenuBarItem.allCases.filter { $0.group == group }) { item in
                        let selected = configuration.items.contains(item)
                        Button {
                            update { configuration in
                                if selected {
                                    configuration.items.removeAll { $0 == item }
                                } else {
                                    configuration.items.append(item)
                                }
                            }
                        } label: {
                            Chip(symbol: item.symbol ?? "percent", text: item.title, filled: selected)
                        }
                        .buttonStyle(.plain)
                    }
                }
            }
            Divider()
            HStack {
                Spacer()
                Button {
                    monitor.menuBarConfiguration = .default
                } label: {
                    Label(String(localized: "Reset"), systemImage: "arrow.counterclockwise")
                }
                Button(role: .destructive) {
                    update { $0.items.removeAll() }
                } label: {
                    Label(String(localized: "Clear All"), systemImage: "trash")
                }
            }
        }
    }

    // MARK: - Tuning

    private var tuningSection: some View {
        DashCard(title: String(localized: "Layout"), icon: "slider.horizontal.3", tint: .gray) {
            sliderRow(
                title: String(localized: "Item Spacing"),
                value: configuration.spacing,
                range: MenuBarConfiguration.spacingRange,
                step: 1,
                format: { String(format: "%.0f pt", $0) },
                set: { value in update { $0.spacing = value } }
            )
            sliderRow(
                title: String(localized: "Update Interval"),
                value: configuration.updateInterval,
                range: MenuBarConfiguration.updateIntervalRange,
                step: 1,
                format: { String(format: String(localized: "%.0f s"), $0) },
                set: { value in update { $0.updateInterval = value } }
            )
        }
    }

    private func sliderRow(
        title: String,
        value: Double,
        range: ClosedRange<Double>,
        step: Double,
        format: @escaping (Double) -> String,
        set: @escaping (Double) -> Void
    ) -> some View {
        HStack(spacing: 12) {
            Text(title)
                .font(.system(size: 12))
                .foregroundStyle(.secondary)
                .frame(width: 120, alignment: .leading)
            Slider(value: Binding(get: { value }, set: set), in: range, step: step)
            Text(format(value))
                .font(.system(size: 12, weight: .medium))
                .monospacedDigit()
                .frame(width: 48, alignment: .trailing)
        }
    }
}

private struct CatalogGroup<Content: View>: View {
    let title: String
    var subtitle: String? = nil
    @ViewBuilder let content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 6) {
                Text(title)
                    .font(.system(size: 12, weight: .semibold))
                if let subtitle {
                    Text(subtitle)
                        .font(.system(size: 11))
                        .foregroundStyle(.tertiary)
                }
            }
            FlowLayout(spacing: 8) { content }
        }
        .padding(.bottom, 4)
    }
}

private struct Chip: View {
    let symbol: String?
    var image: NSImage? = nil
    let text: String?
    let filled: Bool
    var onRemove: (() -> Void)? = nil

    var body: some View {
        HStack(spacing: 5) {
            if let image {
                Image(nsImage: image)
                    .renderingMode(image.isTemplate ? .template : .original)
            } else if let symbol {
                Image(systemName: symbol)
                    .font(.system(size: 11, weight: .medium))
            }
            if let text {
                Text(text)
                    .font(.system(size: 12, weight: .medium))
                    .monospacedDigit()
            }
            if let onRemove {
                Button(action: onRemove) {
                    Image(systemName: "xmark")
                        .font(.system(size: 8, weight: .bold))
                }
                .buttonStyle(.plain)
                .opacity(0.7)
            }
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 6)
        .foregroundStyle(filled ? Color.white : Color.accentColor)
        .background(filled ? Color.accentColor : Color.clear, in: Capsule())
        .overlay(Capsule().strokeBorder(Color.accentColor, lineWidth: filled ? 0 : 1.5))
        .contentShape(Capsule())
    }
}

/// Left-aligned wrapping layout for chips.
struct FlowLayout: Layout {
    var spacing: CGFloat = 8

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        let rows = arrange(subviews: subviews, width: proposal.width ?? .infinity)
        let height = rows.last.map { $0.y + $0.height } ?? 0
        let width = rows.map(\.width).max() ?? 0
        return CGSize(width: proposal.width ?? width, height: height)
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        for row in arrange(subviews: subviews, width: bounds.width) {
            var x = bounds.minX
            for index in row.indices {
                let size = subviews[index].sizeThatFits(.unspecified)
                subviews[index].place(at: CGPoint(x: x, y: bounds.minY + row.y), proposal: ProposedViewSize(size))
                x += size.width + spacing
            }
        }
    }

    private struct Row {
        var indices: [Int] = []
        var y: CGFloat = 0
        var width: CGFloat = 0
        var height: CGFloat = 0
    }

    private func arrange(subviews: Subviews, width: CGFloat) -> [Row] {
        var rows: [Row] = []
        var current = Row()
        for index in subviews.indices {
            let size = subviews[index].sizeThatFits(.unspecified)
            let needed = current.indices.isEmpty ? size.width : current.width + spacing + size.width
            if needed > width, !current.indices.isEmpty {
                rows.append(current)
                current = Row(y: current.y + current.height + spacing)
            }
            current.width = current.indices.isEmpty ? size.width : current.width + spacing + size.width
            current.height = max(current.height, size.height)
            current.indices.append(index)
        }
        if !current.indices.isEmpty { rows.append(current) }
        return rows
    }
}
