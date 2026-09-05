package services

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"api-v2/internal/models"
	"api-v2/internal/utils"
)

// MonitorService implementa o serviço de monitoramento de usuários online
type MonitorService struct {
	sshUsers     int
	v2rayUsers   int
	dtProtoUsers int
	vtproxyUsers int
	mutex        sync.RWMutex
	stopChan     chan bool

	// Cache detalhado de usuários online
	sshUsersList     []models.SSHUserOnline
	v2rayUsersList   []models.V2RayUserOnline
	dtProtoUsersList []models.DTProtoUserOnline
	vtproxyUsersList []models.VTProxyUserOnline

	// Cache de UUIDs V2Ray (email -> uuid) - pre-alocado
	v2rayUUIDCache map[string]string

	// Caminhos possíveis para logs V2Ray
	v2rayLogPaths  []string
	currentLogPath string

	// Caminho do config V2Ray (injetado)
	v2rayConfigPath string
	v2rayAvailable  bool

	// Cache com TTL para evitar leituras excessivas
	cacheExpiry   time.Time
	cacheDuration time.Duration
	// Regex pre-compilado para evitar recompilação a cada linha
	v2rayLogRegex *regexp.Regexp

	// Múltiplas instâncias / controle de limpeza
	disableV2RayLogCleanup bool
	logCleanupMutex        sync.Mutex

	// Caminho do passwd para validação de shell (/etc/passwd por padrão)
	passwdPath string
}

// NewMonitorService cria uma nova instância do serviço de monitoramento
func NewMonitorService(v2rayConfigPath string) *MonitorService {
	// Pre-compilar regex para extração de logs V2Ray (evita recompilação a cada linha)
	v2rayLogRegex := regexp.MustCompile(`(\d{4}\/\d{2}\/\d{2} \d{2}:\d{2}:\d{2}).*?(accepted|rejected).*?email:\s*([\w._%+-]+@[\w.-]+\.[a-zA-Z]{2,})`)

	return &MonitorService{
		sshUsers:               0,
		v2rayUsers:             0,
		dtProtoUsers:           0,
		vtproxyUsers:           0,
		stopChan:               make(chan bool),
		v2rayUUIDCache:         make(map[string]string, 100),
		sshUsersList:           make([]models.SSHUserOnline, 0, 50),
		v2rayUsersList:         make([]models.V2RayUserOnline, 0, 100),
		dtProtoUsersList:       make([]models.DTProtoUserOnline, 0, 100),
		vtproxyUsersList:       make([]models.VTProxyUserOnline, 0, 50),
		cacheDuration:          10 * time.Second,
		v2rayLogRegex:          v2rayLogRegex,
		v2rayConfigPath:        v2rayConfigPath,
		v2rayAvailable:         false,
		passwdPath:             "/etc/passwd",
		disableV2RayLogCleanup: parseEnvBool("SENTINEL_DISABLE_V2RAY_LOG_CLEANUP", false),
		v2rayLogPaths: []string{
			"/var/log/xray/access.log",
			"/usr/local/etc/xray/access.log",
			"/etc/xray/access.log",
			"/var/log/v2ray/access.log",
			"/var/log/xray.log",
			"/usr/local/var/log/xray/access.log",
		},
	}
}

// SetPasswdPath define o caminho do arquivo passwd (útil para testes unitários ou customizações)
func (m *MonitorService) SetPasswdPath(path string) {
	m.mutex.Lock()
	m.passwdPath = path
	m.mutex.Unlock()
}

// Start inicia o serviço de monitoramento
func (m *MonitorService) Start() {
	log.Println("🚀 Iniciando serviço de monitoramento de usuários online...")

	// Verificar e auto-configurar logs do V2Ray se o config.json existir
	if m.ensureV2RayConfigAndLogs() {
		log.Println("✅ Monitoramento de V2Ray/Xray ativo e logs sincronizados")
		m.loadV2RayUUIDCache()
	} else {
		log.Println("ℹ️ V2Ray/Xray não detectado no servidor (monitoramento de V2Ray pausado aguardando instalação).")
	}

	// Verificar e otimizar logs excessivamente grandes no arranque para evitar OOM e poupar disco
	m.checkAndOptimizeLogFiles()

	// Iniciar goroutines de monitoramento
	go m.monitorSSHUsers()
	go m.monitorV2RayUsers()
	go m.monitorDTProtoUsers()
	go m.monitorVTProxyUsers()
	if m.disableV2RayLogCleanup {
		log.Println("ℹ️ Limpeza automática do access.log desativada (SENTINEL_DISABLE_V2RAY_LOG_CLEANUP); várias instâncias podem partilhar o log sem escrita concorrente.")
	} else {
		go m.cleanV2RayLogs()
	}
	go m.reloadV2RayUUIDCache()

	log.Println("✅ Serviço de monitoramento iniciado")
}

// Stop para o serviço de monitoramento
func (m *MonitorService) Stop() {
	log.Println("🛑 Parando serviço de monitoramento...")
	close(m.stopChan)

	// Liberar memória dos caches
	m.mutex.Lock()
	m.v2rayUUIDCache = nil
	m.sshUsersList = nil
	m.v2rayUsersList = nil
	m.dtProtoUsersList = nil
	m.vtproxyUsersList = nil
	m.mutex.Unlock()

	log.Println("✅ Serviço de monitoramento parado")
}

// GetOnlineUsers retorna os usuários online do cache
func (m *MonitorService) GetOnlineUsers() models.OnlineUsersResponse {
	m.mutex.RLock()

	// Verificar se o cache ainda é válido
	if time.Now().Before(m.cacheExpiry) {
		defer m.mutex.RUnlock()
		return models.NewOnlineUsersResponse(m.sshUsers, m.v2rayUsers, m.dtProtoUsers, m.vtproxyUsers)
	}
	m.mutex.RUnlock()

	// Cache expirado, atualizar
	m.updateSSHUsers()
	m.updateV2RayUsers()
	m.updateDTProtoUsers()
	m.updateVTProxyUsers()

	m.mutex.Lock()
	m.cacheExpiry = time.Now().Add(m.cacheDuration)
	m.mutex.Unlock()

	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return models.NewOnlineUsersResponse(m.sshUsers, m.v2rayUsers, m.dtProtoUsers, m.vtproxyUsers)
}

// GetDetailedOnlineUsers retorna a lista detalhada de usuários online do cache
func (m *MonitorService) GetDetailedOnlineUsers() models.DetailedUsersResponse {
	m.mutex.RLock()

	// Verificar se o cache ainda é válido
	if time.Now().Before(m.cacheExpiry) {
		sshList := m.sshUsersList
		if sshList == nil {
			sshList = []models.SSHUserOnline{}
		}
		v2rayList := m.v2rayUsersList
		if v2rayList == nil {
			v2rayList = []models.V2RayUserOnline{}
		}
		dtProtoList := m.dtProtoUsersList
		if dtProtoList == nil {
			dtProtoList = []models.DTProtoUserOnline{}
		}
		vtproxyList := m.vtproxyUsersList
		if vtproxyList == nil {
			vtproxyList = []models.VTProxyUserOnline{}
		}
		defer m.mutex.RUnlock()
		return models.NewDetailedUsersResponse(sshList, v2rayList, dtProtoList, vtproxyList)
	}
	m.mutex.RUnlock()

	// Cache expirado, atualizar
	m.updateSSHUsers()
	m.updateV2RayUsers()
	m.updateDTProtoUsers()
	m.updateVTProxyUsers()

	m.mutex.Lock()
	m.cacheExpiry = time.Now().Add(m.cacheDuration)
	m.mutex.Unlock()

	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Garantir que slices não sejam nil
	sshList := m.sshUsersList
	if sshList == nil {
		sshList = []models.SSHUserOnline{}
	}
	v2rayList := m.v2rayUsersList
	if v2rayList == nil {
		v2rayList = []models.V2RayUserOnline{}
	}
	dtProtoList := m.dtProtoUsersList
	if dtProtoList == nil {
		dtProtoList = []models.DTProtoUserOnline{}
	}
	vtproxyList := m.vtproxyUsersList
	if vtproxyList == nil {
		vtproxyList = []models.VTProxyUserOnline{}
	}

	return models.NewDetailedUsersResponse(sshList, v2rayList, dtProtoList, vtproxyList)
}

// ensureV2RayConfigAndLogs verifica se o V2Ray/Xray existe e auto-configura os logs se necessário
func (m *MonitorService) ensureV2RayConfigAndLogs() bool {
	configPaths := []string{
		m.v2rayConfigPath,
		"/etc/xray/config.json",
		"/usr/local/etc/xray/config.json",
		"/etc/v2ray/config.json",
		"/usr/local/etc/v2ray/config.json",
	}

	for _, p := range configPaths {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			m.v2rayConfigPath = p
			m.autoConfigureV2RayLog(p)
			m.mutex.Lock()
			m.v2rayAvailable = true
			m.mutex.Unlock()
			return true
		}
	}

	// Nenhum config encontrado. Checar se ao menos algum log existe
	m.findV2RayLogFileSilently()
	if m.currentLogPath != "" {
		m.mutex.Lock()
		m.v2rayAvailable = true
		m.mutex.Unlock()
		return true
	}

	m.mutex.Lock()
	m.v2rayAvailable = false
	m.mutex.Unlock()
	return false
}

// autoConfigureV2RayLog inspeciona o config.json e injeta a seção log se ausente
func (m *MonitorService) autoConfigureV2RayLog(configPath string) {
	content, err := os.ReadFile(configPath)
	if err != nil {
		return
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(content, &cfg); err != nil {
		return
	}

	// Verificar se já tem log de acesso configurado
	needUpdate := false
	logObj, hasLog := cfg["log"].(map[string]interface{})
	if !hasLog || logObj == nil {
		needUpdate = true
	} else {
		access, _ := logObj["access"].(string)
		if strings.TrimSpace(access) == "" || access == "none" {
			needUpdate = true
		}
	}

	if needUpdate {
		log.Printf("⚙️ Injetando configuração de log no V2Ray/Xray (%s)...", configPath)

		accessLogPath := "/var/log/xray/access.log"
		_ = utils.EnsureLogFileAccessible(accessLogPath)

		cfg["log"] = map[string]interface{}{
			"access":      accessLogPath,
			"dnsLog":      false,
			"error":       "",
			"loglevel":    "info",
			"maskAddress": "",
		}

		if newJSON, err := json.MarshalIndent(cfg, "", "  "); err == nil {
			tmp := configPath + ".sentinel.tmp"
			if err := os.WriteFile(tmp, newJSON, 0644); err == nil {
				if err := os.Rename(tmp, configPath); err == nil {
					log.Printf("✅ Log de acesso V2Ray/Xray configurado com sucesso em %s", accessLogPath)
					m.currentLogPath = accessLogPath
					_ = utils.EnsureLogFileAccessible(accessLogPath)
					// Recarregar serviço para começar a gravar logs imediatamente
					_ = utils.ExecuteCommandQuiet("systemctl", "reload", "xray")
					_ = utils.ExecuteCommandQuiet("systemctl", "reload", "v2ray")
				}
			}
		}
	}

	if m.currentLogPath == "" {
		if logObj, ok := cfg["log"].(map[string]interface{}); ok {
			if access, ok := logObj["access"].(string); ok && access != "" && access != "none" {
				_ = utils.EnsureLogFileAccessible(access)
				if _, err := os.Stat(access); err == nil {
					m.currentLogPath = access
				}
			}
		}
		if m.currentLogPath == "" {
			m.findV2RayLogFileSilently()
		}
	}

	if m.currentLogPath != "" {
		_ = utils.EnsureLogFileAccessible(m.currentLogPath)
	}

	// Garantir que os logs do systemd (journald) estejam suprimidos (StandardOutput=null / StandardError=null)
	_, _ = utils.EnsureSystemdServiceLogsDisabled("xray", "v2ray")
}

// findV2RayLogFileSilently procura arquivo de log disponível sem poluir o console
func (m *MonitorService) findV2RayLogFileSilently() {
	for _, path := range m.v2rayLogPaths {
		if _, err := os.Stat(path); err == nil {
			m.currentLogPath = path
			return
		}
	}
}

// loadV2RayUUIDCache carrega o cache de UUIDs do arquivo de configuração V2Ray
func (m *MonitorService) loadV2RayUUIDCache() {
	if m.v2rayConfigPath == "" {
		return
	}
	if _, err := os.Stat(m.v2rayConfigPath); err != nil {
		m.mutex.Lock()
		m.v2rayUUIDCache = make(map[string]string)
		m.mutex.Unlock()
		return
	}

	content, err := os.ReadFile(m.v2rayConfigPath)
	if err != nil {
		return
	}

	var config map[string]interface{}
	if err := json.Unmarshal(content, &config); err != nil {
		return
	}

	// Pre-alocar cache se possível
	var estimatedUsers int
	if inbounds, ok := config["inbounds"].([]interface{}); ok {
		for _, inbound := range inbounds {
			if inboundMap, ok := inbound.(map[string]interface{}); ok {
				if settings, ok := inboundMap["settings"].(map[string]interface{}); ok {
					if clients, ok := settings["clients"].([]interface{}); ok {
						estimatedUsers = len(clients)
					}
				}
			}
		}
	}

	newCache := make(map[string]string, estimatedUsers)

	// Procurar por usuários na configuração
	if inbounds, ok := config["inbounds"].([]interface{}); ok {
		for _, inbound := range inbounds {
			if inboundMap, ok := inbound.(map[string]interface{}); ok {
				if settings, ok := inboundMap["settings"].(map[string]interface{}); ok {
					if clients, ok := settings["clients"].([]interface{}); ok {
						for _, client := range clients {
							if clientMap, ok := client.(map[string]interface{}); ok {
								if email, ok := clientMap["email"].(string); ok {
									if uuid, ok := clientMap["id"].(string); ok {
										newCache[email] = uuid
									}
								}
							}
						}
					}
				}
			}
		}
	}

	m.mutex.Lock()
	m.v2rayUUIDCache = newCache
	m.mutex.Unlock()

	if len(newCache) > 0 {
		log.Printf("📋 Cache de UUIDs V2Ray carregado: %d usuários", len(newCache))
	}
}

// getV2RayUUID busca o UUID de um usuário V2Ray pelo email
func (m *MonitorService) getV2RayUUID(email string) string {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	if uuid, exists := m.v2rayUUIDCache[email]; exists {
		return uuid
	}
	return ""
}

// reloadV2RayUUIDCache recarrega o cache de UUIDs e verifica se o V2Ray foi instalado
func (m *MonitorService) reloadV2RayUUIDCache() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if m.ensureV2RayConfigAndLogs() {
				m.loadV2RayUUIDCache()
			}
		case <-m.stopChan:
			return
		}
	}
}

// monitorSSHUsers monitora usuários SSH online
func (m *MonitorService) monitorSSHUsers() {
	ticker := time.NewTicker(3 * time.Minute)
	defer ticker.Stop()

	// Atualizar imediatamente
	m.updateSSHUsers()

	for {
		select {
		case <-ticker.C:
			m.updateSSHUsers()
		case <-m.stopChan:
			return
		}
	}
}

// monitorV2RayUsers monitora usuários V2Ray online
func (m *MonitorService) monitorV2RayUsers() {
	ticker := time.NewTicker(3 * time.Minute)
	defer ticker.Stop()

	// Atualizar imediatamente
	m.updateV2RayUsers()

	for {
		select {
		case <-ticker.C:
			m.updateV2RayUsers()
		case <-m.stopChan:
			return
		}
	}
}

// monitorDTProtoUsers monitora usuários DT-Proto online (lê stats.json periodicamente)
func (m *MonitorService) monitorDTProtoUsers() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	// Atualizar imediatamente
	m.updateDTProtoUsers()

	for {
		select {
		case <-ticker.C:
			m.updateDTProtoUsers()
		case <-m.stopChan:
			return
		}
	}
}

// updateSSHUsers atualiza o número de usuários SSH online
func (m *MonitorService) updateSSHUsers() {
	// Obter lista detalhada de usuários SSH
	sshUsersList := m.getSSHUsersList()

	// Contar usuários
	sshUsers := len(sshUsersList)

	m.mutex.Lock()
	m.sshUsers = sshUsers
	m.sshUsersList = sshUsersList
	m.mutex.Unlock()

	log.Printf("👤 Usuários SSH online: %d", sshUsers)
}

// sshUserRegex extrai o username dos argumentos do processo sshd (ex: sshd: user1 [priv], sshd: user1@pts/0)
var sshUserRegex = regexp.MustCompile(`sshd:\s*([a-zA-Z0-9._-]+)`)

// extractSSHUsername extrai o username do título do processo sshd
func extractSSHUsername(line string) string {
	matches := sshUserRegex.FindStringSubmatch(line)
	if len(matches) > 1 {
		user := matches[1]
		if !strings.Contains(user, "/") {
			return user
		}
	}
	return ""
}

// parseSSHProcesses analisa a saída dos processos SSH e filtra apenas usuários válidos com /bin/false
func (m *MonitorService) parseSSHProcesses(rawOutput string, validUsers map[string]bool) []models.SSHUserOnline {
	if len(rawOutput) == 0 || len(validUsers) == 0 {
		return []models.SSHUserOnline{}
	}

	onlineMap := make(map[string]struct{})
	lines := strings.Split(rawOutput, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		// 1. Verificar coluna do usuário dono do processo (excluindo root e sistema)
		userCol := fields[0]
		if validUsers[userCol] {
			onlineMap[userCol] = struct{}{}
		}

		// 2. Extrair username dos argumentos do processo (ex: "sshd: username [priv]", "sshd: username [net]", "sshd: username@pts/0")
		if len(fields) > 1 {
			if extractedUser := extractSSHUsername(line); extractedUser != "" {
				if validUsers[extractedUser] {
					onlineMap[extractedUser] = struct{}{}
				}
			}
		}
	}

	users := make([]models.SSHUserOnline, 0, len(onlineMap))
	for username := range onlineMap {
		users = append(users, models.SSHUserOnline{
			Username: username,
		})
	}

	// Ordenação alfabética para garantir consistência e estabilidade na resposta JSON
	sort.Slice(users, func(i, j int) bool {
		return users[i].Username < users[j].Username
	})

	return users
}

// getSSHUsersList obtém a lista detalhada de usuários SSH online que possuem estritamente /bin/false
func (m *MonitorService) getSSHUsersList() []models.SSHUserOnline {
	passwdFile := m.passwdPath
	if passwdFile == "" {
		passwdFile = "/etc/passwd"
	}

	validUsers, err := utils.GetUsersWithBinFalseShell(passwdFile)
	if err != nil {
		// Se não conseguir ler /etc/passwd (ou arquivo inexistente no SO), retorna vazio
		return []models.SSHUserOnline{}
	}

	if len(validUsers) == 0 {
		return []models.SSHUserOnline{}
	}

	// Executar comando para detectar processos sshd diretamente
	cmd := exec.Command("sh", "-c", "ps -C sshd,dropbear -o user=,args= 2>/dev/null || pgrep -f 'sshd:' | xargs -r ps -o user=,args= 2>/dev/null || true")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("⚠️ Erro ao executar comando de monitoramento SSH: %v", err)
		return []models.SSHUserOnline{}
	}

	rawOutput := strings.TrimSpace(string(output))
	if len(rawOutput) == 0 {
		// Nenhum processo SSH ativo
		return []models.SSHUserOnline{}
	}

	return m.parseSSHProcesses(rawOutput, validUsers)
}

// ReadLogTail lê de forma segura apenas os últimos maxBytes de um arquivo de log,
// evitando carregar arquivos gigantes (como logs de múltiplos GB) na memória RAM.
func ReadLogTail(filePath string, maxBytes int64) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}

	fileSize := stat.Size()
	if fileSize == 0 {
		return []string{}, nil
	}

	readSize := fileSize
	offset := int64(0)
	if fileSize > maxBytes {
		readSize = maxBytes
		offset = fileSize - maxBytes
	}

	buf := make([]byte, readSize)
	n, err := file.ReadAt(buf, offset)
	if err != nil && err != io.EOF {
		return nil, err
	}
	buf = buf[:n]

	// Se truncamos a partir do meio do arquivo, a primeira linha pode estar incompleta.
	// Avançar até a primeira quebra de linha '\n'.
	str := string(buf)
	if offset > 0 {
		if idx := strings.IndexByte(str, '\n'); idx != -1 {
			str = str[idx+1:]
		}
	}

	lines := strings.Split(str, "\n")
	return lines, nil
}

// safeTrimLargeLogFile reduz de forma atômica um arquivo de log gigante para os últimos keepBytes
func (m *MonitorService) safeTrimLargeLogFile(filePath string, maxBytes int64, keepBytes int64) bool {
	stat, err := os.Stat(filePath)
	if err != nil || stat.Size() <= maxBytes {
		return false
	}

	m.logCleanupMutex.Lock()
	defer m.logCleanupMutex.Unlock()

	// Re-checar tamanho após lock para garantir que outra rotina não o tenha feito
	stat, err = os.Stat(filePath)
	if err != nil || stat.Size() <= maxBytes {
		return false
	}

	sizeMB := stat.Size() / (1024 * 1024)
	keepMB := keepBytes / (1024 * 1024)
	log.Printf("⚠️ Arquivo de log %s muito grande (%d MB). Reduzindo para os últimos %d MB para economizar memória e disco...",
		filePath, sizeMB, keepMB)

	lines, err := ReadLogTail(filePath, keepBytes)
	if err != nil || len(lines) == 0 {
		return false
	}

	tmpPath := fmt.Sprintf("%s.cleanup.%d.tmp", filePath, os.Getpid())
	if err := os.WriteFile(tmpPath, []byte(strings.Join(lines, "\n")), 0666); err != nil {
		log.Printf("❌ Erro ao escrever arquivo temporário de limpeza: %v", err)
		return false
	}
	_ = os.Chmod(tmpPath, 0666)

	if err := os.Rename(tmpPath, filePath); err != nil {
		log.Printf("❌ Erro ao substituir log reduzido: %v", err)
		_ = os.Remove(tmpPath)
		return false
	}

	_ = utils.EnsureLogFileAccessible(filePath)

	log.Printf("✅ Log %s otimizado com sucesso!", filePath)
	return true
}

// checkAndOptimizeLogFiles verifica se algum log existente é excessivamente grande e otimiza no arranque
func (m *MonitorService) checkAndOptimizeLogFiles() {
	pathsToCheck := append([]string{}, m.v2rayLogPaths...)
	if m.currentLogPath != "" {
		pathsToCheck = append(pathsToCheck, m.currentLogPath)
	}

	maxSize := int64(20 * 1024 * 1024) // 20 MB limite padrão para disparar auto-trim
	keepSize := int64(2 * 1024 * 1024) // 2 MB para manter

	if envMB := os.Getenv("SENTINEL_V2RAY_LOG_MAX_MB"); envMB != "" {
		if mb, err := strconv.ParseInt(envMB, 10, 64); err == nil && mb > 0 {
			maxSize = mb * 1024 * 1024
		}
	}

	for _, path := range pathsToCheck {
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			m.safeTrimLargeLogFile(path, maxSize, keepSize)
		}
	}
}

// updateV2RayUsers atualiza o número de usuários V2Ray online
func (m *MonitorService) updateV2RayUsers() {
	m.mutex.RLock()
	available := m.v2rayAvailable
	m.mutex.RUnlock()

	if !available {
		m.mutex.Lock()
		m.v2rayUsers = 0
		m.v2rayUsersList = []models.V2RayUserOnline{}
		m.mutex.Unlock()
		return
	}

	// Tentar ler arquivo de log atual ou procurar arquivo disponível
	if m.currentLogPath == "" {
		m.findV2RayLogFileSilently()
	}

	maxTailBytes := int64(2 * 1024 * 1024) // 2 MB é suficiente para milhares de conexões recentes
	if envTail := os.Getenv("SENTINEL_V2RAY_LOG_MAX_TAIL_MB"); envTail != "" {
		if mb, err := strconv.ParseInt(envTail, 10, 64); err == nil && mb > 0 {
			maxTailBytes = mb * 1024 * 1024
		}
	}

	var lines []string
	var foundLogPath string

	if m.currentLogPath != "" {
		if l, err := ReadLogTail(m.currentLogPath, maxTailBytes); err == nil && len(l) > 0 {
			lines = l
			foundLogPath = m.currentLogPath
		}
	}

	if len(lines) == 0 {
		for _, logPath := range m.v2rayLogPaths {
			if l, err := ReadLogTail(logPath, maxTailBytes); err == nil && len(l) > 0 {
				lines = l
				foundLogPath = logPath
				m.currentLogPath = logPath
				break
			}
		}
	}

	if len(lines) == 0 {
		m.mutex.Lock()
		m.v2rayUsers = 0
		m.v2rayUsersList = []models.V2RayUserOnline{}
		m.mutex.Unlock()
		return
	}

	currentTime := time.Now()
	interval := 5 * time.Minute

	// Limitar processamento: últimas 5000 linhas (performance)
	startIdx := 0
	if len(lines) > 5000 {
		startIdx = len(lines) - 5000
	}

	// Map para armazenar usuários únicos com suas informações
	uniqueUsers := make(map[string]models.V2RayUserOnline)

	// Contadores para debug
	totalLines := 0
	validTimestamps := 0
	validUsers := 0
	oldLogsCount := 0

	// Processar linhas do final para o início (logs mais recentes primeiro)
	for i := len(lines) - 1; i >= startIdx; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		totalLines++

		timestamp := m.extractTimestampFromLog(line)
		if timestamp.IsZero() {
			// Log de debug para linhas que não conseguiram parsear timestamp
			if totalLines <= 5 {
				log.Printf("⚠️  Falha ao parsear timestamp na linha: %s", line)
			}
			continue
		}

		validTimestamps++

		timeDifference := currentTime.Sub(timestamp)
		if timeDifference < 0 {
			// Timestamp no futuro (erro de timezone ou relógio)
			continue
		}
		if timeDifference > interval {
			// Logs muito antigos - incrementar contador
			oldLogsCount++
			// Parar após 100 logs antigos consecutivos (performance)
			if oldLogsCount > 100 {
				break
			}
			continue
		} else {
			// Reset contador ao encontrar log recente
			oldLogsCount = 0
		}

		// Otimização: parar após encontrar 500 usuários únicos
		if len(uniqueUsers) >= 500 {
			log.Printf("⚡ Atingido limite de 500 usuários, parando processamento antecipado")
			break
		}

		user, accepted := m.extractUserFromLog(line)
		if user != "" && accepted {
			validUsers++
			// Buscar UUID do cache
			uuid := m.getV2RayUUID(user)

			// Se já existe, manter o mais recente
			if existing, exists := uniqueUsers[user]; !exists || timestamp.After(existing.LastConnection) {
				uniqueUsers[user] = models.V2RayUserOnline{
					Email:          user,
					UUID:           uuid,
					LastConnection: timestamp,
				}
			}
		} else if totalLines <= 5 {
			// Log de debug para linhas que não conseguiram extrair email
			log.Printf("⚠️  Falha ao extrair email na linha: %s", line)
		}
	}

	// Converter map para slice
	var v2rayUsersList []models.V2RayUserOnline
	for _, user := range uniqueUsers {
		v2rayUsersList = append(v2rayUsersList, user)
	}

	v2rayUsers := len(v2rayUsersList)

	m.mutex.Lock()
	m.v2rayUsers = v2rayUsers
	m.v2rayUsersList = v2rayUsersList
	m.cacheExpiry = time.Now().Add(m.cacheDuration) // Atualizar cache expiry
	m.mutex.Unlock()

	// Log detalhado para debug
	log.Printf("📊 Log V2Ray: %s | Total linhas: %d | Timestamps válidos: %d | Emails extraídos: %d | Usuários únicos online: %d",
		foundLogPath, totalLines, validTimestamps, validUsers, v2rayUsers)
}

// updateDTProtoUsers atualiza o número e lista de usuários DT-Proto online
func (m *MonitorService) updateDTProtoUsers() {
	// Tentar ler o arquivo de stats do DT-Proto
	statsData, err := os.ReadFile("/var/lib/proto-server/stats.json")
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("❌ Erro ao ler /var/lib/proto-server/stats.json: %v", err)
		}
		m.mutex.Lock()
		m.dtProtoUsers = 0
		m.dtProtoUsersList = []models.DTProtoUserOnline{}
		m.mutex.Unlock()
		return
	}

	// Parse JSON
	var statsMap map[string]map[string]interface{}
	if err := json.Unmarshal(statsData, &statsMap); err != nil {
		log.Printf("❌ Erro ao parsing JSON do DT-Proto: %v", err)
		m.mutex.Lock()
		m.dtProtoUsers = 0
		m.dtProtoUsersList = []models.DTProtoUserOnline{}
		m.mutex.Unlock()
		return
	}

	// Extrair IDs dos usuários online
	var dtProtoUsersList []models.DTProtoUserOnline
	for _, userStats := range statsMap {
		if id, ok := userStats["id"].(string); ok && id != "" {
			dtProtoUsersList = append(dtProtoUsersList, models.DTProtoUserOnline{
				ID: id,
			})
		}
	}

	dtProtoUsers := len(dtProtoUsersList)

	m.mutex.Lock()
	m.dtProtoUsers = dtProtoUsers
	m.dtProtoUsersList = dtProtoUsersList
	m.mutex.Unlock()

	log.Printf("🔗 Usuários DT-Proto online: %d", dtProtoUsers)
}

// updateVTProxyUsers atualiza o número e lista de usuários VTproxy online via /usr/local/bin/proxy-server --onlines-json
func (m *MonitorService) updateVTProxyUsers() {
	res, err := utils.GetProxyOnlineUsers()
	if err != nil {
		m.mutex.Lock()
		m.vtproxyUsers = 0
		m.vtproxyUsersList = []models.VTProxyUserOnline{}
		m.mutex.Unlock()
		return
	}

	vtproxyUsersList := make([]models.VTProxyUserOnline, 0, len(res.Users))
	for username, count := range res.Users {
		vtproxyUsersList = append(vtproxyUsersList, models.VTProxyUserOnline{
			Username:    username,
			Connections: count,
			Count:       count,
		})
	}

	sort.Slice(vtproxyUsersList, func(i, j int) bool {
		return vtproxyUsersList[i].Username < vtproxyUsersList[j].Username
	})

	vtproxyUsers := len(vtproxyUsersList)
	if res.Total > 0 && vtproxyUsers == 0 {
		vtproxyUsers = res.Total
	}

	m.mutex.Lock()
	m.vtproxyUsers = vtproxyUsers
	m.vtproxyUsersList = vtproxyUsersList
	m.mutex.Unlock()

	log.Printf("🔌 Usuários VTproxy online: %d (conexões totais: %d)", len(vtproxyUsersList), res.Total)
}

// monitorVTProxyUsers monitora usuários VTproxy online periodicamente
func (m *MonitorService) monitorVTProxyUsers() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Atualizar imediatamente
	m.updateVTProxyUsers()

	for {
		select {
		case <-ticker.C:
			m.updateVTProxyUsers()
		case <-m.stopChan:
			return
		}
	}
}

// cleanV2RayLogs limpa logs V2Ray antigos
func (m *MonitorService) cleanV2RayLogs() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.performV2RayLogCleanup()
		case <-m.stopChan:
			return
		}
	}
}

// performV2RayLogCleanup executa a limpeza de logs V2Ray de forma segura sem risco de OOM
func (m *MonitorService) performV2RayLogCleanup() {
	if m.currentLogPath == "" {
		return
	}

	// 1. Se o arquivo estiver muito grande (> 20 MB), faz trim direto para os últimos 2 MB
	if m.safeTrimLargeLogFile(m.currentLogPath, 20*1024*1024, 2*1024*1024) {
		return
	}

	// 2. Se o arquivo estiver em tamanho seguro (<= 20 MB), faz a limpeza por timestamp
	m.logCleanupMutex.Lock()
	defer m.logCleanupMutex.Unlock()

	log.Println("🧹 Iniciando limpeza de logs V2Ray antigos...")

	threshold := time.Now().Add(-12 * time.Hour)
	lines, err := ReadLogTail(m.currentLogPath, 20*1024*1024)
	if err != nil {
		log.Printf("❌ Erro ao ler arquivo de log V2Ray para limpeza: %v", err)
		return
	}

	estimatedKeep := int(float64(len(lines)) * 0.8)
	newLogContent := make([]string, 0, estimatedKeep)
	var removed, kept int

	for _, line := range lines {
		if len(line) < 26 {
			continue
		}

		tsStr := line[:26]
		ts, err := time.ParseInLocation("2006/01/02 15:04:05.000000", tsStr, time.Local)
		if err != nil {
			newLogContent = append(newLogContent, line)
			kept++
			continue
		}

		if ts.After(threshold) {
			newLogContent = append(newLogContent, line)
			kept++
		} else {
			removed++
		}
	}

	// Ficheiro tmp único por PID (evita corrida entre processos no mesmo .tmp)
	tmpPath := fmt.Sprintf("%s.cleanup.%d.tmp", m.currentLogPath, os.Getpid())
	if err := os.WriteFile(tmpPath, []byte(strings.Join(newLogContent, "\n")), 0666); err != nil {
		log.Printf("❌ Erro ao escrever tmp de limpeza: %v", err)
		return
	}
	_ = os.Chmod(tmpPath, 0666)

	if err := os.Rename(tmpPath, m.currentLogPath); err != nil {
		log.Printf("❌ Erro ao renomear arquivo de log após limpeza: %v", err)
		os.Remove(tmpPath)
		return
	}

	_ = utils.EnsureLogFileAccessible(m.currentLogPath)

	log.Printf("✅ Limpeza de logs V2Ray concluída: %d linhas removidas, %d mantidas", removed, kept)
}

// extractUserFromLog extrai o email do log do V2Ray
func (m *MonitorService) extractUserFromLog(line string) (string, bool) {
	// Usar regex pre-compilado (evita recompilação a cada linha)
	matches := m.v2rayLogRegex.FindStringSubmatch(line)
	if len(matches) > 3 {
		return matches[3], matches[2] == "accepted"
	}
	return "", false
}

// extractTimestampFromLog extrai o timestamp do log do V2Ray
// Formato esperado: 2025/11/05 12:10:04.764929 ou 2025/11/05 12:10:04
func (m *MonitorService) extractTimestampFromLog(line string) time.Time {
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return time.Time{}
	}

	loc, err := time.LoadLocation("Local")
	if err != nil {
		loc = time.UTC // Fallback para UTC se não conseguir carregar local
	}

	// Tentar parsear com microssegundos primeiro (formato mais comum)
	timeStr := parts[0] + " " + parts[1]
	timestamp, err := time.ParseInLocation("2006/01/02 15:04:05.000000", timeStr, loc)
	if err == nil {
		return timestamp
	}

	// Se falhar, tentar sem microssegundos
	timestamp, err = time.ParseInLocation("2006/01/02 15:04:05", timeStr, loc)
	if err == nil {
		return timestamp
	}

	// Se ainda falhar, retornar zero time
	return time.Time{}
}

// GetSystemResources retorna informações de recursos do sistema (CPU, RAM e total de contas criadas/expiradas)
func (m *MonitorService) GetSystemResources() models.SystemResources {
	memInfo := m.getMemoryInfo()
	cpuInfo := m.getCPUInfo()
	accountsInfo := m.getAccountsInfo()

	return models.SystemResources{
		Memory:        memInfo,
		CPU:           cpuInfo,
		TotalAccounts: accountsInfo.Total,
		TotalExpired:  accountsInfo.TotalExpired,
		Accounts:      accountsInfo,
	}
}

// getAccountsInfo obtém a contagem de contas criadas e expiradas (SSH e V2Ray)
func (m *MonitorService) getAccountsInfo() models.AccountsInfo {
	totalSSH := utils.CountTotalSSHUsers()
	expiredSSH := utils.CountTotalExpiredSSHUsers()

	totalV2Ray, expiredV2Ray := m.getV2RayAccountsCount()

	return models.AccountsInfo{
		TotalSSH:     totalSSH,
		TotalV2Ray:   totalV2Ray,
		Total:        totalSSH + totalV2Ray,
		ExpiredSSH:   expiredSSH,
		ExpiredV2Ray: expiredV2Ray,
		TotalExpired: expiredSSH + expiredV2Ray,
	}
}

// getV2RayAccountsCount obtém a contagem de contas V2Ray totais e expiradas
func (m *MonitorService) getV2RayAccountsCount() (total int, expired int) {
	if m.v2rayConfigPath == "" {
		return 0, 0
	}
	content, err := os.ReadFile(m.v2rayConfigPath)
	if err != nil {
		m.mutex.RLock()
		total = len(m.v2rayUUIDCache)
		m.mutex.RUnlock()
		return total, 0
	}
	var config map[string]interface{}
	if err := json.Unmarshal(content, &config); err != nil {
		m.mutex.RLock()
		total = len(m.v2rayUUIDCache)
		m.mutex.RUnlock()
		return total, 0
	}

	now := time.Now().Truncate(time.Minute)
	inbounds, ok := config["inbounds"].([]interface{})
	if !ok {
		return 0, 0
	}

	seenUUIDs := make(map[string]bool)
	for _, inbound := range inbounds {
		inboundMap, ok := inbound.(map[string]interface{})
		if !ok {
			continue
		}
		settings, ok := inboundMap["settings"].(map[string]interface{})
		if !ok {
			continue
		}
		clients, ok := settings["clients"].([]interface{})
		if !ok {
			continue
		}
		for _, c := range clients {
			clientMap, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			uuid, _ := clientMap["id"].(string)
			if uuid != "" && seenUUIDs[uuid] {
				continue
			}
			if uuid != "" {
				seenUUIDs[uuid] = true
			}
			total++
			if expStr, ok := clientMap["expiration_date"].(string); ok && expStr != "" {
				if expTime, err := time.Parse(time.RFC3339, expStr); err == nil {
					if !expTime.After(now) {
						expired++
					}
				}
			}
		}
	}
	return total, expired
}

// getMemoryInfo obtém informações de memória usando cat /proc/meminfo
func (m *MonitorService) getMemoryInfo() models.MemoryInfo {
	cmd := exec.Command("cat", "/proc/meminfo")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("❌ Erro ao ler /proc/meminfo: %v", err)
		return models.MemoryInfo{}
	}

	var total, available, free, cached, buffers uint64

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		key := fields[0]
		value := fields[1]

		// Converter KB para uint64
		val, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			continue
		}

		switch key {
		case "MemTotal:":
			total = val
		case "MemAvailable:":
			available = val
		case "MemFree:":
			free = val
		case "Cached:":
			cached = val
		case "Buffers:":
			buffers = val
		}
	}

	// Calcular memória usada
	used := total - free - cached - buffers
	if available > 0 {
		used = total - available
	}

	// Calcular percentual de uso
	usagePercent := 0.0
	if total > 0 {
		usagePercent = float64(used) / float64(total) * 100.0
	}

	return models.MemoryInfo{
		Total:        total,
		Available:    available,
		Used:         used,
		Free:         free,
		UsagePercent: usagePercent,
	}
}

// getCPUInfo obtém informações de CPU usando cat /proc/stat
func (m *MonitorService) getCPUInfo() models.CPUInfo {
	// Primeira leitura
	cmd1 := exec.Command("cat", "/proc/stat")
	output1, err := cmd1.Output()
	if err != nil {
		log.Printf("❌ Erro ao ler /proc/stat: %v", err)
		return models.CPUInfo{}
	}

	// Aguardar 1 segundo
	time.Sleep(1 * time.Second)

	// Segunda leitura
	cmd2 := exec.Command("cat", "/proc/stat")
	output2, err := cmd2.Output()
	if err != nil {
		log.Printf("❌ Erro ao ler /proc/stat: %v", err)
		return models.CPUInfo{}
	}

	// Parse das duas leituras
	stats1 := m.parseCPUStat(string(output1))
	stats2 := m.parseCPUStat(string(output2))

	// Calcular uso percentual
	usagePercent := m.calculateCPUUsage(stats1, stats2)
	cores := countLogicalCPUsFromProcStat(string(output1))

	return models.CPUInfo{
		Cores:        cores,
		UsagePercent: usagePercent,
		User:         stats2["user"],
		Nice:         stats2["nice"],
		System:       stats2["system"],
		Idle:         stats2["idle"],
		IOWait:       stats2["iowait"],
		IRQ:          stats2["irq"],
		SoftIRQ:      stats2["softirq"],
		Steal:        stats2["steal"],
	}
}

// countLogicalCPUsFromProcStat conta linhas cpuN em /proc/stat (CPUs lógicas).
func countLogicalCPUsFromProcStat(procStat string) int {
	n := 0
	for _, line := range strings.Split(procStat, "\n") {
		line = strings.TrimSpace(line)
		if len(line) < 5 || !strings.HasPrefix(line, "cpu") {
			continue
		}
		// Ignora a linha agregada "cpu ..."; aceita apenas "cpu0", "cpu1", ...
		switch line[3] {
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			n++
		}
	}
	return n
}

// parseCPUStat faz parse das estatísticas de CPU de /proc/stat
func (m *MonitorService) parseCPUStat(output string) map[string]uint64 {
	stats := make(map[string]uint64)

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 8 {
			continue
		}

		stats["user"], _ = strconv.ParseUint(fields[1], 10, 64)
		stats["nice"], _ = strconv.ParseUint(fields[2], 10, 64)
		stats["system"], _ = strconv.ParseUint(fields[3], 10, 64)
		stats["idle"], _ = strconv.ParseUint(fields[4], 10, 64)
		stats["iowait"], _ = strconv.ParseUint(fields[5], 10, 64)
		stats["irq"], _ = strconv.ParseUint(fields[6], 10, 64)
		stats["softirq"], _ = strconv.ParseUint(fields[7], 10, 64)
		if len(fields) > 8 {
			stats["steal"], _ = strconv.ParseUint(fields[8], 10, 64)
		}
		break
	}

	return stats
}

// calculateCPUUsage calcula o uso percentual da CPU
func (m *MonitorService) calculateCPUUsage(stats1, stats2 map[string]uint64) float64 {
	// Calcular total de ticks
	total1 := stats1["user"] + stats1["nice"] + stats1["system"] + stats1["idle"] + stats1["iowait"] + stats1["irq"] + stats1["softirq"] + stats1["steal"]
	total2 := stats2["user"] + stats2["nice"] + stats2["system"] + stats2["idle"] + stats2["iowait"] + stats2["irq"] + stats2["softirq"] + stats2["steal"]

	idle1 := stats1["idle"]
	idle2 := stats2["idle"]

	totalDiff := float64(total2 - total1)
	idleDiff := float64(idle2 - idle1)

	if totalDiff == 0 {
		return 0.0
	}

	usage := (1.0 - idleDiff/totalDiff) * 100.0

	if usage < 0 {
		return 0.0
	}
	if usage > 100 {
		return 100.0
	}

	return usage
}

func parseEnvBool(key string, defaultVal bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return defaultVal
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return defaultVal
	}
}
