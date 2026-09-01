package services

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"api-v2/internal/models"
	"api-v2/internal/utils"
)

type mockCronScheduler struct {
	addedJobs []string
}

func (m *mockCronScheduler) AddTestCronjob(id, cronType string, hoursFromNow int) error {
	m.addedJobs = append(m.addedJobs, fmt.Sprintf("%s:%s:%d", id, cronType, hoursFromNow))
	return nil
}

func setupTestSSHService(t *testing.T) (*SSHService, func()) {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "ssh_service_test_*")
	if err != nil {
		t.Fatalf("Erro ao criar temp dir: %v", err)
	}

	initialPasswd := `root:x:0:0:root:/root:/bin/bash
sshd:x:105:65534::/run/sshd:/usr/sbin/nologin
existing1:x:1001:1001::/nonexistent:/bin/false
`
	initialShadow := `root:$6$salt$hash:19800:0:99999:7:::
sshd:*:19800:0:99999:7:::
existing1:$6$salt$userhash:19800:0:99999:7::19850:
`
	initialGroup := `root:x:0:
existing1:x:1001:
`
	initialGShadow := `root:*::
existing1:!::
`
	_ = os.WriteFile(filepath.Join(tempDir, "passwd"), []byte(initialPasswd), 0644)
	_ = os.WriteFile(filepath.Join(tempDir, "shadow"), []byte(initialShadow), 0640)
	_ = os.WriteFile(filepath.Join(tempDir, "group"), []byte(initialGroup), 0644)
	_ = os.WriteFile(filepath.Join(tempDir, "gshadow"), []byte(initialGShadow), 0640)

	store := utils.NewUnixStore(tempDir)
	service := NewSSHService()
	service.SetStore(store)

	cleanup := func() {
		_ = os.RemoveAll(tempDir)
	}
	return service, cleanup
}

func TestSSHService_CreateBatchUsers(t *testing.T) {
	service, cleanup := setupTestSSHService(t)
	defer cleanup()

	// Criar 100 usuários em lote
	users := make([]models.SSHUser, 100)
	for i := 0; i < 100; i++ {
		users[i] = models.SSHUser{
			Username:     fmt.Sprintf("user_%03d", i),
			Password:     "senhaSegura123",
			ValidateDays: 30,
		}
	}

	resp := service.CreateUsers(users)
	if resp.Error {
		t.Fatalf("Erro ao criar usuários em lote: %s", resp.Message)
	}

	if len(resp.Details) != 100 {
		t.Errorf("Esperava 100 detalhes, obteve %d", len(resp.Details))
	}

	for _, d := range resp.Details {
		if !d.Success {
			t.Errorf("Usuário %s falhou na criação: %s", d.Username, d.Message)
		}
	}

	// Verificar se usuário existe no store
	if err := service.store.Load(); err != nil {
		t.Fatalf("Erro ao recarregar store: %v", err)
	}

	if len(service.store.Passwd) != 103 { // 3 iniciais + 100 novos
		t.Errorf("Esperava 103 entradas no passwd, obteve %d", len(service.store.Passwd))
	}
}

func TestSSHService_UpdatePasswordAndValidate(t *testing.T) {
	service, cleanup := setupTestSSHService(t)
	defer cleanup()

	// Atualizar senha de existing1
	resp := service.UpdatePassword("existing1", "novaSenha123")
	if !resp.Success {
		t.Fatalf("Falha ao atualizar senha: %s", resp.Message)
	}

	// Atualizar validade
	resp2 := service.UpdateValidate("existing1", 60)
	if !resp2.Success {
		t.Fatalf("Falha ao atualizar validade: %s", resp2.Message)
	}

	// Tentar atualizar root (não deve permitir)
	respRoot := service.UpdatePassword("root", "hacked")
	if respRoot.Success {
		t.Errorf("Não deveria permitir atualizar senha do root")
	}
}

func TestSSHService_DeleteBatchAndAll(t *testing.T) {
	service, cleanup := setupTestSSHService(t)
	defer cleanup()

	// Criar usuários
	users := []models.SSHUser{
		{Username: "testdel1", Password: "pwd", ValidateDays: 10},
		{Username: "testdel2", Password: "pwd", ValidateDays: 10},
		{Username: "testdel3", Password: "pwd", ValidateDays: 10},
	}
	_ = service.CreateUsers(users)

	// Deletar lote
	delResp := service.DeleteUsers([]string{"testdel1", "testdel3", "root"})
	if !delResp.Error {
		t.Errorf("Esperava hasErrors = true por conta da tentativa no root")
	}

	if delResp.TotalDeleted != 2 {
		t.Errorf("Esperava 2 deletados, obteve %d", delResp.TotalDeleted)
	}

	// Adicionar um usuário com /bin/bash (ex: conta administrativa humana)
	service.store.UpsertUser("admin_humano", "$6$hash", 1005, 1005, "", "/bin/bash")
	_ = service.store.Save()

	// Deletar todos restantes (/bin/false e /usr/sbin/nologin)
	delAllResp := service.DeleteAllUsers()
	if delAllResp.Error {
		t.Fatalf("Erro ao deletar todos: %s", delAllResp.Message)
	}

	_ = service.store.Load()
	// Esperado: root (UID 0), sshd (UID 105) e admin_humano (UID 1005 com /bin/bash) devem ser preservados!
	if len(service.store.Passwd) != 3 {
		t.Errorf("Esperava 3 entradas restantes no passwd (root, sshd, admin_humano com /bin/bash), obteve %d", len(service.store.Passwd))
	}
	foundAdmin := false
	for _, p := range service.store.Passwd {
		if p.Username == "admin_humano" {
			foundAdmin = true
			break
		}
	}
	if !foundAdmin {
		t.Errorf("admin_humano com /bin/bash deveria ter sido preservado pelo DeleteAllUsers")
	}
}

func TestSSHService_TestUser(t *testing.T) {
	service, cleanup := setupTestSSHService(t)
	defer cleanup()

	cronMock := &mockCronScheduler{}
	resp := service.CreateTestUser(models.SSHUserTestRequest{
		Username: "teste4horas",
		Password: "senhaTeste123",
		Time:     4,
	}, cronMock)

	if resp.Error {
		t.Fatalf("Erro ao criar usuário de teste: %s", resp.Message)
	}

	if len(cronMock.addedJobs) != 1 || cronMock.addedJobs[0] != "teste4horas:ssh:4" {
		t.Errorf("Cronjob não registrado corretamente: %v", cronMock.addedJobs)
	}
}
