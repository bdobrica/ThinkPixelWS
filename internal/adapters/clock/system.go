// Package clock provides clock adapters.
package clock

import "time"

// System reads the process wall clock and normalizes it to UTC.
type System struct{}

func (System) Now() time.Time { return time.Now().UTC() }
