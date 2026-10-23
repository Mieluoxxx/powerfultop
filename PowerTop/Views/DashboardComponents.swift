import AppKit
import Charts
import SwiftUI

extension Color {
    /// Raised card surface: white on the light window background, a faint lift in dark mode.
    static let dashCard = Color(nsColor: NSColor(name: nil) { appearance in
        appearance.bestMatch(from: [.darkAqua, .aqua]) == .darkAqua
            ? NSColor(white: 1, alpha: 0.055)
            : NSColor(white: 1, alpha: 0.85)
    })

    static let dashCardBorder = Color.primary.opacity(0.07)
}

struct DashCard<Content: View>: View {
    let title: String
    let icon: String
    let tint: Color
    @ViewBuilder let content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(spacing: 8) {
                Image(systemName: icon)
                    .font(.system(size: 11, weight: .semibold))
                    .foregroundStyle(tint)
                    .frame(width: 24, height: 24)
                    .background(tint.opacity(0.16), in: RoundedRectangle(cornerRadius: 7, style: .continuous))
                Text(title)
                    .font(.system(size: 13, weight: .semibold))
            }
            .padding(.bottom, 2)

            content
        }
        .padding(18)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .dashSurface()
    }
}

extension View {
    func dashSurface(cornerRadius: CGFloat = 18) -> some View {
        background(Color.dashCard, in: RoundedRectangle(cornerRadius: cornerRadius, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: cornerRadius, style: .continuous).strokeBorder(Color.dashCardBorder))
    }
}

/// Lead figure of a card, e.g. "48.2 W / 90 W" with a caption underneath.
struct CardHero: View {
    let value: String
    var rated: String? = nil
    var caption: String? = nil
    var size: CGFloat = 28

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            HStack(alignment: .firstTextBaseline, spacing: 4) {
                Text(value)
                    .font(.system(size: size, weight: .bold, design: .rounded))
                if let rated {
                    Text("/ \(rated)")
                        .font(.system(size: size * 0.5, weight: .medium, design: .rounded))
                        .foregroundStyle(.tertiary)
                }
            }
            .monospacedDigit()
            .lineLimit(1)
            .minimumScaleFactor(0.7)
            if let caption {
                Text(caption)
                    .font(.system(size: 11))
                    .foregroundStyle(.secondary)
            }
        }
        .padding(.bottom, 2)
    }
}

/// "Label    value / rated" — rated is the adapter or design limit the value is measured against.
struct CardRow: View {
    let label: String
    let value: String
    var rated: String? = nil
    var emphasized: Bool = false

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 8) {
            Text(label)
                .foregroundStyle(.secondary)
                .lineLimit(1)
                .layoutPriority(1)
            Spacer(minLength: 8)
            HStack(alignment: .firstTextBaseline, spacing: 3) {
                Text(value)
                    .fontWeight(emphasized ? .semibold : .medium)
                    .foregroundStyle(.primary)
                if let rated {
                    Text("/ \(rated)")
                        .foregroundStyle(.tertiary)
                }
            }
            .monospacedDigit()
            .lineLimit(1)
            .truncationMode(.middle)
            .textSelection(.enabled)
        }
        .font(.system(size: 12))
        .frame(maxWidth: .infinity)
    }
}

struct CardBarRow: View {
    let label: String
    let value: String
    let fraction: Double?
    let tint: Color

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            CardRow(label: label, value: value)
            LevelBar(fraction: fraction ?? 0, tint: tint, height: 5)
                .opacity(fraction == nil ? 0 : 1)
        }
    }
}

struct CardNote: View {
    let text: String

    init(_ text: String) { self.text = text }

    var body: some View {
        Text(text)
            .font(.system(size: 10))
            .foregroundStyle(.tertiary)
            .fixedSize(horizontal: false, vertical: true)
    }
}

/// One short line with an icon; long explanations go into the tooltip instead of the card.
struct StatusNote: View {
    let icon: String
    let tint: Color
    let text: String
    var detail: String? = nil

    var body: some View {
        Label {
            Text(text)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
        } icon: {
            Image(systemName: icon)
                .foregroundStyle(tint)
        }
        .font(.system(size: 11))
        .help(detail ?? text)
    }
}

struct StripItem: View {
    let icon: String
    let tint: Color
    let label: String
    let value: String
    /// Hides the label; the icon identifies the item and the label moves to the tooltip.
    var compact = false
    var tooltip: String? = nil

    private var helpText: String {
        [compact ? label : nil, tooltip].compactMap { $0 }.filter { !$0.isEmpty }.joined(separator: "\n")
    }

    var body: some View {
        HStack(spacing: 6) {
            Image(systemName: icon)
                .foregroundStyle(tint)
            if !compact {
                Text(label)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }
            Text(value)
                .fontWeight(.semibold)
                .monospacedDigit()
                .lineLimit(1)
        }
        .font(.system(size: 12))
        .padding(.horizontal, 10)
        .fixedSize()
        .help(helpText)
    }
}

struct StripDivider: View {
    var body: some View {
        Divider().frame(height: 14)
    }
}

/// Live value over the last 30 minutes, styled after the reference dashboard's chart panels.
struct ChartPanel: View {
    let title: String
    let value: String
    var detail: String? = nil
    let icon: String
    let tint: Color
    let points: [(date: Date, value: Double)]
    var domain: ClosedRange<Double>? = nil

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(alignment: .firstTextBaseline) {
                VStack(alignment: .leading, spacing: 2) {
                    Text(title)
                        .font(.system(size: 11))
                        .foregroundStyle(.secondary)
                    Text(value)
                        .font(.system(size: 22, weight: .bold, design: .rounded))
                        .monospacedDigit()
                }
                Spacer()
                VStack(alignment: .trailing, spacing: 4) {
                    Image(systemName: icon)
                        .foregroundStyle(tint)
                    if let detail {
                        Text(detail)
                            .font(.system(size: 10))
                            .foregroundStyle(.tertiary)
                            .monospacedDigit()
                    }
                }
            }

            if points.count >= 2 {
                chart
            } else {
                Text(String(localized: "Collecting data…"))
                    .font(.system(size: 11))
                    .foregroundStyle(.tertiary)
                    .frame(maxWidth: .infinity, minHeight: 110)
            }
        }
        .padding(16)
        .frame(maxWidth: .infinity, alignment: .topLeading)
        .dashSurface()
    }

    private var chart: some View {
        ChartPanelChart(points: points, tint: tint, domain: domain, height: 110)
    }
}

/// Gradient area chart shared by dashboard panels and the system page sparklines.
struct ChartPanelChart: View {
    let points: [(date: Date, value: Double)]
    let tint: Color
    var domain: ClosedRange<Double>? = nil
    var height: CGFloat = 110

    var body: some View {
        let scale = domain ?? autoDomain
        return Chart {
            ForEach(points, id: \.date) { point in
                // Fill from the visible floor; the default zero baseline spills below a raised axis.
                AreaMark(
                    x: .value("Time", point.date),
                    yStart: .value("Value", scale.lowerBound),
                    yEnd: .value("Value", point.value)
                )
                .foregroundStyle(LinearGradient(
                    colors: [tint.opacity(0.35), tint.opacity(0.02)],
                    startPoint: .top,
                    endPoint: .bottom
                ))
                .interpolationMethod(.monotone)
                LineMark(x: .value("Time", point.date), y: .value("Value", point.value))
                    .foregroundStyle(tint)
                    .lineStyle(StrokeStyle(lineWidth: 1.6))
                    .interpolationMethod(.monotone)
            }
        }
        .chartXAxis(.hidden)
        .chartYAxis {
            AxisMarks(position: .trailing, values: .automatic(desiredCount: 3)) { _ in
                AxisGridLine().foregroundStyle(Color.primary.opacity(0.06))
                AxisValueLabel().font(.system(size: 9)).foregroundStyle(.tertiary)
            }
        }
        .chartYScale(domain: scale)
        .frame(height: height)
        .clipped()
    }

    /// Pads flat series so a steady 11.1 W doesn't render as a noisy full-height line.
    private var autoDomain: ClosedRange<Double> {
        let values = points.map(\.value)
        let low = values.min() ?? 0
        let high = values.max() ?? 1
        let pad = max((high - low) * 0.25, max(abs(high) * 0.1, 1))
        return max(0, low - pad)...(high + pad)
    }
}

struct MasonryItem: Identifiable {
    let id: String
    let weight: Double
    let view: AnyView
}

/// Fills each card into the currently shortest column, keeping source order within a column.
struct MasonryColumns: View {
    let items: [MasonryItem]
    let columnCount: Int
    let spacing: CGFloat

    var body: some View {
        HStack(alignment: .top, spacing: spacing) {
            ForEach(Array(columns.enumerated()), id: \.offset) { _, column in
                VStack(spacing: spacing) {
                    ForEach(column) { $0.view }
                }
                .frame(maxWidth: .infinity, alignment: .top)
            }
        }
    }

    private var columns: [[MasonryItem]] {
        var result = Array(repeating: [MasonryItem](), count: max(columnCount, 1))
        var heights = Array(repeating: 0.0, count: result.count)
        for item in items {
            let target = heights.indices.min { heights[$0] < heights[$1] } ?? 0
            result[target].append(item)
            heights[target] += item.weight
        }
        return result
    }
}
