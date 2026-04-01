package handlers

import (
	"net/http"
	"strconv"

	"api-v2/internal/cron"
	"api-v2/internal/models"
	"api-v2/internal/services"

	"github.com/gin-gonic/gin"
)

// SSHHandlers implementa os handlers SSH
type SSHHandlers struct {
	sshService  *services.SSHService
	cronService *cron.CronjobService
}

// NewSSHHandlers cria uma nova instância dos handlers SSH
func NewSSHHandlers(sshService *services.SSHService, cronService *cron.CronjobService) *SSHHandlers {
	return &SSHHandlers{
		sshService:  sshService,
		cronService: cronService,
	}
}

// CreateUsers cria usuários SSH
func (h *SSHHandlers) CreateUsers(c *gin.Context) {
	var users []models.SSHUser
	if err := c.ShouldBindJSON(&users); err != nil {
		HandleBadRequest(c, "Os dados do usuário devem ser um array")
		return
	}

	// Validar cada usuário
	for i, user := range users {
		if err := user.Validate(); err != nil {
			HandleValidationError(c, "Dados de usuário inválidos", []models.ValidationError{
				{
					Field:   "users",
					Tag:     "validation",
					Value:   strconv.Itoa(i),
					Message: err.Error(),
				},
			})
			return
		}
	}

	result := h.sshService.CreateUsers(users)
	status := http.StatusOK
	if result.Error {
		status = http.StatusBadRequest
	}

	c.JSON(status, result)
}

// UpdateUser atualiza um usuário SSH
func (h *SSHHandlers) UpdateUser(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		HandleBadRequest(c, "Nome de usuário inválido")
		return
	}

	var updateRequest models.SSHUserUpdateRequest
	if err := c.ShouldBindJSON(&updateRequest); err != nil {
		HandleBadRequest(c, "Dados de atualização inválidos")
		return
	}

	if err := updateRequest.Validate(); err != nil {
		HandleValidationError(c, "Dados de atualização inválidos", []models.ValidationError{
			{
				Field:   "update",
				Tag:     "validation",
				Value:   "",
				Message: err.Error(),
			},
		})
		return
	}

	var result models.SSHUserResponse
	var results []models.SSHUserResponse

	if updateRequest.Password != nil {
		result = h.sshService.UpdatePassword(username, *updateRequest.Password)
		results = append(results, result)
	}

	if updateRequest.ValidateDays != nil {
		result = h.sshService.UpdateValidate(username, *updateRequest.ValidateDays)
		results = append(results, result)
	}

	if len(results) == 0 {
		HandleBadRequest(c, "Nenhum parâmetro de atualização válido fornecido.")
		return
	}

	// Se só uma operação, retornar formato original para compatibilidade
	if len(results) == 1 {
		status := http.StatusOK
		if !results[0].Success {
			status = http.StatusBadRequest
		}
		c.JSON(status, results[0])
		return
	}

	// Múltiplas operações: verificar se todas tiveram sucesso
	allSuccess := true
	for _, r := range results {
		if !r.Success {
			allSuccess = false
			break
		}
	}

	status := http.StatusOK
	if !allSuccess {
		status = http.StatusBadRequest
	}

	c.JSON(status, gin.H{
		"username": username,
		"success":  allSuccess,
		"details":  results,
	})
}

// DeleteUsers deleta usuários SSH
func (h *SSHHandlers) DeleteUsers(c *gin.Context) {
	var usernames []string
	if err := c.ShouldBindJSON(&usernames); err != nil {
		HandleBadRequest(c, "Envie a lista de usuarios SSH a serem deletados")
		return
	}

	if len(usernames) == 0 {
		HandleBadRequest(c, "Envie a lista de usuarios SSH a serem deletados")
		return
	}

	result := h.sshService.DeleteUsers(usernames)
	status := http.StatusOK
	if result.Error {
		status = http.StatusBadRequest
	}

	c.JSON(status, result)
}

// CreateTestUser cria um usuário de teste SSH
func (h *SSHHandlers) CreateTestUser(c *gin.Context) {
	var request models.SSHUserTestRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		HandleBadRequest(c, "Dados de usuário teste inválidos")
		return
	}

	if err := request.Validate(); err != nil {
		HandleValidationError(c, "Dados de usuário teste inválidos", []models.ValidationError{
			{
				Field:   "test_user",
				Tag:     "validation",
				Value:   "",
				Message: err.Error(),
			},
		})
		return
	}

	result := h.sshService.CreateTestUser(request, h.cronService)
	status := http.StatusOK
	if result.Error {
		status = http.StatusBadRequest
	}

	c.JSON(status, result)
}

// DisableUser desabilita um usuário SSH
func (h *SSHHandlers) DisableUser(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		HandleBadRequest(c, "Nome de usuário é obrigatório")
		return
	}

	result := h.sshService.DisableUser(username)
	status := http.StatusOK
	if !result.Success {
		status = http.StatusBadRequest
	}

	c.JSON(status, result)
}

// EnableUser habilita um usuário SSH
func (h *SSHHandlers) EnableUser(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		HandleBadRequest(c, "Nome de usuário é obrigatório")
		return
	}

	var request models.SSHUserEnableRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		// Se não tiver body, usar nil (sem dias)
		request.Days = nil
	}

	result := h.sshService.EnableUser(username, request.Days)
	status := http.StatusOK
	if !result.Success {
		status = http.StatusBadRequest
	}

	c.JSON(status, result)
}

// DeleteAllUsers deleta todos os usuários SSH
func (h *SSHHandlers) DeleteAllUsers(c *gin.Context) {
	result := h.sshService.DeleteAllUsers()
	status := http.StatusOK
	if result.Error {
		status = http.StatusBadRequest
	}

	c.JSON(status, result)
}
