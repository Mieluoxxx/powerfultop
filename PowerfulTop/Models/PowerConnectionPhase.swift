import Foundation

/// User-visible connection phase driven by IOPS events and telemetry convergence.
enum PowerConnectionPhase: Equatable {
    case onBattery
    case connectingAC
    case onAC
}

/// Settled power situation derived from one snapshot plus the connection phase.
enum PowerState: Equatable {
    case onBattery
    case connectingAC
    case acIdle
    case charging(rateW: Double)
    /// AC is connected but the battery still discharges to cover the load.
    case supplementing(rateW: Double)
}
