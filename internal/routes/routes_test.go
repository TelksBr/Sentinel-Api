package routes_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"api-v2/internal/cron"
	"api-v2/internal/middleware"
	"api-v2/internal/routes"
	"api-v2/internal/services"

	"github.com/gin-gonic/gin"
)

func TestVersionRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	sshService := services.NewSSHService()
	v2rayService := services.NewV2RayService()
	monitorService := services.NewMonitorService(v2rayService.GetConfigPath())
	cronService := cron.NewCronjobService(sshService, v2rayService)
	authMiddleware := middleware.NewAuthMiddleware("test-api-key")

	expectedVersion := "1.2.3"
	router := routes.SetupRoutes(sshService, v2rayService, monitorService, cronService, authMiddleware, expectedVersion)

	t.Run("GET / returns version and message", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Esperava status 200, obteve %d", w.Code)
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Erro ao decodificar JSON: %v", err)
		}

		if resp["message"] != "🟢 API running !" {
			t.Errorf("Esperava message '🟢 API running !', obteve '%v'", resp["message"])
		}

		if resp["version"] != expectedVersion {
			t.Errorf("Esperava version '%s', obteve '%v'", expectedVersion, resp["version"])
		}
	})

	t.Run("GET /version returns version", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/version", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Esperava status 200, obteve %d", w.Code)
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Erro ao decodificar JSON: %v", err)
		}

		if resp["version"] != expectedVersion {
			t.Errorf("Esperava version '%s', obteve '%v'", expectedVersion, resp["version"])
		}
	})
}

func TestDefaultVersionRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	sshService := services.NewSSHService()
	v2rayService := services.NewV2RayService()
	monitorService := services.NewMonitorService(v2rayService.GetConfigPath())
	cronService := cron.NewCronjobService(sshService, v2rayService)
	authMiddleware := middleware.NewAuthMiddleware("test-api-key")

	router := routes.SetupRoutes(sshService, v2rayService, monitorService, cronService, authMiddleware)

	req, _ := http.NewRequest("GET", "/version", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Esperava status 200, obteve %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Erro ao decodificar JSON: %v", err)
	}

	if resp["version"] != "dev" {
		t.Errorf("Esperava version padrão 'dev', obteve '%v'", resp["version"])
	}
}
