package models

// Config representa a configuração da API
type Config struct {
	Authorization string `json:"authorization"`
}

// Cronjob representa um job agendado
type Cronjob struct {
	ID       string `json:"id"`       // username ou uuid
	Type     string `json:"type"`     // "ssh" ou "v2ray"
	ExecTime string `json:"execTime"` // Data/hora de execução em ISO
	Executed bool   `json:"executed"` // Se já foi executado
}

// V2RayConfig representa a configuração do V2Ray
type V2RayConfig struct {
	Log               interface{} `json:"log,omitempty"`
	Routing           interface{} `json:"routing,omitempty"`
	DNS               interface{} `json:"dns,omitempty"`
	Inbounds          []Inbound   `json:"inbounds"`
	Outbounds         interface{} `json:"outbounds,omitempty"`
	Transport         interface{} `json:"transport,omitempty"`
	Policy            interface{} `json:"policy,omitempty"`
	API               interface{} `json:"api,omitempty"`
	Stats             interface{} `json:"stats,omitempty"`
	Reverse           interface{} `json:"reverse,omitempty"`
	FakeDNS           interface{} `json:"fakedns,omitempty"`
	Observatory       interface{} `json:"observatory,omitempty"`
	BurstObservatory  interface{} `json:"burstObservatory,omitempty"`
}

// Inbound representa um inbound do V2Ray
type Inbound struct {
	Settings *InboundSettings `json:"settings,omitempty"`
}

// InboundSettings representa as configurações de um inbound
type InboundSettings struct {
	Clients []Client `json:"clients"`
}

// Client representa um cliente V2Ray
type Client struct {
	ID             string `json:"id"`
	Email          string `json:"email"`
	ExpirationDate string `json:"expiration_date,omitempty"`
}
