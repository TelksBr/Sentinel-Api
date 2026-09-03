package utils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigureSystemdNullLogs_InsertNew(t *testing.T) {
	initial := `[Unit]
Description=Xray Service
After=network.target

[Service]
User=nobody
ExecStart=/usr/local/bin/xray run -config /etc/xray/config.json
Restart=on-failure

[Install]
WantedBy=multi-user.target
`

	newContent, modified := ConfigureSystemdNullLogs(initial)
	if !modified {
		t.Fatalf("Esperava que modified fosse true")
	}

	if !strings.Contains(newContent, "StandardOutput=null") {
		t.Errorf("Esperava conter StandardOutput=null no resultado")
	}
	if !strings.Contains(newContent, "StandardError=null") {
		t.Errorf("Esperava conter StandardError=null no resultado")
	}

	// Certificar que foi inserido dentro de [Service]
	serviceIdx := strings.Index(newContent, "[Service]")
	installIdx := strings.Index(newContent, "[Install]")
	stdOutIdx := strings.Index(newContent, "StandardOutput=null")

	if stdOutIdx < serviceIdx || stdOutIdx > installIdx {
		t.Errorf("StandardOutput=null deveria estar entre [Service] e [Install]")
	}
}

func TestConfigureSystemdNullLogs_ReplaceExisting(t *testing.T) {
	initial := `[Unit]
Description=Xray Service

[Service]
User=nobody
StandardOutput=journal
StandardError=syslog
ExecStart=/usr/local/bin/xray run -config /etc/xray/config.json

[Install]
WantedBy=multi-user.target
`

	newContent, modified := ConfigureSystemdNullLogs(initial)
	if !modified {
		t.Fatalf("Esperava que modified fosse true")
	}

	if strings.Contains(newContent, "StandardOutput=journal") {
		t.Errorf("Não deveria mais conter StandardOutput=journal")
	}
	if strings.Contains(newContent, "StandardError=syslog") {
		t.Errorf("Não deveria mais conter StandardError=syslog")
	}
	if !strings.Contains(newContent, "StandardOutput=null") {
		t.Errorf("Esperava conter StandardOutput=null")
	}
	if !strings.Contains(newContent, "StandardError=null") {
		t.Errorf("Esperava conter StandardError=null")
	}
}

func TestConfigureSystemdNullLogs_Idempotent(t *testing.T) {
	initial := `[Unit]
Description=Xray Service

[Service]
StandardOutput=null
StandardError=null
User=nobody
ExecStart=/usr/local/bin/xray run -config /etc/xray/config.json

[Install]
WantedBy=multi-user.target
`

	newContent, modified := ConfigureSystemdNullLogs(initial)
	if modified {
		t.Errorf("Não deveria modificar um arquivo já configurado perfeitamente")
	}
	if newContent != initial {
		t.Errorf("Conteúdo não deveria ter mudado")
	}
}

func TestConfigureSystemdNullLogs_NoServiceSection(t *testing.T) {
	initial := `[Unit]
Description=Just a unit

[Install]
WantedBy=multi-user.target
`

	newContent, modified := ConfigureSystemdNullLogs(initial)
	if modified {
		t.Errorf("Não deveria modificar arquivo sem seção [Service]")
	}
	if newContent != initial {
		t.Errorf("Conteúdo não deveria ter mudado")
	}
}

func TestEnsureSystemdServiceLogsDisabled(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "systemd_test_*")
	if err != nil {
		t.Fatalf("Erro ao criar tempDir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	serviceFile := filepath.Join(tempDir, "xray.service")
	initialContent := `[Unit]
Description=Xray Service

[Service]
ExecStart=/usr/bin/xray
`
	if err := os.WriteFile(serviceFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("Erro ao salvar arquivo de teste: %v", err)
	}

	// Testar chamada direta de Configure e Save
	content, _ := os.ReadFile(serviceFile)
	newContent, modified := ConfigureSystemdNullLogs(string(content))
	if !modified {
		t.Fatalf("Esperava modified == true")
	}
	_ = os.WriteFile(serviceFile, []byte(newContent), 0644)

	reloaded, err := os.ReadFile(serviceFile)
	if err != nil {
		t.Fatalf("Erro ao reler arquivo: %v", err)
	}

	if !strings.Contains(string(reloaded), "StandardOutput=null") {
		t.Errorf("StandardOutput=null não foi persistido no arquivo")
	}
}

func TestEnsureLogFileAccessible(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "log_perm_test_*")
	if err != nil {
		t.Fatalf("Erro ao criar tempDir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logPath := filepath.Join(tempDir, "subdir", "access.log")

	// Deve criar o diretório e arquivo com sucesso
	if err := EnsureLogFileAccessible(logPath); err != nil {
		t.Fatalf("EnsureLogFileAccessible retornou erro: %v", err)
	}

	statDir, err := os.Stat(filepath.Dir(logPath))
	if err != nil {
		t.Fatalf("Diretório não foi criado: %v", err)
	}
	if !statDir.IsDir() {
		t.Errorf("Esperava diretório criado")
	}

	statFile, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("Arquivo não foi criado: %v", err)
	}
	if statFile.IsDir() {
		t.Errorf("Esperava arquivo criado, não diretório")
	}

	// Chamar novamente deve ser idempotente
	if err := EnsureLogFileAccessible(logPath); err != nil {
		t.Fatalf("Segunda chamada a EnsureLogFileAccessible falhou: %v", err)
	}

	// Caminho vazio ou "none" não deve dar erro
	if err := EnsureLogFileAccessible(""); err != nil {
		t.Errorf("EnsureLogFileAccessible com string vazia deveria retornar nil")
	}
	if err := EnsureLogFileAccessible("none"); err != nil {
		t.Errorf("EnsureLogFileAccessible com 'none' deveria retornar nil")
	}
}

