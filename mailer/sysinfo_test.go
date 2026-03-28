package mailer

import (
	"context"
	"testing"
)

func TestCollectSysInfo_nonEmptyHostname(t *testing.T) {
	t.Parallel()
	si := CollectSysInfo(context.Background())
	if si.Hostname == "" {
		t.Fatal("expected hostname")
	}
}
