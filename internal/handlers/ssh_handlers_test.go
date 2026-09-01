package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"api-v2/internal/cron"
	"api-v2/internal/handlers"
	"api-v2/internal/models"
	"api-v2/internal/services"
	"api-v2/internal/utils"

	"github.com/gin-gonic/gin"
)

func setupTestRouter(t *testing.T) (*gin.Engine, *services.SSHService, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	tempDir, err := os.MkdirTemp("", "ssh_handler_test_*")
	if err != nil {
		t.Fatalf("Erro ao criar temp dir: %v", err)
	}

	initialPasswd := `root:x:0:0:root:/root:/bin/bash
sshd:x:105:65534::/run/sshd:/usr/sbin/nologin
expired_user:x:1001:1001::/nonexistent:/bin/false
`
	initialShadow := `root:$6$salt$hash:19800:0:99999:7:::
sshd:*:19800:0:99999:7:::
expired_user:$6$salt$userhash:19800:0:99999:7::1000:
`
	initialGroup := `root:x:0:
expired_user:x:1001:
`
	initialGShadow := `root:*::
expired_user:!::
`
	_ = os.WriteFile(filepath.Join(tempDir, "passwd"), []byte(initialPasswd), 0644)
	_ = os.WriteFile(filepath.Join(tempDir, "shadow"), []byte(initialShadow), 0640)
	_ = os.WriteFile(filepath.Join(tempDir, "group"), []byte(initialGroup), 0644)
	_ = os.WriteFile(filepath.Join(tempDir, "gshadow"), []byte(initialGShadow), 0640)

	store := utils.NewUnixStore(tempDir)
	sshService := services.NewSSHService()
	sshService.SetStore(store)

	v2rayService := services.NewV2RayService()
	cronService := cron.NewCronjobService(sshService, v2rayService)

	sshHandlers := handlers.NewSSHHandlers(sshService, cronService)

	r := gin.New()
	ssh := r.Group("/ssh_user")
	{
		ssh.POST("/delete_expired", sshHandlers.DeleteExpiredUsers)
		ssh.POST("/delete_all", sshHandlers.DeleteAllUsers)
	}

	utils.ProcessKillDisabled = true

	cleanup := func() {
		utils.ProcessKillDisabled = false
		_ = os.RemoveAll(tempDir)
	}

	return r, sshService, cleanup
}

func TestSSHHandlers_DeleteExpiredUsers(t *testing.T) {
	router, _, cleanup := setupTestRouter(t)
	defer cleanup()

	req, _ := http.NewRequest(http.MethodPost, "/ssh_user/delete_expired", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Esperava status 200 OK, obteve %d: %s", w.Code, w.Body.String())
	}

	var resp models.SSHUserCreateResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Erro ao desserializar resposta JSON: %v", err)
	}

	if resp.Error {
		t.Errorf("Esperava resp.Error == false, obteve true (%s)", resp.Message)
	}

	if resp.TotalDeleted != 1 {
		t.Errorf("Esperava 1 usuário deletado, obteve %d", resp.TotalDeleted)
	}

	if len(resp.Details) != 1 || resp.Details[0].Username != "expired_user" {
		t.Errorf("Detalhes da resposta inesperados: %v", resp.Details)
	}
}
