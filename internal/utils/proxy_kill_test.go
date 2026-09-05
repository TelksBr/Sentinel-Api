package utils

import (
	"encoding/json"
	"os"
	"testing"
)

func TestProxyKill_ConfigAndDisabled(t *testing.T) {
	// Testar valor padrão
	os.Unsetenv("PROXY_SERVER_BIN")
	if p := GetProxyServerBinPath(); p != DefaultProxyServerBin {
		t.Errorf("GetProxyServerBinPath() = %s, esperava %s", p, DefaultProxyServerBin)
	}

	// Testar customizado por variável de ambiente
	os.Setenv("PROXY_SERVER_BIN", "/custom/bin/proxy-server")
	if p := GetProxyServerBinPath(); p != "/custom/bin/proxy-server" {
		t.Errorf("GetProxyServerBinPath() = %s, esperava /custom/bin/proxy-server", p)
	}
	os.Unsetenv("PROXY_SERVER_BIN")

	// Testar comportamento com ProxyKillDisabled = true
	ProxyKillDisabled = true
	defer func() {
		ProxyKillDisabled = false
	}()

	out, err := ProxyServerKillUser("test_user")
	if err != nil {
		t.Errorf("ProxyServerKillUser retornou erro com ProxyKillDisabled=true: %v", err)
	}
	if out != "" {
		t.Errorf("Esperava saída vazia, obteve %s", out)
	}

	// Testar com usuário reservado
	ProxyKillDisabled = false
	outRoot, errRoot := ProxyServerKillUser("root")
	if errRoot != nil || outRoot != "" {
		t.Errorf("ProxyServerKillUser não deve tentar matar usuário root")
	}

	// Testar binário inexistente (retorna erro gracioso, sem pânico)
	_, errNotFound := ProxyServerKillUser("random_user_123")
	if errNotFound == nil {
		t.Errorf("Esperava erro ao tentar executar binário inexistente no host")
	}

	// Testar ProxyServerKillUsers com lista vazia
	ProxyServerKillUsers([]string{})
	ProxyServerKillUsers([]string{"user1", "user2"})
}

func TestProxyOnlines_JSONParsing(t *testing.T) {
	sampleJSON := `{
  "total": 3,
  "users": {
    "joao": 2,
    "maria": 1
  },
  "updatedAt": "2026-09-05T14:30:00Z"
}`
	var result ProxyOnlinesResult
	if err := json.Unmarshal([]byte(sampleJSON), &result); err != nil {
		t.Fatalf("Erro ao decodificar JSON de exemplo: %v", err)
	}

	if result.Total != 3 {
		t.Errorf("Total = %d, esperava 3", result.Total)
	}
	if len(result.Users) != 2 {
		t.Errorf("len(Users) = %d, esperava 2", len(result.Users))
	}
	if result.Users["joao"] != 2 {
		t.Errorf("Users[joao] = %d, esperava 2", result.Users["joao"])
	}
	if result.Users["maria"] != 1 {
		t.Errorf("Users[maria] = %d, esperava 1", result.Users["maria"])
	}
	if result.UpdatedAt != "2026-09-05T14:30:00Z" {
		t.Errorf("UpdatedAt = %s, esperava 2026-09-05T14:30:00Z", result.UpdatedAt)
	}
}

