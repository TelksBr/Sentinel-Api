//go:build !linux

package utils

// ProcessKillDisabled pode ser ativado em testes unitários para não matar processos do host.
var ProcessKillDisabled = false

// KillActiveProcessesForUIDs stub para sistemas não-Linux (desenvolvimento no Windows/macOS).
func KillActiveProcessesForUIDs(uids []int) int {
	return 0
}
