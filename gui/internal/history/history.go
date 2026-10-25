// Package history keeps the short in-memory history behind the dashboard charts; nothing is written to disk.
package history

import (
	"sync"
	"time"
)

const (
	Window         = 30 * time.Minute
	SampleInterval = 5 * time.Second
)

// Metric names one charted value.
type Metric string

const (
	BatteryPercent Metric = "batteryPercent"
	SystemPower    Metric = "systemPower"
	CPUTemperature Metric = "cpuTemperature"
	FanRPM         Metric = "fanRPM"
	BatteryTemp    Metric = "batteryTemperature"
	CPUUsage       Metric = "cpuUsage"
	MemoryUsage    Metric = "memoryUsage"
)

// Point is one value of a series.
type Point struct {
	Time  time.Time
	Value float64
}

// Sample is every metric known at one moment; a missing metric leaves a gap in its series.
type Sample struct {
	Time   time.Time
	Values map[Metric]float64
}

// History is safe for concurrent use.
type History struct {
	mu      sync.Mutex
	samples []Sample
}

// Append keeps samples evenly spaced and at most Window old.
func (h *History) Append(s Sample) {
	h.mu.Lock()
	defer h.mu.Unlock()
	// Ignore bursts (wake, manual refresh) so spacing stays even for the charts.
	if n := len(h.samples); n > 0 && s.Time.Sub(h.samples[n-1].Time) < SampleInterval/2 {
		return
	}
	h.samples = append(h.samples, s)
	cutoff := s.Time.Add(-Window)
	first := 0
	for first < len(h.samples) && h.samples[first].Time.Before(cutoff) {
		first++
	}
	h.samples = h.samples[first:]
}

// Series returns the metric's points in time order.
func (h *History) Series(m Metric) []Point {
	h.mu.Lock()
	defer h.mu.Unlock()
	var points []Point
	for _, s := range h.samples {
		if v, ok := s.Values[m]; ok {
			points = append(points, Point{s.Time, v})
		}
	}
	return points
}
