package handlers

import (
	"net/http"

	"api-v2/internal/cron"
	"api-v2/internal/models"
	"api-v2/internal/services"
	"github.com/gin-gonic/gin"
)

// V2RayHandlers implementa os handlers V2Ray
type V2RayHandlers struct {
	v2rayService *services.V2RayService
	cronService  *cron.CronjobService
}

// NewV2RayHandlers cria uma nova instância dos handlers V2Ray
func NewV2RayHandlers(v2rayService *services.V2RayService, cronService *cron.CronjobService) *V2RayHandlers {
	return &V2RayHandlers{
		v2rayService: v2rayService,
		cronService:  cronService,
	}
}

// CreateUsers cria usuários V2Ray
func (h *V2RayHandlers) CreateUsers(c *gin.Context) {
	var users []models.V2RayUser
	if err := c.ShouldBindJSON(&users); err != nil {
		HandleBadRequest(c, "Envie uma lista válida.")
		return
	}

	// Validar cada usuário
	for i, user := range users {
		if err := user.Validate(); err != nil {
			HandleValidationError(c, "Dados de usuário V2Ray inválidos", []models.ValidationError{
				{
					Field:   "users",
					Tag:     "validation",
					Value:   string(rune(i)),
					Message: err.Error(),
				},
			})
			return
		}
	}

	result := h.v2rayService.CreateUsers(users)
	status := http.StatusOK
	if result.Error {
		status = http.StatusBadRequest
	}

	c.JSON(status, result)
}

// DeleteUsers deleta usuários V2Ray
func (h *V2RayHandlers) DeleteUsers(c *gin.Context) {
	var request models.V2RayUserDeleteRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		HandleBadRequest(c, "Informe um UUID.")
		return
	}

	if err := request.Validate(); err != nil {
		HandleValidationError(c, "Dados de deleção inválidos", []models.ValidationError{
			{
				Field:   "uuids",
				Tag:     "validation",
				Value:   "",
				Message: err.Error(),
			},
		})
		return
	}

	result := h.v2rayService.DeleteUsers(request.UUIDs)
	status := http.StatusOK
	if result.Error {
		status = http.StatusBadRequest
	}

	c.JSON(status, result)
}

// CreateTestUser cria um usuário de teste V2Ray
func (h *V2RayHandlers) CreateTestUser(c *gin.Context) {
	var users []models.V2RayUser
	if err := c.ShouldBindJSON(&users); err != nil {
		HandleBadRequest(c, "O campo user_data é obrigatório e deve ser um array.")
		return
	}

	// Validar cada usuário
	for i, user := range users {
		if err := user.Validate(); err != nil {
			HandleValidationError(c, "Dados de usuário V2Ray de teste inválidos", []models.ValidationError{
				{
					Field:   "users",
					Tag:     "validation",
					Value:   string(rune(i)),
					Message: err.Error(),
				},
			})
			return
		}
	}

	result := h.v2rayService.CreateUsers(users)
	status := http.StatusOK
	if result.Error {
		status = http.StatusBadRequest
	}

	c.JSON(status, result)
}

// UpdateValidate atualiza a validade de um usuário V2Ray
func (h *V2RayHandlers) UpdateValidate(c *gin.Context) {
	uuid := c.Param("uuid")
	if uuid == "" {
		HandleBadRequest(c, "UUID é obrigatório")
		return
	}

	var request models.V2RayUserUpdateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		HandleBadRequest(c, "Dados de atualização inválidos")
		return
	}

	if err := request.Validate(); err != nil {
		HandleValidationError(c, "Dados de atualização inválidos", []models.ValidationError{
			{
				Field:   "validate",
				Tag:     "validation",
				Value:   "",
				Message: err.Error(),
			},
		})
		return
	}

	result := h.v2rayService.UpdateValidate(uuid, request.ValidateDays)
	status := http.StatusOK
	if !result.Success {
		status = http.StatusBadRequest
	}

	c.JSON(status, result)
}

// DisableUser desabilita um usuário V2Ray
func (h *V2RayHandlers) DisableUser(c *gin.Context) {
	uuid := c.Param("uuid")
	if uuid == "" {
		HandleBadRequest(c, "UUID é obrigatório")
		return
	}

	result := h.v2rayService.DisableUser(uuid)
	status := http.StatusOK
	if !result.Success {
		status = http.StatusBadRequest
	}

	c.JSON(status, result)
}

// EnableUser habilita um usuário V2Ray
func (h *V2RayHandlers) EnableUser(c *gin.Context) {
	uuid := c.Param("uuid")
	if uuid == "" {
		HandleBadRequest(c, "UUID é obrigatório")
		return
	}

	var request models.V2RayUserEnableRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		HandleBadRequest(c, "Dados de habilitação inválidos")
		return
	}

	if err := request.Validate(); err != nil {
		HandleValidationError(c, "Dados de habilitação inválidos", []models.ValidationError{
			{
				Field:   "enable",
				Tag:     "validation",
				Value:   "",
				Message: err.Error(),
			},
		})
		return
	}

	result := h.v2rayService.EnableUser(uuid, request.ExpirationDate)
	status := http.StatusOK
	if !result.Success {
		status = http.StatusBadRequest
	}

	c.JSON(status, result)
}

// DeleteAllUsers deleta todos os usuários V2Ray
func (h *V2RayHandlers) DeleteAllUsers(c *gin.Context) {
	result := h.v2rayService.DeleteAllUsers()
	status := http.StatusOK
	if result.Error {
		status = http.StatusBadRequest
	}

	c.JSON(status, result)
}