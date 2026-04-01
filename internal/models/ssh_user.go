package models

import (
	"fmt"
	"time"

	"github.com/go-playground/validator/v10"
)

// Singleton validator — thread-safe, reutilizável
var sshValidate = validator.New()

// SSHUser representa um usuário SSH
type SSHUser struct {
	Username     string `json:"username" validate:"required,min=3,max=32,alphanum"`
	Password     string `json:"password" validate:"required,min=4"`
	Limit        int    `json:"limit" validate:"min=0"`
	ValidateDays int    `json:"validate" validate:"required,min=1"`
	IsTest       bool   `json:"is_test"`
	Time         int    `json:"time" validate:"min=0"` // Tempo em horas para cronjob
}

// SSHUserResponse representa a resposta de operações SSH
type SSHUserResponse struct {
	Username string `json:"username"`
	Success  bool   `json:"success"`
	Message  string `json:"message"`
}

// SSHUserCreateResponse representa a resposta de criação de usuários SSH
type SSHUserCreateResponse struct {
	Error        bool              `json:"error"`
	Message      string            `json:"message"`
	Details      []SSHUserResponse `json:"details"`
	TotalBefore  int               `json:"total_before,omitempty"`
	TotalDeleted int               `json:"total_deleted,omitempty"`
	TotalAfter   int               `json:"total_after,omitempty"`
	NotDeleted   []SSHUserResponse `json:"not_deleted,omitempty"`
}

// SSHUserTestRequest representa a requisição de teste SSH
type SSHUserTestRequest struct {
	Username string `json:"username" validate:"required,min=3,max=32,alphanum"`
	Password string `json:"password" validate:"required,min=4"`
	Time     int    `json:"time" validate:"required,min=1"`
}

// SSHUserUpdateRequest representa a requisição de atualização SSH
type SSHUserUpdateRequest struct {
	Password     *string `json:"password,omitempty"`
	ValidateDays *int    `json:"validate,omitempty" validate:"omitempty,min=1"`
}

// SSHUserEnableRequest representa a requisição de habilitação SSH
type SSHUserEnableRequest struct {
	Days *int `json:"days,omitempty" validate:"omitempty,min=1"`
}

// Validate valida a estrutura SSHUser
func (u *SSHUser) Validate() error {
	return sshValidate.Struct(u)
}

// Validate valida a estrutura SSHUserTestRequest
func (r *SSHUserTestRequest) Validate() error {
	return sshValidate.Struct(r)
}

// Validate valida a estrutura SSHUserUpdateRequest
func (r *SSHUserUpdateRequest) Validate() error {
	// Validar campos se fornecidos
	if r.Password != nil && *r.Password != "" {
		if len(*r.Password) < 4 {
			return fmt.Errorf("a senha deve ter no mínimo 4 caracteres")
		}
	}
	if r.ValidateDays != nil && *r.ValidateDays < 1 {
		return fmt.Errorf("os dias de validade devem ser no mínimo 1")
	}
	return nil
}

// Validate valida a estrutura SSHUserEnableRequest
func (r *SSHUserEnableRequest) Validate() error {
	return sshValidate.Struct(r)
}

// GetExpirationDate calcula a data de expiração baseada nos dias de validade
func (u *SSHUser) GetExpirationDate() time.Time {
	return time.Now().AddDate(0, 0, u.ValidateDays)
}

// GetTestExpirationDate calcula a data de expiração para usuários de teste
func (u *SSHUser) GetTestExpirationDate() time.Time {
	return time.Now().Add(time.Duration(u.Time) * time.Hour)
}

// IsReservedUsername verifica se o username está na lista de reservados
func (u *SSHUser) IsReservedUsername() bool {
	reserved := []string{"root", "admin", "sshd", "www-data", "postgres", "mysql", "nginx", "apache"}
	username := u.Username

	for _, reservedName := range reserved {
		if username == reservedName {
			return true
		}
	}
	return false
}
