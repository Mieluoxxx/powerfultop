package thermal

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeSMC holds typed keys; mode keys only accept writes once Ftst is 1, as M3+ firmware does.
type fakeSMC struct {
	values map[string]Value
	writes []string
	keys   []string
}

func newFakeSMC() *fakeSMC {
	f := &fakeSMC{values: map[string]Value{
		"FNum": {Type: "ui8 ", Bytes: []byte{2}},
		"Ftst": {Type: "ui8 ", Bytes: []byte{0}},
		"F0Md": {Type: "ui8 ", Bytes: []byte{3}},
		"F1md": {Type: "ui8 ", Bytes: []byte{3}},
		"F0Mn": {Type: "flt ", Bytes: FloatBytes(2317)},
		"F0Mx": {Type: "flt ", Bytes: FloatBytes(7826)},
		"F1Mn": {Type: "flt ", Bytes: FloatBytes(2317)},
		"F1Mx": {Type: "flt ", Bytes: FloatBytes(7826)},
		"F0Tg": {Type: "flt ", Bytes: FloatBytes(0)},
		"F1Tg": {Type: "flt ", Bytes: FloatBytes(0)},
		"Tp01": {Type: "flt ", Bytes: FloatBytes(60)},
	}}
	f.keys = []string{"FNum", "Tp01", "TC0P"}
	return f
}

func (f *fakeSMC) Read(key string) (Value, bool) { v, ok := f.values[key]; return v, ok }

func (f *fakeSMC) Write(key string, b []byte) (uint8, bool) {
	v, ok := f.values[key]
	if !ok || len(v.Bytes) != len(b) {
		return 0, false
	}
	f.writes = append(f.writes, key)
	if (key == "F0Md" || key == "F1md") && b[0] == 1 && !bytes.Equal(f.values["Ftst"].Bytes, []byte{1}) {
		return 0x84, true
	}
	f.values[key] = Value{Type: v.Type, Bytes: append([]byte(nil), b...)}
	return 0, true
}

func (f *fakeSMC) KeyCount() int { return len(f.keys) }

func (f *fakeSMC) KeyAt(i int) (string, bool) { return f.keys[i], i < len(f.keys) }

func (f *fakeSMC) float(key string) float64 { v, _ := f.values[key].Float(); return v }

type clock struct{ t time.Time }

func (c *clock) now() time.Time        { return c.t }
func (c *clock) sleep(d time.Duration) { c.t = c.t.Add(d) }

func newController(t *testing.T, smc SMC) (*FanController, *clock, string) {
	marker := filepath.Join(t.TempDir(), "engaged")
	c := NewFanController(smc, marker)
	clk := &clock{t: time.Unix(1_000_000, 0)}
	c.now, c.sleep = clk.now, clk.sleep
	return c, clk, marker
}

func TestControllerEngagesClampsAndRestores(t *testing.T) {
	smc := newFakeSMC()
	c, _, marker := newController(t, smc)
	if err := c.Apply([]float64{1000, 9000}); err != nil {
		t.Fatal(err)
	}
	if !c.Engaged() || smc.float("Ftst") != 1 || smc.float("F0Md") != 1 || smc.float("F1md") != 1 {
		t.Error("diagnostic unlock, then both mode keys (Md and the M5 md) go manual")
	}
	if smc.float("F0Tg") != 2317 || smc.float("F1Tg") != 7826 {
		t.Errorf("targets clamp to each fan's range: %v %v", smc.float("F0Tg"), smc.float("F1Tg"))
	}
	if _, err := os.Stat(marker); err != nil {
		t.Error("the engaged marker exists while the helper holds the fans")
	}
	c.RestoreAutomatic()
	if c.Engaged() || smc.float("Ftst") != 0 || smc.float("F0Md") != 0 {
		t.Error("restore clears manual mode and the unlock")
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("restore removes the marker")
	}
}

func TestControllerHeartbeatAndFailsafe(t *testing.T) {
	smc := newFakeSMC()
	c, clk, _ := newController(t, smc)
	_ = c.Apply([]float64{3000, 3000})
	clk.t = clk.t.Add(HeartbeatTimeout + time.Second)
	c.Tick()
	if c.Engaged() {
		t.Error("fans go back to macOS when the app stops checking in")
	}

	_ = c.Apply([]float64{3000, 3000})
	smc.values["Tp01"] = Value{Type: "flt ", Bytes: FloatBytes(FailsafeCelsius)}
	c.Tick()
	c.Tick()
	if !c.Engaged() {
		t.Error("two hot ticks are a spike, not overheating")
	}
	c.Tick()
	if c.Engaged() {
		t.Error("three hot ticks hand the fans back")
	}
	if err := c.Apply([]float64{3000, 3000}); err == nil {
		t.Error("control pauses for a minute after overheating")
	}
}

func TestControllerRestoresAfterCrash(t *testing.T) {
	smc := newFakeSMC()
	smc.values["Ftst"] = Value{Type: "ui8 ", Bytes: []byte{1}}
	smc.values["F0Md"] = Value{Type: "ui8 ", Bytes: []byte{1}}
	marker := filepath.Join(t.TempDir(), "engaged")
	if err := os.WriteFile(marker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	NewFanController(smc, marker)
	if smc.float("Ftst") != 0 || smc.float("F0Md") != 0 {
		t.Error("a marker left by a crash makes the next start hand the fans back")
	}
}

func TestControllerGivesUpWhenModeNeverLands(t *testing.T) {
	smc := newFakeSMC()
	delete(smc.values, "Ftst")
	c, _, _ := newController(t, smc)
	if err := c.Apply([]float64{3000, 3000}); err == nil || c.Engaged() {
		t.Error("without the unlock the mode write never lands; the controller must give up after the timeout")
	}
}
