package mailer

import "time"

// Clock returns the current time. Inject a stub in tests.
type Clock func() time.Time

// SystemClock returns [time.Now] and is the default [Clock] used when
// [Config.Now] is left unset.
func SystemClock() time.Time { return time.Now() }
