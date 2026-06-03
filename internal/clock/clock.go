// Package clock owns the single process-wide [time.Now] call so callers can
// inject a clock for testability instead of reading the wall clock directly.
package clock

import "time"

// Now returns the current local time.
func Now() time.Time { return time.Now() }
