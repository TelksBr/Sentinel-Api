package utils

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

var (
	xrayOwnerOnce sync.Once
	xrayOwnerUser string
	xrayOwnerGrp  string
)

// DetectXrayRuntimeOwner devolve o User/Group com que o Xray/V2Ray corre (systemd).
// Fallback: nobody + primeiro grupo existente entre nogroup, nobody e o próprio user.
func DetectXrayRuntimeOwner() (user, group string) {
	xrayOwnerOnce.Do(func() {
		xrayOwnerUser, xrayOwnerGrp = detectXrayRuntimeOwner()
	})
	return xrayOwnerUser, xrayOwnerGrp
}

func detectXrayRuntimeOwner() (string, string) {
	user, group := "", ""
	for _, svc := range []string{"xray", "v2ray"} {
		out, err := ExecuteCommand("systemctl", "show", svc, "-p", "User", "-p", "Group", "-p", "LoadState")
		if err != nil {
			continue
		}
		loaded := false
		svcUser, svcGroup := "", ""
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			switch {
			case strings.HasPrefix(line, "LoadState="):
				loaded = strings.TrimPrefix(line, "LoadState=") == "loaded"
			case strings.HasPrefix(line, "User="):
				svcUser = strings.TrimSpace(strings.TrimPrefix(line, "User="))
			case strings.HasPrefix(line, "Group="):
				svcGroup = strings.TrimSpace(strings.TrimPrefix(line, "Group="))
			}
		}
		if !loaded {
			continue
		}
		if svcUser != "" {
			user = svcUser
		}
		if svcGroup != "" {
			group = svcGroup
		}
		if user != "" {
			break
		}
	}

	if user == "" {
		user = "nobody"
	}
	if group == "" {
		for _, candidate := range []string{"nogroup", "nobody", user} {
			if unixGroupExists(candidate) {
				group = candidate
				break
			}
		}
	}
	if group == "" {
		group = user
	}
	return user, group
}

func unixGroupExists(name string) bool {
	if name == "" {
		return false
	}
	if err := ExecuteCommandQuiet("getent", "group", name); err == nil {
		return true
	}
	data, err := os.ReadFile("/etc/group")
	if err != nil {
		return false
	}
	prefix := name + ":"
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

func isDedicatedXrayLogDir(dir string) bool {
	dir = filepath.Clean(dir)
	base := strings.ToLower(filepath.Base(dir))
	if base == "log" || base == "var" || dir == "/" || dir == "/var/log" {
		return false
	}
	switch base {
	case "xray", "v2ray":
		return true
	}
	lower := strings.ToLower(filepath.ToSlash(dir))
	return strings.Contains(lower, "/log/xray") || strings.Contains(lower, "/log/v2ray")
}

func applyXrayLogOwnership(path string) {
	user, group := DetectXrayRuntimeOwner()
	owner := user + ":" + group
	_ = os.Chmod(path, 0666)
	_ = ExecuteCommandQuiet("chmod", "666", path)
	_ = ExecuteCommandQuiet("chown", owner, path)

	dir := filepath.Dir(path)
	if isDedicatedXrayLogDir(dir) {
		_ = os.Chmod(dir, 0775)
		_ = ExecuteCommandQuiet("chmod", "775", dir)
		_ = ExecuteCommandQuiet("chown", owner, dir)
	}
}

// ReplaceLogFileAtomic substitui dest pelo tmp aplicando dono/permissões do Xray no tmp
// ANTES do rename, para não haver janela em que o ficheiro fica root:644.
func ReplaceLogFileAtomic(dest, tmp string) error {
	if dest == "" || tmp == "" {
		return fmt.Errorf("caminhos de log inválidos")
	}
	if _, err := os.Stat(dest); err == nil {
		_ = ExecuteCommandQuiet("chown", "--reference="+dest, tmp)
	}
	applyXrayLogOwnership(tmp)
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	applyXrayLogOwnership(dest)
	return nil
}

// EnsureCommonXrayLogPaths garante pasta/ficheiro/dono em todos os access.log habituais
// e nos caminhos extra (ex.: o path do config.json).
func EnsureCommonXrayLogPaths(extra ...string) {
	paths := []string{
		"/var/log/xray/access.log",
		"/var/log/v2ray/access.log",
		"/usr/local/etc/xray/access.log",
		"/etc/xray/access.log",
		"/usr/local/var/log/xray/access.log",
	}
	paths = append(paths, extra...)
	seen := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" || p == "none" {
			continue
		}
		p = filepath.Clean(p)
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		_ = EnsureLogFileAccessible(p)
	}
}

// EnsureXrayLogrotateSafe escreve um logrotate que recria o access.log com o user do Xray,
// evitando create 0640 root que volta a causar permission denied.
func EnsureXrayLogrotateSafe() {
	user, group := DetectXrayRuntimeOwner()
	content := fmt.Sprintf(`/var/log/xray/*.log
/var/log/v2ray/*.log
{
    daily
    rotate 7
    missingok
    notifempty
    copytruncate
    create 0666 %s %s
}
`, user, group)

	path := "/etc/logrotate.d/sentinel-xray-logs"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return
	}
	log.Printf("⚙️ logrotate de access.log alinhado a %s:%s (%s)", user, group, path)
}

// EnsureLogFileAccessible cria o diretório/arquivo de access.log e aplica dono do Xray/V2Ray
// (não nobody:nogroup fixo) para o serviço nunca falhar com permission denied.
func EnsureLogFileAccessible(logFilePath string) error {
	if strings.TrimSpace(logFilePath) == "" || logFilePath == "none" {
		return nil
	}

	logFilePath = filepath.Clean(logFilePath)
	dir := filepath.Dir(logFilePath)

	dirMode := os.FileMode(0775)
	if !isDedicatedXrayLogDir(dir) {
		dirMode = 0777
	}
	if err := os.MkdirAll(dir, dirMode); err != nil {
		log.Printf("⚠️ Erro ao criar diretório de log %s: %v", dir, err)
	}
	_ = os.Chmod(dir, dirMode)

	if _, err := os.Stat(logFilePath); os.IsNotExist(err) {
		if f, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666); err == nil {
			_ = f.Close()
		}
	}

	applyXrayLogOwnership(logFilePath)
	return nil
}
