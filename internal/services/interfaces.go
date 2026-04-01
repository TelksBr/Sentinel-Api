package services

import "api-v2/internal/models"

// SSHServiceInterface define a interface pública do serviço SSH.
type SSHServiceInterface interface {
	CreateUsers(users []models.SSHUser) models.SSHUserCreateResponse
	CreateTestUser(request models.SSHUserTestRequest, cronService TestCronjobScheduler) models.SSHUserCreateResponse
	UpdatePassword(username, password string) models.SSHUserResponse
	UpdateValidate(username string, days int) models.SSHUserResponse
	DeleteUsers(usernames []string) models.SSHUserCreateResponse
	DeleteAllUsers() models.SSHUserCreateResponse
	DisableUser(username string) models.SSHUserResponse
	EnableUser(username string, days *int) models.SSHUserResponse
}

// V2RayServiceInterface define a interface pública do serviço V2Ray.
type V2RayServiceInterface interface {
	CreateUsers(users []models.V2RayUser) models.V2RayUserCreateResponse
	DeleteUsers(uuids []string) models.V2RayUserCreateResponse
	DeleteAllUsers() models.V2RayUserCreateResponse
	UpdateValidate(uuid string, days int) models.V2RayUserResponse
	DisableUser(uuid string) models.V2RayUserResponse
	EnableUser(uuid string, expirationDate *string) models.V2RayUserResponse
	RemoveExpiredUsers() error
	GetConfigPath() string
}

// MonitorServiceInterface define a interface pública do serviço de monitoramento.
type MonitorServiceInterface interface {
	Start()
	Stop()
	GetOnlineUsers() models.OnlineUsersResponse
	GetDetailedOnlineUsers() models.DetailedUsersResponse
	GetSystemResources() models.SystemResources
}
