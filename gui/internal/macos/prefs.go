//go:build darwin

package macos

// Preferences reads and writes a preferences domain through cfprefsd, the store behind
// UserDefaults, so the Go app shares the Swift app's settings under the same bundle identifier.
type Preferences struct {
	domain string
}

// AppPreferences is the running app's own domain, as UserDefaults.standard.
func AppPreferences() Preferences {
	return Preferences{domain: MainBundleID()}
}

// DomainPreferences addresses another domain, or an absolute plist path.
func DomainPreferences(domain string) Preferences {
	return Preferences{domain: domain}
}

func (p Preferences) Domain() string { return p.domain }

// Synchronize drops what this process cached of the domain, so the next read sees what other
// processes wrote meanwhile; writing back a stale copy would undo their changes.
func (p Preferences) Synchronize() {
	if p.domain == "" || load() != nil {
		return
	}
	d := CFString(p.domain)
	defer cfRelease(d)
	cfPreferencesAppSynchronize(d)
}

// HostPreferences addresses a domain for the current user on this host only, as
// defaults -currentHost does; Control Center keeps the menu bar's checkboxes there.
type HostPreferences struct {
	domain string
}

func CurrentHostPreferences(domain string) HostPreferences {
	return HostPreferences{domain: domain}
}

// Value synchronizes first: these domains belong to system processes that write them at any time.
func (p HostPreferences) Value(key string) (any, bool) {
	if p.domain == "" || load() != nil || kCFPreferencesCurrentHost == 0 {
		return nil, false
	}
	k, d := CFString(key), CFString(p.domain)
	defer cfRelease(k)
	defer cfRelease(d)
	cfPreferencesSynchronize(d, kCFPreferencesCurrentUser, kCFPreferencesCurrentHost)
	ref := cfPreferencesCopyValue(k, d, kCFPreferencesCurrentUser, kCFPreferencesCurrentHost)
	if ref == 0 {
		return nil, false
	}
	defer cfRelease(ref)
	value, err := FromCF(ref)
	return value, err == nil
}

// Set stores value, or removes the key when value is nil, and synchronizes.
func (p HostPreferences) Set(key string, value any) error {
	if p.domain == "" {
		return nil
	}
	if err := load(); err != nil {
		return err
	}
	k, d := CFString(key), CFString(p.domain)
	defer cfRelease(k)
	defer cfRelease(d)
	var ref uintptr
	if value != nil {
		var err error
		if ref, err = ToCF(value); err != nil {
			return err
		}
		defer cfRelease(ref)
	}
	cfPreferencesSetValue(k, ref, d, kCFPreferencesCurrentUser, kCFPreferencesCurrentHost)
	cfPreferencesSynchronize(d, kCFPreferencesCurrentUser, kCFPreferencesCurrentHost)
	return nil
}

// Value returns the stored value, decoded as ParsePlist does, and whether there is one.
func (p Preferences) Value(key string) (any, bool) {
	if p.domain == "" || load() != nil {
		return nil, false
	}
	k, d := CFString(key), CFString(p.domain)
	defer cfRelease(k)
	defer cfRelease(d)
	ref := cfPreferencesCopyAppValue(k, d)
	if ref == 0 {
		return nil, false
	}
	defer cfRelease(ref)
	value, err := FromCF(ref)
	return value, err == nil
}

// Set stores value, or removes the key when value is nil, and synchronizes.
func (p Preferences) Set(key string, value any) error {
	if p.domain == "" {
		return nil
	}
	if err := load(); err != nil {
		return err
	}
	k, d := CFString(key), CFString(p.domain)
	defer cfRelease(k)
	defer cfRelease(d)
	var ref uintptr
	if value != nil {
		var err error
		if ref, err = ToCF(value); err != nil {
			return err
		}
		defer cfRelease(ref)
	}
	cfPreferencesSetAppValue(k, ref, d)
	cfPreferencesAppSynchronize(d)
	return nil
}

func (p Preferences) Bool(key string) (bool, bool) {
	v, ok := p.Value(key)
	if !ok {
		return false, false
	}
	return Bool(map[string]any{"v": v}, "v")
}

func (p Preferences) Float(key string) (float64, bool) {
	switch v, _ := p.Value(key); n := v.(type) {
	case float64:
		return n, true
	case int64:
		return float64(n), true
	}
	return 0, false
}

func (p Preferences) Int(key string) (int64, bool) {
	v, ok := p.Value(key)
	if !ok {
		return 0, false
	}
	return Int(map[string]any{"v": v}, "v")
}

func (p Preferences) String(key string) (string, bool) {
	v, _ := p.Value(key)
	s, ok := v.(string)
	return s, ok
}

// Data returns a data value, such as the JSON the Swift app stores with JSONEncoder.
func (p Preferences) Data(key string) ([]byte, bool) {
	v, _ := p.Value(key)
	b, ok := v.([]byte)
	return b, ok
}

// Keys lists the domain's keys for the current user on any host, where UserDefaults writes.
func (p Preferences) Keys() []string {
	if p.domain == "" || load() != nil || kCFPreferencesCurrentUser == 0 {
		return nil
	}
	d := CFString(p.domain)
	defer cfRelease(d)
	list := cfPreferencesCopyKeyList(d, kCFPreferencesCurrentUser, kCFPreferencesAnyHost)
	if list == 0 {
		return nil
	}
	defer cfRelease(list)
	value, err := FromCF(list)
	if err != nil {
		return nil
	}
	items, _ := value.([]any)
	keys := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			keys = append(keys, s)
		}
	}
	return keys
}
