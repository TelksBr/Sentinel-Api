package models

import (
	"fmt"
	"time"

	"github.com/go-playground/validator/v10"
)

// Singleton validator — thread-safe, reutilizável
var v2rayValidate = validator.New()

// V2RayUser representa um usuário V2Ray
type V2RayUser struct {
	UUID           string `json:"uuid" validate:"required,uuid4"`
	ExpirationDate string `json:"expiration_date" validate:"required"` // Data de expiração no formato ISO
}

// V2RayUserResponse representa a resposta de operações V2Ray
type V2RayUserResponse struct {
	UUID           string `json:"uuid"`
	Email          string `json:"email"`
	ExpirationDate string `json:"expiration_date"`
	Success        bool   `json:"success"`
	Message        string `json:"message"`
}

// V2RayUserCreateResponse representa a resposta de criação de usuários V2Ray
type V2RayUserCreateResponse struct {
	Error        bool                `json:"error"`
	Message      string              `json:"message"`
	Users        []V2RayUserResponse `json:"users"`
	TotalBefore  int                 `json:"total_before,omitempty"`
	TotalDeleted int                 `json:"total_deleted,omitempty"`
	TotalAfter   int                 `json:"total_after,omitempty"`
	NotDeleted   []V2RayUserResponse `json:"not_deleted,omitempty"`
}

// V2RayUserDeleteRequest representa a requisição de deleção V2Ray
type V2RayUserDeleteRequest struct {
	UUIDs []string `json:"uuids" validate:"required,min=1,dive,uuid4"`
}

// V2RayUserUpdateRequest representa a requisição de atualização V2Ray
type V2RayUserUpdateRequest struct {
	ValidateDays int `json:"validate" validate:"required,min=1"`
}

// V2RayUserEnableRequest representa a requisição de habilitação V2Ray
type V2RayUserEnableRequest struct {
	ExpirationDate *string `json:"expiration_date,omitempty" validate:"omitempty"`
}

// Validate valida a estrutura V2RayUser
func (u *V2RayUser) Validate() error {
	// Validação adicional para data de expiração
	if _, err := time.Parse(time.RFC3339, u.ExpirationDate); err != nil {
		return fmt.Errorf("parsing time \"%s\" as \"%s\": cannot parse \"%s\" as \"%s\"",
			u.ExpirationDate, time.RFC3339, u.ExpirationDate, "2006")
	}

	return v2rayValidate.Struct(u)
}

// Validate valida a estrutura V2RayUserDeleteRequest
func (r *V2RayUserDeleteRequest) Validate() error {
	return v2rayValidate.Struct(r)
}

// Validate valida a estrutura V2RayUserUpdateRequest
func (r *V2RayUserUpdateRequest) Validate() error {
	return v2rayValidate.Struct(r)
}

// Validate valida a estrutura V2RayUserEnableRequest
func (r *V2RayUserEnableRequest) Validate() error {
	// Validação adicional para data de expiração se fornecida
	if r.ExpirationDate != nil && *r.ExpirationDate != "" {
		if _, err := time.Parse(time.RFC3339, *r.ExpirationDate); err != nil {
			return fmt.Errorf("parsing time \"%s\" as \"%s\": cannot parse \"%s\" as \"%s\"",
				*r.ExpirationDate, time.RFC3339, *r.ExpirationDate, "2006")
		}
	}

	return v2rayValidate.Struct(r)
}

// GetExpirationTime retorna o tempo de expiração como time.Time
func (u *V2RayUser) GetExpirationTime() (time.Time, error) {
	return time.Parse(time.RFC3339, u.ExpirationDate)
}

// IsExpired verifica se o usuário está expirado
func (u *V2RayUser) IsExpired() bool {
	expirationTime, err := u.GetExpirationTime()
	if err != nil {
		return true // Se não conseguir parsear, considera expirado
	}

	// Comparar até os minutos (como na implementação original)
	now := time.Now()
	return expirationTime.Truncate(time.Minute).Before(now.Truncate(time.Minute)) ||
		expirationTime.Truncate(time.Minute).Equal(now.Truncate(time.Minute))
}

// GenerateEmail gera um email determinístico baseado no UUID
func (u *V2RayUser) GenerateEmail() string {
	domains := []string{"gmail.com", "yahoo.com", "outlook.com", "protonmail.com"}
	username := u.UUID[:8] // Usa primeira parte do UUID

	// Calcula índice do domínio baseado no UUID
	hash := 0
	for _, char := range u.UUID {
		hash += int(char)
	}
	domainIndex := hash % len(domains)

	return "v2ray_" + username + "@" + domains[domainIndex]
}

// GetExpirationDateFromDays calcula a data de expiração baseada nos dias
func (u *V2RayUser) GetExpirationDateFromDays(days int) string {
	return time.Now().AddDate(0, 0, days).Format(time.RFC3339)
}
