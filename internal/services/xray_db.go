package services

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"sync"

	"api-v2/internal/models"

	_ "modernc.org/sqlite"
)

// XrayDB gerencia a persistência e sincronização com o banco SQLite3 (xraycore.db)
type XrayDB struct {
	db      *sql.DB
	path    string
	enabled bool
	mu      sync.Mutex
}

// DetectXrayDBPath procura o arquivo xraycore.db em caminhos conhecidos.
// Retorna o caminho se encontrado e válido, ou vazio ("") se não existir.
func DetectXrayDBPath(customPath string) string {
	candidatePaths := make([]string, 0, 7)

	if customPath != "" {
		candidatePaths = append(candidatePaths, customPath)
	}

	if envPath := os.Getenv("XRAY_DB_PATH"); envPath != "" {
		candidatePaths = append(candidatePaths, envPath)
	}
	if envPath := os.Getenv("SSHCORE_DB_PATH"); envPath != "" {
		candidatePaths = append(candidatePaths, envPath)
	}

	// Caminhos padrão de instalação no Linux e relativo (/opt/sshcore/xraycore.db como prioritário)
	candidatePaths = append(candidatePaths,
		"/opt/sshcore/xraycore.db",
		"/etc/xray/xraycore.db",
		"/usr/local/etc/xray/xraycore.db",
		"/etc/sshplus/xraycore.db",
		"./xraycore.db",
	)

	for _, p := range candidatePaths {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}

	return ""
}

// NewXrayDB inicializa uma nova conexão com o banco SQLite3 se o caminho for fornecido.
func NewXrayDB(dbPath string) (*XrayDB, error) {
	if dbPath == "" {
		return &XrayDB{enabled: false}, nil
	}

	// Configuração com busy_timeout e WAL para segurança contra concorrência
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("falha ao abrir sqlite em %s: %w", dbPath, err)
	}

	// SQLite trabalha com locks por arquivo; limitar a 1 conexão aberta evita 'database is locked'
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("falha ao conectar ao sqlite em %s: %w", dbPath, err)
	}

	// Verificar se a tabela xray_clients existe
	var tableCount int
	err = db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='xray_clients';").Scan(&tableCount)
	if err != nil || tableCount == 0 {
		_ = db.Close()
		return nil, fmt.Errorf("tabela xray_clients não encontrada no banco %s", dbPath)
	}

	return &XrayDB{
		db:      db,
		path:    dbPath,
		enabled: true,
	}, nil
}

// IsEnabled retorna true se o banco SQLite foi detectado e está ativo
func (x *XrayDB) IsEnabled() bool {
	return x != nil && x.enabled && x.db != nil
}

// GetPath retorna o caminho do arquivo do banco
func (x *XrayDB) GetPath() string {
	if x == nil {
		return ""
	}
	return x.path
}

// Close fecha a conexão com o banco
func (x *XrayDB) Close() error {
	if !x.IsEnabled() {
		return nil
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	x.enabled = false
	return x.db.Close()
}

// BatchUpsertClients insere ou atualiza múltiplos clientes na tabela xray_clients
func (x *XrayDB) BatchUpsertClients(clients []models.XrayClient) error {
	if !x.IsEnabled() || len(clients) == 0 {
		return nil
	}

	x.mu.Lock()
	defer x.mu.Unlock()

	tx, err := x.db.Begin()
	if err != nil {
		return fmt.Errorf("erro ao iniciar transação sqlite: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	stmt, err := tx.Prepare(`
		INSERT INTO xray_clients (
			uuid, name, email, inbound_tag, expires_at, max_conns, quota_bytes, quota_action,
			throttle_mbps, total_uplink, total_downlink, last_active, active_connections, active_devices
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(uuid) DO UPDATE SET
			name = excluded.name,
			email = excluded.email,
			inbound_tag = excluded.inbound_tag,
			expires_at = excluded.expires_at,
			max_conns = excluded.max_conns;
	`)
	if err != nil {
		return fmt.Errorf("erro ao preparar statement sqlite: %w", err)
	}
	defer stmt.Close()

	for _, c := range clients {
		tag := c.InboundTag
		if tag == "" {
			tag = "inbound-sshplus"
		}
		maxConns := c.MaxConns
		if maxConns <= 0 {
			maxConns = 1
		}
		quotaAction := c.QuotaAction
		if quotaAction == "" {
			quotaAction = "block"
		}

		_, err := stmt.Exec(
			c.UUID,
			c.Name,
			c.Email,
			tag,
			c.ExpiresAt,
			maxConns,
			c.QuotaBytes,
			quotaAction,
			c.ThrottleMbps,
			c.TotalUplink,
			c.TotalDownlink,
			c.LastActive,
			c.ActiveConnections,
			c.ActiveDevices,
		)
		if err != nil {
			return fmt.Errorf("erro ao inserir cliente %s no sqlite: %w", c.UUID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("erro ao comitar transação sqlite: %w", err)
	}

	return nil
}

// UpdateExpiration atualiza a data de expiração (Unix timestamp em segundos) de um cliente
func (x *XrayDB) UpdateExpiration(uuid string, expiresAt int64) error {
	if !x.IsEnabled() {
		return nil
	}

	x.mu.Lock()
	defer x.mu.Unlock()

	_, err := x.db.Exec("UPDATE xray_clients SET expires_at = ? WHERE uuid = ?", expiresAt, uuid)
	if err != nil {
		return fmt.Errorf("erro ao atualizar expiração do cliente %s no sqlite: %w", uuid, err)
	}

	return nil
}

// DeleteClients remove clientes pelo UUID da tabela xray_clients e limpa conexões em xray_conns
func (x *XrayDB) DeleteClients(uuids []string) error {
	if !x.IsEnabled() || len(uuids) == 0 {
		return nil
	}

	x.mu.Lock()
	defer x.mu.Unlock()

	tx, err := x.db.Begin()
	if err != nil {
		return fmt.Errorf("erro ao iniciar transação de deleção sqlite: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	delClientsStmt, err := tx.Prepare("DELETE FROM xray_clients WHERE uuid = ?")
	if err != nil {
		return fmt.Errorf("erro ao preparar deleção de clientes: %w", err)
	}
	defer delClientsStmt.Close()

	delConnsStmt, err := tx.Prepare("DELETE FROM xray_conns WHERE uuid = ?")
	if err != nil {
		return fmt.Errorf("erro ao preparar deleção de conexões: %w", err)
	}
	defer delConnsStmt.Close()

	for _, id := range uuids {
		if _, err := delClientsStmt.Exec(id); err != nil {
			return fmt.Errorf("erro ao deletar cliente %s: %w", id, err)
		}
		if _, err := delConnsStmt.Exec(id); err != nil {
			return fmt.Errorf("erro ao deletar conexões do cliente %s: %w", id, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("erro ao comitar deleção sqlite: %w", err)
	}

	return nil
}

// DeleteAllClients remove todos os registros de xray_clients e xray_conns
func (x *XrayDB) DeleteAllClients() error {
	if !x.IsEnabled() {
		return nil
	}

	x.mu.Lock()
	defer x.mu.Unlock()

	tx, err := x.db.Begin()
	if err != nil {
		return fmt.Errorf("erro ao iniciar transação sqlite: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.Exec("DELETE FROM xray_clients;"); err != nil {
		return fmt.Errorf("erro ao limpar xray_clients: %w", err)
	}
	if _, err := tx.Exec("DELETE FROM xray_conns;"); err != nil {
		return fmt.Errorf("erro ao limpar xray_conns: %w", err)
	}

	return tx.Commit()
}

// DeleteExpiredClients remove clientes cuja expiração já venceu (expires_at <= nowEpoch)
func (x *XrayDB) DeleteExpiredClients(nowEpoch int64) (int64, error) {
	if !x.IsEnabled() {
		return 0, nil
	}

	x.mu.Lock()
	defer x.mu.Unlock()

	tx, err := x.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("erro ao iniciar transação sqlite: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	res, err := tx.Exec("DELETE FROM xray_clients WHERE expires_at > 0 AND expires_at <= ?;", nowEpoch)
	if err != nil {
		return 0, fmt.Errorf("erro ao remover clientes expirados: %w", err)
	}
	deleted, _ := res.RowsAffected()

	// Remover conexões órfãs
	_, _ = tx.Exec("DELETE FROM xray_conns WHERE uuid NOT IN (SELECT uuid FROM xray_clients);")

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("erro ao comitar remoção de expirados: %w", err)
	}

	return deleted, nil
}

// GetClient busca um cliente pelo UUID
func (x *XrayDB) GetClient(uuid string) (*models.XrayClient, error) {
	if !x.IsEnabled() {
		return nil, nil
	}

	row := x.db.QueryRow(`
		SELECT uuid, name, email, inbound_tag, expires_at, max_conns, quota_bytes, quota_action,
		       throttle_mbps, total_uplink, total_downlink, last_active, active_connections, active_devices
		FROM xray_clients WHERE uuid = ?
	`, uuid)

	var c models.XrayClient
	err := row.Scan(
		&c.UUID,
		&c.Name,
		&c.Email,
		&c.InboundTag,
		&c.ExpiresAt,
		&c.MaxConns,
		&c.QuotaBytes,
		&c.QuotaAction,
		&c.ThrottleMbps,
		&c.TotalUplink,
		&c.TotalDownlink,
		&c.LastActive,
		&c.ActiveConnections,
		&c.ActiveDevices,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return &c, nil
}

// CountClients retorna o total de clientes na tabela xray_clients
func (x *XrayDB) CountClients() (int, error) {
	if !x.IsEnabled() {
		return 0, nil
	}

	var count int
	err := x.db.QueryRow("SELECT count(*) FROM xray_clients").Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// LogStatus exibe status do banco no log
func (x *XrayDB) LogStatus() {
	if !x.IsEnabled() {
		log.Println("ℹ️ Banco SQLite (xraycore.db) não ativo.")
		return
	}
	count, _ := x.CountClients()
	log.Printf("📦 Banco SQLite ativo em '%s' com %d cliente(s) registrados.", x.path, count)
}
