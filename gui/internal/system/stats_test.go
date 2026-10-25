package system

import (
	"math"
	"reflect"
	"testing"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

// Mirrors testCPULoad in the Swift regression tests.
func TestLoadBetween(t *testing.T) {
	before := []CoreTicks{{User: 100, System: 50, Idle: 850}, {Idle: 1000}}
	after := []CoreTicks{{User: 160, System: 70, Idle: 870}, {User: 10, Idle: 1090}}
	load, ok := LoadBetween(before, after, []CoreKind{Efficiency, Performance})
	if !ok || !approx(load.Usage(), 0.45) || !approx(load.User, 0.35) {
		t.Fatalf("load %+v", load)
	}
	if !load.HasEfficiency || !approx(load.Efficiency, 0.8) || !load.HasPerformance || !approx(load.Performance, 0.1) {
		t.Errorf("clusters %+v", load)
	}
	if len(load.PerCore) != 2 {
		t.Errorf("per-core values %v", load.PerCore)
	}
	reset := []CoreTicks{{User: 5, Idle: 5}, {User: 20, Idle: 1180}}
	afterReset, _ := LoadBetween(after, reset, []CoreKind{Efficiency, Performance})
	if afterReset.PerCore[0] != 0 {
		t.Error("a core whose counters went backwards contributes nothing")
	}
	if _, ok := LoadBetween(nil, after, nil); ok {
		t.Error("no load without a previous sample")
	}
}

// Mirrors testMemoryAndProcessParsing in the Swift regression tests.
func TestMemoryAndProcessParsing(t *testing.T) {
	const gib, page = uint64(1 << 30), uint64(16_384)
	m := NewMemoryStats(128*gib, 46*gib/page, 2*gib/page, 7*gib/page, 3*gib/page, page, 0, PressureNormal)
	if m.AppBytes != 44*gib || m.UsedBytes() != 54*gib || !approx(m.UsedFraction(), 54.0/128.0) {
		t.Errorf("memory %+v", m)
	}

	ps := "  410  33.5 WindowServer\n 5269  25.1 ego Helper (Renderer)\n    1  20.6 launchd\n"
	cpu := ParseProcessCPU(ps, 2)
	if names := []string{cpu[0].Name, cpu[1].Name}; !reflect.DeepEqual(names, []string{"WindowServer", "ego Helper (Renderer)"}) || cpu[0].Value != 33.5 {
		t.Errorf("cpu %+v", cpu)
	}

	top := "Processes: 1076 total, 3 running\nPID    COMMAND         MEM\n1110   OrbStack Helper 1730M\n410    WindowServer    1461M+\n5892   WeChat          583K\n"
	mem := ParseProcessMemory(top, 8)
	var names []string
	for _, p := range mem {
		names = append(names, p.Name)
	}
	if !reflect.DeepEqual(names, []string{"OrbStack Helper", "WindowServer", "WeChat"}) {
		t.Errorf("memory names %v", names)
	}
	if mem[0].Value != 1730*1_048_576 || mem[2].Value != 583*1024 {
		t.Errorf("memory values %+v", mem)
	}
	if v, _ := ParseTopMemory("1.5G"); v != 1.5*1_073_741_824 {
		t.Errorf("fractional gigabytes %v", v)
	}
	if n := ParseProcessNames("  5218 ego Helper (Renderer)\n  410 WindowServer\n"); n[5218] != "ego Helper (Renderer)" {
		t.Errorf("names %v", n)
	}
}

func TestFormatBytes(t *testing.T) {
	cases := map[float64]string{
		583 * 1024:        "583 KB",
		1.4 * (1 << 20):   "1.4 MB",
		42.46 * (1 << 30): "42.46 GB",
		128 * (1 << 30):   "128 GB",
		1.5 * (1 << 30):   "1.5 GB",
		0:                 "Zero KB",
	}
	for in, want := range cases {
		if got := FormatBytes(in); got != want {
			t.Errorf("FormatBytes(%v) = %q, want %q", in, got, want)
		}
	}
}
