package mailer

import (
	"time"

	"github.com/agoodkind/send-email/internal/clock"
)

// Clock returns the current time. Inject a stub in tests.
type Clock func() time.Time

// SystemClock returns the current time via [clock.Now] and is the default
// [Clock] used when [Config.Now] is left unset.
func SystemClock() time.Time { return clock.Now() }
