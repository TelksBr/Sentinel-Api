package services

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"api-v2/internal/models"

	_ "modernc.org/sqlite"
)

// helper para criar um banco de teste com o schema do xraycore.db
func setupTestDB(t *testing.T) (string, func()) {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "xray_db_test_*")
	if err != nil {
		t.Fatalf("falha ao criar temp dir: %v", err)
	}

	dbPath := filepath.Join(tempDir, "xraycore.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("falha ao abrir sqlite de teste: %v", err)
	}

	schema := `
	CREATE TABLE xray_clients (
	  uuid                TEXT PRIMARY KEY,
	  name                TEXT NOT NULL DEFAULT '',
	  email               TEXT NOT NULL DEFAULT '',
	  inbound_tag         TEXT NOT NULL DEFAULT 'inbound-sshplus',
	  expires_at          INTEGER,
	  max_conns           INTEGER NOT NULL DEFAULT 0,
	  quota_bytes         INTEGER NOT NULL DEFAULT 0,
	  quota_action        TEXT NOT NULL DEFAULT 'block',
	  throttle_mbps       INTEGER NOT NULL DEFAULT 0,
	  total_uplink        INTEGER NOT NULL DEFAULT 0,
	  total_downlink      INTEGER NOT NULL DEFAULT 0,
	  last_active         INTEGER NOT NULL DEFAULT 0,
	  active_connections  INTEGER NOT NULL DEFAULT 0,
	  active_devices      INTEGER NOT NULL DEFAULT 0
	);

	CREATE TABLE xray_conns (
	  uuid           TEXT NOT NULL,
	  origem         TEXT NOT NULL,
	  ativos         INTEGER NOT NULL DEFAULT 0,
	  dispositivos   INTEGER NOT NULL DEFAULT 0,
	  atualizado_em  INTEGER NOT NULL DEFAULT (strftime('%s','now')),
	  PRIMARY KEY (uuid, origem)
	);
	`

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		t.Fatalf("falha ao criar schema de teste: %v", err)
	}
	db.Close()

	cleanup := func() {
		_ = os.RemoveAll(tempDir)
	}

	return dbPath, cleanup
}

func TestDetectXrayDBPath(t *testing.T) {
	// Caminho inexistente
	nonExistent := DetectXrayDBPath(filepath.Join(os.TempDir(), "non_existent_xray_db_12345.db"))
	if nonExistent != "" {
		t.Errorf("esperado vazio para caminho inexistente, obteve '%s'", nonExistent)
	}

	// Caminho existente
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	detected := DetectXrayDBPath(dbPath)
	if detected != dbPath {
		t.Errorf("esperado '%s', obteve '%s'", dbPath, detected)
	}
}

func TestXrayDB_Lifecycle(t *testing.T) {
	dbPath, cleanup := setupTestDB(t)
	defer cleanup()

	xrayDB, err := NewXrayDB(dbPath)
	if err != nil {
		t.Fatalf("falha ao inicializar XrayDB: %v", err)
	}
	defer xrayDB.Close()

	if !xrayDB.IsEnabled() {
		t.Fatal("esperado que o banco estivesse habilitado")
	}

	// 1. Inserir clientes
	now := time.Now().Unix()
	clients := []models.XrayClient{
		{
			UUID:        "550e8400-e29b-41d4-a716-446655440001",
			Name:        "user1",
			Email:       "user1@test.com",
			InboundTag:  "inbound-sshplus",
			ExpiresAt:   now + 3600, // Expira em 1 hora
			MaxConns:    2,
			QuotaAction: "block",
		},
		{
			UUID:        "550e8400-e29b-41d4-a716-446655440002",
			Name:        "user2_expired",
			Email:       "user2@test.com",
			InboundTag:  "inbound-sshplus",
			ExpiresAt:   now - 3600, // Já expirado
			MaxConns:    1,
			QuotaAction: "block",
		},
	}

	if err := xrayDB.BatchUpsertClients(clients); err != nil {
		t.Fatalf("BatchUpsertClients falhou: %v", err)
	}

	// Verificar contagem
	count, err := xrayDB.CountClients()
	if err != nil || count != 2 {
		t.Fatalf("esperado 2 clientes, obteve %d (err: %v)", count, err)
	}

	// 2. Buscar cliente e verificar campos
	c1, err := xrayDB.GetClient("550e8400-e29b-41d4-a716-446655440001")
	if err != nil || c1 == nil {
		t.Fatalf("GetClient falhou: %v", err)
	}
	if c1.Name != "user1" || c1.MaxConns != 2 {
		t.Errorf("dados incorretos do cliente: %+v", c1)
	}

	// 3. Atualizar expiração
	newExpiry := now + 7200
	if err := xrayDB.UpdateExpiration(c1.UUID, newExpiry); err != nil {
		t.Fatalf("UpdateExpiration falhou: %v", err)
	}
	c1Updated, _ := xrayDB.GetClient(c1.UUID)
	if c1Updated.ExpiresAt != newExpiry {
		t.Errorf("esperado expiry %d, obteve %d", newExpiry, c1Updated.ExpiresAt)
	}

	// 4. Remover clientes expirados
	deleted, err := xrayDB.DeleteExpiredClients(now)
	if err != nil {
		t.Fatalf("DeleteExpiredClients falhou: %v", err)
	}
	if deleted != 1 {
		t.Errorf("esperado 1 cliente expirado deletado, obteve %d", deleted)
	}

	// Restou apenas o cliente válido
	countAfterExpire, _ := xrayDB.CountClients()
	if countAfterExpire != 1 {
		t.Errorf("esperado 1 cliente restante, obteve %d", countAfterExpire)
	}

	// 5. Deletar cliente específico
	if err := xrayDB.DeleteClients([]string{c1.UUID}); err != nil {
		t.Fatalf("DeleteClients falhou: %v", err)
	}
	countAfterDelete, _ := xrayDB.CountClients()
	if countAfterDelete != 0 {
		t.Errorf("esperado 0 clientes restantes, obteve %d", countAfterDelete)
	}
}

func TestXrayDB_DisabledWhenNilOrEmpty(t *testing.T) {
	db, err := NewXrayDB("")
	if err != nil {
		t.Fatalf("esperado sem erro para path vazio, obteve: %v", err)
	}
	if db.IsEnabled() {
		t.Error("esperado que XrayDB estivesse desabilitado quando path vazio")
	}

	// Operações em db desabilitado não devem dar panic nem erro
	if err := db.BatchUpsertClients([]models.XrayClient{{UUID: "test"}}); err != nil {
		t.Errorf("esperado sem erro em db desabilitado, obteve: %v", err)
	}
	if err := db.UpdateExpiration("test", 12345); err != nil {
		t.Errorf("esperado sem erro em db desabilitado, obteve: %v", err)
	}
	if err := db.DeleteClients([]string{"test"}); err != nil {
		t.Errorf("esperado sem erro em db desabilitado, obteve: %v", err)
	}
	if err := db.DeleteAllClients(); err != nil {
		t.Errorf("esperado sem erro em db desabilitado, obteve: %v", err)
	}
	deleted, err := db.DeleteExpiredClients(time.Now().Unix())
	if err != nil || deleted != 0 {
		t.Errorf("esperado 0 deleted sem erro, obteve %d, %v", deleted, err)
	}
	if err := db.Close(); err != nil {
		t.Errorf("esperado sem erro no Close, obteve: %v", err)
	}
}
