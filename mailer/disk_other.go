//go:build !linux

package mailer

func linuxDiskRoot() string {
	return "N/A"
}
