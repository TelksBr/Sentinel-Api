//go:build !linux

package utils

// KillActiveProcessesForUIDs stub para sistemas não-Linux (desenvolvimento no Windows/macOS).
func KillActiveProcessesForUIDs(uids []int) int {
	return 0
}
