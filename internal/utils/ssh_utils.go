package utils

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	LOG_FILE                   = "./logs/ssh_user_creation_errors.log"
	SSH_EXPIRATION_BACKUP_FILE = "./data/ssh_user_expiration_backup.json"
)

// SSHExpirationBackup armazena as datas de expiração originais dos usuários desativados
type SSHExpirationBackup struct {
	Users map[string]string `json:"users"` // username -> expiration_date (YYYY-MM-DD ou "" para nunca)
}

// EnsureLogDir garante que o diretório de logs existe
func EnsureLogDir() error {
	logDir := filepath.Dir(LOG_FILE)
	return os.MkdirAll(logDir, 0755)
}

// WriteLog escreve uma mensagem no arquivo de log
func WriteLog(message string) error {
	if err := EnsureLogDir(); err != nil {
		return err
	}

	// Verificar se o arquivo existe, se não, criar
	if _, err := os.Stat(LOG_FILE); os.IsNotExist(err) {
		if err := os.WriteFile(LOG_FILE, []byte(fmt.Sprintf("%s - Log file created\n", time.Now().Format(time.RFC3339))), 0644); err != nil {
			return err
		}
	}

	// Adicionar log
	file, err := os.OpenFile(LOG_FILE, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	logMessage := fmt.Sprintf("%s - %s\n", time.Now().Format(time.RFC3339), message)
	_, err = file.WriteString(logMessage)
	return err
}

// ExecuteCommand executa um comando shell e retorna o resultado
func ExecuteCommand(command string, args ...string) (string, error) {
	cmd := exec.Command(command, args...)
	output, err := cmd.Output()
	return string(output), err
}

// ExecuteCommandQuiet executa um comando shell silenciosamente (ignora erros)
func ExecuteCommandQuiet(command string, args ...string) error {
	cmd := exec.Command(command, args...)
	return cmd.Run()
}

// CheckUserExists verifica se um usuário existe
func CheckUserExists(username string) (bool, error) {
	err := ExecuteCommandQuiet("id", username)
	if err != nil {
		// Se o comando falhou, o usuário não existe
		return false, nil
	}
	return true, nil
}

// DeleteUser deleta um usuário do sistema
func DeleteUser(username string) error {
	return ExecuteCommandQuiet("userdel", "-f", "-r", username)
}

// CalculateExpirationDate calcula a data de expiração baseada nos dias
func CalculateExpirationDate(days int) (string, error) {
	output, err := ExecuteCommand("date", "+%Y-%m-%d", "-d", fmt.Sprintf("+%d days", days))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

// HashPassword gera hash da senha usando openssl
func HashPassword(password string) (string, error) {
	output, err := ExecuteCommand("openssl", "passwd", "-6", password)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

// CreateUser cria um usuário no sistema
func CreateUser(username, password, expirationDate string) error {
	// Gerar hash da senha
	hashedPassword, err := HashPassword(password)
	if err != nil {
		return fmt.Errorf("erro ao gerar hash da senha: %v", err)
	}

	// Criar usuário com retry logic
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		err = ExecuteCommandQuiet("useradd", "-e", expirationDate, "-M", "-s", "/bin/false", "-p", hashedPassword, username)
		if err == nil {
			return nil // Sucesso
		}

		// Se não é erro de lock, falha imediatamente
		if !strings.Contains(err.Error(), "cannot lock /etc/passwd") {
			return err
		}

		// Se é erro de lock e ainda tem tentativas, aguarda
		if i < maxRetries-1 {
			time.Sleep(1 * time.Second)
		}
	}

	// Última tentativa
	return ExecuteCommandQuiet("useradd", "-e", expirationDate, "-M", "-s", "/bin/false", "-p", hashedPassword, username)
}

// sanitizeUsername valida que o username contém apenas caracteres seguros (padrão POSIX)
// DEVE ser chamado ANTES de qualquer uso em comandos do sistema
func sanitizeUsername(username string) error {
	if len(username) == 0 || len(username) > 32 {
		return fmt.Errorf("username com tamanho inválido: %d", len(username))
	}
	for _, c := range username {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' || c == '.') {
			return fmt.Errorf("username contém caracteres inválidos: %s", username)
		}
	}
	return nil
}

// KillUserProcesses mata todos os processos de um usuário incluindo sessões SSH
// Usa pkill direto (sem bash -c) para evitar command injection
func KillUserProcesses(username string) error {
	if err := sanitizeUsername(username); err != nil {
		return err
	}

	// 1. Matar processos do usuário graciosamente (SIGTERM)
	ExecuteCommandQuiet("pkill", "-u", username)

	// Aguardar processos terminarem graciosamente
	time.Sleep(1 * time.Second)

	// 2. Forçar kill em todos os processos restantes (SIGKILL)
	ExecuteCommandQuiet("pkill", "-KILL", "-u", username)

	return nil
}

// KillUserProcessesForced mata todos os processos de um usuário de forma agressiva (SIGKILL direto)
// Usado durante deleção de usuários para garantir que todos os processos sejam mortos
func KillUserProcessesForced(username string) error {
	if err := sanitizeUsername(username); err != nil {
		return err
	}

	// Matar todos os processos do usuário com SIGKILL direto (sem bash -c)
	ExecuteCommandQuiet("pkill", "-KILL", "-u", username)

	return nil
}

// HasUserProcesses verifica se existem processos do usuário
func HasUserProcesses(username string) (bool, error) {
	if err := sanitizeUsername(username); err != nil {
		return false, err
	}

	// Usar pgrep diretamente (sem bash -c) — retorna exit 0 se encontrar processos
	err := ExecuteCommandQuiet("pgrep", "-u", username)
	if err != nil {
		// pgrep retorna exit 1 se não encontrar processos
		return false, nil
	}
	return true, nil
}

// GetUserExpirationDate obtém a data de expiração atual do usuário
func GetUserExpirationDate(username string) (string, error) {
	output, err := ExecuteCommand("chage", "-l", username)
	if err != nil {
		return "", err
	}

	// Procura por "Account expires" na saída do chage
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "Account expires") || strings.Contains(line, "Conta expira") {
			// Formato: "Account expires           : nov 30, 2025" ou "never"
			parts := strings.Split(line, ":")
			if len(parts) < 2 {
				return "", nil
			}
			expiryStr := strings.TrimSpace(parts[1])
			if strings.ToLower(expiryStr) == "never" || strings.ToLower(expiryStr) == "nunca" {
				return "", nil // Sem expiração
			}
			// Parsear data no formato "nov 30, 2025" e converter para "2025-11-30"
			// Isso requer parsing da data, por simplicidade, vamos retornar a string original
			// e fazer parsing no EnableUser
			return expiryStr, nil
		}
	}

	return "", nil // Não encontrou expiração
}

// ParseExpirationDateFromChage converte a saída do chage para formato YYYY-MM-DD
func ParseExpirationDateFromChage(chageOutput string) (string, error) {
	chageOutput = strings.ToLower(chageOutput)
	// Remover espaços extras
	chageOutput = strings.TrimSpace(chageOutput)

	if chageOutput == "never" || chageOutput == "nunca" {
		return "", nil // Sem expiração
	}

	// Formato típico: "nov 30, 2025" (português) ou "Nov 30, 2025" (inglês)
	// Vamos usar date para converter
	output, err := ExecuteCommand("date", "-d", chageOutput, "+%Y-%m-%d")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

// CalculateDaysUntilExpiration calcula quantos dias até a expiração
func CalculateDaysUntilExpiration(expirationDateStr string) (int, error) {
	if expirationDateStr == "" {
		return 0, nil // Sem expiração
	}

	// Parsear data no formato YYYY-MM-DD
	expDate, err := time.Parse("2006-01-02", expirationDateStr)
	if err != nil {
		return 0, err
	}

	now := time.Now()
	diff := expDate.Sub(now)
	days := int(diff.Hours() / 24)

	return days, nil
}

// saveExpirationBackup salva a data de expiração original antes de desativar
func saveExpirationBackup(username, expirationDate string) error {
	// Garantir que o diretório existe
	if err := os.MkdirAll(filepath.Dir(SSH_EXPIRATION_BACKUP_FILE), 0755); err != nil {
		return err
	}

	// Carregar backup existente
	backup := SSHExpirationBackup{Users: make(map[string]string)}
	if data, err := os.ReadFile(SSH_EXPIRATION_BACKUP_FILE); err == nil {
		json.Unmarshal(data, &backup)
	}
	if backup.Users == nil {
		backup.Users = make(map[string]string)
	}

	// Salvar data de expiração original
	backup.Users[username] = expirationDate

	// Escrever arquivo de forma atômica
	data, err := json.MarshalIndent(backup, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := SSH_EXPIRATION_BACKUP_FILE + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}

	return os.Rename(tmpFile, SSH_EXPIRATION_BACKUP_FILE)
}

// loadExpirationBackup carrega a data de expiração original salva
func loadExpirationBackup(username string) (string, bool) {
	data, err := os.ReadFile(SSH_EXPIRATION_BACKUP_FILE)
	if err != nil {
		return "", false
	}

	var backup SSHExpirationBackup
	if err := json.Unmarshal(data, &backup); err != nil {
		return "", false
	}

	expirationDate, exists := backup.Users[username]
	return expirationDate, exists
}

// RemoveExpirationBackup remove o backup de expiração (função pública)
func RemoveExpirationBackup(username string) error {
	return removeExpirationBackup(username)
}

// removeExpirationBackup remove o backup após restaurar
func removeExpirationBackup(username string) error {
	data, err := os.ReadFile(SSH_EXPIRATION_BACKUP_FILE)
	if err != nil {
		return nil // Arquivo não existe, nada a remover
	}

	var backup SSHExpirationBackup
	if err := json.Unmarshal(data, &backup); err != nil {
		return nil // Erro ao parsear, ignorar
	}

	if backup.Users == nil {
		return nil
	}

	delete(backup.Users, username)

	// Se não há mais usuários, remover arquivo
	if len(backup.Users) == 0 {
		return os.Remove(SSH_EXPIRATION_BACKUP_FILE)
	}

	// Reescrever arquivo
	data, err = json.MarshalIndent(backup, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := SSH_EXPIRATION_BACKUP_FILE + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}

	return os.Rename(tmpFile, SSH_EXPIRATION_BACKUP_FILE)
}

// DisableUser desabilita um usuário (bloqueia a conta e mata processos)
// Fluxo invertido: primeiro desativa/bloqueia, depois mata processos para evitar que processos reabram
// Salva a data de expiração original antes de desativar para restaurar depois
func DisableUser(username string) error {
	// ANTES DE DESATIVAR: Salvar data de expiração original
	// Ler diretamente do chage para obter informação precisa
	output, err := ExecuteCommand("chage", "-l", username)
	var expirationDate string

	if err == nil {
		// Procura por "Account expires" na saída do chage
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			if strings.Contains(line, "Account expires") || strings.Contains(line, "Conta expira") {
				parts := strings.Split(line, ":")
				if len(parts) >= 2 {
					expiryStr := strings.TrimSpace(parts[1])
					if strings.ToLower(expiryStr) == "never" || strings.ToLower(expiryStr) == "nunca" {
						// Sem expiração, salvar como vazio
						expirationDate = ""
						break
					}
					// Tentar parsear a data
					parsedDate, err := ParseExpirationDateFromChage(expiryStr)
					if err == nil {
						expirationDate = parsedDate
						break
					}
				}
			}
		}
	}
	// Se não encontrou ou deu erro, salvar como vazio (assumir sem expiração ou erro)
	// Isso é seguro pois ao reativar sem days, se não houver backup válido, usará 30 dias padrão
	saveExpirationBackup(username, expirationDate)

	// PRIMEIRO: Bloquear a conta do usuário (impede novas conexões)
	if err := ExecuteCommandQuiet("usermod", "-L", username); err != nil {
		return err
	}

	// Define shell como nologin para impedir login
	if err := ExecuteCommandQuiet("usermod", "-s", "/usr/sbin/nologin", username); err != nil {
		return err
	}

	// Define data de expiração para ontem (desativa a conta)
	yesterday, err := ExecuteCommand("date", "+%Y-%m-%d", "-d", "yesterday")
	if err != nil {
		// Se não conseguir calcular ontem, usar data fixa passada
		yesterday = "1970-01-01"
	}
	if err := ExecuteCommandQuiet("usermod", "-e", strings.TrimSpace(yesterday), username); err != nil {
		return err
	}

	// DEPOIS: Matar todos os processos do usuário (incluindo sshd)
	// Com a conta bloqueada, não haverá novos processos sendo criados
	KillUserProcessesForced(username)

	return nil
}

// EnableUser habilita um usuário (desbloqueia, restaura shell e atualiza expiração)
// Se days não for fornecido, restaura a data de expiração original salva antes de desativar
func EnableUser(username string, days *int) error {
	// 1. Desbloquear a conta do usuário
	if err := ExecuteCommandQuiet("usermod", "-U", username); err != nil {
		return err
	}

	// 2. Restaurar shell padrão (bash) para permitir login
	if err := ExecuteCommandQuiet("usermod", "-s", "/bin/bash", username); err != nil {
		return err
	}

	// 3. Definir data de expiração
	var expirationDate string
	var err error

	if days != nil && *days > 0 {
		// Se dias foram fornecidos, usar eles
		expirationDate, err = CalculateExpirationDate(*days)
		if err != nil {
			return fmt.Errorf("erro ao calcular data de expiração: %v", err)
		}
	} else {
		// Se não forneceu dias, tentar restaurar a data de expiração original
		if originalDate, exists := loadExpirationBackup(username); exists {
			if originalDate == "" {
				// Não tinha expiração original, remover expiração (nunca expira)
				expirationDate = ""
			} else {
				// Verificar se a data original ainda é válida (não expirou)
				daysUntilExp, err := CalculateDaysUntilExpiration(originalDate)
				if err == nil && daysUntilExp > 0 {
					// Data original ainda é válida, restaurar
					expirationDate = originalDate
				} else {
					// Data original já expirou, usar 30 dias como padrão
					expirationDate, err = CalculateExpirationDate(30)
					if err != nil {
						return fmt.Errorf("erro ao calcular data de expiração padrão: %v", err)
					}
				}
			}
		} else {
			// Não encontrou backup, usar 30 dias como padrão
			expirationDate, err = CalculateExpirationDate(30)
			if err != nil {
				return fmt.Errorf("erro ao calcular data de expiração padrão: %v", err)
			}
		}
	}

	// Aplicar data de expiração
	if expirationDate == "" {
		// Remover expiração (nunca expira)
		if err := ExecuteCommandQuiet("usermod", "-e", "", username); err != nil {
			return err
		}
	} else {
		if err := ExecuteCommandQuiet("usermod", "-e", expirationDate, username); err != nil {
			return err
		}
	}

	// 4. Remover backup após restaurar (limpar)
	removeExpirationBackup(username)

	// 5. Desbloquear a senha se estiver bloqueada
	ExecuteCommandQuiet("passwd", "-u", username)

	return nil
}

// UpdateExpirationDate atualiza apenas a data de expiração de um usuário
func UpdateExpirationDate(username, expirationDate string) error {
	return ExecuteCommandQuiet("usermod", "-e", expirationDate, username)
}

// IsReservedUsername verifica se o username está na lista de reservados
func IsReservedUsername(username string) bool {
	reserved := []string{"root", "admin", "sys", "sshd", "www-data", "postgres", "mysql", "nginx", "apache", "systemd-network", "systemd-resolve", "messagebus", "syslog", "daemon", "bin", "sync", "games", "man", "lp", "mail", "news", "uucp", "proxy", "backup", "list", "irc", "gnats", "nobody", "_apt", "systemd-timesync", "systemd-bus-proxy", "uuidd", "tcpdump", "tss", "landscape", "pollinate", "ubuntu", "debian"}
	usernameLower := strings.ToLower(username)

	for _, reservedName := range reserved {
		if usernameLower == reservedName {
			return true
		}
	}
	return false
}

// GetUserUID retorna o UID de um usuário
func GetUserUID(username string) (int, error) {
	cmd := exec.Command("id", "-u", username)
	output, err := cmd.Output()
	if err != nil {
		return -1, fmt.Errorf("erro ao obter UID do usuário %s: %v", username, err)
	}

	uidStr := strings.TrimSpace(string(output))
	uid, err := strconv.Atoi(uidStr)
	if err != nil {
		return -1, fmt.Errorf("erro ao converter UID %s para inteiro: %v", uidStr, err)
	}

	return uid, nil
}

// ListSSHUsers lista todos os usuários SSH criados (excluindo usuários reservados e do sistema)
// Retorna uma lista de usernames de usuários com shell /bin/false, /bin/bash, /bin/sh ou /usr/sbin/nologin
func ListSSHUsers() ([]string, error) {
	// Comando para listar usuários SSH: grep -v '^root:' /etc/passwd | grep -E ':/bin/(false|bash|sh)$|:/usr/sbin/nologin$' | cut -d: -f1
	cmd := exec.Command("bash", "-c", "grep -v '^root:' /etc/passwd | grep -E ':/bin/(false|bash|sh)$|:/usr/sbin/nologin$' | cut -d: -f1 | sort -u")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("erro ao listar usuários SSH: %v", err)
	}

	// Processar output e filtrar usuários reservados
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	users := []string{}

	for _, line := range lines {
		username := strings.TrimSpace(line)
		if username == "" {
			continue
		}

		if !IsReservedUsername(username) {
			users = append(users, username)
		}
	}

	return users, nil
}
