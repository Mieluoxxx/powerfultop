package thermal

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

// Mirrors testSMCParamLayout: AppleSMC rejects every call unless the struct matches the kernel's
// 80-byte SMCKeyData_t.
func TestSMCParamLayout(t *testing.T) {
	if ParamSize != 80 || offsetDataSize != 28 || offsetResult != 40 || offsetCommand != 42 || offsetData32 != 44 || offsetBytes != 48 {
		t.Fatal("SMCKeyData_t offsets changed")
	}
	if FourCC("F0Ac") != 0x4630_4163 {
		t.Errorf("fourCC packs big-endian: %#x", FourCC("F0Ac"))
	}
	if FourCCString(0x4630_4163) != "F0Ac" {
		t.Error("fourCC round trip")
	}
	var p Param
	p.setKey(FourCC("F0Ac"))
	if p.key() != FourCC("F0Ac") || p[0] != 'c' {
		t.Error("the key is stored native-endian, as the C struct holds it")
	}
	if v, _ := (Value{Type: "flt ", Bytes: FloatBytes(2317)}).Float(); v != 2317 {
		t.Errorf("flt round trip: %v", v)
	}
	if v, _ := (Value{Type: "ui8 ", Bytes: []byte{3}}).Float(); v != 3 {
		t.Error("ui8 mode")
	}
	if v, _ := (Value{Type: "ui16", Bytes: []byte{1, 2}}).Float(); v != 258 {
		t.Error("ui16 is big-endian")
	}
}

// Mirrors testFanTargets.
func TestFanTargets(t *testing.T) {
	fan := FanReading{MinRPM: 2317, MaxRPM: 7826, Mode: 3}
	hot, _ := Summarize([]float64{60, 70})
	temps := map[SensorGroup]TemperatureSummary{GroupCPU: hot}

	if _, ok := FanTargetRPM(Automatic, fan, temps); ok {
		t.Error("automatic leaves fans to macOS")
	}
	check := func(mode FanControlMode, temps map[SensorGroup]TemperatureSummary, want float64, msg string) {
		t.Helper()
		got, ok := FanTargetRPM(mode, fan, temps)
		if !ok || math.Abs(got-want) > 1e-9 {
			t.Errorf("%s: got %v", msg, got)
		}
	}
	check(FullBlast, temps, 7826, "full blast uses the fan maximum")
	check(Constant(1000), temps, 2317, "clamp below minimum")
	check(Constant(9000), temps, 7826, "clamp above maximum")
	ramp := Sensor(GroupCPU, 50, 90)
	check(ramp, temps, 2317+(7826-2317)*0.5, "70 °C is halfway between 50 and 90 °C")
	cold, _ := Summarize([]float64{40})
	check(ramp, map[SensorGroup]TemperatureSummary{GroupCPU: cold}, 2317, "below the ramp stays at minimum")
	check(ramp, nil, 7826, "missing sensor fails towards cooling")

	data, _ := json.Marshal(ramp)
	var decoded FanControlMode
	if err := json.Unmarshal(data, &decoded); err != nil || decoded != ramp {
		t.Errorf("mode persists: %s -> %+v %v", data, decoded, err)
	}
}

// The Swift app saved modes with JSONEncoder; those must load unchanged.
func TestFanModeReadsSwiftEncoding(t *testing.T) {
	cases := map[string]FanControlMode{
		`{"automatic":{}}`:                                   Automatic,
		`{"fullBlast":{}}`:                                   FullBlast,
		`{"constant":{"rpm":3000}}`:                          Constant(3000),
		`{"sensor":{"group":"gpu","lowerC":55,"upperC":85}}`: Sensor(GroupGPU, 55, 85),
	}
	for raw, want := range cases {
		var got FanControlMode
		if err := json.Unmarshal([]byte(raw), &got); err != nil || got != want {
			t.Errorf("%s: got %+v %v", raw, got, err)
		}
		encoded, _ := json.Marshal(want)
		var back map[string]any
		var orig map[string]any
		_ = json.Unmarshal(encoded, &back)
		_ = json.Unmarshal([]byte(raw), &orig)
		if !reflect.DeepEqual(back, orig) {
			t.Errorf("encodes %s, Swift wrote %s", encoded, raw)
		}
	}
}

// Mirrors testSensorGrouping.
func TestSensorGrouping(t *testing.T) {
	groups := GroupSensorKeys([]string{"Tp00", "Te04", "Tg0K", "TB0T", "TB1T", "Tf04", "TBXT0", "TPD0", "TC0P"})
	if !reflect.DeepEqual(groups[GroupCPU], []string{"Tp00", "Te04"}) {
		t.Errorf("performance and efficiency cores: %v", groups[GroupCPU])
	}
	if !reflect.DeepEqual(groups[GroupGPU], []string{"Tg0K"}) {
		t.Errorf("GPU sensors: %v", groups[GroupGPU])
	}
	if !reflect.DeepEqual(groups[GroupBattery], []string{"TB0T", "TB1T"}) {
		t.Errorf("pack thermistors only: %v", groups[GroupBattery])
	}
	if IsPlausibleTemperature(0) || IsPlausibleTemperature(200) || !IsPlausibleTemperature(45) {
		t.Error("plausible temperature window")
	}
}

// Mirrors testTemperatureSmoothing.
func TestTemperatureSmoothing(t *testing.T) {
	calm := map[SensorGroup]TemperatureSummary{GroupCPU: {AverageC: 42, MaximumC: 54, SensorCount: 112}}
	spike := map[SensorGroup]TemperatureSummary{GroupCPU: {AverageC: 44, MaximumC: 79, SensorCount: 112}}

	if first := SmoothTemperatures(nil, spike, 0); first[GroupCPU].MaximumC != 79 {
		t.Error("first sample seeds the average")
	}
	// Live trace: max jumped 54 -> 79 °C within one 2 s refresh while the average moved 2 °C.
	damped := SmoothTemperatures(calm, spike, 2)
	alpha := 1 - math.Exp(-2.0/10.0)
	if math.Abs(damped[GroupCPU].MaximumC-(54+25*alpha)) > 1e-9 || damped[GroupCPU].MaximumC >= 60 {
		t.Errorf("a one-sample spike stays damped: %v", damped[GroupCPU].MaximumC)
	}
	sustained := calm
	for range 30 {
		sustained = SmoothTemperatures(sustained, spike, 2)
	}
	if math.Abs(sustained[GroupCPU].MaximumC-79) >= 0.1 {
		t.Error("sustained heat converges within a minute")
	}
}

// Mirrors testForeignFanControlDetection.
func TestForeignFanControlDetection(t *testing.T) {
	manualFan := FanReading{ActualRPM: 5500, MinRPM: 2317, MaxRPM: 7826, TargetRPM: 5517, Mode: 1}
	systemFan := FanReading{MinRPM: 2317, MaxRPM: 7826, Mode: 3}
	ours := Snapshot{Fans: []FanReading{manualFan}, DiagnosticUnlocked: true}
	idle := Snapshot{Fans: []FanReading{systemFan}}

	// Regression: a timed-out first request made PowerfulTop treat its own helper as a rival,
	// stop sending heartbeats, and the watchdog handed fans back every 30 s.
	if IsForeignFanControl(ours, true, false) {
		t.Error("our helper's manual mode is not a conflict")
	}
	if !IsForeignFanControl(ours, false, false) {
		t.Error("manual mode without our marker belongs to another tool")
	}
	if IsForeignFanControl(idle, false, false) {
		t.Error("system-managed fans are free to take")
	}
	if !IsForeignFanControl(idle, false, true) {
		t.Error("a running fan app always counts as a conflict")
	}
}

func TestRequestValidation(t *testing.T) {
	valid := []Request{{Op: OpVersion}, {Op: OpRestore}, {Op: OpSetTargets, RPMs: []float64{2000, 3000}}}
	for _, r := range valid {
		if err := r.Validate(); err != nil {
			t.Errorf("%+v: %v", r, err)
		}
	}
	invalid := []Request{
		{Op: "writeKey"},
		{Op: OpSetTargets},
		{Op: OpSetTargets, RPMs: []float64{math.NaN()}},
		{Op: OpSetTargets, RPMs: []float64{math.Inf(1)}},
		{Op: OpSetTargets, RPMs: []float64{-1}},
		{Op: OpSetTargets, RPMs: make([]float64, maxFans+1)},
	}
	for _, r := range invalid {
		if r.Validate() == nil {
			t.Errorf("%+v must be refused", r)
		}
	}
}
