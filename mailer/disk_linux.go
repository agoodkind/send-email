//go:build linux

package mailer

import (
	"fmt"
	"syscall"
)

func linuxDiskRoot() string {
	var st syscall.Statfs_t
	err := syscall.Statfs("/", &st)
	if err != nil {
		return "N/A"
	}
	total := st.Blocks * uint64(st.Bsize)
	free := st.Bavail * uint64(st.Bsize)
	used := total - free
	pct := float64(0)
	if total > 0 {
		pct = float64(used) * 100 / float64(total)
	}
	return fmt.Sprintf("%.1fGiB/%.1fGiB (%.0f%%)",
		float64(used)/(1<<30), float64(total)/(1<<30), pct)
}
