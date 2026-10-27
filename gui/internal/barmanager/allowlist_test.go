package barmanager

import (
	"reflect"
	"testing"
)

func allowKey(id string) map[string]any {
	return map[string]any{"bundle": map[string]any{"_0": id}}
}

func allowValue(id string, allowed bool) map[string]any {
	return map[string]any{
		"isAllowed":         allowed,
		"location":          allowKey(id),
		"menuItemLocations": []any{allowKey(id)},
	}
}

func owners(ids ...string) []allowOwner {
	out := make([]allowOwner, len(ids))
	for i, id := range ids {
		out[i] = allowOwner{bundleID: id}
	}
	return out
}

func TestAllowedStateMatchesTheItemsProcess(t *testing.T) {
	list := []any{
		allowKey("a"), allowValue("a", true),
		// WPS lists its app, but its item belongs to the cloud service helper.
		allowKey("com.kingsoft.wpsoffice.mac"), map[string]any{
			"isAllowed": false, "location": allowKey("com.kingsoft.wpsoffice.mac"),
			"menuItemLocations": []any{allowKey("cn.wps.wpscloudsvr")},
		},
		// An ad-hoc signed app goes by its executable's file URL.
		allowKey("ing.miel.typingpet"), map[string]any{
			"isAllowed": true,
			"menuItemLocations": []any{map[string]any{"adhocBinary": map[string]any{"_0": map[string]any{
				"relative": "file:///Users/me/dist/Typing%20Pet.app/Contents/MacOS/TypingPet"}}}},
		},
		allowKey("c"), map[string]any{"location": allowKey("c")},
	}
	cases := []struct {
		owner           allowOwner
		allowed, listed bool
	}{
		{allowOwner{bundleID: "a"}, true, true},
		{allowOwner{bundleID: "cn.wps.wpscloudsvr"}, false, true},
		{allowOwner{bundleID: "x.typingpet", executable: "/Users/me/dist/Typing Pet.app/Contents/MacOS/TypingPet"}, true, true},
		{allowOwner{bundleID: "x.typingpet", executable: "/elsewhere/TypingPet"}, false, false},
		// isAllowed missing means allowed, MenuBarAgent's default.
		{allowOwner{bundleID: "c"}, true, true},
		{allowOwner{bundleID: "new"}, false, false},
	}
	for _, tc := range cases {
		if allowed, listed := allowedState(list, tc.owner); allowed != tc.allowed || listed != tc.listed {
			t.Errorf("%+v: allowed %v listed %v", tc.owner, allowed, listed)
		}
	}
	out, changed, missing := settingAllowed(list, owners("cn.wps.wpscloudsvr"), true)
	if !changed || len(missing) > 0 {
		t.Fatalf("changed %v missing %v", changed, missing)
	}
	if allowed, _ := allowedState(out, allowOwner{bundleID: "cn.wps.wpscloudsvr"}); !allowed {
		t.Error("WPS's entry should be switched through its helper")
	}
}

func TestSettingAllowedKeepsEveryOtherField(t *testing.T) {
	list := []any{allowKey("a"), allowValue("a", true), allowKey("b"), allowValue("b", true)}
	out, changed, missing := settingAllowed(list, owners("b", "new"), false)
	if !changed || !reflect.DeepEqual(missing, []string{"new"}) {
		t.Fatalf("changed %v missing %v", changed, missing)
	}
	if want := []any{allowKey("a"), allowValue("a", true), allowKey("b"), allowValue("b", false)}; !reflect.DeepEqual(out, want) {
		t.Errorf("got %v", out)
	}
	if list[3].(map[string]any)["isAllowed"] != true {
		t.Error("the list read must not change: it may be written back if the write fails")
	}
	if _, changed, _ := settingAllowed(out, owners("b"), false); changed {
		t.Error("already switched off")
	}
}
