// Package thermal reads temperatures and fans from the SMC and drives the fans through the root helper.
package thermal

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
)

// FanReading is one fan as the SMC reports it.
type FanReading struct {
	Index     int
	ActualRPM float64
	MinRPM    float64
	MaxRPM    float64
	TargetRPM float64
	// Mode is raw F%dMd: 0 auto, 1 manual, 3 system-managed (Apple Silicon default); -1 when unreadable.
	Mode int
}

func (f FanReading) IsManual() bool { return f.Mode == 1 }

// SensorGroup is a sensor family verified to track load on Apple Silicon (M4 Max probe).
type SensorGroup string

const (
	GroupCPU     SensorGroup = "cpu"
	GroupGPU     SensorGroup = "gpu"
	GroupBattery SensorGroup = "battery"
)

// SensorGroups lists the groups in display order.
var SensorGroups = []SensorGroup{GroupCPU, GroupGPU, GroupBattery}

// Matches reports whether an SMC key belongs to the group. Tp*/Te* are performance/efficiency
// cores, Tg* the GPU, TB?T the pack thermistors. Tf* stays excluded: it held 90.2 °C idle and
// under load, so it is a limit, not a reading.
func (g SensorGroup) Matches(key string) bool {
	switch g {
	case GroupCPU:
		return strings.HasPrefix(key, "Tp") || strings.HasPrefix(key, "Te")
	case GroupGPU:
		return strings.HasPrefix(key, "Tg")
	case GroupBattery:
		return len(key) == 4 && strings.HasPrefix(key, "TB") && strings.HasSuffix(key, "T")
	}
	return false
}

// TemperatureSummary condenses one group's sensors.
type TemperatureSummary struct {
	AverageC    float64
	MaximumC    float64
	SensorCount int
}

// Summarize returns the summary of readings, false when there are none.
func Summarize(readings []float64) (TemperatureSummary, bool) {
	if len(readings) == 0 {
		return TemperatureSummary{}, false
	}
	sum, hi := 0.0, math.Inf(-1)
	for _, r := range readings {
		sum += r
		hi = math.Max(hi, r)
	}
	return TemperatureSummary{AverageC: sum / float64(len(readings)), MaximumC: hi, SensorCount: len(readings)}, true
}

// SmoothTemperatures averages exponentially over timeConstant seconds (10 by default). Per-core
// hotspot sensors (Tp1x, Tp1o, …) jump 20–25 °C within half a second as single cores burst, so the
// raw group maximum swings wildly; this keeps a brief spike to a few degrees while sustained heating
// still comes through within tens of seconds.
func SmoothTemperatures(previous, current map[SensorGroup]TemperatureSummary, elapsedSeconds float64) map[SensorGroup]TemperatureSummary {
	const timeConstant = 10.0
	alpha := 1 - math.Exp(-math.Max(elapsedSeconds, 0)/timeConstant)
	result := make(map[SensorGroup]TemperatureSummary, len(current))
	for group, sample := range current {
		last, ok := previous[group]
		if !ok {
			result[group] = sample
			continue
		}
		result[group] = TemperatureSummary{
			AverageC:    last.AverageC + alpha*(sample.AverageC-last.AverageC),
			MaximumC:    last.MaximumC + alpha*(sample.MaximumC-last.MaximumC),
			SensorCount: sample.SensorCount,
		}
	}
	return result
}

// Snapshot is one reading of the fans and temperatures, plus the control state around them.
type Snapshot struct {
	Fans         []FanReading
	Temperatures map[SensorGroup]TemperatureSummary
	// DiagnosticUnlocked is Ftst = 1: something has put the SMC into diagnostic (manual fan) mode.
	DiagnosticUnlocked bool
	// ConflictingTool names the fan tool that runs, "" when the conflict shows only in the fans' state.
	ConflictingTool string
	// DisplayPowerW is SMC PDBR, built-in display power; it tracks brightness from 0.2 W to about 7 W on an M4 Max.
	DisplayPowerW *float64

	Mode           FanControlMode
	LastCustomMode FanControlMode
	HelperStatus   HelperStatus
	// Installing is true while the administrator prompt or the install script runs.
	Installing bool
	// Controlling is true while PowerfulTop holds the fans in a non-automatic mode.
	Controlling bool
	// ConflictDetected means another fan tool runs or already holds the fans; PowerfulTop then only monitors.
	ConflictDetected bool
	// LastError is the last failure to show; ErrCancelled and the other sentinels have localized texts.
	LastError error
	// Available is false when the SMC can't be opened, as on Intel or in a VM.
	Available bool
}

func (s Snapshot) HasFans() bool { return len(s.Fans) > 0 }

func (s Snapshot) AnyFanManual() bool {
	for _, f := range s.Fans {
		if f.IsManual() {
			return true
		}
	}
	return false
}

// HottestC is the highest group maximum, false without readings.
func (s Snapshot) HottestC() (float64, bool) {
	hi, ok := math.Inf(-1), false
	for _, t := range s.Temperatures {
		hi, ok = math.Max(hi, t.MaximumC), true
	}
	return hi, ok
}

// MaxActualRPM is the fastest fan's speed, 0 without fans.
func (s Snapshot) MaxActualRPM() float64 {
	hi := 0.0
	for _, f := range s.Fans {
		hi = math.Max(hi, f.ActualRPM)
	}
	return hi
}

// IsPlausibleTemperature discards sentinels and unused channels.
func IsPlausibleTemperature(celsius float64) bool {
	return celsius > 5 && celsius < 130
}

// GroupSensorKeys sorts SMC keys into sensor groups, keeping their order.
func GroupSensorKeys(keys []string) map[SensorGroup][]string {
	groups := map[SensorGroup][]string{}
	for _, key := range keys {
		for _, g := range SensorGroups {
			if g.Matches(key) {
				groups[g] = append(groups[g], key)
				break
			}
		}
	}
	return groups
}

// IsForeignFanControl reports another tool's hold. Manual fan state only counts as another tool's
// when our helper's marker is absent; otherwise a single failed request would make PowerfulTop back
// off from its own control and let the watchdog fire.
func IsForeignFanControl(s Snapshot, helperHoldsFans, otherToolRunning bool) bool {
	if otherToolRunning {
		return true
	}
	if !s.AnyFanManual() && !s.DiagnosticUnlocked {
		return false
	}
	return !helperHoldsFans
}

// FanControlModeKind names a fan control mode.
type FanControlModeKind int

const (
	ModeAutomatic FanControlModeKind = iota
	ModeFullBlast
	ModeConstant
	// ModeSensor ramps linearly from the fan minimum at LowerC to its maximum at UpperC, like Macs Fan Control.
	ModeSensor
)

// FanControlMode is how PowerfulTop drives the fans.
type FanControlMode struct {
	Kind   FanControlModeKind
	RPM    float64
	Group  SensorGroup
	LowerC float64
	UpperC float64
}

var (
	Automatic       = FanControlMode{Kind: ModeAutomatic}
	FullBlast       = FanControlMode{Kind: ModeFullBlast}
	DefaultConstant = Constant(3000)
	DefaultSensor   = Sensor(GroupCPU, 55, 85)
)

func Constant(rpm float64) FanControlMode { return FanControlMode{Kind: ModeConstant, RPM: rpm} }

func Sensor(group SensorGroup, lowerC, upperC float64) FanControlMode {
	return FanControlMode{Kind: ModeSensor, Group: group, LowerC: lowerC, UpperC: upperC}
}

func (m FanControlMode) IsAutomatic() bool { return m.Kind == ModeAutomatic }

// MarshalJSON writes Swift's synthesized Codable shape, e.g. {"constant":{"rpm":3000}}, so a mode
// saved by the Swift app keeps working.
func (m FanControlMode) MarshalJSON() ([]byte, error) {
	switch m.Kind {
	case ModeFullBlast:
		return json.Marshal(map[string]any{"fullBlast": map[string]any{}})
	case ModeConstant:
		return json.Marshal(map[string]any{"constant": map[string]any{"rpm": m.RPM}})
	case ModeSensor:
		return json.Marshal(map[string]any{"sensor": map[string]any{"group": m.Group, "lowerC": m.LowerC, "upperC": m.UpperC}})
	default:
		return json.Marshal(map[string]any{"automatic": map[string]any{}})
	}
}

func (m *FanControlMode) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	switch {
	case raw["automatic"] != nil:
		*m = Automatic
	case raw["fullBlast"] != nil:
		*m = FullBlast
	case raw["constant"] != nil:
		var v struct {
			RPM float64 `json:"rpm"`
		}
		if err := json.Unmarshal(raw["constant"], &v); err != nil {
			return err
		}
		*m = Constant(v.RPM)
	case raw["sensor"] != nil:
		var v struct {
			Group  SensorGroup `json:"group"`
			LowerC float64     `json:"lowerC"`
			UpperC float64     `json:"upperC"`
		}
		if err := json.Unmarshal(raw["sensor"], &v); err != nil {
			return err
		}
		*m = Sensor(v.Group, v.LowerC, v.UpperC)
	default:
		return errors.New("thermal: unknown fan control mode")
	}
	return nil
}

// FanTargetRPM is the target for one fan, false when the system should stay in charge.
func FanTargetRPM(mode FanControlMode, fan FanReading, temperatures map[SensorGroup]TemperatureSummary) (float64, bool) {
	var target float64
	switch mode.Kind {
	case ModeAutomatic:
		return 0, false
	case ModeFullBlast:
		target = fan.MaxRPM
	case ModeConstant:
		target = mode.RPM
	case ModeSensor:
		reading, ok := temperatures[mode.Group]
		if !ok {
			// Without a reading, fail towards cooling rather than silence.
			return fan.MaxRPM, true
		}
		span := math.Max(mode.UpperC-mode.LowerC, 1)
		fraction := math.Min(math.Max((reading.MaximumC-mode.LowerC)/span, 0), 1)
		target = fan.MinRPM + (fan.MaxRPM-fan.MinRPM)*fraction
	}
	return math.Min(math.Max(target, fan.MinRPM), fan.MaxRPM), true
}
