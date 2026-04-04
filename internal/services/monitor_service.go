package services

import (
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"api-v2/internal/models"
)

// MonitorService implementa o serviço de monitoramento de usuários online
type MonitorService struct {
	sshUsers    int
	v2rayUsers  int
	dtProtoUsers int
	mutex         sync.RWMutex
	stopChan      chan bool

	// Cache detalhado de usuários online
	sshUsersList    []models.SSHUserOnline
	v2rayUsersList  []models.V2RayUserOnline
	dtProtoUsersList []models.DTProtoUserOnline

	// Cache de UUIDs V2Ray (email -> uuid) - pre-alocado
	v2rayUUIDCache map[string]string

	// Caminhos possíveis para logs V2Ray
	v2rayLogPaths  []string
	currentLogPath string

	// Caminho do config V2Ray (injetado)
	v2rayConfigPath string

	// Cache com TTL para evitar leituras excessivas
	cacheExpiry   time.Time
	cacheDuration time.Duration
	// Regex pre-compilado para evitar recompilação a cada linha
	v2rayLogRegex *regexp.Regexp
}

// NewMonitorService cria uma nova instância do serviço de monitoramento
func NewMonitorService(v2rayConfigPath string) *MonitorService {
	// Pre-compilar regex para extração de logs V2Ray (evita recompilação a cada linha)
	v2rayLogRegex := regexp.MustCompile(`(\d{4}\/\d{2}\/\d{2} \d{2}:\d{2}:\d{2}).*?(accepted|rejected).*?email:\s*([\w._%+-]+@[\w.-]+\.[a-zA-Z]{2,})`)

	return &MonitorService{
		sshUsers:        0,
		v2rayUsers:      0,
		dtProtoUsers:    0,
		stopChan:        make(chan bool),
		v2rayUUIDCache:  make(map[string]string, 100),
		sshUsersList:    make([]models.SSHUserOnline, 0, 50),
		v2rayUsersList:  make([]models.V2RayUserOnline, 0, 100),
		dtProtoUsersList: make([]models.DTProtoUserOnline, 0, 100),
		cacheDuration:   10 * time.Second,
		v2rayLogRegex:   v2rayLogRegex,
		v2rayConfigPath: v2rayConfigPath,
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

// Start inicia o serviço de monitoramento
func (m *MonitorService) Start() {
	log.Println("🚀 Iniciando serviço de monitoramento de usuários online...")

	// Encontrar arquivo de log V2Ray disponível
	m.findV2RayLogFile()

	// Carregar cache de UUIDs V2Ray
	m.loadV2RayUUIDCache()

	// Iniciar goroutines de monitoramento
	go m.monitorSSHUsers()
	go m.monitorV2RayUsers()
	go m.monitorDTProtoUsers()
	go m.cleanV2RayLogs()
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
	m.mutex.Unlock()

	log.Println("✅ Serviço de monitoramento parado")
}

// GetOnlineUsers retorna os usuários online do cache
func (m *MonitorService) GetOnlineUsers() models.OnlineUsersResponse {
	m.mutex.RLock()

	// Verificar se o cache ainda é válido
	if time.Now().Before(m.cacheExpiry) {
		defer m.mutex.RUnlock()
		return models.NewOnlineUsersResponse(m.sshUsers, m.v2rayUsers, m.dtProtoUsers)
	}
	m.mutex.RUnlock()

	// Cache expirado, atualizar
	m.updateV2RayUsers()
	m.updateDTProtoUsers()

	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return models.NewOnlineUsersResponse(m.sshUsers, m.v2rayUsers, m.dtProtoUsers)
}

// GetDetailedOnlineUsers retorna a lista detalhada de usuários online do cache
func (m *MonitorService) GetDetailedOnlineUsers() models.DetailedUsersResponse {
	m.mutex.RLock()

	// Verificar se o cache ainda é válido
	if time.Now().Before(m.cacheExpiry) {
		v2rayList := m.v2rayUsersList
		if v2rayList == nil {
			v2rayList = []models.V2RayUserOnline{}
		}
		dtProtoList := m.dtProtoUsersList
		if dtProtoList == nil {
			dtProtoList = []models.DTProtoUserOnline{}
		}
		defer m.mutex.RUnlock()
		return models.NewDetailedUsersResponse(m.sshUsersList, v2rayList, dtProtoList)
	}
	m.mutex.RUnlock()

	// Cache expirado, atualizar
	m.updateV2RayUsers()
	m.updateDTProtoUsers()

	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Garantir que slices não sejam nil
	v2rayList := m.v2rayUsersList
	if v2rayList == nil {
		v2rayList = []models.V2RayUserOnline{}
	}
	dtProtoList := m.dtProtoUsersList
	if dtProtoList == nil {
		dtProtoList = []models.DTProtoUserOnline{}
	}

	return models.NewDetailedUsersResponse(m.sshUsersList, v2rayList, dtProtoList)
}

// findV2RayLogFile encontra o primeiro arquivo de log V2Ray disponível
func (m *MonitorService) findV2RayLogFile() {
	for _, path := range m.v2rayLogPaths {
		if _, err := os.Stat(path); err == nil {
			m.currentLogPath = path
			log.Printf("📁 Arquivo de log V2Ray encontrado: %s", path)
			return
		}
	}
	log.Println("⚠️  Nenhum arquivo de log V2Ray encontrado nos caminhos padrão")
}

// loadV2RayUUIDCache carrega o cache de UUIDs do arquivo de configuração V2Ray
func (m *MonitorService) loadV2RayUUIDCache() {
	content, err := os.ReadFile(m.v2rayConfigPath)
	if err != nil {
		log.Printf("❌ Erro ao ler arquivo de configuração V2Ray (%s): %v", m.v2rayConfigPath, err)
		return
	}

	var config map[string]interface{}
	if err := json.Unmarshal(content, &config); err != nil {
		log.Printf("❌ Erro ao fazer parse do JSON de configuração V2Ray: %v", err)
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

	// Re-criar cache com tamanho conhecido para evitar realocações
	if estimatedUsers > 0 {
		m.v2rayUUIDCache = make(map[string]string, estimatedUsers)
	}

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
										m.v2rayUUIDCache[email] = uuid
									}
								}
							}
						}
					}
				}
			}
		}
	}

	log.Printf("📋 Cache de UUIDs V2Ray carregado: %d usuários", len(m.v2rayUUIDCache))
}

// getV2RayUUID busca o UUID de um usuário V2Ray pelo email
func (m *MonitorService) getV2RayUUID(email string) string {
	if uuid, exists := m.v2rayUUIDCache[email]; exists {
		return uuid
	}
	return ""
}

// reloadV2RayUUIDCache recarrega o cache de UUIDs periodicamente
func (m *MonitorService) reloadV2RayUUIDCache() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.loadV2RayUUIDCache()
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

// getSSHUsersList obtém a lista detalhada de usuários SSH online
func (m *MonitorService) getSSHUsersList() []models.SSHUserOnline {
	// Comando otimizado: processar apenas sshd diretamente, sem listar tudo
	// pgrep retorna apenas PIDs de processos sshd, depois obtemos detalhes
	cmd := exec.Command("sh", "-c", "pgrep -f 'sshd:' | xargs -r ps -o user= 2>/dev/null | grep -v '^root$' | sort -u || true")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("❌ Erro ao executar comando SSH otimizado: %v", err)
		// Fallback para comando 'who'
		return m.getSSHUsersListFallback()
	}

	// Se não há output (nenhum usuário SSH), usar fallback
	if len(strings.TrimSpace(string(output))) == 0 {
		log.Printf("ℹ️ Nenhum usuário SSH encontrado pelo comando otimizado, usando fallback")
		return m.getSSHUsersListFallback()
	}

	lines := strings.Split(string(output), "\n")
	var users []models.SSHUserOnline
	userMap := make(map[string]models.SSHUserOnline, 50) // Pre-alocar para ~50 usuários SSH

	// Comando otimizado já retorna usernames únicos, sem duplicatas
	for _, line := range lines {
		username := strings.TrimSpace(line)
		if username == "" {
			continue
		}

		// Usar username como chave única (evitar duplicatas)
		if _, exists := userMap[username]; !exists {
			userMap[username] = models.SSHUserOnline{
				Username: username,
			}
		}
	}

	// Converter map para slice
	for _, user := range userMap {
		users = append(users, user)
	}

	return users
}

// getSSHUsersListFallback fallback usando comando 'who'
func (m *MonitorService) getSSHUsersListFallback() []models.SSHUserOnline {
	cmd := exec.Command("who")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("❌ Erro ao executar comando 'who': %v", err)
		return []models.SSHUserOnline{}
	}

	lines := strings.Split(string(output), "\n")
	var users []models.SSHUserOnline

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse da saída do comando 'who'
		// Formato: usuario pts/0 2024-01-15 10:30 (192.168.1.100)
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		username := fields[0]

		users = append(users, models.SSHUserOnline{
			Username: username,
		})
	}

	return users
}

// updateV2RayUsers atualiza o número de usuários V2Ray online
func (m *MonitorService) updateV2RayUsers() {
	// Tentar ler todos os arquivos de log V2Ray disponíveis
	var allContent strings.Builder
	var foundLogPath string

	for _, logPath := range m.v2rayLogPaths {
		if content, err := os.ReadFile(logPath); err == nil {
			allContent.WriteString(string(content))
			allContent.WriteString("\n")
			foundLogPath = logPath
			break // Usar o primeiro arquivo encontrado
		}
	}

	if allContent.Len() == 0 {
		log.Printf("❌ Nenhum arquivo de log V2Ray encontrado nos caminhos: %v", m.v2rayLogPaths)
		m.mutex.Lock()
		m.v2rayUsers = 0
		m.v2rayUsersList = []models.V2RayUserOnline{}
		m.mutex.Unlock()
		return
	}

	lines := strings.Split(allContent.String(), "\n")
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
		log.Printf("❌ Erro ao ler /var/lib/proto-server/stats.json: %v", err)
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

// performV2RayLogCleanup executa a limpeza de logs V2Ray
func (m *MonitorService) performV2RayLogCleanup() {
	if m.currentLogPath == "" {
		return
	}

	log.Println("🧹 Iniciando limpeza de logs V2Ray antigos...")

	threshold := time.Now().Add(-12 * time.Hour)
	content, err := os.ReadFile(m.currentLogPath)
	if err != nil {
		log.Printf("❌ Erro ao ler arquivo de log V2Ray para limpeza: %v", err)
		return
	}

	lines := strings.Split(string(content), "\n")
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

	// Escrever em arquivo temporário e rename atômico para minimizar data loss
	tmpPath := m.currentLogPath + ".cleanup.tmp"
	if err := os.WriteFile(tmpPath, []byte(strings.Join(newLogContent, "\n")), 0644); err != nil {
		log.Printf("❌ Erro ao escrever tmp de limpeza: %v", err)
		return
	}

	if err := os.Rename(tmpPath, m.currentLogPath); err != nil {
		log.Printf("❌ Erro ao renomear arquivo de log após limpeza: %v", err)
		os.Remove(tmpPath)
		return
	}

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

// GetSystemResources retorna informações de recursos do sistema (CPU e RAM)
func (m *MonitorService) GetSystemResources() models.SystemResources {
	memInfo := m.getMemoryInfo()
	cpuInfo := m.getCPUInfo()

	return models.SystemResources{
		Memory: memInfo,
		CPU:    cpuInfo,
	}
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

	return models.CPUInfo{
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
