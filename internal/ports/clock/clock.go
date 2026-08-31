// Package clock defines the service's injectable source of wall-clock time.
package clock

import "time"

// Clock supplies the current time. Application and domain services accept this
// interface instead of reading the process clock directly.
type Clock interface {
	Now() time.Time
}
