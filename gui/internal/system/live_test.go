//go:build darwin

package system

import (
	"testing"
	"time"
)

// Reads this Mac through purego, so a broken binding fails here rather than in the app.
func TestMonitorLive(t *testing.T) {
	if len(readTicks()) == 0 {
		t.Skip("host_processor_info unavailable")
	}
	m := NewMonitor()
	m.Start()
	defer m.Stop()
	time.Sleep(300 * time.Millisecond)
	m.refresh()
	s := m.Snapshot()
	if len(s.CPU.PerCore) == 0 || s.CPU.Usage() < 0 || s.CPU.Usage() > 1 {
		t.Errorf("cpu %+v", s.CPU)
	}
	if s.Memory == nil || s.Memory.TotalBytes == 0 || s.Memory.UsedFraction() <= 0 || s.Memory.UsedFraction() > 1 {
		t.Errorf("memory %+v", s.Memory)
	}
	if len(s.LoadAverage) != 3 || s.BootTime.IsZero() || s.BootTime.After(time.Now()) {
		t.Errorf("load %v boot %v", s.LoadAverage, s.BootTime)
	}
	if len(s.CoreKinds) != len(s.CPU.PerCore) {
		t.Logf("core kinds %d for %d cores", len(s.CoreKinds), len(s.CPU.PerCore))
	}
	m.sampleProcesses(true)
	s = m.Snapshot()
	if len(s.TopCPU) == 0 || len(s.TopMemory) == 0 {
		t.Errorf("process lists cpu=%d mem=%d", len(s.TopCPU), len(s.TopMemory))
	}
	t.Logf("cpu %.0f%% cores %d kinds %v mem %s/%s pressure %d load %v top %q / %q", s.CPU.Usage()*100, len(s.CPU.PerCore), s.CoreKinds,
		FormatBytes(float64(s.Memory.UsedBytes())), FormatBytes(float64(s.Memory.TotalBytes)), s.Memory.Pressure, s.LoadAverage, s.TopCPU[0].Name, s.TopMemory[0].Name)
}
