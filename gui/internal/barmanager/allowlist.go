package barmanager

import (
	"net/url"
	"path/filepath"
)

// The "Allow in the Menu Bar" list and the system items' switches are adapted from Chest
// (https://github.com/boringbar/chest, MIT License, © 2026 BORINGBAR, LLC), which measured them
// on macOS 27.0.1.
//
// Control Center keeps the list under trackedApplications as a binary property list: an array
// alternating keys and entries,
//
//	[ {bundle: {_0: "com.example.app"}},
//	  {isAllowed: true, location: {…}, menuItemLocations: [{…}]}, … ]
//
// MenuBarAgent creates every entry and matches items by menuItemLocations; an entry written from
// scratch is ignored and replaced, so only isAllowed of an existing entry ever changes.

// allowOwner is the process an item belongs to, as the list may name it: an entry's
// menuItemLocations hold the bundle identifier of the process that draws the item, which can be a
// helper of the entry's app (WPS's cloud service, the Passwords extra), or the executable of an
// ad-hoc signed binary.
type allowOwner struct {
	bundleID   string
	executable string
}

// names reports whether the entry under key is the one for o.
func (o allowOwner) names(key any, entry map[string]any) bool {
	if o.bundleID != "" && allowKeyBundle(key) == o.bundleID {
		return true
	}
	locations, _ := entry["menuItemLocations"].([]any)
	for _, l := range locations {
		loc, _ := l.(map[string]any)
		if bundle, _ := loc["bundle"].(map[string]any); bundle != nil {
			if id, _ := bundle["_0"].(string); id != "" && id == o.bundleID {
				return true
			}
		}
		if adhoc, _ := loc["adhocBinary"].(map[string]any); adhoc != nil && o.executable != "" {
			inner, _ := adhoc["_0"].(map[string]any)
			if raw, _ := inner["relative"].(string); raw != "" {
				if u, err := url.Parse(raw); err == nil && filepath.Clean(u.Path) == filepath.Clean(o.executable) {
					return true
				}
			}
		}
	}
	return false
}

// allowedState reads o's switch: allowed, and whether MenuBarAgent has listed it at all.
func allowedState(list []any, o allowOwner) (allowed, listed bool) {
	for i := 0; i+1 < len(list); i += 2 {
		entry, ok := list[i+1].(map[string]any)
		if !ok || !o.names(list[i], entry) {
			continue
		}
		on, set := entry["isAllowed"].(bool)
		return on || !set, true
	}
	return false, false
}

// settingAllowed is list with isAllowed set for every entry of owners; list itself is left alone.
// missing are the bundle identifiers of owners MenuBarAgent hasn't listed yet.
func settingAllowed(list []any, owners []allowOwner, allowed bool) (out []any, changed bool, missing []string) {
	found := make([]bool, len(owners))
	out = append([]any(nil), list...)
	for i := 0; i+1 < len(out); i += 2 {
		entry, ok := out[i+1].(map[string]any)
		if !ok {
			continue
		}
		match := false
		for j, o := range owners {
			if o.names(out[i], entry) {
				found[j], match = true, true
			}
		}
		if !match {
			continue
		}
		if current, set := entry["isAllowed"].(bool); set && current == allowed {
			continue
		}
		copied := make(map[string]any, len(entry))
		for k, v := range entry {
			copied[k] = v
		}
		copied["isAllowed"] = allowed
		out[i+1] = copied
		changed = true
	}
	for j, o := range owners {
		if !found[j] {
			missing = append(missing, o.bundleID)
		}
	}
	return out, changed, missing
}

func allowKeyBundle(key any) string {
	k, _ := key.(map[string]any)
	bundle, _ := k["bundle"].(map[string]any)
	id, _ := bundle["_0"].(string)
	return id
}
