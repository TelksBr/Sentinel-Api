package services

import (
	"fmt"
	"log"
	"os"
	"time"

	"api-v2/internal/models"
	"api-v2/internal/utils"
)

// TestCronjobScheduler interface para agendar remoção de usuários de teste
// Usado para evitar dependência circular entre services e cron
type TestCronjobScheduler interface {
	AddTestCronjob(id, cronType string, hoursFromNow int) error
}

// SSHService implementa os serviços SSH
type SSHService struct{}

// NewSSHService cria uma nova instância do serviço SSH
func NewSSHService() *SSHService {
	return &SSHService{}
}

// CreateUsers cria usuários SSH
func (s *SSHService) CreateUsers(users []models.SSHUser) models.SSHUserCreateResponse {
	results := []models.SSHUserResponse{}
	log.Printf("Iniciando criação de usuários SSH.")

	for _, user := range users {
		result := s.createSingleUser(user)
		results = append(results, result)
	}

	// Verificar se houve erros
	hasErrors := false
	for _, result := range results {
		if !result.Success {
			hasErrors = true
			break
		}
	}

	message := "All users created successfully"
	if hasErrors {
		message = "Some users failed to be created"
	}

	return models.SSHUserCreateResponse{
		Error:   hasErrors,
		Message: message,
		Details: results,
	}
}

// createSingleUser cria um único usuário SSH
func (s *SSHService) createSingleUser(user models.SSHUser) models.SSHUserResponse {
	// Validar username reservado
	if utils.IsReservedUsername(user.Username) {
		errorMessage := fmt.Sprintf("Reserved username cannot be used: %s", user.Username)
		utils.WriteLog(errorMessage)
		return models.SSHUserResponse{
			Username: user.Username,
			Success:  false,
			Message:  errorMessage,
		}
	}

	// Verificar se usuário já existe
	userExists, err := utils.CheckUserExists(user.Username)
	if err != nil {
		errorMessage := fmt.Sprintf("Error checking if user exists: %v", err)
		utils.WriteLog(errorMessage)
		return models.SSHUserResponse{
			Username: user.Username,
			Success:  false,
			Message:  errorMessage,
		}
	}

	// Se usuário existe, deletar primeiro
	if userExists {
		if err := utils.DeleteUser(user.Username); err != nil {
			errorMessage := fmt.Sprintf("Error deleting existing user: %v", err)
			utils.WriteLog(errorMessage)
			return models.SSHUserResponse{
				Username: user.Username,
				Success:  false,
				Message:  errorMessage,
			}
		}
		// Aguardar um pouco após deletar
		time.Sleep(100 * time.Millisecond)
	}

	// Calcular data de expiração
	expirationDate, err := utils.CalculateExpirationDate(user.ValidateDays)
	if err != nil {
		errorMessage := fmt.Sprintf("Error calculating expiration date: %v", err)
		utils.WriteLog(errorMessage)
		return models.SSHUserResponse{
			Username: user.Username,
			Success:  false,
			Message:  errorMessage,
		}
	}

	// Criar usuário
	if err := utils.CreateUser(user.Username, user.Password, expirationDate); err != nil {
		errorMessage := fmt.Sprintf("Error creating user: %v", err)
		utils.WriteLog(errorMessage)
		return models.SSHUserResponse{
			Username: user.Username,
			Success:  false,
			Message:  errorMessage,
		}
	}

	return models.SSHUserResponse{
		Username: user.Username,
		Success:  true,
		Message:  "User created successfully",
	}
}

// CreateTestUser cria um usuário de teste SSH
func (s *SSHService) CreateTestUser(request models.SSHUserTestRequest, cronService TestCronjobScheduler) models.SSHUserCreateResponse {
	// Criar usuário de teste
	testUser := models.SSHUser{
		Username:     request.Username,
		Password:     request.Password,
		Limit:        0,
		ValidateDays: 3, // 3 dias para usuário de teste (máximo 72 horas)
		IsTest:       true,
		Time:         request.Time,
	}

	// Criar usuário
	result := s.createSingleUser(testUser)

	// Se criou com sucesso, agendar remoção via cronjob
	if result.Success {
		err := cronService.AddTestCronjob(request.Username, "ssh", request.Time)
		if err != nil {
			log.Printf("Erro ao agendar remoção de usuário teste %s: %v", request.Username, err)
		} else {
			log.Printf("Usuário teste %s criado e agendado para remoção em %d horas", request.Username, request.Time)
		}
	}

	return models.SSHUserCreateResponse{
		Error:   !result.Success,
		Message: result.Message,
		Details: []models.SSHUserResponse{result},
	}
}

// UpdatePassword atualiza a senha de um usuário SSH
func (s *SSHService) UpdatePassword(username, password string) models.SSHUserResponse {
	// Verificar se usuário existe
	userExists, err := utils.CheckUserExists(username)
	if err != nil {
		errorMessage := fmt.Sprintf("Error checking if user exists: %v", err)
		utils.WriteLog(errorMessage)
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  errorMessage,
		}
	}

	if !userExists {
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  "User not found",
		}
	}

	// Gerar hash da nova senha
	hashedPassword, err := utils.HashPassword(password)
	if err != nil {
		errorMessage := fmt.Sprintf("Error hashing password: %v", err)
		utils.WriteLog(errorMessage)
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  errorMessage,
		}
	}

	// Atualizar senha usando usermod
	if err := utils.ExecuteCommandQuiet("usermod", "-p", hashedPassword, username); err != nil {
		errorMessage := fmt.Sprintf("Error updating password: %v", err)
		utils.WriteLog(errorMessage)
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  errorMessage,
		}
	}

	return models.SSHUserResponse{
		Username: username,
		Success:  true,
		Message:  "Password updated successfully",
	}
}

// UpdateValidate atualiza a validade de um usuário SSH
func (s *SSHService) UpdateValidate(username string, days int) models.SSHUserResponse {
	// Verificar se usuário existe
	userExists, err := utils.CheckUserExists(username)
	if err != nil {
		errorMessage := fmt.Sprintf("Error checking if user exists: %v", err)
		utils.WriteLog(errorMessage)
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  errorMessage,
		}
	}

	if !userExists {
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  "User not found",
		}
	}

	// Calcular nova data de expiração
	expirationDate, err := utils.CalculateExpirationDate(days)
	if err != nil {
		errorMessage := fmt.Sprintf("Error calculating expiration date: %v", err)
		utils.WriteLog(errorMessage)
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  errorMessage,
		}
	}

	// Atualizar data de expiração
	if err := utils.UpdateExpirationDate(username, expirationDate); err != nil {
		errorMessage := fmt.Sprintf("Error updating expiration date: %v", err)
		utils.WriteLog(errorMessage)
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  errorMessage,
		}
	}

	return models.SSHUserResponse{
		Username: username,
		Success:  true,
		Message:  "Expiration date updated successfully",
	}
}

// DeleteUsers deleta usuários SSH
func (s *SSHService) DeleteUsers(usernames []string) models.SSHUserCreateResponse {
	results := []models.SSHUserResponse{}

	for _, username := range usernames {
		result := s.deleteSingleUser(username)
		results = append(results, result)
	}

	// Verificar se houve erros
	hasErrors := false
	for _, result := range results {
		if !result.Success {
			hasErrors = true
			break
		}
	}

	message := "All users deleted successfully"
	if hasErrors {
		message = "Some users failed to be deleted"
	}

	return models.SSHUserCreateResponse{
		Error:   hasErrors,
		Message: message,
		Details: results,
	}
}

// deleteSingleUser deleta um único usuário SSH
// Implementação igual ao V1 para garantir fechamento instantâneo de tunnels
func (s *SSHService) deleteSingleUser(username string) models.SSHUserResponse {
	// Proteção contra deleção de usuários reservados/sistema
	if utils.IsReservedUsername(username) {
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  "Cannot delete reserved/system user",
		}
	}

	// Verificar se usuário existe
	userExists, err := utils.CheckUserExists(username)
	if err != nil {
		errorMessage := fmt.Sprintf("Error checking if user exists: %v", err)
		utils.WriteLog(errorMessage)
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  errorMessage,
		}
	}

	if !userExists {
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  "User does not exist or was already deleted.",
		}
	}

	// PRIMEIRO: Desativar o usuário para impedir novas conexões
	if err := utils.ExecuteCommandQuiet("usermod", "-L", username); err != nil {
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  fmt.Sprintf("Failed to disable user %s: %v", username, err),
		}
	}
	if err := utils.ExecuteCommandQuiet("usermod", "-s", "/usr/sbin/nologin", username); err != nil {
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  fmt.Sprintf("Failed to set nologin shell for user %s: %v", username, err),
		}
	}

	// Função para verificar se existem processos do usuário
	hasUserProcesses := func() (bool, error) {
		return utils.HasUserProcesses(username)
	}

	attempts := 0
	deletionSuccess := false

	// Loop de tentativas
	for attempts < 3 && !deletionSuccess {
		// PRIMEIRO: Tenta deletar usuário com força (userdel -f -r)
		// Com o usuário desativado, não haverá novos processos sendo criados durante a deleção
		err := utils.DeleteUser(username)
		if err != nil {
			attempts++
			time.Sleep(1 * time.Second)
			continue
		}

		// DEPOIS: Matar todos os processos restantes do usuário
		// Mata processos de forma forçada após a deleção para garantir que nenhum processo sobrou
		utils.KillUserProcessesForced(username)

		// Verifica se ainda existem processos
		hasProcesses, _ := hasUserProcesses()
		if hasProcesses {
			attempts++
			time.Sleep(1 * time.Second)
			// Tentar matar processos novamente
			utils.KillUserProcessesForced(username)
			continue
		}

		// Verifica se usuário foi realmente removido
		stillExists, _ := utils.CheckUserExists(username)
		if !stillExists {
			deletionSuccess = true
			break
		}
		attempts++
		time.Sleep(1 * time.Second)
	}

	// Última verificação: garantir que não há processos restantes
	if deletionSuccess {
		// Matar processos uma última vez para garantir que nenhum sobrou
		utils.KillUserProcessesForced(username)
		// Remover backup de expiração se existir
		utils.RemoveExpirationBackup(username)
	}

	if deletionSuccess {
		return models.SSHUserResponse{
			Username: username,
			Success:  true,
			Message:  "User deleted successfully",
		}
	}

	return models.SSHUserResponse{
		Username: username,
		Success:  false,
		Message:  "Failed to remove user and kill all processes after multiple attempts.",
	}
}

// DisableUser desabilita um usuário SSH
func (s *SSHService) DisableUser(username string) models.SSHUserResponse {
	// Verificar se usuário existe
	userExists, err := utils.CheckUserExists(username)
	if err != nil {
		errorMessage := fmt.Sprintf("Error checking if user exists: %v", err)
		utils.WriteLog(errorMessage)
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  errorMessage,
		}
	}

	if !userExists {
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  "User not found",
		}
	}

	// Desabilitar usuário (bloqueia e mata processos)
	if err := utils.DisableUser(username); err != nil {
		errorMessage := fmt.Sprintf("Error disabling user: %v", err)
		utils.WriteLog(errorMessage)
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  errorMessage,
		}
	}

	return models.SSHUserResponse{
		Username: username,
		Success:  true,
		Message:  "User disabled successfully",
	}
}

// EnableUser habilita um usuário SSH
// Implementação igual ao V1 para garantir reativação completa
func (s *SSHService) EnableUser(username string, days *int) models.SSHUserResponse {
	// Verificar se usuário existe
	userExists, err := utils.CheckUserExists(username)
	if err != nil {
		errorMessage := fmt.Sprintf("Error checking if user exists: %v", err)
		utils.WriteLog(errorMessage)
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  errorMessage,
		}
	}

	if !userExists {
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  "User not found",
		}
	}

	// Habilitar usuário (igual ao V1: desbloqueia, restaura shell, atualiza expiração)
	if err := utils.EnableUser(username, days); err != nil {
		errorMessage := fmt.Sprintf("Error enabling user: %v", err)
		utils.WriteLog(errorMessage)
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  errorMessage,
		}
	}

	return models.SSHUserResponse{
		Username: username,
		Success:  true,
		Message:  "User enabled successfully",
	}
}

// DeleteAllUsers deleta todos os usuários SSH (exceto usuários reservados)
// Usa a função ListSSHUsers para obter a lista e depois chama DeleteUsers para cada um
func (s *SSHService) DeleteAllUsers() models.SSHUserCreateResponse {
	log.Println("Iniciando deleção de todos os usuários SSH.")

	// Listar todos os usuários SSH criados
	usernames, err := utils.ListSSHUsers()
	if err != nil {
		errorMessage := fmt.Sprintf("Erro ao listar usuários SSH: %v", err)
		utils.WriteLog(errorMessage)
		return models.SSHUserCreateResponse{
			Error:   true,
			Message: errorMessage,
			Details: []models.SSHUserResponse{},
		}
	}

	// Filtrar usuários com UID baixo (usuários do sistema) como proteção adicional
	filteredUsernames := []string{}
	for _, username := range usernames {
		// Verificar UID do usuário
		uid, err := utils.GetUserUID(username)
		if err != nil {
			utils.WriteLog(fmt.Sprintf("Erro ao obter UID do usuário %s: %v", username, err))
			continue
		}

		// Pular usuários do sistema (UID < 1000)
		if uid < 1000 {
			utils.WriteLog(fmt.Sprintf("Pulando usuário do sistema %s (UID: %d)", username, uid))
			continue
		}

		filteredUsernames = append(filteredUsernames, username)
	}
	usernames = filteredUsernames

	if len(usernames) == 0 {
		return models.SSHUserCreateResponse{
			Error:   false,
			Message: "Nenhum usuário SSH encontrado para deletar",
			Details: []models.SSHUserResponse{},
		}
	}

	log.Printf("Encontrados %d usuários SSH para deletar.", len(usernames))

	// Usar a função DeleteUsers existente que já tem toda a lógica sofisticada
	// de desativar usuários, matar processos forçadamente e deletar
	result := s.DeleteUsers(usernames)

	// Totais para resposta
	totalBefore := len(usernames)
	deletedSuccess := 0
	notDeleted := []models.SSHUserResponse{}
	for _, d := range result.Details {
		if d.Success {
			deletedSuccess++
		} else {
			notDeleted = append(notDeleted, d)
		}
	}
	result.TotalBefore = totalBefore
	result.TotalDeleted = deletedSuccess
	if deletedSuccess > totalBefore {
		deletedSuccess = totalBefore
	}
	result.TotalAfter = totalBefore - deletedSuccess
	result.NotDeleted = notDeleted

	// Limpar arquivo de backup de expiração (todos os usuários foram deletados)
	// Tentar remover o arquivo de backup se existir
	backupFile := "./data/ssh_user_expiration_backup.json"
	if err := os.Remove(backupFile); err != nil && !os.IsNotExist(err) {
		utils.WriteLog(fmt.Sprintf("Aviso: não foi possível remover arquivo de backup: %v", err))
	}

	// Ajustar mensagem para refletir que foram deletados todos os usuários
	if !result.Error {
		result.Message = fmt.Sprintf("Todos os usuários SSH foram deletados com sucesso (%d usuários)", len(usernames))
	}

	return result
}
