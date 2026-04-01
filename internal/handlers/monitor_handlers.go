package handlers

import (
	"net/http"

	"api-v2/internal/services"

	"github.com/gin-gonic/gin"
)

// MonitorHandlers implementa os handlers de monitoramento
type MonitorHandlers struct {
	monitorService *services.MonitorService
}

// NewMonitorHandlers cria uma nova instância dos handlers de monitoramento
func NewMonitorHandlers(monitorService *services.MonitorService) *MonitorHandlers {
	return &MonitorHandlers{
		monitorService: monitorService,
	}
}

// GetOnlineUsers retorna os usuários online (SSH e V2Ray) do cache
func (h *MonitorHandlers) GetOnlineUsers(c *gin.Context) {
	// Sempre retorna do cache - nunca chama função direta
	response := h.monitorService.GetOnlineUsers()

	c.JSON(http.StatusOK, response)
}

// GetDetailedOnlineUsers retorna a lista detalhada de usuários online (SSH e V2Ray) do cache
func (h *MonitorHandlers) GetDetailedOnlineUsers(c *gin.Context) {
	// Sempre retorna do cache - nunca chama função direta
	response := h.monitorService.GetDetailedOnlineUsers()

	c.JSON(http.StatusOK, response)
}

// GetSystemResources retorna informações de recursos do sistema (CPU e RAM)
func (h *MonitorHandlers) GetSystemResources(c *gin.Context) {
	response := h.monitorService.GetSystemResources()

	c.JSON(http.StatusOK, response)
}