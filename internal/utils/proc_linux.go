//go:build linux

package utils

import (
	"os"
	"strconv"
	"syscall"
)

// KillActiveProcessesForUIDs varre /proc em Go puro e envia SIGKILL direto via syscall apenas para PIDs ativos.
// Zero forks de subprocessos, zero comandos pkill, consumo de CPU imperceptível.
func KillActiveProcessesForUIDs(uids []int) int {
	if len(uids) == 0 {
		return 0
	}

	targetMap := make(map[int]bool, len(uids))
	for _, u := range uids {
		if u >= 0 {
			targetMap[u] = true
		}
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
		if err != nil {
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
