package notify

import "testing"

func TestListenersMayRegisterWhileNotified(t *testing.T) {
	var l Listeners
	calls := 0
	l.Add(func() {
		calls++
		l.Add(func() { calls += 10 })
	})
	l.Notify()
	if calls != 1 {
		t.Fatalf("first notify: %d calls, want only the listener registered before it", calls)
	}
	l.Notify()
	if calls != 12 {
		t.Fatalf("second notify: calls = %d, want 12", calls)
	}
}
