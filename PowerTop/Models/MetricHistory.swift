import Foundation

struct MetricSample: Equatable, Identifiable {
    let date: Date
    let batteryPercent: Double
    let systemPowerW: Double
    let cpuAverageC: Double?
    let fanRPM: Double?
    let batteryTemperatureC: Double?

    var id: Date { date }
}

/// Short in-memory history for the dashboard sparklines; nothing is written to disk.
struct MetricHistory: Equatable {
    static let window: TimeInterval = 30 * 60
    static let sampleInterval: TimeInterval = 5

    private(set) var samples: [MetricSample] = []

    mutating func append(_ sample: MetricSample) {
        // Ignore bursts (wake, manual refresh) so spacing stays even for the charts.
        if let last = samples.last, sample.date.timeIntervalSince(last.date) < Self.sampleInterval * 0.5 {
            return
        }
        samples.append(sample)
        let cutoff = sample.date.addingTimeInterval(-Self.window)
        if let firstKept = samples.firstIndex(where: { $0.date >= cutoff }), firstKept > 0 {
            samples.removeFirst(firstKept)
        }
    }

    func series(_ value: (MetricSample) -> Double?) -> [(date: Date, value: Double)] {
        samples.compactMap { sample in value(sample).map { (sample.date, $0) } }
    }
}
