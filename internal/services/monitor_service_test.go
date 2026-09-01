package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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

