package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

const DefaultProxyServerBin = "/usr/local/bin/proxy-server"

// ProxyKillDisabled pode ser ativado em testes unitários para não tentar executar comandos no host.
var ProxyKillDisabled = false

// ProxyOnlinesResult representa a estrutura retornada por /usr/local/bin/proxy-server --onlines-json
type ProxyOnlinesResult struct {
	Total     int            `json:"total"`
	Users     map[string]int `json:"users"`
	UpdatedAt string         `json:"updatedAt"`
}

// GetProxyOnlineUsers executa /usr/local/bin/proxy-server --onlines-json e faz parse do resultado
func GetProxyOnlineUsers() (ProxyOnlinesResult, error) {
	if ProxyKillDisabled {
		return ProxyOnlinesResult{Users: make(map[string]int)}, nil
	}

	binPath := GetProxyServerBinPath()
	if _, err := os.Stat(binPath); err != nil {
		return ProxyOnlinesResult{Users: make(map[string]int)}, fmt.Errorf("binário proxy-server não encontrado em %s: %w", binPath, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath, "--onlines-json")
	output, err := cmd.Output()
	if err != nil {
		return ProxyOnlinesResult{Users: make(map[string]int)}, fmt.Errorf("erro ao executar %s --onlines-json: %w", binPath, err)
	}

	raw := strings.TrimSpace(string(output))
	startIdx := strings.Index(raw, "{")
	lastIdx := strings.LastIndex(raw, "}")
	if startIdx != -1 && lastIdx != -1 && lastIdx > startIdx {
		raw = raw[startIdx : lastIdx+1]
	}

	var result ProxyOnlinesResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return ProxyOnlinesResult{Users: make(map[string]int)}, fmt.Errorf("erro ao decodificar JSON do proxy-server: %w (raw: %s)", err, raw)
	}

	if result.Users == nil {
		result.Users = make(map[string]int)
	}

	return result, nil
}

// GetProxyServerBinPath retorna o caminho do executável do proxy-server configurado ou o padrão.
func GetProxyServerBinPath() string {
	if p := strings.TrimSpace(os.Getenv("PROXY_SERVER_BIN")); p != "" {
		return p
	}
	return DefaultProxyServerBin
}

// ProxyServerKillUser executa /usr/local/bin/proxy-server --kill-user=<username>
// Retorna a saída do comando ou erro caso o comando falhe.
func ProxyServerKillUser(username string) (string, error) {
	if ProxyKillDisabled || username == "" || IsReservedUsername(username) {
		return "", nil
	}

	binPath := GetProxyServerBinPath()
	if _, err := os.Stat(binPath); err != nil {
		// Binário não encontrado no host atual (ambiente de desenvolvimento ou proxy em outro caminho)
		return "", fmt.Errorf("binário proxy-server não encontrado em %s: %w", binPath, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath, "--kill-user="+username)
	output, err := cmd.CombinedOutput()
	outStr := strings.TrimSpace(string(output))
	if err != nil {
		return outStr, fmt.Errorf("falha ao executar proxy-server --kill-user=%s: %w (%s)", username, err, outStr)
	}

	return outStr, nil
}

// ProxyServerKillUsers derruba conexões ativas no proxy para uma lista de usernames.
func ProxyServerKillUsers(usernames []string) {
	if len(usernames) == 0 || ProxyKillDisabled {
		return
	}

	binPath := GetProxyServerBinPath()
	if _, err := os.Stat(binPath); err != nil {
		// Silencioso se o proxy não estiver instalado neste host
		return
	}

	for _, username := range usernames {
		if username == "" || IsReservedUsername(username) {
			continue
		}
		out, err := ProxyServerKillUser(username)
		if err != nil {
			log.Printf("⚠️ [Proxy Kill] Erro ao derrubar conexão do usuário %s: %v", username, err)
		} else if out != "" {
			log.Printf("🔌 [Proxy Kill] %s", out)
		}
	}
}
