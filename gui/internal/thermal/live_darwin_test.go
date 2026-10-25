//go:build darwin

package thermal

import "testing"

// Reads this Mac's SMC as an ordinary user, as the monitor does; writes stay with the helper.
func TestReadSMCLive(t *testing.T) {
	smc, err := OpenSMC()
	if err != nil {
		t.Skip("no AppleSMC:", err)
	}
	defer smc.Close()
	if smc.KeyCount() == 0 {
		t.Fatal("no SMC keys")
	}
	keys := discoverSensorKeys(smc)
	s := readSnapshot(smc, keys)
	cpu, ok := s.Temperatures[GroupCPU]
	if !ok || !IsPlausibleTemperature(cpu.AverageC) {
		t.Errorf("CPU temperature: %+v", s.Temperatures)
	}
	for _, f := range s.Fans {
		if f.MaxRPM <= f.MinRPM {
			t.Errorf("fan range: %+v", f)
		}
	}
	t.Logf("fans %+v cpu %+v diag %v display %v other tool %v", s.Fans, cpu, s.DiagnosticUnlocked, s.DisplayPowerW, otherFanTool())
}
