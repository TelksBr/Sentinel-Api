package services

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	"api-v2/internal/models"
	"api-v2/internal/utils"
)

// TestCronjobScheduler interface para agendar remoção de usuários de teste
// Usado para evitar dependência circular entre services e cron
type TestCronjobScheduler interface {
	AddTestCronjob(id, cronType string, hoursFromNow int) error
}

// SSHService implementa os serviços SSH de alta performance com manipulação direta de arquivos Unix.
type SSHService struct {
	store *utils.UnixStore
}

// NewSSHService cria uma nova instância do serviço SSH
func NewSSHService() *SSHService {
	return &SSHService{
		store: utils.DefaultUnixStore,
	}
}

// SetStore permite injetar um store customizado (usado principalmente em testes)
func (s *SSHService) SetStore(store *utils.UnixStore) {
	s.store = store
}

// CreateUsers cria múltiplos usuários SSH em uma única transação atômica
func (s *SSHService) CreateUsers(users []models.SSHUser) models.SSHUserCreateResponse {
	if len(users) == 0 {
		return models.SSHUserCreateResponse{
			Error:   false,
			Message: "Nenhum usuário para criar",
			Details: []models.SSHUserResponse{},
		}
	}

	log.Printf("🚀 Iniciando criação atômica em lote de %d usuários SSH...", len(users))
	start := time.Now()

	results := make([]models.SSHUserResponse, len(users))
	validIndices := make([]int, 0, len(users))
	passwordsToHash := make([]string, 0, len(users))

	// 1. Pré-validação rápida de usernames
	for i, user := range users {
		if utils.IsReservedUsername(user.Username) {
			errMsg := fmt.Sprintf("Reserved username cannot be used: %s", user.Username)
			utils.WriteLog(errMsg)
			results[i] = models.SSHUserResponse{
				Username: user.Username,
				Success:  false,
				Message:  errMsg,
			}
			continue
		}
		validIndices = append(validIndices, i)
		passwordsToHash = append(passwordsToHash, user.Password)
	}

	// 2. Gerar hashes SHA-512 em paralelo em todos os núcleos da CPU
	numWorkers := runtime.NumCPU()
	if numWorkers < 2 {
		numWorkers = 2
	}
	hashes, err := utils.BatchSha512Crypt(passwordsToHash, numWorkers)
	if err != nil {
		log.Printf("❌ Erro ao computar hashes das senhas: %v", err)
		return models.SSHUserCreateResponse{
			Error:   true,
			Message: fmt.Sprintf("Erro ao gerar hashes de senha: %v", err),
			Details: results,
		}
	}

	// 3. Bloqueio global e atualização em memória dos arquivos do Unix
	unlock, err := s.store.Lock()
	if err != nil {
		log.Printf("❌ Erro ao adquirir lock do UnixStore: %v", err)
		return models.SSHUserCreateResponse{
			Error:   true,
			Message: fmt.Sprintf("Erro ao adquirir lock do sistema: %v", err),
			Details: results,
		}
	}
	defer unlock()

	if err := s.store.Load(); err != nil {
		log.Printf("❌ Erro ao carregar tabelas do UnixStore: %v", err)
		return models.SSHUserCreateResponse{
			Error:   true,
			Message: fmt.Sprintf("Erro ao ler arquivos de usuários do sistema: %v", err),
			Details: results,
		}
	}

	// 4. Inserir/Atualizar cada usuário no store
	for idx, originalIndex := range validIndices {
		user := users[originalIndex]
		hash := hashes[idx]

		if s.store.IsSystemUser(user.Username) {
			errMsg := fmt.Sprintf("Reserved/system username cannot be used: %s", user.Username)
			results[originalIndex] = models.SSHUserResponse{
				Username: user.Username,
				Success:  false,
				Message:  errMsg,
			}
			continue
		}

		var expireDays string
		if user.IsTest {
			expireDays = utils.DaysToShadowExpireDays(4)
		} else {
			expireDays = utils.DaysToShadowExpireDays(user.ValidateDays)
		}

		s.store.UpsertUser(user.Username, hash, 0, 0, expireDays, "/bin/false")

		results[originalIndex] = models.SSHUserResponse{
			Username: user.Username,
			Success:  true,
			Message:  "User created successfully",
		}
	}

	// 5. Escrita atômica em disco
	if err := s.store.Save(); err != nil {
		log.Printf("❌ Erro ao salvar tabelas Unix: %v", err)
		return models.SSHUserCreateResponse{
			Error:   true,
			Message: fmt.Sprintf("Erro ao persistir arquivos de usuários: %v", err),
			Details: results,
		}
	}

	elapsed := time.Since(start)
	log.Printf("✅ Criação de %d usuários SSH concluída com sucesso em %s", len(users), elapsed)

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

// CreateTestUser cria um usuário de teste SSH e agenda sua remoção
func (s *SSHService) CreateTestUser(request models.SSHUserTestRequest, cronService TestCronjobScheduler) models.SSHUserCreateResponse {
	testUser := models.SSHUser{
		Username:     request.Username,
		Password:     request.Password,
		Limit:        0,
		ValidateDays: 4, // 4 dias como margem de segurança no shadow
		IsTest:       true,
		Time:         request.Time,
	}

	resp := s.CreateUsers([]models.SSHUser{testUser})

	// Se criou com sucesso, agendar remoção via cronjob
	if !resp.Error && len(resp.Details) > 0 && resp.Details[0].Success {
		if err := cronService.AddTestCronjob(request.Username, "ssh", request.Time); err != nil {
			log.Printf("⚠️ Erro ao agendar remoção de usuário teste %s: %v", request.Username, err)
		} else {
			log.Printf("⏰ Usuário teste %s criado e agendado para remoção em %d horas", request.Username, request.Time)
		}
	}

	return resp
}

// UpdatePassword atualiza a senha de um usuário SSH
func (s *SSHService) UpdatePassword(username, password string) models.SSHUserResponse {
	if utils.IsReservedUsername(username) {
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  "Cannot modify reserved/system user",
		}
	}

	hashedPassword, err := utils.Sha512Crypt(password, "", 5000)
	if err != nil {
		errMsg := fmt.Sprintf("Error hashing password: %v", err)
		utils.WriteLog(errMsg)
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  errMsg,
		}
	}

	unlock, err := s.store.Lock()
	if err != nil {
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  fmt.Sprintf("Error locking system files: %v", err),
		}
	}
	defer unlock()

	if err := s.store.Load(); err != nil {
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  fmt.Sprintf("Error loading system files: %v", err),
		}
	}

	if s.store.IsSystemUser(username) {
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  "Cannot modify reserved/system user",
		}
	}

	// Verificar se existe
	found := false
	for i := range s.store.Shadow {
		if s.store.Shadow[i].Username == username {
			s.store.Shadow[i].PasswordHash = hashedPassword
			s.store.Shadow[i].LastChanged = fmt.Sprintf("%d", time.Now().Unix()/86400)
			found = true
			break
		}
	}

	if !found {
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  "User not found",
		}
	}

	if err := s.store.Save(); err != nil {
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  fmt.Sprintf("Error saving system files: %v", err),
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
	if utils.IsReservedUsername(username) {
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  "Cannot modify reserved/system user",
		}
	}

	expireDays := utils.DaysToShadowExpireDays(days)

	unlock, err := s.store.Lock()
	if err != nil {
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  fmt.Sprintf("Error locking system files: %v", err),
		}
	}
	defer unlock()

	if err := s.store.Load(); err != nil {
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  fmt.Sprintf("Error loading system files: %v", err),
		}
	}

	if s.store.IsSystemUser(username) {
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  "Cannot modify reserved/system user",
		}
	}

	found := false
	for i := range s.store.Shadow {
		if s.store.Shadow[i].Username == username {
			s.store.Shadow[i].ExpireDays = expireDays
			found = true
			break
		}
	}

	if !found {
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  "User not found",
		}
	}

	if err := s.store.Save(); err != nil {
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  fmt.Sprintf("Error saving system files: %v", err),
		}
	}

	return models.SSHUserResponse{
		Username: username,
		Success:  true,
		Message:  "Expiration date updated successfully",
	}
}

// DeleteUsers deleta usuários SSH em lote com encerramento imediato de túneis
func (s *SSHService) DeleteUsers(usernames []string) models.SSHUserCreateResponse {
	if len(usernames) == 0 {
		return models.SSHUserCreateResponse{
			Error:   false,
			Message: "Nenhum usuário para deletar",
			Details: []models.SSHUserResponse{},
		}
	}

	log.Printf("🗑️ Iniciando deleção atômica de %d usuários SSH...", len(usernames))
	start := time.Now()

	unlock, err := s.store.Lock()
	if err != nil {
		return models.SSHUserCreateResponse{
			Error:   true,
			Message: fmt.Sprintf("Erro ao adquirir lock do sistema: %v", err),
			Details: []models.SSHUserResponse{},
		}
	}
	defer unlock()

	if err := s.store.Load(); err != nil {
		return models.SSHUserCreateResponse{
			Error:   true,
			Message: fmt.Sprintf("Erro ao ler arquivos de usuários: %v", err),
			Details: []models.SSHUserResponse{},
		}
	}

	totalBefore := len(s.store.Passwd)

	// 1. Remover do UnixStore em memória
	deletedUIDs, notFound, sysUsers := s.store.DeleteUsers(usernames)

	// 2. Persistir alterações atomicamente
	if err := s.store.Save(); err != nil {
		return models.SSHUserCreateResponse{
			Error:   true,
			Message: fmt.Sprintf("Erro ao persistir remoção de usuários: %v", err),
			Details: []models.SSHUserResponse{},
		}
	}

	// 3. Encerrar sessões e túneis SSH de todos os UIDs removidos em lote (chunks de 500)
	utils.TerminateUserSessionsByUIDs(deletedUIDs)

	// 4. Limpar backups de expiração em uma única operação atômica de disco
	_ = utils.RemoveExpirationBackups(usernames)

	// 5. Montar detalhes de resposta
	deletedSet := make(map[string]bool)
	notFoundSet := make(map[string]bool, len(notFound))
	for _, nf := range notFound {
		notFoundSet[nf] = true
	}
	sysSet := make(map[string]bool, len(sysUsers))
	for _, sys := range sysUsers {
		sysSet[sys] = true
	}

	results := make([]models.SSHUserResponse, 0, len(usernames))
	notDeleted := make([]models.SSHUserResponse, 0)
	hasErrors := false

	for _, u := range usernames {
		if sysSet[u] {
			hasErrors = true
			resp := models.SSHUserResponse{
				Username: u,
				Success:  false,
				Message:  "Cannot delete reserved/system user",
			}
			results = append(results, resp)
			notDeleted = append(notDeleted, resp)
		} else if notFoundSet[u] {
			hasErrors = true
			resp := models.SSHUserResponse{
				Username: u,
				Success:  false,
				Message:  "User does not exist or was already deleted.",
			}
			results = append(results, resp)
			notDeleted = append(notDeleted, resp)
		} else {
			deletedSet[u] = true
			results = append(results, models.SSHUserResponse{
				Username: u,
				Success:  true,
				Message:  "User deleted successfully",
			})
		}
	}

	elapsed := time.Since(start)
	log.Printf("✅ Deleção de %d usuários concluída em %s (%d removidos com sucesso)", len(usernames), elapsed, len(deletedUIDs))

	message := "All users deleted successfully"
	if hasErrors {
		message = "Some users failed to be deleted"
	}

	return models.SSHUserCreateResponse{
		Error:        hasErrors,
		Message:      message,
		Details:      results,
		TotalBefore:  totalBefore,
		TotalDeleted: len(deletedUIDs),
		TotalAfter:   len(s.store.Passwd),
		NotDeleted:   notDeleted,
	}
}

// DisableUser desabilita um usuário SSH (bloqueio, nologin, expiração no passado e kill de processos)
func (s *SSHService) DisableUser(username string) models.SSHUserResponse {
	if utils.IsReservedUsername(username) {
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  "Cannot disable reserved/system user",
		}
	}

	unlock, err := s.store.Lock()
	if err != nil {
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  fmt.Sprintf("Error locking system files: %v", err),
		}
	}
	defer unlock()

	if err := s.store.Load(); err != nil {
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  fmt.Sprintf("Error loading system files: %v", err),
		}
	}

	// Salvar expiração atual para backup antes de desativar
	for _, sh := range s.store.Shadow {
		if sh.Username == username {
			_ = utils.SaveExpirationBackup(username, sh.ExpireDays)
			break
		}
	}

	uid, err := s.store.SetUserDisabled(username)
	if err != nil {
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  err.Error(),
		}
	}

	if err := s.store.Save(); err != nil {
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  fmt.Sprintf("Error saving system files: %v", err),
		}
	}

	// Matar túneis/sessões ativas
	utils.TerminateUserSessions(username, uid)

	return models.SSHUserResponse{
		Username: username,
		Success:  true,
		Message:  "User disabled successfully",
	}
}

// EnableUser habilita um usuário SSH restaurando shell e validade
func (s *SSHService) EnableUser(username string, days *int) models.SSHUserResponse {
	if utils.IsReservedUsername(username) {
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  "Cannot enable reserved/system user",
		}
	}

	var expireDays string
	if days != nil && *days > 0 {
		expireDays = utils.DaysToShadowExpireDays(*days)
	} else {
		// Restaurar do backup se existir
		if origExpire, exists := utils.LoadExpirationBackup(username); exists && origExpire != "" {
			expireDays = origExpire
		} else {
			expireDays = utils.DaysToShadowExpireDays(30) // padrão 30 dias
		}
	}

	unlock, err := s.store.Lock()
	if err != nil {
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  fmt.Sprintf("Error locking system files: %v", err),
		}
	}
	defer unlock()

	if err := s.store.Load(); err != nil {
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  fmt.Sprintf("Error loading system files: %v", err),
		}
	}

	if err := s.store.SetUserEnabled(username, expireDays); err != nil {
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  err.Error(),
		}
	}

	if err := s.store.Save(); err != nil {
		return models.SSHUserResponse{
			Username: username,
			Success:  false,
			Message:  fmt.Sprintf("Error saving system files: %v", err),
		}
	}

	_ = utils.RemoveExpirationBackup(username)

	return models.SSHUserResponse{
		Username: username,
		Success:  true,
		Message:  "User enabled successfully",
	}
}

// DeleteAllUsers deleta todos os usuários SSH não-sistema
func (s *SSHService) DeleteAllUsers() models.SSHUserCreateResponse {
	log.Println("🗑️ Iniciando deleção de todos os usuários SSH não-sistema...")

	unlock, err := s.store.Lock()
	if err != nil {
		return models.SSHUserCreateResponse{
			Error:   true,
			Message: fmt.Sprintf("Erro ao adquirir lock do sistema: %v", err),
			Details: []models.SSHUserResponse{},
		}
	}
	defer unlock()

	if err := s.store.Load(); err != nil {
		return models.SSHUserCreateResponse{
			Error:   true,
			Message: fmt.Sprintf("Erro ao ler arquivos de usuários: %v", err),
			Details: []models.SSHUserResponse{},
		}
	}

	totalBefore := len(s.store.Passwd)

	deletedUIDs, totalDeleted := s.store.DeleteAllNonSystemUsers()
	if totalDeleted == 0 {
		return models.SSHUserCreateResponse{
			Error:        false,
			Message:      "Nenhum usuário SSH encontrado para deletar",
			Details:      []models.SSHUserResponse{},
			TotalBefore:  totalBefore,
			TotalDeleted: 0,
			TotalAfter:   totalBefore,
		}
	}

	if err := s.store.Save(); err != nil {
		return models.SSHUserCreateResponse{
			Error:   true,
			Message: fmt.Sprintf("Erro ao persistir deleção total: %v", err),
			Details: []models.SSHUserResponse{},
		}
	}

	// Encerrar túneis/sessões de todos os UIDs em lote (chunks de 500)
	utils.TerminateUserSessionsByUIDs(deletedUIDs)

	_ = os.Remove("./data/ssh_user_expiration_backup.json")

	log.Printf("✅ Deleção total concluída: %d usuários removidos", totalDeleted)

	return models.SSHUserCreateResponse{
		Error:        false,
		Message:      fmt.Sprintf("Todos os usuários SSH foram deletados com sucesso (%d usuários)", totalDeleted),
		Details:      []models.SSHUserResponse{},
		TotalBefore:  totalBefore,
		TotalDeleted: totalDeleted,
		TotalAfter:   len(s.store.Passwd),
	}
}

// DeleteExpiredUsers deleta todos os usuários SSH não-sistema cuja data de expiração já passou
func (s *SSHService) DeleteExpiredUsers() models.SSHUserCreateResponse {
	log.Println("🗑️ Iniciando deleção de usuários SSH expirados...")
	start := time.Now()

	unlock, err := s.store.Lock()
	if err != nil {
		return models.SSHUserCreateResponse{
			Error:   true,
			Message: fmt.Sprintf("Erro ao adquirir lock do sistema: %v", err),
			Details: []models.SSHUserResponse{},
		}
	}
	defer unlock()

	if err := s.store.Load(); err != nil {
		return models.SSHUserCreateResponse{
			Error:   true,
			Message: fmt.Sprintf("Erro ao ler arquivos de usuários: %v", err),
			Details: []models.SSHUserResponse{},
		}
	}

	totalBefore := len(s.store.Passwd)

	deletedUsernames, deletedUIDs, totalDeleted := s.store.DeleteExpiredUsers()
	if totalDeleted == 0 {
		return models.SSHUserCreateResponse{
			Error:        false,
			Message:      "Nenhum usuário SSH expirado encontrado para deletar",
			Details:      []models.SSHUserResponse{},
			TotalBefore:  totalBefore,
			TotalDeleted: 0,
			TotalAfter:   totalBefore,
		}
	}

	if err := s.store.Save(); err != nil {
		return models.SSHUserCreateResponse{
			Error:   true,
			Message: fmt.Sprintf("Erro ao persistir remoção de usuários expirados: %v", err),
			Details: []models.SSHUserResponse{},
		}
	}

	// Encerrar túneis/sessões de todos os UIDs expirados em lote
	utils.TerminateUserSessionsByUIDs(deletedUIDs)

	// Limpar backups de expiração dos usuários deletados
	_ = utils.RemoveExpirationBackups(deletedUsernames)

	details := make([]models.SSHUserResponse, 0, len(deletedUsernames))
	for _, u := range deletedUsernames {
		details = append(details, models.SSHUserResponse{
			Username: u,
			Success:  true,
			Message:  "User deleted successfully (expired)",
		})
	}

	elapsed := time.Since(start)
	log.Printf("✅ Deleção de %d usuários SSH expirados concluída em %s", totalDeleted, elapsed)

	return models.SSHUserCreateResponse{
		Error:        false,
		Message:      fmt.Sprintf("%d usuário(s) SSH expirado(s) deletado(s) com sucesso", totalDeleted),
		Details:      details,
		TotalBefore:  totalBefore,
		TotalDeleted: totalDeleted,
		TotalAfter:   len(s.store.Passwd),
	}
}

