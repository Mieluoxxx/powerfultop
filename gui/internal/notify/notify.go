// Package notify is the change notification behind every monitor's and store's OnChange.
package notify

import "sync"

// Listeners is safe for concurrent use. Notify calls them outside its lock, so a listener may
// register another one, and an owner must release its own lock first, as listeners read it back.
type Listeners struct {
	mu  sync.Mutex
	fns []func()
}

func (l *Listeners) Add(fn func()) {
	l.mu.Lock()
	l.fns = append(l.fns, fn)
	l.mu.Unlock()
}

func (l *Listeners) Notify() {
	l.mu.Lock()
	fns := append([]func(){}, l.fns...)
	l.mu.Unlock()
	for _, fn := range fns {
		fn()
	}
}
