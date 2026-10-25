//go:build darwin

package display

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"powerfultop/internal/macos"
)

// ddcRetryInterval: DDC/CI can be switched on in the monitor's own menu without any display
// change, so failed probes are retried, but rarely: each probe costs about half a second of I²C retries.
const ddcRetryInterval = 60 * time.Second

// Controller tracks every online display. Start, SetLevel, SetActive and Shutdown are meant for the
// main thread; DDC probes and writes run on their own goroutines.
type Controller struct {
	mu          sync.Mutex
	displays    []Brightness
	links       map[uint32]*ddcLink
	ddcFailedAt map[uint32]time.Time
	probing     map[uint32]bool
	baseline    map[uint32]*gammaTable
	listeners   []func()
	started     bool
	active      bool
	pollStop    chan struct{}
	observer    func()
	prefs       macos.Preferences
}

func NewController() *Controller {
	return &Controller{
		links:       map[uint32]*ddcLink{},
		ddcFailedAt: map[uint32]time.Time{},
		probing:     map[uint32]bool{},
		baseline:    map[uint32]*gammaTable{},
		prefs:       macos.AppPreferences(),
	}
}

// Start reads the displays and follows display changes.
func (d *Controller) Start() {
	d.mu.Lock()
	if d.started {
		d.mu.Unlock()
		return
	}
	d.started = true
	d.mu.Unlock()
	d.observer = observeScreenChanges(d.displaysChanged)
	d.refresh()
}

// Shutdown restores the gamma tables; the system restores ColorSync when the process exits, and
// this covers the gap before that.
func (d *Controller) Shutdown() {
	d.SetActive(false)
	if d.observer != nil {
		d.observer()
		d.observer = nil
	}
	d.mu.Lock()
	dimmed := len(d.baseline) > 0
	d.baseline = map[uint32]*gammaTable{}
	d.started = false
	d.mu.Unlock()
	if dimmed {
		restoreColorSync()
	}
}

func (d *Controller) Snapshot() []Brightness {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]Brightness(nil), d.displays...)
}

func (d *Controller) OnChange(fn func()) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.listeners = append(d.listeners, fn)
}

func (d *Controller) changed() {
	d.mu.Lock()
	listeners := append([]func(){}, d.listeners...)
	d.mu.Unlock()
	for _, fn := range listeners {
		fn()
	}
}

// SetActive polls native brightness while the controls are on screen: it also moves with the keyboard keys.
func (d *Controller) SetActive(active bool) {
	d.mu.Lock()
	if active == d.active {
		d.mu.Unlock()
		return
	}
	d.active = active
	if !active {
		if d.pollStop != nil {
			close(d.pollStop)
			d.pollStop = nil
		}
		d.mu.Unlock()
		return
	}
	stop := make(chan struct{})
	d.pollStop = stop
	d.mu.Unlock()

	d.refresh()
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				d.pollNative()
			}
		}
	}()
}

// SetLevel sets a display's brightness, 0…1.
func (d *Controller) SetLevel(id uint32, level float64) {
	level = math.Min(math.Max(level, 0), 1)
	d.mu.Lock()
	index := d.indexOf(id)
	if index < 0 {
		d.mu.Unlock()
		return
	}
	d.displays[index].Level = level
	method := d.displays[index].Method
	link := d.links[id]
	d.mu.Unlock()

	switch method {
	case Native:
		setNativeBrightness(id, level)
	case DDC:
		if link != nil {
			link.send(level)
		}
	case Software:
		d.applySoftware(level, id)
		_ = d.prefs.Set(softwareKey(id), level)
	}
	d.changed()
}

func (d *Controller) indexOf(id uint32) int {
	for i, display := range d.displays {
		if display.ID == id {
			return i
		}
	}
	return -1
}

func (d *Controller) displaysChanged() {
	d.mu.Lock()
	for _, link := range d.links {
		link.release()
	}
	d.links = map[uint32]*ddcLink{}
	d.ddcFailedAt = map[uint32]time.Time{}
	d.baseline = map[uint32]*gammaTable{}
	d.mu.Unlock()
	d.refresh()
}

func (d *Controller) refresh() {
	names := screenNames()
	var displays []Brightness
	var probe []uint32
	for _, id := range onlineDisplays() {
		b := Brightness{ID: id, Name: names[id], BuiltIn: isBuiltIn(id)}
		d.mu.Lock()
		link := d.links[id]
		failedAt, failed := d.ddcFailedAt[id]
		d.mu.Unlock()
		if level, ok := nativeBrightness(id); ok {
			b.Method, b.Level = Native, level
		} else if link != nil {
			b.Method, b.Level = DDC, link.currentLevel()
		} else {
			saved, ok := d.prefs.Float(softwareKey(id))
			if !ok {
				saved = 1
			}
			if saved < 1 {
				d.applySoftware(saved, id)
			}
			if !failed || time.Since(failedAt) >= ddcRetryInterval {
				probe = append(probe, id)
			}
			b.Method, b.Level = Software, saved
		}
		displays = append(displays, b)
	}
	d.mu.Lock()
	d.displays = displays
	d.mu.Unlock()
	for _, id := range probe {
		d.probeDDC(id)
	}
	d.changed()
}

func (d *Controller) probeDDC(id uint32) {
	d.mu.Lock()
	if d.probing[id] {
		d.mu.Unlock()
		return
	}
	d.probing[id] = true
	claimed := map[int]bool{}
	for _, link := range d.links {
		claimed[link.location] = true
	}
	name := ""
	if i := d.indexOf(id); i >= 0 {
		name = d.displays[i].Name
	}
	d.mu.Unlock()

	prefix := edidUUIDPrefix(vendorNumber(id), modelNumber(id))
	go func() {
		link := connectDDC(prefix, name, claimed)
		d.mu.Lock()
		delete(d.probing, id)
		index := d.indexOf(id)
		if index < 0 {
			d.mu.Unlock()
			if link != nil {
				link.release()
			}
			return
		}
		if link == nil {
			d.ddcFailedAt[id] = time.Now()
			d.mu.Unlock()
			return
		}
		dimmed := d.displays[index].Level < 1
		d.links[id] = link
		d.displays[index].Method = DDC
		d.displays[index].Level = link.currentLevel()
		d.mu.Unlock()
		// Hardware dimming replaces software dimming rather than stacking on it.
		if dimmed {
			d.restoreGamma(id)
		}
		d.changed()
	}()
}

func (d *Controller) pollNative() {
	changed := false
	d.mu.Lock()
	for i := range d.displays {
		if d.displays[i].Method != Native {
			continue
		}
		if level, ok := nativeBrightness(d.displays[i].ID); ok && math.Abs(level-d.displays[i].Level) > 0.001 {
			d.displays[i].Level = level
			changed = true
		}
	}
	d.mu.Unlock()
	if changed {
		d.changed()
	}
}

func softwareKey(id uint32) string {
	return fmt.Sprintf("softwareBrightness.%d-%d-%d", vendorNumber(id), modelNumber(id), serialNumber(id))
}

// applySoftware scales the display's own calibrated table so colour calibration survives dimming.
func (d *Controller) applySoftware(level float64, id uint32) {
	d.mu.Lock()
	baseline := d.baseline[id]
	if baseline == nil {
		if baseline = readGamma(id); baseline != nil {
			d.baseline[id] = baseline
		}
	}
	d.mu.Unlock()
	if baseline != nil {
		baseline.scaled(gammaCeiling(level)).apply(id)
	}
}

func (d *Controller) restoreGamma(id uint32) {
	d.mu.Lock()
	baseline := d.baseline[id]
	delete(d.baseline, id)
	d.mu.Unlock()
	if baseline != nil {
		baseline.apply(id)
	}
}

func onlineDisplays() []uint32 {
	ids := onlineDisplayList()
	var shown []uint32
	for _, id := range ids {
		// Mirror targets share the source's picture; one slider per visible picture is enough.
		if mirrorsDisplay(id) == 0 {
			shown = append(shown, id)
		}
	}
	sort.SliceStable(shown, func(i, j int) bool { return isBuiltIn(shown[i]) && !isBuiltIn(shown[j]) })
	return shown
}
