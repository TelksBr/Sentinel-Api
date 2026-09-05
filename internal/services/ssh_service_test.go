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

	utils.ProcessKillDisabled = true

	cleanup := func() {
		utils.ProcessKillDisabled = false
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
	service.store.UpsertUser("admin_humano", "$6$hash", 1005, 1005, "", "/bin/bash", 0)
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

	// Verificar se a validade no shadow foi definida para 4 dias
	_ = service.store.Load()
	expectedExpireDays := utils.DaysToShadowExpireDays(4)
	found := false
	for _, s := range service.store.Shadow {
		if s.Username == "teste4horas" {
			found = true
			if s.ExpireDays != expectedExpireDays {
				t.Errorf("ExpireDays no shadow = %s, esperado %s (4 dias)", s.ExpireDays, expectedExpireDays)
			}
			break
		}
	}
	if !found {
		t.Errorf("Usuário teste4horas não encontrado no shadow")
	}

	// Verificar que DeleteExpiredUsers não remove o usuário de teste no mesmo dia
	respDel := service.DeleteExpiredUsers()
	for _, delUser := range respDel.Details {
		if delUser.Username == "teste4horas" {
			t.Errorf("Usuário de teste não deveria ter sido removido por DeleteExpiredUsers hoje")
		}
	}
}

func TestSSHService_DeleteExpiredUsers(t *testing.T) {
	service, cleanup := setupTestSSHService(t)
	defer cleanup()

	// existing1 no mock tem ExpireDays: 19850 (no passado)
	// Vamos criar um usuário ativo (validade 30 dias)
	validUser := models.SSHUser{
		Username:     "usuario_ativo",
		Password:     "senhaAtiva123",
		ValidateDays: 30,
	}
	respCreate := service.CreateUsers([]models.SSHUser{validUser})
	if respCreate.Error {
		t.Fatalf("Erro ao criar usuário ativo: %s", respCreate.Message)
	}

	// Deletar expirados
	respDel := service.DeleteExpiredUsers()
	if respDel.Error {
		t.Fatalf("Erro ao deletar usuários expirados: %s", respDel.Message)
	}

	if respDel.TotalDeleted != 1 {
		t.Errorf("Esperava 1 usuário expirado deletado (existing1), obteve %d", respDel.TotalDeleted)
	}

	if len(respDel.Details) != 1 || respDel.Details[0].Username != "existing1" {
		t.Errorf("Detalhe de deleção inesperado: %v", respDel.Details)
	}

	// Chamar novamente (não deve haver mais nenhum expirado)
	respDel2 := service.DeleteExpiredUsers()
	if respDel2.Error {
		t.Fatalf("Erro na segunda chamada de DeleteExpiredUsers: %s", respDel2.Message)
	}
	if respDel2.TotalDeleted != 0 {
		t.Errorf("Esperava 0 deletados na segunda chamada, obteve %d", respDel2.TotalDeleted)
	}

	// Verificar se usuario_ativo, root e sshd ainda existem
	_ = service.store.Load()
	foundAtivo := false
	foundExisting1 := false
	for _, p := range service.store.Passwd {
		if p.Username == "usuario_ativo" {
			foundAtivo = true
		}
		if p.Username == "existing1" {
			foundExisting1 = true
		}
	}
	if !foundAtivo {
		t.Errorf("usuario_ativo deveria continuar existindo no passwd")
	}
	if foundExisting1 {
		t.Errorf("existing1 não deveria mais existir no passwd")
	}
}

func TestSSHService_UpdateLimit(t *testing.T) {
	service, cleanup := setupTestSSHService(t)
	defer cleanup()

	// Criar usuário com limit 2
	respCreate := service.CreateUsers([]models.SSHUser{
		{
			Username:     "limit_user",
			Password:     "senhaForte123",
			ValidateDays: 30,
			Limit:        2,
		},
	})
	if respCreate.Error || len(respCreate.Details) != 1 || !respCreate.Details[0].Success {
		t.Fatalf("Erro ao criar usuário com limite: %v", respCreate)
	}

	// Verificar se limite 2 foi persistido no store
	_ = service.store.Load()
	limit, exists := service.store.GetUserLimit("limit_user")
	if !exists || limit != 2 {
		t.Errorf("Esperava limite 2, obteve %d (exists=%v)", limit, exists)
	}

	// Atualizar limite para 5
	respUpdate := service.UpdateLimit("limit_user", 5)
	if !respUpdate.Success {
		t.Fatalf("Falha ao atualizar limite: %s", respUpdate.Message)
	}

	// Verificar persistência
	_ = service.store.Load()
	limitUpdated, existsUpdated := service.store.GetUserLimit("limit_user")
	if !existsUpdated || limitUpdated != 5 {
		t.Errorf("Esperava limite 5 após update, obteve %d (exists=%v)", limitUpdated, existsUpdated)
	}

	// Tentar atualizar usuário do sistema ou reservado (deve falhar)
	respSys := service.UpdateLimit("root", 10)
	if respSys.Success {
		t.Errorf("UpdateLimit em usuário root deveria ter falhado")
	}

	// Tentar atualizar usuário inexistente (deve falhar)
	respNotFound := service.UpdateLimit("nao_existe_mesmo", 10)
	if respNotFound.Success {
		t.Errorf("UpdateLimit em usuário inexistente deveria ter falhado")
	}
}

func TestSSHService_CreateTestUserWithLimit(t *testing.T) {
	service, cleanup := setupTestSSHService(t)
	defer cleanup()

	mockCron := &mockCronScheduler{}
	resp := service.CreateTestUser(models.SSHUserTestRequest{
		Username: "test_user_limit",
		Password: "senhaTeste123",
		Time:     2,
		Limit:    3,
	}, mockCron)

	if resp.Error || len(resp.Details) != 1 || !resp.Details[0].Success {
		t.Fatalf("Erro ao criar usuário teste com limite: %v", resp)
	}

	_ = service.store.Load()
	limit, exists := service.store.GetUserLimit("test_user_limit")
	if !exists || limit != 3 {
		t.Errorf("Esperava limite 3 no usuário teste, obteve %d (exists=%v)", limit, exists)
	}
}



