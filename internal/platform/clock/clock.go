// Package clock makes time injectable so business logic and jobs are testable.
package clock

import "time"

// Clock reports the current time.
type Clock interface {
	Now() time.Time
	Since(t time.Time) time.Duration
}

type systemClock struct{}

func (systemClock) Now() time.Time                  { return time.Now() }
func (systemClock) Since(t time.Time) time.Duration { return time.Since(t) }

// System returns a Clock backed by the operating system.
func System() Clock { return systemClock{} }
