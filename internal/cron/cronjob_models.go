package cron

// Cronjob representa um job agendado
type Cronjob struct {
	ID       string `json:"id"`       // username ou uuid
	Type     string `json:"type"`     // "ssh" ou "v2ray"
	ExecTime string `json:"execTime"` // Data/hora de execução em ISO
	Executed bool   `json:"executed"` // Se já foi executado
}
