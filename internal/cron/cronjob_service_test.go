package cron

import (
	"api-v2/internal/services"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupTestCronService(t *testing.T) (*CronjobService, string, func()) {
	tempDir, err := os.MkdirTemp("", "cron_test_*")
	if err != nil {
		t.Fatalf("Erro ao criar tempDir: %v", err)
	}

	cronFile := filepath.Join(tempDir, "cronjobs.json")
	_ = os.WriteFile(cronFile, []byte("[]"), 0644)

	os.Setenv("SENTINEL_CRONJOBS_FILE", cronFile)

	sshService := services.NewSSHService()
	v2rayService := services.NewV2RayService()

	cs := &CronjobService{
		sshService:   sshService,
		v2rayService: v2rayService,
	}

	cleanup := func() {
		os.Unsetenv("SENTINEL_CRONJOBS_FILE")
		os.RemoveAll(tempDir)
	}

	return cs, cronFile, cleanup
}

func TestCronjobService_PurgeAlreadyDeletedUsers(t *testing.T) {
	cs, _, cleanup := setupTestCronService(t)
	defer cleanup()

	pastTime := time.Now().Add(-2 * time.Hour).Format(time.RFC3339)

	initialJob := Cronjob{
		ID:       "user_inexistente_9999",
		Type:     "ssh",
		ExecTime: pastTime,
		Executed: false,
	}

	// Usuário não existe no sistema; executeCronjob deve considerar resolvido e não retornar erro
	if err := cs.executeCronjob(&initialJob); err != nil {
		t.Errorf("executeCronjob deveria retornar nil (sucesso) para usuário já deletado, retornou: %v", err)
	}
}

func TestCronjobService_RemovePendingJobs(t *testing.T) {
	cs, _, cleanup := setupTestCronService(t)
	defer cleanup()

	// Testar chamada de RemovePendingSSHTestCronjobs com lista vazia
	n, err := cs.RemovePendingSSHTestCronjobs([]string{})
	if err != nil || n != 0 {
		t.Errorf("Esperava 0 removidos para lista vazia, obteve %d (err=%v)", n, err)
	}

	// Testar RemoveAllPendingSSHTestCronjobs
	_, err = cs.RemoveAllPendingSSHTestCronjobs()
	if err != nil {
		t.Errorf("RemoveAllPendingSSHTestCronjobs falhou: %v", err)
	}
}

func TestCronjobService_AddTestCronjobReplacesPending(t *testing.T) {
	cs, cronFile, cleanup := setupTestCronService(t)
	defer cleanup()

	if err := cs.AddTestCronjob("user1", "ssh", 1); err != nil {
		t.Fatalf("primeiro AddTestCronjob: %v", err)
	}
	if err := cs.AddTestCronjob("user1", "ssh", 3); err != nil {
		t.Fatalf("segundo AddTestCronjob: %v", err)
	}

	jobs, err := cs.loadCronjobs()
	if err != nil {
		t.Fatalf("loadCronjobs: %v", err)
	}

	pending := 0
	var last Cronjob
	for _, job := range jobs {
		if job.ID == "user1" && job.Type == "ssh" && !job.Executed {
			pending++
			last = job
		}
	}
	if pending != 1 {
		t.Fatalf("esperava 1 job pendente após renovar teste, obteve %d (ficheiro=%s)", pending, cronFile)
	}

	execTime, err := time.Parse(time.RFC3339, last.ExecTime)
	if err != nil {
		t.Fatalf("execTime inválido: %v", err)
	}
	until := time.Until(execTime)
	if until < 2*time.Hour || until > 4*time.Hour {
		t.Errorf("execTime deveria estar ~3h no futuro, falta %s", until)
	}
}

func TestCronjobService_ExecuteExpiredSSHUsers(t *testing.T) {
	cs, _, cleanup := setupTestCronService(t)
	defer cleanup()

	// Chamar executeExpiredSSHUsers diretamente para garantir que não ocorra pânico
	cs.executeExpiredSSHUsers()
}
