import Foundation
import Observation

@MainActor
@Observable
final class HistoryRecorder {
    private(set) var history = MetricHistory()

    private let monitor: PowerMonitor
    private let thermal: ThermalMonitor
    private let system: SystemMonitor
    private var timer: Timer?

    init(monitor: PowerMonitor, thermal: ThermalMonitor, system: SystemMonitor) {
        self.monitor = monitor
        self.thermal = thermal
        self.system = system
    }

    func start() {
        guard timer == nil else { return }
        record()
        let newTimer = Timer(timeInterval: MetricHistory.sampleInterval, repeats: true) { [weak self] _ in
            Task { @MainActor in self?.record() }
        }
        RunLoop.main.add(newTimer, forMode: .common)
        timer = newTimer
    }

    func stop() {
        timer?.invalidate()
        timer = nil
    }

    private func record() {
        guard monitor.isDataAvailable else { return }
        let data = monitor.currentData
        let snapshot = thermal.snapshot
        history.append(MetricSample(
            date: Date(),
            batteryPercent: Double(data.displayBatteryPercent),
            systemPowerW: data.systemPowerW,
            cpuAverageC: snapshot.temperatures[.cpu]?.averageC,
            fanRPM: snapshot.hasFans ? snapshot.fans.map(\.actualRPM).max() : nil,
            batteryTemperatureC: batteryTemperatureC(data: data, thermal: snapshot),
            cpuUsage: system.cpu.perCore.isEmpty ? nil : system.cpu.usage * 100,
            memoryUsage: system.memory.map { $0.usedFraction * 100 }
        ))
    }
}

/// Some machines drop `Temperature` from AppleSmartBattery; the SMC pack thermistors still report it.
func batteryTemperatureC(data: PowerData, thermal: ThermalSnapshot) -> Double? {
    data.batteryTemperatureC ?? thermal.temperatures[.battery]?.averageC
}
