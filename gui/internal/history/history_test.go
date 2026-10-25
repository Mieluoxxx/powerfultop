package history

import (
	"testing"
	"time"
)

// Mirrors testMetricHistoryWindow in the Swift regression tests.
func TestWindowAndBursts(t *testing.T) {
	var h History
	start := time.Unix(0, 0)
	for i := 0; i <= 400; i++ {
		h.Append(Sample{Time: start.Add(time.Duration(i) * SampleInterval), Values: map[Metric]float64{SystemPower: float64(i)}})
	}
	h.Append(Sample{Time: start.Add(400*SampleInterval + time.Second), Values: map[Metric]float64{SystemPower: -1}})
	points := h.Series(SystemPower)
	last := points[len(points)-1]
	if last.Value != 400 {
		t.Errorf("a burst sample must be ignored, last = %v", last.Value)
	}
	if span := last.Time.Sub(points[0].Time); span > Window {
		t.Errorf("history spans %v, more than the window", span)
	}
	if len(h.Series(FanRPM)) != 0 {
		t.Error("a metric never recorded has no points")
	}
}
