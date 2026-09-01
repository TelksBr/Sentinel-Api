//go:build linux

package utils

import (
	"os"
	"strconv"
	"syscall"
)

// ProcessKillDisabled pode ser ativado em testes unitários para não matar processos do host.
var ProcessKillDisabled = false

// KillActiveProcessesForUIDs varre /proc em Go puro e envia SIGKILL direto via syscall apenas para PIDs ativos.
// Zero forks de subprocessos, zero comandos pkill, consumo de CPU imperceptível.
func KillActiveProcessesForUIDs(uids []int) int {
	if len(uids) == 0 || ProcessKillDisabled {
		return 0
	}

	currentPID := os.Getpid()
	currentPPID := os.Getppid()
	currentUID := os.Getuid()

	targetMap := make(map[int]bool, len(uids))
	for _, u := range uids {
		// Proteção vital: nunca matar processos do root (UID 0) nem do próprio usuário executando o processo
		if u > 0 && u != currentUID {
			targetMap[u] = true
		}
	}

	if len(targetMap) == 0 {
		return 0
	}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}

	killed := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 1 || pid == currentPID || pid == currentPPID {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			uid := int(stat.Uid)
			if targetMap[uid] {
				// Envia sinal SIGKILL instantâneo diretamente pelo kernel Linux
				if err := syscall.Kill(pid, syscall.SIGKILL); err == nil {
					killed++
				}
			}
		}
	}

	return killed
}
