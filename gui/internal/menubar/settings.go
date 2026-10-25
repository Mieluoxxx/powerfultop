//go:build darwin

package menubar

import (
	"sync"

	"powerfultop/internal/macos"
)

// Keys of the Swift app's UserDefaults, read from the same domain so settings carry over.
const (
	configurationKey         = "menuBarConfiguration"
	legacyDisplayStyleKey    = "menuBarDisplayStyle"
	legacyShowPowerInMenuBar = "showPowerInMenuBar"
)

// Settings holds the configuration, persisted as the Swift app did. Safe for concurrent use.
type Settings struct {
	prefs     macos.Preferences
	mu        sync.Mutex
	cfg       Configuration
	listeners []func()
}

// NewSettings loads the configuration from prefs, usually macos.AppPreferences().
func NewSettings(prefs macos.Preferences) *Settings {
	s := &Settings{prefs: prefs}
	s.Load()
	return s
}

// Load rereads the stored configuration, falling back to the pre-catalog settings.
func (s *Settings) Load() {
	cfg := load(s.prefs)
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
}

func load(prefs macos.Preferences) Configuration {
	if data, ok := prefs.Data(configurationKey); ok {
		if cfg, err := DecodeConfiguration(data); err == nil {
			return cfg
		}
	}
	// Earlier builds stored one of three fixed styles, and before that a single power toggle.
	if raw, ok := prefs.String(legacyDisplayStyleKey); ok {
		switch style := LegacyStyle(raw); style {
		case LegacyBatteryPercent, LegacyBatteryPercentPower, LegacyPowerOnly:
			return FromLegacy(style)
		}
	}
	if on, _ := prefs.Bool(legacyShowPowerInMenuBar); on {
		return FromLegacy(LegacyBatteryPercentPower)
	}
	return FromLegacy(LegacyBatteryPercent)
}

// Save writes the configuration as the Swift app's JSONEncoder data.
func (s *Settings) Save() error {
	data, err := s.Configuration().Encode()
	if err != nil {
		return err
	}
	return s.prefs.Set(configurationKey, data)
}

// Configuration returns a copy of the current configuration.
func (s *Settings) Configuration() Configuration {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg := s.cfg
	cfg.Items = append([]Item(nil), s.cfg.Items...)
	return cfg
}

// Update changes the configuration, saves it and notifies listeners when it changed.
func (s *Settings) Update(change func(*Configuration)) {
	s.mu.Lock()
	next := s.cfg
	next.Items = append([]Item(nil), s.cfg.Items...)
	change(&next)
	if next.Equal(s.cfg) {
		s.mu.Unlock()
		return
	}
	s.cfg = next
	listeners := append([]func(){}, s.listeners...)
	s.mu.Unlock()
	_ = s.Save()
	for _, fn := range listeners {
		fn()
	}
}

// Set replaces the configuration, as Reset does.
func (s *Settings) Set(cfg Configuration) {
	s.Update(func(c *Configuration) { *c = cfg })
}

// OnChange registers fn, called on the goroutine that changed the configuration, so the app can
// redraw the tray and reschedule its refresh at the new interval.
func (s *Settings) OnChange(fn func()) {
	s.mu.Lock()
	s.listeners = append(s.listeners, fn)
	s.mu.Unlock()
}
