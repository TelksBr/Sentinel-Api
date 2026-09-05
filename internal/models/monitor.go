package models

import "time"

// OnlineUsersResponse representa a resposta de usuários online
type OnlineUsersResponse struct {
	SSHUsers     int `json:"ssh_users"`
	V2RayUsers   int `json:"v2ray_users"`
	DTProtoUsers int `json:"dt_proto_users"`
	VTproxy      int `json:"VTproxy"`
	VTProxyUsers int `json:"vtproxy_users,omitempty"`
	TotalUsers   int `json:"total_users"`
}

// NewOnlineUsersResponse cria uma nova resposta de usuários online
func NewOnlineUsersResponse(sshUsers, v2rayUsers, dtProtoUsers, vtproxyUsers int) OnlineUsersResponse {
	return OnlineUsersResponse{
		SSHUsers:     sshUsers,
		V2RayUsers:   v2rayUsers,
		DTProtoUsers: dtProtoUsers,
		VTproxy:      vtproxyUsers,
		VTProxyUsers: vtproxyUsers,
		TotalUsers:   sshUsers + v2rayUsers + dtProtoUsers + vtproxyUsers,
	}
}

// SSHUserOnline representa um usuário SSH online
type SSHUserOnline struct {
	Username string `json:"username"`
}

// V2RayUserOnline representa um usuário V2Ray online
type V2RayUserOnline struct {
	Email          string    `json:"email"`
	UUID           string    `json:"uuid"`
	LastConnection time.Time `json:"last_connection"`
}

// DTProtoUserOnline representa um usuário DT-Proto online
type DTProtoUserOnline struct {
	ID string `json:"id"`
}

// VTProxyUserOnline representa um usuário VTproxy online
type VTProxyUserOnline struct {
	Username    string `json:"username"`
	Connections int    `json:"connections"`
	Count       int    `json:"count,omitempty"`
}

// DetailedUsersResponse representa a resposta detalhada de usuários online
type DetailedUsersResponse struct {
	SSHUsers     []SSHUserOnline     `json:"ssh_users"`
	V2RayUsers   []V2RayUserOnline   `json:"v2ray_users"`
	DTProtoUsers []DTProtoUserOnline `json:"dt_proto_users"`
	VTproxy      []VTProxyUserOnline `json:"VTproxy"`
	VTProxyUsers []VTProxyUserOnline `json:"vtproxy_users,omitempty"`
	TotalSSH     int                 `json:"total_ssh"`
	TotalV2Ray   int                 `json:"total_v2ray"`
	TotalDTProto int                 `json:"total_dt_proto"`
	TotalVTProxy int                 `json:"total_vtproxy"`
	TotalUsers   int                 `json:"total_users"`
}

// NewDetailedUsersResponse cria uma nova resposta detalhada de usuários online
func NewDetailedUsersResponse(sshUsers []SSHUserOnline, v2rayUsers []V2RayUserOnline, dtProtoUsers []DTProtoUserOnline, vtproxyUsers []VTProxyUserOnline) DetailedUsersResponse {
	return DetailedUsersResponse{
		SSHUsers:     sshUsers,
		V2RayUsers:   v2rayUsers,
		DTProtoUsers: dtProtoUsers,
		VTproxy:      vtproxyUsers,
		VTProxyUsers: vtproxyUsers,
		TotalSSH:     len(sshUsers),
		TotalV2Ray:   len(v2rayUsers),
		TotalDTProto: len(dtProtoUsers),
		TotalVTProxy: len(vtproxyUsers),
		TotalUsers:   len(sshUsers) + len(v2rayUsers) + len(dtProtoUsers) + len(vtproxyUsers),
	}
}

// SystemResources representa as informações de recursos do sistema e contagem de contas criadas
type SystemResources struct {
	Memory        MemoryInfo   `json:"memory"`
	CPU           CPUInfo      `json:"cpu"`
	TotalAccounts int          `json:"total_accounts"`
	TotalExpired  int          `json:"total_expired"`
	Accounts      AccountsInfo `json:"accounts"`
}

// AccountsInfo representa o resumo de todas as contas cadastradas no servidor
type AccountsInfo struct {
	TotalSSH     int `json:"total_ssh"`
	TotalV2Ray   int `json:"total_v2ray"`
	Total        int `json:"total"`
	ExpiredSSH   int `json:"expired_ssh"`
	ExpiredV2Ray int `json:"expired_v2ray"`
	TotalExpired int `json:"total_expired"`
}

// MemoryInfo representa informações de memória
type MemoryInfo struct {
	Total        uint64  `json:"total"`         // KB
	Available    uint64  `json:"available"`     // KB
	Used         uint64  `json:"used"`          // KB
	Free         uint64  `json:"free"`          // KB
	UsagePercent float64 `json:"usage_percent"` // %
}

// CPUInfo representa informações de CPU
type CPUInfo struct {
	Cores        int     `json:"cores"`
	UsagePercent float64 `json:"usage_percent"` // %
	User         uint64  `json:"user"`
	Nice         uint64  `json:"nice"`
	System       uint64  `json:"system"`
	Idle         uint64  `json:"idle"`
	IOWait       uint64  `json:"iowait"`
	IRQ          uint64  `json:"irq"`
	SoftIRQ      uint64  `json:"softirq"`
	Steal        uint64  `json:"steal"`
}
