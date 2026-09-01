package utils

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// FindSystemdServiceFile localiza o arquivo .service do systemd para um serviço
// Prioriza `systemctl show -p FragmentPath`, depois `systemctl status` e por fim caminhos padrão no filesystem.
func FindSystemdServiceFile(serviceName string) string {
	// 1. Tentar extrair via `systemctl show -p FragmentPath <service>`
	if out, err := ExecuteCommand("systemctl", "show", "-p", "FragmentPath", serviceName); err == nil {
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "FragmentPath=") {
				path := strings.TrimPrefix(line, "FragmentPath=")
				if path != "" && path != "/dev/null" {
					if _, err := os.Stat(path); err == nil {
						return filepath.Clean(path)
					}
				}
			}
		}
	}

	// 2. Tentar extrair da saída de `systemctl status <service>`
	// Formato típico do systemd: "Loaded: loaded (/etc/systemd/system/xray.service; enabled; vendor preset: enabled)"
	if out, err := ExecuteCommand("systemctl", "status", serviceName); err == nil {
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Loaded:") {
				start := strings.Index(line, "(")
				end := strings.Index(line, ";")
				if start != -1 && end != -1 && end > start+1 {
					path := strings.TrimSpace(line[start+1 : end])
					if _, err := os.Stat(path); err == nil {
						return filepath.Clean(path)
					}
				}
			}
		}
	}

	// 3. Fallback para caminhos comuns no Linux
	candidates := []string{
		fmt.Sprintf("/etc/systemd/system/%s.service", serviceName),
		fmt.Sprintf("/lib/systemd/system/%s.service", serviceName),
		fmt.Sprintf("/usr/lib/systemd/system/%s.service", serviceName),
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return filepath.Clean(path)
		}
	}

	return ""
}

// ConfigureSystemdNullLogs insere ou atualiza StandardOutput=null e StandardError=null na seção [Service]
func ConfigureSystemdNullLogs(content string) (newContent string, modified bool) {
	lines := strings.Split(content, "\n")
	hasServiceSection := false
	inServiceSection := false
	serviceHeaderIndex := -1

	hasStdOut := false
	hasStdErr := false
	stdOutIndex := -1
	stdErrIndex := -1

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			if strings.EqualFold(trimmed, "[Service]") {
				inServiceSection = true
				hasServiceSection = true
				serviceHeaderIndex = i
			} else {
				inServiceSection = false
			}
			continue
		}

		if inServiceSection {
			if strings.HasPrefix(trimmed, "StandardOutput=") {
				hasStdOut = true
				stdOutIndex = i
			} else if strings.HasPrefix(trimmed, "StandardError=") {
				hasStdErr = true
				stdErrIndex = i
			}
		}
	}

	if !hasServiceSection {
		return content, false
	}

	// Verificar se já está perfeitamente configurado
	if hasStdOut && hasStdErr {
		if strings.TrimSpace(lines[stdOutIndex]) == "StandardOutput=null" &&
			strings.TrimSpace(lines[stdErrIndex]) == "StandardError=null" {
			return content, false // Nenhuma alteração necessária
		}
	}

	var newLines []string
	for i, line := range lines {
		if i == stdOutIndex {
			newLines = append(newLines, "StandardOutput=null")
			continue
		}
		if i == stdErrIndex {
			newLines = append(newLines, "StandardError=null")
			continue
		}

		newLines = append(newLines, line)

		// Se não tinha StandardOutput ou StandardError, inserir logo após o cabeçalho [Service]
		if i == serviceHeaderIndex {
			if !hasStdOut {
				newLines = append(newLines, "StandardOutput=null")
			}
			if !hasStdErr {
				newLines = append(newLines, "StandardError=null")
			}
		}
	}

	return strings.Join(newLines, "\n"), true
}

// EnsureSystemdServiceLogsDisabled garante que os serviços informados não enviem stdout/stderr para o systemd
func EnsureSystemdServiceLogsDisabled(serviceNames ...string) (int, error) {
	if len(serviceNames) == 0 {
		serviceNames = []string{"xray", "v2ray"}
	}

	modifiedCount := 0
	var reloadRequired bool

	for _, name := range serviceNames {
		serviceFile := FindSystemdServiceFile(name)
		if serviceFile == "" {
			continue
		}

		contentBytes, err := os.ReadFile(serviceFile)
		if err != nil {
			log.Printf("⚠️ Não foi possível ler arquivo de serviço %s: %v", serviceFile, err)
			continue
		}

		newContent, modified := ConfigureSystemdNullLogs(string(contentBytes))
		if !modified {
			continue
		}

		log.Printf("⚙️ Atualizando %s com StandardOutput=null e StandardError=null...", serviceFile)
		if err := os.WriteFile(serviceFile, []byte(newContent), 0644); err != nil {
			log.Printf("❌ Erro ao salvar %s: %v", serviceFile, err)
			continue
		}

		modifiedCount++
		reloadRequired = true
		log.Printf("✅ %s configurado com sucesso (StandardOutput=null / StandardError=null).", serviceFile)

		// Reiniciar o serviço específico para aplicar imediatamente
		_ = ExecuteCommandQuiet("systemctl", "daemon-reload")
		_ = ExecuteCommandQuiet("systemctl", "restart", name)
		log.Printf("🔄 Serviço %s reiniciado via systemd.", name)
	}

	if reloadRequired {
		_ = ExecuteCommandQuiet("systemctl", "daemon-reload")
	}

	return modifiedCount, nil
}
