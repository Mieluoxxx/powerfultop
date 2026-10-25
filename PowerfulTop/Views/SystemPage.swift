import SwiftUI

/// CPU and memory in the style of Stats: rings up top, history, a breakdown, then top processes.
struct SystemPage: View {
    let system: SystemMonitor
    let thermal: ThermalMonitor
    let history: HistoryRecorder
    let columnCount: Int

    var body: some View {
        let columns = Array(repeating: GridItem(.flexible(), spacing: 16, alignment: .top), count: columnCount)
        LazyVGrid(columns: columns, alignment: .leading, spacing: 16) {
            CPUCard(system: system, thermal: thermal, history: history)
            MemoryCard(system: system, history: history)
        }
        .onAppear { system.processSamplingActive = true }
        .onDisappear { system.processSamplingActive = false }
    }
}

private struct CPUCard: View {
    let system: SystemMonitor
    let thermal: ThermalMonitor
    let history: HistoryRecorder

    private var cpu: CPULoad { system.cpu }

    var body: some View {
        DashCard(title: String(localized: "CPU"), icon: "cpu", tint: .blue) {
            HStack(spacing: 18) {
                Spacer(minLength: 0)
                if let temperature = thermal.snapshot.temperatures[.cpu]?.averageC {
                    RingGauge(fraction: temperature / 100, label: String(format: "%.0f°", temperature), caption: String(localized: "Temperature"), tint: .orange, size: 64)
                }
                RingGauge(fraction: cpu.usage, label: String(format: "%.0f%%", cpu.usage * 100), caption: String(localized: "Usage"), tint: .blue, size: 88)
                if let oneMinute = system.loadAverage.first {
                    RingGauge(
                        fraction: oneMinute / Double(max(cpu.perCore.count, 1)),
                        label: String(format: "%.2f", oneMinute),
                        caption: String(localized: "Load"),
                        tint: .purple,
                        size: 64
                    )
                }
                Spacer(minLength: 0)
            }
            .padding(.vertical, 4)

            MiniChart(points: history.history.series { $0.cpuUsage }, tint: .blue, domain: 0...100)

            CoreBars(perCore: cpu.perCore, kinds: system.coreKinds)

            SectionLabel(String(localized: "Details"))
            LegendRow(color: .red, label: String(localized: "System"), value: percent(cpu.system))
            LegendRow(color: .blue, label: String(localized: "User"), value: percent(cpu.user))
            LegendRow(color: .gray, label: String(localized: "Idle"), value: percent(cpu.idle))
            if let efficiency = cpu.efficiency {
                LegendRow(color: .teal, label: String(localized: "Efficiency Cores"), value: percent(efficiency))
            }
            if let performance = cpu.performance {
                LegendRow(color: .indigo, label: String(localized: "Performance Cores"), value: percent(performance))
            }
            if let boot = system.bootDate {
                CardRow(label: String(localized: "Uptime"), value: uptimeText(since: boot))
            }

            if system.loadAverage.count == 3 {
                SectionLabel(String(localized: "Load Average"))
                CardRow(label: String(localized: "1 min"), value: String(format: "%.2f", system.loadAverage[0]))
                CardRow(label: String(localized: "5 min"), value: String(format: "%.2f", system.loadAverage[1]))
                CardRow(label: String(localized: "15 min"), value: String(format: "%.2f", system.loadAverage[2]))
            }

            SectionLabel(String(localized: "Top Processes"))
            ProcessList(processes: system.topCPU) { String(format: "%.1f%%", $0) }
        }
    }

    private func percent(_ fraction: Double) -> String {
        String(format: "%.0f%%", fraction * 100)
    }

    private func uptimeText(since boot: Date) -> String {
        let formatter = DateComponentsFormatter()
        formatter.allowedUnits = [.day, .hour, .minute]
        formatter.maximumUnitCount = 2
        formatter.unitsStyle = .short
        return formatter.string(from: boot, to: Date()) ?? "—"
    }
}

private struct MemoryCard: View {
    let system: SystemMonitor
    let history: HistoryRecorder

    var body: some View {
        DashCard(title: String(localized: "Memory"), icon: "memorychip", tint: .green) {
            if let memory = system.memory {
                HStack(spacing: 24) {
                    Spacer(minLength: 0)
                    RingGauge(
                        fraction: memory.usedFraction,
                        label: String(format: "%.0f%%", memory.usedFraction * 100),
                        caption: pressureText(memory.pressure),
                        tint: pressureTint(memory.pressure),
                        size: 88
                    )
                    Spacer(minLength: 0)
                }
                .padding(.vertical, 4)

                MiniChart(points: history.history.series { $0.memoryUsage }, tint: .green, domain: 0...100)

                SectionLabel(String(localized: "Details"))
                CardRow(label: String(localized: "Used"), value: formatBytes(Double(memory.usedBytes)), rated: formatBytes(Double(memory.totalBytes)), emphasized: true)
                StackedBar(segments: [
                    (Double(memory.appBytes), .blue),
                    (Double(memory.wiredBytes), .orange),
                    (Double(memory.compressedBytes), .red),
                ], total: Double(memory.totalBytes))
                LegendRow(color: .blue, label: String(localized: "App Memory"), value: formatBytes(Double(memory.appBytes)))
                LegendRow(color: .orange, label: String(localized: "Wired Memory"), value: formatBytes(Double(memory.wiredBytes)))
                LegendRow(color: .red, label: String(localized: "Compressed"), value: formatBytes(Double(memory.compressedBytes)))
                LegendRow(color: .gray, label: String(localized: "Available"), value: formatBytes(Double(memory.availableBytes)))
                CardRow(label: String(localized: "Swap Used"), value: formatBytes(Double(memory.swapUsedBytes)))
            }

            SectionLabel(String(localized: "Top Processes"))
            ProcessList(processes: system.topMemory) { formatBytes($0) }
        }
    }

    private func pressureText(_ pressure: MemoryPressure) -> String {
        switch pressure {
        case .normal: return String(localized: "Pressure Normal")
        case .warning: return String(localized: "Pressure Warning")
        case .critical: return String(localized: "Pressure Critical")
        }
    }

    private func pressureTint(_ pressure: MemoryPressure) -> Color {
        switch pressure {
        case .normal: return .green
        case .warning: return .yellow
        case .critical: return .red
        }
    }
}

// MARK: - Components

struct RingGauge: View {
    let fraction: Double
    let label: String
    let caption: String
    let tint: Color
    var size: CGFloat = 72

    var body: some View {
        VStack(spacing: 6) {
            ZStack {
                Circle().stroke(Color.primary.opacity(0.1), lineWidth: size * 0.1)
                Circle()
                    .trim(from: 0, to: min(max(fraction, 0), 1))
                    .stroke(tint, style: StrokeStyle(lineWidth: size * 0.1, lineCap: .round))
                    .rotationEffect(.degrees(-90))
                Text(label)
                    .font(.system(size: size * 0.22, weight: .semibold, design: .rounded))
                    .monospacedDigit()
            }
            .frame(width: size, height: size)
            Text(caption)
                .font(.system(size: 10))
                .foregroundStyle(.secondary)
        }
    }
}

private struct MiniChart: View {
    let points: [(date: Date, value: Double)]
    let tint: Color
    let domain: ClosedRange<Double>

    var body: some View {
        ChartPanelChart(points: points, tint: tint, domain: domain, height: 70)
            .padding(8)
            .background(Color.primary.opacity(0.04), in: RoundedRectangle(cornerRadius: 10, style: .continuous))
    }
}

private struct CoreBars: View {
    let perCore: [Double]
    let kinds: [CoreKind]

    var body: some View {
        HStack(alignment: .bottom, spacing: 3) {
            ForEach(Array(perCore.enumerated()), id: \.offset) { index, usage in
                let tint: Color = index < kinds.count && kinds[index] == .efficiency ? .teal : .indigo
                ZStack(alignment: .bottom) {
                    RoundedRectangle(cornerRadius: 2).fill(Color.primary.opacity(0.06))
                    RoundedRectangle(cornerRadius: 2).fill(tint.opacity(0.85))
                        .frame(height: max(2, 44 * min(max(usage, 0), 1)))
                }
                .frame(height: 44)
            }
        }
        .padding(8)
        .background(Color.primary.opacity(0.04), in: RoundedRectangle(cornerRadius: 10, style: .continuous))
    }
}

private struct StackedBar: View {
    let segments: [(value: Double, color: Color)]
    let total: Double

    var body: some View {
        GeometryReader { proxy in
            HStack(spacing: 1) {
                ForEach(Array(segments.enumerated()), id: \.offset) { _, segment in
                    Rectangle()
                        .fill(segment.color)
                        .frame(width: total > 0 ? proxy.size.width * segment.value / total : 0)
                }
                Rectangle().fill(Color.primary.opacity(0.1))
            }
        }
        .frame(height: 8)
        .clipShape(Capsule())
    }
}

private struct LegendRow: View {
    let color: Color
    let label: String
    let value: String

    var body: some View {
        HStack(spacing: 8) {
            RoundedRectangle(cornerRadius: 3)
                .fill(color)
                .frame(width: 10, height: 10)
            CardRow(label: label, value: value)
        }
    }
}

private struct SectionLabel: View {
    let text: String

    init(_ text: String) { self.text = text }

    var body: some View {
        HStack(spacing: 8) {
            Rectangle().fill(Color.primary.opacity(0.1)).frame(height: 1)
            Text(text)
                .font(.system(size: 10, weight: .semibold))
                .foregroundStyle(.tertiary)
                .fixedSize()
            Rectangle().fill(Color.primary.opacity(0.1)).frame(height: 1)
        }
        .padding(.top, 6)
    }
}

private struct ProcessList: View {
    let processes: [ProcessUsage]
    let format: (Double) -> String

    var body: some View {
        if processes.isEmpty {
            Text(String(localized: "Collecting data…"))
                .font(.system(size: 11))
                .foregroundStyle(.tertiary)
        } else {
            ForEach(processes) { process in
                CardRow(label: process.name, value: format(process.value))
            }
        }
    }
}
