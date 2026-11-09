//go:build linux

package mailer

import (
	"fmt"
	"syscall"
)

func linuxDiskRoot() string {
	var filesystem syscall.Statfs_t
	err := syscall.Statfs("/", &filesystem)
	if err != nil {
		return notAvailable
	}

	if filesystem.Bsize < 0 {
		return notAvailable
	}

	const percentage = 100
	const bytesPerGiB = 1024 * 1024 * 1024

	blockSize := uint64(filesystem.Bsize)
	total := filesystem.Blocks * blockSize
	free := filesystem.Bavail * blockSize
	used := total - free
	percent := float64(0)
	if total > 0 {
		percent = float64(used) * percentage / float64(total)
	}

	return fmt.Sprintf(
		"%.1fGiB/%.1fGiB (%.0f%%)",
		float64(used)/bytesPerGiB,
		float64(total)/bytesPerGiB,
		percent,
	)
}
