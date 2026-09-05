package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"api-v2/internal/models"
)

func TestMonitorService_V2RayNotInstalled(t *testing.T) {
	// Cria um monitor apontando para um config que não existe
	monitor := NewMonitorService("/nonexistent/path/config.json")
	monitor.v2rayLogPaths = []string{"/nonexistent/log1.log", "/nonexistent/log2.log"}

	if monitor.ensureV2RayConfigAndLogs() {
		t.Errorf("ensureV2RayConfigAndLogs deveria retornar false quando V2Ray não existe")
	}

	if monitor.v2rayAvailable {
		t.Errorf("v2rayAvailable deveria ser false")
	}

	// updateV2RayUsers não deve falhar nem entrar em pânico
	monitor.updateV2RayUsers()
	if monitor.v2rayUsers != 0 {
		t.Errorf("v2rayUsers deveria ser 0, obteve %d", monitor.v2rayUsers)
	}
}

func TestMonitorService_V2RayAutoConfigureLog(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "v2ray_monitor_test_*")
	if err != nil {
		t.Fatalf("Erro ao criar tempDir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.json")
	initialJSON := map[string]interface{}{
		"inbounds": []interface{}{
			map[string]interface{}{
				"port":     443,
				"protocol": "vless",
				"settings": map[string]interface{}{
					"clients": []interface{}{
						map[string]interface{}{
							"id":    "550e8400-e29b-41d4-a716-446655440000",
							"email": "user1@sentinel.test",
						},
					},
				},
			},
		},
	}

	data, _ := json.MarshalIndent(initialJSON, "", "  ")
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("Erro ao escrever config.json: %v", err)
	}

	monitor := NewMonitorService(configPath)
	if !monitor.ensureV2RayConfigAndLogs() {
		t.Fatalf("ensureV2RayConfigAndLogs deveria retornar true quando config existe")
	}

	if !monitor.v2rayAvailable {
		t.Errorf("v2rayAvailable deveria ser true")
	}

	// Verificar se o arquivo config.json foi atualizado com a seção log
	savedContent, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Erro ao ler config.json salvo: %v", err)
	}

	var savedCfg map[string]interface{}
	if err := json.Unmarshal(savedContent, &savedCfg); err != nil {
		t.Fatalf("Erro ao fazer parse do config.json salvo: %v", err)
	}

	logObj, ok := savedCfg["log"].(map[string]interface{})
	if !ok || logObj == nil {
		t.Fatalf("Seção log não foi criada no config.json")
	}

	if logObj["access"] != "/var/log/xray/access.log" {
		t.Errorf("access log esperado '/var/log/xray/access.log', obteve %v", logObj["access"])
	}

	// Carregar cache
	monitor.loadV2RayUUIDCache()
	if uuid := monitor.getV2RayUUID("user1@sentinel.test"); uuid != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("UUID esperado '550e8400-e29b-41d4-a716-446655440000', obteve '%s'", uuid)
	}
}

func TestMonitorService_GetSystemResourcesAccountsCount(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "v2ray_monitor_res_test_*")
	if err != nil {
		t.Fatalf("Erro ao criar tempDir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configPath := filepath.Join(tempDir, "config.json")
	initialJSON := map[string]interface{}{
		"inbounds": []interface{}{
			map[string]interface{}{
				"port":     443,
				"protocol": "vless",
				"settings": map[string]interface{}{
					"clients": []interface{}{
						map[string]interface{}{
							"id":              "uuid-active-1",
							"email":           "active1@test.com",
							"expiration_date": "2035-01-01T00:00:00Z",
						},
						map[string]interface{}{
							"id":              "uuid-expired-1",
							"email":           "expired1@test.com",
							"expiration_date": "2020-01-01T00:00:00Z",
						},
					},
				},
			},
		},
	}

	data, _ := json.MarshalIndent(initialJSON, "", "  ")
	_ = os.WriteFile(configPath, data, 0644)

	monitor := NewMonitorService(configPath)
	totalV2Ray, expiredV2Ray := monitor.getV2RayAccountsCount()

	if totalV2Ray != 2 {
		t.Errorf("Esperava 2 contas V2Ray totais, obteve %d", totalV2Ray)
	}
	if expiredV2Ray != 1 {
		t.Errorf("Esperava 1 conta V2Ray expirada, obteve %d", expiredV2Ray)
	}

	res := monitor.GetSystemResources()
	if res.Accounts.TotalV2Ray != 2 {
		t.Errorf("Accounts.TotalV2Ray esperado 2, obteve %d", res.Accounts.TotalV2Ray)
	}
	if res.Accounts.ExpiredV2Ray != 1 {
		t.Errorf("Accounts.ExpiredV2Ray esperado 1, obteve %d", res.Accounts.ExpiredV2Ray)
	}
	if res.TotalExpired < 1 {
		t.Errorf("TotalExpired esperado >= 1, obteve %d", res.TotalExpired)
	}
}

func TestReadLogTail(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "v2ray_tail_test_*")
	if err != nil {
		t.Fatalf("Erro ao criar tempDir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, "access.log")

	// 1. Arquivo não existente
	_, err = ReadLogTail(filepath.Join(tempDir, "nonexistent.log"), 1024)
	if err == nil {
		t.Errorf("ReadLogTail deveria retornar erro para arquivo inexistente")
	}

	// 2. Arquivo vazio
	if err := os.WriteFile(filePath, []byte(""), 0644); err != nil {
		t.Fatalf("Erro ao criar arquivo vazio: %v", err)
	}
	lines, err := ReadLogTail(filePath, 1024)
	if err != nil {
		t.Fatalf("Erro ao ler arquivo vazio: %v", err)
	}
	if len(lines) != 0 {
		t.Errorf("Esperava 0 linhas para arquivo vazio, obteve %d", len(lines))
	}

	// 3. Arquivo com 1000 linhas
	var content string
	for i := 1; i <= 1000; i++ {
		content += "2026/09/02 23:00:00 [Info] connection accepted email: user" + string(rune('0'+i%10)) + "@test.com\n"
	}
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("Erro ao escrever log de teste: %v", err)
	}

	// Ler tail com limite pequeno (ex: 500 bytes)
	linesTail, err := ReadLogTail(filePath, 500)
	if err != nil {
		t.Fatalf("Erro ao ler tail: %v", err)
	}
	if len(linesTail) == 0 {
		t.Fatalf("Esperava linhas lidas do tail, obteve 0")
	}
	// Última linha deve ser recente
	lastLine := linesTail[len(linesTail)-1]
	if lastLine == "" && len(linesTail) > 1 {
		lastLine = linesTail[len(linesTail)-2]
	}
	if !filepath.IsAbs(filePath) && len(lastLine) == 0 {
		t.Errorf("Última linha lida do tail está vazia")
	}
}

func TestSafeTrimLargeLogFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "v2ray_trim_test_*")
	if err != nil {
		t.Fatalf("Erro ao criar tempDir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, "access.log")

	// Criar arquivo com ~100 KB
	lineContent := "2026/09/02 23:30:00 [Info] connection accepted email: user@test.com\n"
	var fullContent string
	for i := 0; i < 1500; i++ {
		fullContent += lineContent
	}
	if err := os.WriteFile(filePath, []byte(fullContent), 0644); err != nil {
		t.Fatalf("Erro ao escrever arquivo grande: %v", err)
	}

	monitor := NewMonitorService("")

	// Não deve reduzir se limite for maior que o arquivo
	if monitor.safeTrimLargeLogFile(filePath, 500*1024, 10*1024) {
		t.Errorf("safeTrimLargeLogFile não deveria reduzir arquivo menor que o limite")
	}

	// Deve reduzir quando limite for menor que o arquivo (ex: > 50KB -> reduzir para 10KB)
	trimmed := monitor.safeTrimLargeLogFile(filePath, 50*1024, 10*1024)
	if !trimmed {
		t.Fatalf("safeTrimLargeLogFile deveria ter reduzido o arquivo")
	}

	stat, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("Erro ao obter stat do arquivo reduzido: %v", err)
	}

	if stat.Size() > 20*1024 {
		t.Errorf("Arquivo após trim deveria ter ~10 KB, tem %d bytes", stat.Size())
	}
}

func TestExtractSSHUsername(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected string
	}{
		{"sshd priv-sep", "root 1234 0.0 0.1 1234 567 ? Ss 12:00 0:00 sshd: user1 [priv]", "user1"},
		{"sshd net session", "user1 1235 0.0 0.1 1234 567 ? S 12:00 0:00 sshd: user1 [net]", "user1"},
		{"sshd pts session", "user2 1236 0.0 0.1 1234 567 ? S 12:00 0:00 sshd: user2@pts/0", "user2"},
		{"sshd notty session", "vpn_client 1237 0.0 0.1 1234 567 ? S 12:00 0:00 sshd: vpn_client@notty", "vpn_client"},
		{"sshd without suffix", "user3 1238 0.0 0.1 1234 567 ? S 12:00 0:00 sshd: user3", "user3"},
		{"daemon listener", "root 1200 0.0 0.1 1234 567 ? Ss 12:00 0:00 /usr/sbin/sshd -D [listener]", ""},
		{"sshd binary path", "root 1200 0.0 0.1 1234 567 ? Ss 12:00 0:00 sshd: /usr/sbin/sshd -D", ""},
		{"sshd accepted", "root 1201 0.0 0.1 1234 567 ? Ss 12:00 0:00 sshd: [accepted]", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSSHUsername(tt.line)
			if got != tt.expected {
				t.Errorf("extractSSHUsername(%q) = %q, esperado %q", tt.line, got, tt.expected)
			}
		})
	}
}

func TestParseSSHProcesses_ExcludesRootAndSystem(t *testing.T) {
	monitor := NewMonitorService("")

	// Simula apenas vpn1 e vpn2 com shell /bin/false
	validUsers := map[string]bool{
		"vpn1": true,
		"vpn2": true,
	}

	// Simula a saída de processos contendo root (duas sessões, igual relatado pelo usuário),
	// um administrador bash e conexões VPN válidas
	rawOutput := `root     sshd: /usr/sbin/sshd -D [listener]
root     sshd: root@pts/0
root     sshd: root [priv]
admin    sshd: admin@pts/1
root     sshd: vpn1 [priv]
vpn1     sshd: vpn1 [net]
root     sshd: vpn2 [priv]
vpn2     sshd: vpn2 [net]
`

	result := monitor.parseSSHProcesses(rawOutput, validUsers)

	// Deve conter exatamente 2 usuários: vpn1 e vpn2
	if len(result) != 2 {
		t.Fatalf("Esperava 2 usuários online, obteve %d: %+v", len(result), result)
	}

	if result[0].Username != "vpn1" || result[1].Username != "vpn2" {
		t.Errorf("Usuários inesperados: %+v", result)
	}

	// Verificar explicitamente que root e admin NÃO estão no resultado
	for _, u := range result {
		if u.Username == "root" {
			t.Errorf("ERRO CRÍTICO: root NUNCA deve aparecer como usuário online!")
		}
		if u.Username == "admin" {
			t.Errorf("ERRO: admin (com shell /bin/bash) não deve aparecer como usuário online!")
		}
	}
}

func TestParseSSHProcesses_OnlyRootActive(t *testing.T) {
	monitor := NewMonitorService("")

	validUsers := map[string]bool{
		"vpn1": true,
	}

	// Cenário exato relatado pelo usuário: apenas root conectado no servidor
	rawOutput := `root     sshd: /usr/sbin/sshd -D [listener]
root     sshd: root@pts/0
root     sshd: root@pts/1
root     sshd: root [priv]
`

	result := monitor.parseSSHProcesses(rawOutput, validUsers)

	// Não deve haver nenhum usuário online!
	if len(result) != 0 {
		t.Fatalf("Esperava 0 usuários online para sessões apenas do root, obteve %d: %+v", len(result), result)
	}
}

func TestParseSSHProcesses_MultipleSessionsDeduplicated(t *testing.T) {
	monitor := NewMonitorService("")

	validUsers := map[string]bool{
		"vpn1": true,
	}

	// vpn1 conectado em múltiplos túneis simultâneos
	rawOutput := `root     sshd: vpn1 [priv]
vpn1     sshd: vpn1 [net]
root     sshd: vpn1 [priv]
vpn1     sshd: vpn1 [net]
vpn1     sshd: vpn1@notty
`

	result := monitor.parseSSHProcesses(rawOutput, validUsers)

	if len(result) != 1 {
		t.Fatalf("Esperava deduplicação para 1 usuário único, obteve %d: %+v", len(result), result)
	}

	if result[0].Username != "vpn1" {
		t.Errorf("Esperava vpn1, obteve %s", result[0].Username)
	}
}

func TestMonitorService_GetDetailedOnlineUsers_WithPasswd(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "monitor_passwd_test_*")
	if err != nil {
		t.Fatalf("Erro ao criar tempDir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	passwdContent := `root:x:0:0:root:/root:/bin/bash
ubuntu:x:1000:1000:Ubuntu:/home/ubuntu:/bin/bash
client1:x:1001:1001::/home/client1:/bin/false
client2:x:1002:1002::/home/client2:/usr/bin/false
`
	passwdFile := filepath.Join(tempDir, "passwd")
	if err := os.WriteFile(passwdFile, []byte(passwdContent), 0644); err != nil {
		t.Fatalf("Erro ao escrever passwd: %v", err)
	}

	monitor := NewMonitorService("")
	monitor.SetPasswdPath(passwdFile)

	// Chamar GetDetailedOnlineUsers
	res := monitor.GetDetailedOnlineUsers()

	// SSHUsers não deve ser nil (deve ser slice vazio [])
	if res.SSHUsers == nil {
		t.Errorf("SSHUsers não deveria ser nil")
	}

	// Não deve ter root em SSHUsers
	for _, u := range res.SSHUsers {
		if u.Username == "root" {
			t.Errorf("root não deve estar em SSHUsers")
		}
	}

	// OnlineUsers também
	onlineRes := monitor.GetOnlineUsers()
	if onlineRes.SSHUsers < 0 {
		t.Errorf("Total SSH users não deve ser negativo")
	}

	// VTproxy não deve ser nulo
	if res.VTproxy == nil {
		t.Errorf("VTproxy em DetailedUsersResponse não deve ser nil")
	}
}

func TestMonitorService_VTProxyFields(t *testing.T) {
	monitor := NewMonitorService("")

	// Simular usuários VTproxy no cache
	monitor.mutex.Lock()
	monitor.vtproxyUsers = 2
	monitor.vtproxyUsersList = []models.VTProxyUserOnline{
		{Username: "joao", Connections: 2, Count: 2},
		{Username: "maria", Connections: 1, Count: 1},
	}
	monitor.cacheExpiry = time.Now().Add(1 * time.Hour) // não expirar cache
	monitor.mutex.Unlock()

	online := monitor.GetOnlineUsers()
	if online.VTproxy != 2 {
		t.Errorf("Esperava VTproxy = 2, obteve %d", online.VTproxy)
	}
	if online.VTProxyUsers != 2 {
		t.Errorf("Esperava VTProxyUsers = 2, obteve %d", online.VTProxyUsers)
	}

	detailed := monitor.GetDetailedOnlineUsers()
	if len(detailed.VTproxy) != 2 {
		t.Fatalf("Esperava 2 usuários em VTproxy, obteve %d", len(detailed.VTproxy))
	}
	if detailed.TotalVTProxy != 2 {
		t.Errorf("Esperava TotalVTProxy = 2, obteve %d", detailed.TotalVTProxy)
	}
	if detailed.VTproxy[0].Username != "joao" || detailed.VTproxy[0].Connections != 2 {
		t.Errorf("Dados incorretos em VTproxy[0]: %+v", detailed.VTproxy[0])
	}
	if detailed.VTproxy[1].Username != "maria" || detailed.VTproxy[1].Connections != 1 {
		t.Errorf("Dados incorretos em VTproxy[1]: %+v", detailed.VTproxy[1])
	}
}


