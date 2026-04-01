package cron

import (
	"api-v2/internal/services"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

const (
	CRONJOB_DIR  = "/root/sentinel/temp"
	CRONJOB_FILE = "/root/sentinel/temp/cronjobs.json"
)

// CronjobService gerencia os cronjobs da aplicação
type CronjobService struct {
	cron         *cron.Cron
	sshService   *services.SSHService
	v2rayService *services.V2RayService
	fileMutex    sync.Mutex // protege leitura/escrita do arquivo cronjobs.json
}

// NewCronjobService cria uma nova instância do serviço de cronjobs
func NewCronjobService(sshService *services.SSHService, v2rayService *services.V2RayService) *CronjobService {
	// Criar diretório se não existir
	os.MkdirAll(CRONJOB_DIR, 0755)

	// Criar arquivo de cronjobs se não existir
	if _, err := os.Stat(CRONJOB_FILE); os.IsNotExist(err) {
		os.WriteFile(CRONJOB_FILE, []byte("[]"), 0644)
	}

	return &CronjobService{
		cron:         cron.New(),
		sshService:   sshService,
		v2rayService: v2rayService,
	}
}

// Start inicia o serviço de cronjobs
func (cs *CronjobService) Start() error {
	// Cronjob 1: Usuários de teste (a cada 5 minutos)
	_, err := cs.cron.AddFunc("*/5 * * * *", cs.executeTestUserCronjobs)
	if err != nil {
		return fmt.Errorf("erro ao adicionar cronjob de usuários teste: %v", err)
	}

	// Cronjob 2: Usuários V2Ray expirados (a cada 1 hora)
	_, err = cs.cron.AddFunc("0 */1 * * *", cs.executeExpiredV2RayUsers)
	if err != nil {
		return fmt.Errorf("erro ao adicionar cronjob de usuários V2Ray expirados: %v", err)
	}

	cs.cron.Start()
	log.Println("Serviço de cronjobs iniciado.")
	return nil
}

// Stop para o serviço de cronjobs
func (cs *CronjobService) Stop() {
	cs.cron.Stop()
}

// AddTestCronjob adiciona um cronjob para usuário de teste
func (cs *CronjobService) AddTestCronjob(id, cronType string, hoursFromNow int) error {
	// Validar valor de horas (limitar a um máximo razoável)
	if hoursFromNow < 0 || hoursFromNow > 8760 { // máximo 1 ano
		return fmt.Errorf("horas inválidas: %d (deve estar entre 0 e 8760)", hoursFromNow)
	}

	execTime := time.Now().Add(time.Duration(hoursFromNow) * time.Hour)

	// Validar que a data calculada é válida
	if execTime.IsZero() {
		return fmt.Errorf("data de execução inválida calculada")
	}

	cronjob := Cronjob{
		ID:       id,
		Type:     cronType,
		ExecTime: execTime.Format(time.RFC3339),
		Executed: false,
	}

	return cs.addCronjob(cronjob)
}

// AddV2RayCronjob adiciona um cronjob para usuário V2Ray
func (cs *CronjobService) AddV2RayCronjob(id, execTimeISO string) error {
	cronjob := Cronjob{
		ID:       id,
		Type:     "v2ray",
		ExecTime: execTimeISO,
		Executed: false,
	}

	return cs.addCronjob(cronjob)
}

// executeTestUserCronjobs executa cronjobs de usuários de teste (thread-safe)
func (cs *CronjobService) executeTestUserCronjobs() {
	cs.fileMutex.Lock()
	defer cs.fileMutex.Unlock()

	log.Printf("Executando cronjobs de usuários teste...")

	cronjobs, err := cs.loadCronjobs()
	if err != nil {
		log.Printf("Erro ao carregar cronjobs: %v", err)
		return
	}

	now := time.Now()
	executed := false
	validCronjobs := []Cronjob{}
	invalidCount := 0

	for i := range cronjobs {
		job := &cronjobs[i]
		if job.Executed {
			validCronjobs = append(validCronjobs, *job)
			continue
		}

		// Validar e parsear tempo de execução
		execTime, err := time.Parse(time.RFC3339, job.ExecTime)
		if err != nil {
			log.Printf("❌ Cronjob inválido removido - ID: %s, Tipo: %s, ExecTime: %s, Erro: %v",
				job.ID, job.Type, job.ExecTime, err)
			invalidCount++
			continue // Pular este cronjob inválido
		}

		// Validar que o tempo está em um range razoável (não muito no passado nem muito no futuro)
		minTime := time.Now().AddDate(-1, 0, 0) // 1 ano atrás
		maxTime := time.Now().AddDate(1, 0, 0)  // 1 ano à frente
		if execTime.Before(minTime) || execTime.After(maxTime) {
			log.Printf("❌ Cronjob com data fora do range removido - ID: %s, Tipo: %s, ExecTime: %s",
				job.ID, job.Type, execTime.Format(time.RFC3339))
			invalidCount++
			continue
		}

		validCronjobs = append(validCronjobs, *job)

		if execTime.Before(now) || execTime.Equal(now) {
			if err := cs.executeCronjob(job); err != nil {
				log.Printf("Erro ao executar cronjob %s: %v", job.ID, err)
			} else {
				job.Executed = true
				executed = true
				// Atualizar na lista válida
				for j := range validCronjobs {
					if validCronjobs[j].ID == job.ID {
						validCronjobs[j].Executed = true
						break
					}
				}
			}
		}
	}

	// Se encontrou entradas inválidas, salvar arquivo limpo
	if invalidCount > 0 {
		log.Printf("🧹 Removendo %d cronjob(s) inválido(s) do arquivo...", invalidCount)
		if err := cs.saveCronjobs(validCronjobs); err != nil {
			log.Printf("Erro ao salvar cronjobs após limpeza de inválidos: %v", err)
		}
	}

	if executed {
		cs.saveCronjobs(validCronjobs)
		cs.cleanExecutedCronjobs()
	}
}

// executeExpiredV2RayUsers executa remoção de usuários V2Ray expirados
func (cs *CronjobService) executeExpiredV2RayUsers() {
	log.Println("Iniciando monitoramento de usuários V2Ray expirados...")

	if err := cs.v2rayService.RemoveExpiredUsers(); err != nil {
		log.Printf("Erro ao remover usuários V2Ray expirados: %v", err)
	} else {
		log.Println("Monitoramento de usuários V2Ray expirados concluído.")
	}
}

// executeCronjob executa um cronjob específico
func (cs *CronjobService) executeCronjob(job *Cronjob) error {
	switch job.Type {
	case "ssh":
		// Deletar usuário SSH
		result := cs.sshService.DeleteUsers([]string{job.ID})
		if result.Error {
			return fmt.Errorf("falha ao deletar usuário SSH %s: %s", job.ID, result.Message)
		}
		log.Printf("Usuário SSH removido: %s", job.ID)

	case "v2ray":
		// Deletar usuário V2Ray
		result := cs.v2rayService.DeleteUsers([]string{job.ID})
		if result.Error {
			return fmt.Errorf("falha ao deletar usuário V2Ray %s: %s", job.ID, result.Message)
		}
		log.Printf("Usuário V2Ray removido: %s", job.ID)

	default:
		return fmt.Errorf("tipo de cronjob desconhecido: %s", job.Type)
	}

	return nil
}

// addCronjob adiciona um cronjob ao arquivo (thread-safe)
func (cs *CronjobService) addCronjob(cronjob Cronjob) error {
	cs.fileMutex.Lock()
	defer cs.fileMutex.Unlock()

	cronjobs, err := cs.loadCronjobs()
	if err != nil {
		return err
	}

	cronjobs = append(cronjobs, cronjob)
	return cs.saveCronjobs(cronjobs)
}

// loadCronjobs carrega cronjobs do arquivo JSON
func (cs *CronjobService) loadCronjobs() ([]Cronjob, error) {
	data, err := os.ReadFile(CRONJOB_FILE)
	if err != nil {
		return nil, err
	}

	var cronjobs []Cronjob
	if err := json.Unmarshal(data, &cronjobs); err != nil {
		return nil, err
	}

	return cronjobs, nil
}

// saveCronjobs salva cronjobs no arquivo JSON
func (cs *CronjobService) saveCronjobs(cronjobs []Cronjob) error {
	data, err := json.MarshalIndent(cronjobs, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(CRONJOB_FILE, data, 0644)
}

// cleanExecutedCronjobs remove cronjobs já executados
func (cs *CronjobService) cleanExecutedCronjobs() {
	// NOTA: fileMutex já deve estar adquirido pelo chamador (executeTestUserCronjobs)
	cronjobs, err := cs.loadCronjobs()
	if err != nil {
		log.Printf("Erro ao carregar cronjobs para limpeza: %v", err)
		return
	}

	// Filtrar apenas os não executados
	activeCronjobs := []Cronjob{}
	for _, job := range cronjobs {
		if !job.Executed {
			activeCronjobs = append(activeCronjobs, job)
		}
	}

	// Salvar apenas os ativos
	if err := cs.saveCronjobs(activeCronjobs); err != nil {
		log.Printf("Erro ao salvar cronjobs após limpeza: %v", err)
	}
}
