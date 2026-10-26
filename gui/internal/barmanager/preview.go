package barmanager

// NewPreview builds a manager and store that touch no AppKit, showing snap: for page tests and
// previews of the layout editor.
func NewPreview(supported, enabled, collapsed bool, snap StoreSnapshot) (*Manager, *ItemStore) {
	// No auto-hide: a preview must not collapse itself on a timer.
	m := newManager(nullBar{}, memoryPrefs{enabledKey: enabled, autoHideDelayKey: 0.0}, func(fn func()) { fn() }, supported)
	m.installed = supported && enabled
	m.snap.Collapsed = collapsed
	s := newItemStore(m, nullItems{})
	s.snap = snap
	return m, s
}

type nullBar struct{}

func (nullBar) install()                 {}
func (nullBar) uninstall()               {}
func (nullBar) apply(bool, float64, int) {}
func (nullBar) setToggle(bool)           {}
func (nullBar) displays() []Display      { return nil }
func (nullBar) menuBarInUse() bool       { return false }

type memoryPrefs map[string]any

func (p memoryPrefs) Bool(k string) (bool, bool)     { v, ok := p[k].(bool); return v, ok }
func (p memoryPrefs) Float(k string) (float64, bool) { v, ok := p[k].(float64); return v, ok }
func (p memoryPrefs) Set(k string, v any) error      { p[k] = v; return nil }

type nullItems struct{}

func (nullItems) trusted() bool                    { return true }
func (nullItems) requestTrust() bool               { return true }
func (nullItems) runningApps() []appRef            { return nil }
func (nullItems) readItems([]int32) []rawItem      { return nil }
func (nullItems) overflowButton([]appRef) *Rect    { return nil }
func (nullItems) positions() map[string]float64    { return nil }
func (nullItems) setPosition(string, float64) bool { return false }
func (nullItems) reloadMenuBar()                   {}
func (nullItems) requestLayoutAccess() bool        { return false }
func (nullItems) commandDrag(Point, Point)         {}
func (nullItems) ownPID() int32                    { return 0 }
func (nullItems) appIcon(string) []byte            { return nil }

// SetPreviewIcon gives a preview store the logo of the app at appPath.
func (s *ItemStore) SetPreviewIcon(appPath string, png []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.icons[appPath] = png
}
