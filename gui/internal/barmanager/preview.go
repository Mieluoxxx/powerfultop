package barmanager

// NewPreview builds a store that touches no AppKit, showing snap: for page tests and previews of
// the layout editor.
func NewPreview(snap StoreSnapshot) *ItemStore {
	s := newItemStore(nullItems{}, snap.Supported, nil)
	s.snap = snap
	return s
}

// NewTrayPreview builds a tray store that touches no AppKit, showing snap.
func NewTrayPreview(snap TraySnapshot) *TrayStore {
	s := newTrayStore(nullItems{}, memoryPrefs{}, snap.Supported, "")
	s.snap = snap
	for _, e := range snap.Entries {
		s.members = append(s.members, e.ID)
	}
	return s
}

type nullItems struct{}

func (nullItems) trusted() bool                    { return true }
func (nullItems) requestTrust() bool               { return true }
func (nullItems) runningApps() []appRef            { return nil }
func (nullItems) readItems([]int32) []rawItem      { return nil }
func (nullItems) barAnchors([]appRef) barAnchors   { return barAnchors{} }
func (nullItems) positions() map[string]float64    { return nil }
func (nullItems) setPosition(string, float64) bool { return false }
func (nullItems) reloadMenuBar()                   {}
func (nullItems) requestLayoutAccess() bool        { return false }
func (nullItems) commandDrag(Point, Point)         {}
func (nullItems) ownPID() int32                    { return 0 }
func (nullItems) appIcon(string) []byte            { return nil }
func (nullItems) drawn() map[string]Rect           { return nil }

func (nullItems) allowList() []any                     { return nil }
func (nullItems) setAllowList([]any) bool              { return false }
func (nullItems) requestAllowListAccess() bool         { return false }
func (nullItems) checkbox(string) (int, bool)          { return 0, false }
func (nullItems) setCheckbox(string, checkboxWrite)    {}
func (nullItems) flag(flagSwitch) (bool, bool)         { return false, false }
func (nullItems) setFlag(flagSwitch, bool)             {}
func (nullItems) setLoginItem(loginItem, bool)         {}
func (nullItems) restart(string)                       {}
func (nullItems) runningApp(string) (appRef, bool)     { return appRef{}, false }
func (nullItems) appPath(string) string                { return "" }
func (nullItems) launch(string)                        {}
func (nullItems) setPositions(map[string]float64) bool { return false }
func (nullItems) extra(string) axRef                   { return 0 }
func (nullItems) onBar(string) axRef                   { return 0 }
func (nullItems) menu(axRef) []MenuEntry               { return nil }
func (nullItems) press(axRef)                          {}
func (nullItems) release(axRef)                        {}
func (nullItems) click(Point)                          {}
func (nullItems) showingUI(int32) bool                 { return false }
func (nullItems) ownBundleID() string                  { return "" }

type memoryPrefs struct{}

func (memoryPrefs) Value(string) (any, bool) { return nil, false }
func (memoryPrefs) Bool(string) (bool, bool) { return false, false }
func (memoryPrefs) Set(string, any) error    { return nil }

// SetPreviewIcon gives a preview store the logo of the app at appPath.
func (s *ItemStore) SetPreviewIcon(appPath string, png []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.icons[appPath] = png
}

// SetPreviewIcon gives a preview tray the logo of the app at appPath.
func (s *TrayStore) SetPreviewIcon(appPath string, png []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.icons[appPath] = png
}
