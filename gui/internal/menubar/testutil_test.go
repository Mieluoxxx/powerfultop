package menubar

func ptr[T any](v T) *T { return &v }

func sampleContext() Context {
	return Context{
		DataAvailable: true, BatteryPercent: 50, SystemPowerW: 7.4, BatteryPowerW: ptr(37.1),
		CPUUsage: ptr(0.05), MemoryUsage: ptr(0.43), CPUTemperatureC: ptr(42.6), FanRPM: ptr(0.0),
	}
}
