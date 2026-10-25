package power

import (
	"math"
	"time"
)

type estimatorMode int

const (
	modeNone estimatorMode = iota
	modeBatteryDischarging
	modeSupplementalDischarging
	modeCharging
)

// Estimator smooths instantaneous power for battery time estimates when macOS no longer reports
// AvgTimeToEmpty/Full. The zero value is ready to use.
type Estimator struct {
	dischargeEMA *float64
	chargeEMA    *float64
	lastMode     estimatorMode
	lastSampleAt *time.Time
}

// AppleSmartBattery refreshes telemetry about once a minute while the UI polls every 2 s,
// so samples are weighted by elapsed snapshot time instead of per poll.
const estimatorTimeConstant = 180.0

func (e *Estimator) Reset() { *e = Estimator{} }

// SmoothedPower returns smoothed discharge/charge power (watts) for the current display snapshot.
func (e *Estimator) SmoothedPower(d Data) (dischargeW, chargeW *float64) {
	s := d.PowerState()
	switch {
	case s.Kind == StateCharging && !d.FullyCharged:
		return nil, e.updateCharge(s.RateW, d)
	case s.Kind == StateOnBattery:
		return e.updateDischarge(modeBatteryDischarging, d.SystemPowerW, d.Timestamp), nil
	case s.Kind == StateSupplementing:
		return e.updateDischarge(modeSupplementalDischarging, s.RateW, d.Timestamp), nil
	}
	e.Reset()
	return nil, nil
}

func (e *Estimator) updateDischarge(mode estimatorMode, instant float64, at time.Time) *float64 {
	e.switchMode(mode)
	if instant < 0.2 {
		return e.dischargeEMA
	}
	elapsed, ok := e.consume(at)
	if !ok {
		return e.dischargeEMA
	}
	e.dischargeEMA = ptr(ema(e.dischargeEMA, instant, elapsed))
	return e.dischargeEMA
}

func (e *Estimator) updateCharge(instant float64, d Data) *float64 {
	e.switchMode(modeCharging)
	if instant >= 0.2 {
		if elapsed, ok := e.consume(d.Timestamp); ok {
			e.chargeEMA = ptr(ema(e.chargeEMA, instant, elapsed))
		}
	}
	if e.chargeEMA == nil {
		return nil
	}
	return ptr(chargeTaper(*e.chargeEMA, d.soc(), d.ChargeLimitPercent))
}

func (e *Estimator) switchMode(mode estimatorMode) {
	if e.lastMode != mode {
		*e = Estimator{lastMode: mode}
	}
}

// consume returns the time since the previous sample, or false when this snapshot was already used.
func (e *Estimator) consume(at time.Time) (float64, bool) {
	if e.lastSampleAt == nil {
		e.lastSampleAt = &at
		return 0, true
	}
	elapsed := at.Sub(*e.lastSampleAt).Seconds()
	if elapsed <= 0 {
		return 0, false
	}
	e.lastSampleAt = &at
	return elapsed, true
}

func ema(previous *float64, sample, elapsed float64) float64 {
	if previous == nil {
		return sample
	}
	alpha := 1 - math.Exp(-elapsed/estimatorTimeConstant)
	return *previous + alpha*(sample-*previous)
}

// chargeTaper reduces the effective rate near the target to approximate constant-voltage taper.
func chargeTaper(rateW float64, soc, target int) float64 {
	target = min(max(target, 1), 100)
	taperStart := max(65, target-15)
	if soc <= taperStart || target <= taperStart {
		return rateW
	}
	span := float64(target - taperStart)
	taper := math.Max(0.25, 1-float64(soc-taperStart)/span*0.75)
	return rateW * taper
}
