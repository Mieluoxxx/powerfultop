package barmanager

// NewPreview builds a store that touches no AppKit, showing snap: for page tests and previews of
// the layout editor.
func NewPreview(snap StoreSnapshot) *ItemStore {
	s := newItemStore(nullItems{}, snap.Supported, "")
	s.snap = snap
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

// SetPreviewIcon gives a preview store the logo of the app at appPath.
func (s *ItemStore) SetPreviewIcon(appPath string, png []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.icons[appPath] = png
}
