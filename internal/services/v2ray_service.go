package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tidwall/pretty"
	"github.com/tidwall/sjson"

	"api-v2/internal/models"
	"api-v2/internal/utils"
)

// V2RayService implementa os serviços V2Ray
type V2RayService struct {
	mutex       sync.Mutex
	serviceName string // Cache do nome do serviço detectado
	configPath  string // Cache do caminho do config.json detectado
}

// NewV2RayService cria uma nova instância do serviço V2Ray
func NewV2RayService() *V2RayService {
	return &V2RayService{}
}

// CreateUsers cria usuários V2Ray
func (s *V2RayService) CreateUsers(users []models.V2RayUser) models.V2RayUserCreateResponse {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Ler configuração atual
	cfg, err := s.loadConfigGeneric()
	if err != nil {
		return models.V2RayUserCreateResponse{
			Error:   true,
			Message: fmt.Sprintf("Erro ao carregar configuração: %v", err),
		}
	}

	// Processar cada usuário
	createdUsers := []models.V2RayUserResponse{}
	for _, user := range users {
		email := user.GenerateEmail()
		createdUser := models.V2RayUserResponse{
			UUID:           user.UUID,
			Email:          email,
			ExpirationDate: user.ExpirationDate,
			Success:        true,
			Message:        "Usuário criado com sucesso",
		}
		createdUsers = append(createdUsers, createdUser)
		s.upsertClientInAllInbounds(cfg, user.UUID, email, user.ExpirationDate)
	}

	// Salvar configuração
	if err := s.saveConfigGeneric(cfg); err != nil {
		return models.V2RayUserCreateResponse{
			Error:   true,
			Message: fmt.Sprintf("Erro ao salvar configuração: %v", err),
		}
	}

	// Aguardar 1 segundo antes de reiniciar para evitar problemas de escrita
	time.Sleep(1 * time.Second)

	// Recarregar/Reiniciar serviço Xray/V2Ray (tenta reload primeiro)
	if err := s.restartOrReloadXray(); err != nil {
		utils.WriteLog(fmt.Sprintf("Erro ao recarregar/reiniciar serviço: %v", err))
		// Não falhar a operação por causa do restart
	}

	return models.V2RayUserCreateResponse{
		Error:   false,
		Message: "Usuarios criados com sucesso",
		Users:   createdUsers,
	}
}

// DeleteUsers deleta usuários V2Ray
func (s *V2RayService) DeleteUsers(uuids []string) models.V2RayUserCreateResponse {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Ler configuração atual
	cfg, err := s.loadConfigGeneric()
	if err != nil {
		return models.V2RayUserCreateResponse{
			Error:   true,
			Message: fmt.Sprintf("Erro ao carregar configuração: %v", err),
		}
	}

	deletedUsers := []models.V2RayUserResponse{}
	notDeleted := []models.V2RayUserResponse{}
	notFound := []string{}

	// Processar cada UUID
	for _, uuid := range uuids {
		found := false
		userInfo := models.V2RayUserResponse{UUID: uuid}
		s.removeClientFromAllInbounds(cfg, uuid, &userInfo, &found)

		if found {
			deletedUsers = append(deletedUsers, userInfo)
		} else {
			notFound = append(notFound, uuid)
		}
	}

	// Preencher notDeleted com usuários não encontrados
	for _, nf := range notFound {
		notDeleted = append(notDeleted, models.V2RayUserResponse{
			UUID:    nf,
			Success: false,
			Message: "Usuário não encontrado",
		})
	}

	// Salvar configuração
	if err := s.saveConfigGeneric(cfg); err != nil {
		return models.V2RayUserCreateResponse{
			Error:      true,
			Message:    fmt.Sprintf("Erro ao salvar configuração: %v", err),
			NotDeleted: notDeleted,
		}
	}

	// Aguardar 1 segundo antes de reiniciar para evitar problemas de escrita
	time.Sleep(1 * time.Second)

	// Recarregar/Reiniciar serviço Xray/V2Ray (tenta reload primeiro)
	if err := s.restartOrReloadXray(); err != nil {
		utils.WriteLog(fmt.Sprintf("Erro ao recarregar/reiniciar serviço: %v", err))
		// Não falhar a operação por causa do restart
	}

	response := models.V2RayUserCreateResponse{
		Error:        false,
		Message:      "Usuarios deletados com sucesso",
		Users:        deletedUsers,
		TotalBefore:  len(deletedUsers) + len(notDeleted),
		TotalDeleted: len(deletedUsers),
		TotalAfter:   len(notDeleted),
		NotDeleted:   notDeleted,
	}

	// Adicionar informações sobre usuários não encontrados
	if len(notFound) > 0 {
		response.Message = fmt.Sprintf("Usuarios deletados com sucesso. Não encontrados: %v", notFound)
	}

	return response
}

// UpdateValidate atualiza a validade de um usuário V2Ray
func (s *V2RayService) UpdateValidate(uuid string, days int) models.V2RayUserResponse {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Ler configuração atual
	cfg, err := s.loadConfigGeneric()
	if err != nil {
		return models.V2RayUserResponse{
			UUID:    uuid,
			Success: false,
			Message: fmt.Sprintf("Erro ao carregar configuração: %v", err),
		}
	}

	// Calcular nova data de expiração
	newExpirationDate := time.Now().AddDate(0, 0, days).Format(time.RFC3339)
	found := false

	found = s.updateClientExpirationInAllInbounds(cfg, uuid, newExpirationDate)

	if !found {
		return models.V2RayUserResponse{
			UUID:    uuid,
			Success: false,
			Message: "Usuário não encontrado",
		}
	}

	// Salvar configuração
	if err := s.saveConfigGeneric(cfg); err != nil {
		return models.V2RayUserResponse{
			UUID:    uuid,
			Success: false,
			Message: fmt.Sprintf("Erro ao salvar configuração: %v", err),
		}
	}

	// Aguardar 1 segundo antes de reiniciar para evitar problemas de escrita
	time.Sleep(1 * time.Second)

	// Recarregar/Reiniciar serviço Xray/V2Ray (tenta reload primeiro)
	if err := s.restartOrReloadXray(); err != nil {
		utils.WriteLog(fmt.Sprintf("Erro ao recarregar/reiniciar serviço: %v", err))
		// Não falhar a operação por causa do restart
	}

	return models.V2RayUserResponse{
		UUID:    uuid,
		Success: true,
		Message: "Validade atualizada com sucesso",
	}
}

// DisableUser desabilita um usuário V2Ray (remove o cliente)
func (s *V2RayService) DisableUser(uuid string) models.V2RayUserResponse {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Ler configuração atual
	cfg, err := s.loadConfigGeneric()
	if err != nil {
		return models.V2RayUserResponse{
			UUID:    uuid,
			Success: false,
			Message: fmt.Sprintf("Erro ao carregar configuração: %v", err),
		}
	}

	found := false
	userInfo := models.V2RayUserResponse{UUID: uuid}
	s.removeClientFromAllInbounds(cfg, uuid, &userInfo, &found)

	if !found {
		return models.V2RayUserResponse{
			UUID:    uuid,
			Success: false,
			Message: "Usuário não encontrado",
		}
	}

	// Salvar configuração
	if err := s.saveConfigGeneric(cfg); err != nil {
		return models.V2RayUserResponse{
			UUID:    uuid,
			Success: false,
			Message: fmt.Sprintf("Erro ao salvar configuração: %v", err),
		}
	}

	// Aguardar 1 segundo antes de reiniciar para evitar problemas de escrita
	time.Sleep(1 * time.Second)

	// Recarregar/Reiniciar serviço Xray/V2Ray (tenta reload primeiro)
	if err := s.restartOrReloadXray(); err != nil {
		utils.WriteLog(fmt.Sprintf("Erro ao recarregar/reiniciar serviço: %v", err))
		// Não falhar a operação por causa do restart
	}

	return models.V2RayUserResponse{
		UUID:    uuid,
		Success: true,
		Message: "Usuário desabilitado com sucesso",
	}
}

// EnableUser habilita um usuário V2Ray (define data de expiração)
func (s *V2RayService) EnableUser(uuid string, expirationDate *string) models.V2RayUserResponse {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Ler configuração atual
	cfg, err := s.loadConfigGeneric()
	if err != nil {
		return models.V2RayUserResponse{
			UUID:    uuid,
			Success: false,
			Message: fmt.Sprintf("Erro ao carregar configuração: %v", err),
		}
	}

	// Se não especificou data, usar 30 dias padrão
	if expirationDate == nil || *expirationDate == "" {
		defaultDate := time.Now().AddDate(0, 0, 30).Format(time.RFC3339)
		expirationDate = &defaultDate
	}

	found := s.updateClientExpirationInAllInbounds(cfg, uuid, *expirationDate)

	if !found {
		return models.V2RayUserResponse{
			UUID:    uuid,
			Success: false,
			Message: "Usuário não encontrado",
		}
	}

	// Salvar configuração
	if err := s.saveConfigGeneric(cfg); err != nil {
		return models.V2RayUserResponse{
			UUID:    uuid,
			Success: false,
			Message: fmt.Sprintf("Erro ao salvar configuração: %v", err),
		}
	}

	// Aguardar 1 segundo antes de reiniciar para evitar problemas de escrita
	time.Sleep(1 * time.Second)

	// Recarregar/Reiniciar serviço Xray/V2Ray (tenta reload primeiro)
	if err := s.restartOrReloadXray(); err != nil {
		utils.WriteLog(fmt.Sprintf("Erro ao recarregar/reiniciar serviço: %v", err))
		// Não falhar a operação por causa do restart
	}

	return models.V2RayUserResponse{
		UUID:    uuid,
		Success: true,
		Message: "Usuário habilitado com sucesso",
	}
}

// RemoveExpiredUsers remove usuários V2Ray expirados
func (s *V2RayService) RemoveExpiredUsers() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Ler configuração atual
	cfg, err := s.loadConfigGeneric()
	if err != nil {
		return fmt.Errorf("erro ao carregar configuração: %v", err)
	}

	// Filtrar clientes expirados preservando estrutura
	s.removeExpiredClientsFromAllInbounds(cfg)

	// Salvar configuração com backup
	if err := s.saveConfigGeneric(cfg); err != nil {
		return fmt.Errorf("erro ao salvar configuração: %v", err)
	}

	// Aguardar 1 segundo antes de reiniciar para evitar problemas de escrita
	time.Sleep(1 * time.Second)

	// Recarregar/Reiniciar serviço Xray/V2Ray (tenta reload primeiro)
	if err := s.restartOrReloadXray(); err != nil {
		utils.WriteLog(fmt.Sprintf("Erro ao recarregar/reiniciar serviço: %v", err))
		// Não falhar a operação por causa do restart
	}

	// Log opcional removido: lista de usuários removidos não é mais acumulada aqui

	return nil
}

// getConfigPath detecta e retorna o caminho do config.json
// Verifica múltiplos locais comuns para xray e v2ray
func (s *V2RayService) getConfigPath() string {
	// Usar cache se já detectado
	if s.configPath != "" {
		return s.configPath
	}

	// Ordem de prioridade dos caminhos
	configPaths := []string{
		"/usr/local/etc/xray/config.json",  // Xray instalação padrão
		"/etc/xray/config.json",            // Xray alternativo
		"/etc/v2ray/config.json",           // V2Ray padrão
		"/usr/local/etc/v2ray/config.json", // V2Ray alternativo
	}

	// Verificar qual arquivo existe
	for _, path := range configPaths {
		if _, err := os.Stat(path); err == nil {
			s.configPath = path
			utils.WriteLog(fmt.Sprintf("Config detectado em: %s", path))
			return path
		}
	}

	// Se nenhum encontrado, usar xray padrão (será criado se necessário)
	s.configPath = "/usr/local/etc/xray/config.json"
	utils.WriteLog(fmt.Sprintf("Nenhum config encontrado, usando padrão: %s", s.configPath))
	return s.configPath
}

// loadConfigBytes lê o JSON como bytes preservando ordem original
func (s *V2RayService) loadConfigBytes() ([]byte, error) {
	configPath := s.getConfigPath()
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler config de %s: %w", configPath, err)
	}
	return data, nil
}

// loadConfigGeneric lê o JSON preservando todos os campos (para compatibilidade)
func (s *V2RayService) loadConfigGeneric() (map[string]interface{}, error) {
	data, err := s.loadConfigBytes()
	if err != nil {
		return nil, err
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// saveConfigBytes salva o JSON preservando ordem original
func (s *V2RayService) saveConfigBytes(jsonBytes []byte) error {
	// Validar JSON antes de salvar
	var test map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &test); err != nil {
		return fmt.Errorf("JSON inválido detectado: %v", err)
	}

	configPath := s.getConfigPath()
	dir := filepath.Dir(configPath)
	base := filepath.Base(configPath)
	tmp := filepath.Join(dir, "."+base+".tmp")
	bak := filepath.Join(dir, base+".bak")

	// Criar diretório se não existir
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("erro ao criar diretório %s: %v", dir, err)
	}

	// Backup atual (best effort)
	if current, err := os.ReadFile(configPath); err == nil {
		_ = os.WriteFile(bak, current, 0644)
	}

	// Escrever temp
	if err := os.WriteFile(tmp, jsonBytes, 0644); err != nil {
		return fmt.Errorf("erro ao escrever arquivo temporário: %v", err)
	}

	// Renomear atômico
	if err := os.Rename(tmp, configPath); err != nil {
		return fmt.Errorf("erro ao renomear arquivo: %v", err)
	}

	return nil
}

// saveConfigGeneric salva usando edição cirúrgica preservando ordem
// Usa tidwall/sjson para editar apenas os arrays de clients sem reordenar o resto
// IMPORTANTE: Preserva a ordem dos campos de nível superior (log, routing, dns, inbounds, etc.)
// Dentro de cada client, os campos podem ser reordenados (id, email, expiration_date)
// mas a estrutura geral e ordem dos inbounds é preservada
func (s *V2RayService) saveConfigGeneric(cfg map[string]interface{}) error {
	// Carregar JSON original como bytes para preservar ordem
	originalBytes, err := s.loadConfigBytes()
	if err != nil {
		return fmt.Errorf("erro ao ler JSON original: %v", err)
	}

	// Converter para string para usar sjson/gjson
	jsonStr := string(originalBytes)

	// Atualizar apenas os arrays de clients usando sjson
	// Isso preserva toda a estrutura e ordem original do JSON
	inbounds, ok := cfg["inbounds"].([]interface{})
	if !ok {
		return fmt.Errorf("inbounds não encontrado ou inválido")
	}

	// Encontrar todos os inbounds que têm clients e atualizar
	for i := range inbounds {
		inbound, ok := inbounds[i].(map[string]interface{})
		if !ok {
			continue
		}
		settings, _ := inbound["settings"].(map[string]interface{})
		if settings == nil {
			continue
		}
		clients, _ := settings["clients"].([]interface{})
		if clients == nil {
			continue
		}

		// Path para o array de clients deste inbound
		path := fmt.Sprintf("inbounds.%d.settings.clients", i)

		// Converter clients modificados para JSON
		// Nota: Marshal pode reordenar campos dentro de cada client (id, email, expiration_date)
		// mas preserva a ordem dos clients no array e toda a estrutura do JSON
		clientsJSON, err := json.MarshalIndent(clients, "", "  ")
		if err != nil {
			return fmt.Errorf("erro ao codificar clients: %v", err)
		}

		// Usar sjson.SetRaw para editar apenas este array preservando o resto do JSON
		// Isso mantém a ordem original de todos os campos de nível superior
		jsonStr, err = sjson.SetRaw(jsonStr, path, string(clientsJSON))
		if err != nil {
			return fmt.Errorf("erro ao atualizar clients em %s: %v", path, err)
		}
	}

	// Reformatar o JSON para garantir indentação consistente
	// Usar PrettyOptions para preservar melhor a estrutura
	// Isso corrige problemas de formatação causados pelo sjson.SetRaw
	formattedJSON := pretty.PrettyOptions([]byte(jsonStr), &pretty.Options{
		Width:    0,     // Sem limite de largura
		Prefix:   "",    // Sem prefixo
		Indent:   "  ",  // Indentação de 2 espaços
		SortKeys: false, // NÃO reordenar chaves - preservar ordem original
	})

	// Salvar preservando ordem original do JSON (com formatação corrigida)
	return s.saveConfigBytes(formattedJSON)
}

// upsertClientInAllInbounds adiciona ou atualiza um client em todos os inbounds
func (s *V2RayService) upsertClientInAllInbounds(cfg map[string]interface{}, uuid, email, expiration string) {
	inbounds, ok := cfg["inbounds"].([]interface{})
	if !ok {
		return
	}
	for i := range inbounds {
		inbound, ok := inbounds[i].(map[string]interface{})
		if !ok {
			continue
		}
		settings, _ := inbound["settings"].(map[string]interface{})
		if settings == nil {
			continue
		}
		clients, _ := settings["clients"].([]interface{})
		// Procurar índice existente
		idx := -1
		for j := range clients {
			if m, ok := clients[j].(map[string]interface{}); ok {
				if id, _ := m["id"].(string); id == uuid {
					idx = j
					break
				}
			}
		}
		newClient := map[string]interface{}{
			"id":              uuid,
			"email":           email,
			"expiration_date": expiration,
		}
		if idx >= 0 {
			clients[idx] = newClient
		} else {
			clients = append(clients, newClient)
		}
		settings["clients"] = clients
		inbound["settings"] = settings
		inbounds[i] = inbound
	}
	cfg["inbounds"] = inbounds
}

// removeClientFromAllInbounds remove um client por UUID e retorna info do usuário
func (s *V2RayService) removeClientFromAllInbounds(cfg map[string]interface{}, uuid string, info *models.V2RayUserResponse, found *bool) {
	inbounds, ok := cfg["inbounds"].([]interface{})
	if !ok {
		return
	}
	for i := range inbounds {
		inbound, ok := inbounds[i].(map[string]interface{})
		if !ok {
			continue
		}
		settings, _ := inbound["settings"].(map[string]interface{})
		if settings == nil {
			continue
		}
		clients, _ := settings["clients"].([]interface{})
		filtered := make([]interface{}, 0, len(clients))
		for _, c := range clients {
			if m, ok := c.(map[string]interface{}); ok {
				id, _ := m["id"].(string)
				if id == uuid {
					if email, ok := m["email"].(string); ok {
						info.Email = email
					}
					if exp, ok := m["expiration_date"].(string); ok {
						info.ExpirationDate = exp
					}
					*found = true
					continue
				}
			}
			filtered = append(filtered, c)
		}
		settings["clients"] = filtered
		inbound["settings"] = settings
		inbounds[i] = inbound
	}
	cfg["inbounds"] = inbounds
}

// updateClientExpirationInAllInbounds atualiza expiração de um UUID
func (s *V2RayService) updateClientExpirationInAllInbounds(cfg map[string]interface{}, uuid, expiration string) bool {
	inbounds, ok := cfg["inbounds"].([]interface{})
	if !ok {
		return false
	}
	found := false
	for i := range inbounds {
		inbound, ok := inbounds[i].(map[string]interface{})
		if !ok {
			continue
		}
		settings, _ := inbound["settings"].(map[string]interface{})
		if settings == nil {
			continue
		}
		clients, _ := settings["clients"].([]interface{})
		for j := range clients {
			if m, ok := clients[j].(map[string]interface{}); ok {
				if id, _ := m["id"].(string); id == uuid {
					m["expiration_date"] = expiration
					clients[j] = m
					found = true
				}
			}
		}
		settings["clients"] = clients
		inbound["settings"] = settings
		inbounds[i] = inbound
	}
	cfg["inbounds"] = inbounds
	return found
}

// removeExpiredClientsFromAllInbounds remove clientes expirados
func (s *V2RayService) removeExpiredClientsFromAllInbounds(cfg map[string]interface{}) {
	inbounds, ok := cfg["inbounds"].([]interface{})
	if !ok {
		return
	}
	now := time.Now()
	for i := range inbounds {
		inbound, ok := inbounds[i].(map[string]interface{})
		if !ok {
			continue
		}
		settings, _ := inbound["settings"].(map[string]interface{})
		if settings == nil {
			continue
		}
		clients, _ := settings["clients"].([]interface{})
		filtered := make([]interface{}, 0, len(clients))
		for _, c := range clients {
			keep := true
			if m, ok := c.(map[string]interface{}); ok {
				if expStr, ok := m["expiration_date"].(string); ok && expStr != "" {
					if expTime, err := time.Parse(time.RFC3339, expStr); err == nil {
						if !expTime.After(now.Truncate(time.Minute)) {
							keep = false
						}
					}
				}
			}
			if keep {
				filtered = append(filtered, c)
			}
		}
		settings["clients"] = filtered
		inbound["settings"] = settings
		inbounds[i] = inbound
	}
	cfg["inbounds"] = inbounds
}

// getV2RayServiceName detecta o nome do serviço (xray ou v2ray) com cache
func (s *V2RayService) getV2RayServiceName() string {
	// Se já detectou antes, usar cache
	if s.serviceName != "" {
		return s.serviceName
	}

	// Verificar qual binário está rodando (mais confiável)
	if err := utils.ExecuteCommandQuiet("pgrep", "-x", "xray"); err == nil {
		s.serviceName = "xray"
		return "xray"
	}
	if err := utils.ExecuteCommandQuiet("pgrep", "-x", "v2ray"); err == nil {
		s.serviceName = "v2ray"
		return "v2ray"
	}

	// Verificar se o serviço existe e está ativo
	output, err := utils.ExecuteCommand("systemctl", "is-active", "xray")
	if err == nil && strings.TrimSpace(output) == "active" {
		s.serviceName = "xray"
		return "xray"
	}

	output, err = utils.ExecuteCommand("systemctl", "is-active", "v2ray")
	if err == nil && strings.TrimSpace(output) == "active" {
		s.serviceName = "v2ray"
		return "v2ray"
	}

	// Verificar se o serviço existe (mesmo que inativo) usando systemctl list-units
	output, err = utils.ExecuteCommand("systemctl", "list-units", "--type=service", "--all", "xray.service", "v2ray.service")
	if err == nil {
		if strings.Contains(output, "xray.service") {
			s.serviceName = "xray"
			return "xray"
		}
		if strings.Contains(output, "v2ray.service") {
			s.serviceName = "v2ray"
			return "v2ray"
		}
	}

	// Verificar arquivos de serviço diretamente
	if _, err := os.Stat("/etc/systemd/system/xray.service"); err == nil {
		s.serviceName = "xray"
		return "xray"
	}
	if _, err := os.Stat("/etc/systemd/system/v2ray.service"); err == nil {
		s.serviceName = "v2ray"
		return "v2ray"
	}
	if _, err := os.Stat("/usr/lib/systemd/system/xray.service"); err == nil {
		s.serviceName = "xray"
		return "xray"
	}
	if _, err := os.Stat("/usr/lib/systemd/system/v2ray.service"); err == nil {
		s.serviceName = "v2ray"
		return "v2ray"
	}

	// Default para xray (mais comum)
	s.serviceName = "xray"
	return "xray"
}

// restartOrReloadXray tenta reload primeiro, depois restart se necessário
// Detecta automaticamente se o serviço é xray ou v2ray
func (s *V2RayService) restartOrReloadXray() error {
	serviceName := s.getV2RayServiceName()

	// Tentar reload primeiro (não interrompe conexões ativas)
	if err := utils.ExecuteCommandQuiet("systemctl", "reload", serviceName); err == nil {
		return nil
	}

	// Se reload falhar, tentar restart
	utils.WriteLog(fmt.Sprintf("Reload do serviço %s falhou. Tentando restart...", serviceName))
	if err := utils.ExecuteCommandQuiet("systemctl", "restart", serviceName); err != nil {
		return fmt.Errorf("falha ao reiniciar serviço %s: %v", serviceName, err)
	}

	return nil
}

// DeleteAllUsers deleta todos os usuários V2Ray (remove todos os clientes de todos os inbounds)
func (s *V2RayService) DeleteAllUsers() models.V2RayUserCreateResponse {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Ler configuração atual
	cfg, err := s.loadConfigGeneric()
	if err != nil {
		return models.V2RayUserCreateResponse{
			Error:   true,
			Message: fmt.Sprintf("Erro ao carregar configuração: %v", err),
		}
	}

	deletedUsers := []models.V2RayUserResponse{}
	notDeleted := []models.V2RayUserResponse{}

	// Processar todos os inbounds e remover todos os clientes
	inbounds, ok := cfg["inbounds"].([]interface{})
	if !ok {
		return models.V2RayUserCreateResponse{
			Error:   true,
			Message: "Inbounds não encontrado ou inválido",
		}
	}

	// Remover todos os clientes de todos os inbounds
	for i := range inbounds {
		inbound, ok := inbounds[i].(map[string]interface{})
		if !ok {
			continue
		}
		settings, _ := inbound["settings"].(map[string]interface{})
		if settings == nil {
			continue
		}
		clients, _ := settings["clients"].([]interface{})
		if clients == nil {
			continue
		}

		// Coletar informações dos usuários antes de deletar
		for _, c := range clients {
			if m, ok := c.(map[string]interface{}); ok {
				userInfo := models.V2RayUserResponse{
					UUID:    "",
					Email:   "",
					Success: true,
					Message: "Usuário deletado com sucesso",
				}
				if id, ok := m["id"].(string); ok {
					userInfo.UUID = id
				}
				if email, ok := m["email"].(string); ok {
					userInfo.Email = email
				}
				if exp, ok := m["expiration_date"].(string); ok {
					userInfo.ExpirationDate = exp
				}
				deletedUsers = append(deletedUsers, userInfo)
			}
		}

		// Limpar array de clientes (remover todos)
		settings["clients"] = []interface{}{}
		inbound["settings"] = settings
		inbounds[i] = inbound
	}
	cfg["inbounds"] = inbounds

	totalBefore := len(deletedUsers)

	// Salvar configuração
	if err := s.saveConfigGeneric(cfg); err != nil {
		return models.V2RayUserCreateResponse{
			Error:      true,
			Message:    fmt.Sprintf("Erro ao salvar configuração: %v", err),
			NotDeleted: notDeleted,
		}
	}

	// Aguardar 1 segundo antes de reiniciar para evitar problemas de escrita
	time.Sleep(1 * time.Second)

	// Recarregar/Reiniciar serviço Xray/V2Ray (tenta reload primeiro)
	if err := s.restartOrReloadXray(); err != nil {
		utils.WriteLog(fmt.Sprintf("Erro ao recarregar/reiniciar serviço: %v", err))
		// Não falhar a operação por causa do restart
	}

	return models.V2RayUserCreateResponse{
		Error:        false,
		Message:      fmt.Sprintf("Todos os usuários V2Ray foram deletados com sucesso (%d usuários)", len(deletedUsers)),
		Users:        deletedUsers,
		TotalBefore:  totalBefore,
		TotalDeleted: len(deletedUsers),
		TotalAfter:   0,
		NotDeleted:   notDeleted,
	}
}
