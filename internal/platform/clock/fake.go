package clock

import (
	"sync"
	"time"
)

// Fake is a Clock whose time only moves when the test moves it.
type Fake struct {
	mu  sync.Mutex
	now time.Time
}

// NewFake returns a Fake positioned at t.
func NewFake(t time.Time) *Fake { return &Fake{now: t} }

// Now returns the clock's current, test-controlled time.
func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// Since returns the time elapsed since t, measured against the clock's
// current, test-controlled time.
func (f *Fake) Since(t time.Time) time.Duration { return f.Now().Sub(t) }

// Advance moves the clock forward.
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}

// Set positions the clock at t.
func (f *Fake) Set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = t
}
