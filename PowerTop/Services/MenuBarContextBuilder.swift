import Foundation

/// Snapshot of every value the menu bar can show; shared by the status item and the settings preview.
@MainActor
func makeMenuBarContext(monitor: PowerMonitor, thermal: ThermalMonitor, system: SystemMonitor) -> MenuBarContext {
    let data = monitor.currentData
    let settledAC = data.hasSettledAC
    var context = MenuBarContext()
    context.isDataAvailable = monitor.isDataAvailable
    context.isOnAC = data.effectiveIsOnAC
    context.batteryPercent = data.displayBatteryPercent
    context.timeRemaining = data.estimatedTimeRemainingText
    context.batteryTemperatureC = batteryTemperatureC(data: data, thermal: thermal.snapshot)
    context.batteryCurrentMA = data.batteryAmperageMA
    context.batteryVoltageMV = data.batteryVoltageMV
    switch data.powerState {
    case .charging(let rate): context.batteryPowerW = rate
    case .supplementing(let rate): context.batteryPowerW = -rate
    case .onBattery: context.batteryPowerW = -data.systemPowerW
    case .acIdle, .connectingAC: context.batteryPowerW = 0
    }
    context.systemPowerW = data.systemPowerW
    context.systemPowerWarning = data.menuBarPowerShowsWarning
    context.adapterCurrentA = settledAC ? data.systemCurrentMA.map { Double($0) / 1000 } : nil
    context.adapterVoltageV = settledAC ? data.systemVoltageMV.map { Double($0) / 1000 } : nil
    context.adapterPowerW = settledAC ? data.effectiveACOutputW : nil
    context.health = data.batteryHealthPercent
    context.cycleCount = data.cycleCount
    context.cpuUsage = system.cpu.perCore.isEmpty ? nil : system.cpu.usage
    context.memoryUsage = system.memory?.usedFraction
    context.cpuTemperatureC = thermal.snapshot.temperatures[.cpu]?.averageC
    context.fanRPM = thermal.snapshot.hasFans ? thermal.snapshot.fans.map(\.actualRPM).max() : nil
    return context
}
