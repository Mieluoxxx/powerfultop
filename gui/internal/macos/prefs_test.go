//go:build darwin

package macos

import (
	"reflect"
	"testing"
)

func TestPreferencesRoundTrip(t *testing.T) {
	p := DomainPreferences("io.github.mieluoxxx.PowerfulTop.gotest")
	t.Cleanup(func() { _ = p.Set("value", nil) })
	want := map[string]any{"limit": int64(80), "enabled": true}
	if err := p.Set("value", want); err != nil {
		t.Fatal(err)
	}
	if keys := p.Keys(); len(keys) != 1 || keys[0] != "value" {
		t.Errorf("keys %v", keys)
	}
	got, ok := p.Value("value")
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v %v", got, ok)
	}
	if err := p.Set("value", nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := p.Value("value"); ok {
		t.Error("nil should remove the key")
	}
}
