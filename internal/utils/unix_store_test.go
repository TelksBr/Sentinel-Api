package utils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func createMockUnixEnv(t *testing.T) (string, func()) {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "unix_store_test_*")
	if err != nil {
		t.Fatalf("Erro ao criar temp dir: %v", err)
	}

	initialPasswd := `root:x:0:0:root:/root:/bin/bash
daemon:x:1:1:daemon:/usr/sbin:/usr/sbin/nologin
sshd:x:105:65534::/run/sshd:/usr/sbin/nologin
existinguser:x:1001:1001::/nonexistent:/bin/false
`
	initialShadow := `root:$6$salt$hash:19800:0:99999:7:::
daemon:*:19800:0:99999:7:::
sshd:*:19800:0:99999:7:::
existinguser:$6$salt$userhash:19800:0:99999:7::19850:
`
	initialGroup := `root:x:0:
daemon:x:1:
nogroup:x:65534:
existinguser:x:1001:
`
	initialGShadow := `root:*::
daemon:*::
nogroup:*::
existinguser:!::
`
	_ = os.WriteFile(filepath.Join(tempDir, "passwd"), []byte(initialPasswd), 0644)
	_ = os.WriteFile(filepath.Join(tempDir, "shadow"), []byte(initialShadow), 0640)
	_ = os.WriteFile(filepath.Join(tempDir, "group"), []byte(initialGroup), 0644)
	_ = os.WriteFile(filepath.Join(tempDir, "gshadow"), []byte(initialGShadow), 0640)

	cleanup := func() {
		_ = os.RemoveAll(tempDir)
	}
	return tempDir, cleanup
}

func TestUnixStore_LoadAndSave(t *testing.T) {
	tempDir, cleanup := createMockUnixEnv(t)
	defer cleanup()

	store := NewUnixStore(tempDir)
	if err := store.Load(); err != nil {
		t.Fatalf("Erro ao carregar store: %v", err)
	}

	if len(store.Passwd) != 4 {
		t.Errorf("Esperava 4 entradas no passwd, obteve %d", len(store.Passwd))
	}
	if len(store.Shadow) != 4 {
		t.Errorf("Esperava 4 entradas no shadow, obteve %d", len(store.Shadow))
	}

	// Adicionar um novo usuário
	hash, _ := Sha512Crypt("senha123", "salt1234", 5000)
	isNew := store.UpsertUser("novousuario", hash, 0, 0, DaysToShadowExpireDays(30), "/bin/false")
	if !isNew {
		t.Errorf("Esperava que novousuario fosse novo")
	}

	if err := store.Save(); err != nil {
		t.Fatalf("Erro ao salvar store: %v", err)
	}

	// Recarregar e verificar persistência
	store2 := NewUnixStore(tempDir)
	if err := store2.Load(); err != nil {
		t.Fatalf("Erro ao recarregar store: %v", err)
	}

	if len(store2.Passwd) != 5 {
		t.Errorf("Esperava 5 entradas no passwd após salvar, obteve %d", len(store2.Passwd))
	}

	idx, ok := store2.passwdMap["novousuario"]
	if !ok {
		t.Fatalf("novousuario não encontrado no map")
	}

	if store2.Passwd[idx].UID != 1000 && store2.Passwd[idx].UID != 1002 {
		t.Errorf("UID atribuído inesperado: %d", store2.Passwd[idx].UID)
	}
}

func TestUnixStore_SystemUserProtection(t *testing.T) {
	tempDir, cleanup := createMockUnixEnv(t)
	defer cleanup()

	store := NewUnixStore(tempDir)
	if err := store.Load(); err != nil {
		t.Fatalf("Erro ao carregar store: %v", err)
	}

	if !store.IsSystemUser("root") {
		t.Errorf("root deveria ser considerado system user")
	}
	if !store.IsSystemUser("sshd") {
		t.Errorf("sshd deveria ser considerado system user")
	}
	if store.IsSystemUser("existinguser") {
		t.Errorf("existinguser (UID 1001) não deveria ser system user")
	}

	// Tentar deletar root
	_, _, err := store.DeleteUser("root")
	if err == nil {
		t.Errorf("Deveria ter retornado erro ao tentar deletar root")
	}
}

func TestUnixStore_DeleteBatch(t *testing.T) {
	tempDir, cleanup := createMockUnixEnv(t)
	defer cleanup()

	store := NewUnixStore(tempDir)
	if err := store.Load(); err != nil {
		t.Fatalf("Erro ao carregar store: %v", err)
	}

	// Adicionar mais usuários
	store.UpsertUser("user1", "$6$hash1", 2001, 2001, "19900", "/bin/false")
	store.UpsertUser("user2", "$6$hash2", 2002, 2002, "19900", "/bin/false")
	store.UpsertUser("user3", "$6$hash3", 2003, 2003, "19900", "/bin/false")
	_ = store.Save()

	deletedUIDs, notFound, sysUsers := store.DeleteUsers([]string{"user1", "user3", "root", "naoexiste"})

	if len(deletedUIDs) != 2 {
		t.Errorf("Esperava 2 UIDs deletados, obteve %d (%v)", len(deletedUIDs), deletedUIDs)
	}
	if len(notFound) != 1 || notFound[0] != "naoexiste" {
		t.Errorf("Esperava 'naoexiste' em notFound, obteve %v", notFound)
	}
	if len(sysUsers) != 1 || sysUsers[0] != "root" {
		t.Errorf("Esperava 'root' em sysUsers, obteve %v", sysUsers)
	}

	_ = store.Save()

	// Recarregar e checar
	store2 := NewUnixStore(tempDir)
	_ = store2.Load()
	if _, ok := store2.passwdMap["user1"]; ok {
		t.Errorf("user1 ainda existe no passwd")
	}
	if _, ok := store2.passwdMap["user2"]; !ok {
		t.Errorf("user2 deveria ter sido mantido")
	}
	if _, ok := store2.passwdMap["user3"]; ok {
		t.Errorf("user3 ainda existe no passwd")
	}
}

func TestUnixStore_DisableAndEnable(t *testing.T) {
	tempDir, cleanup := createMockUnixEnv(t)
	defer cleanup()

	store := NewUnixStore(tempDir)
	_ = store.Load()

	// Desabilitar existinguser
	uid, err := store.SetUserDisabled("existinguser")
	if err != nil {
		t.Fatalf("Erro ao desabilitar usuário: %v", err)
	}
	if uid != 1001 {
		t.Errorf("UID incorreto retornado: %d", uid)
	}

	pIdx := store.passwdMap["existinguser"]
	sIdx := store.shadowMap["existinguser"]

	if store.Passwd[pIdx].Shell != "/usr/sbin/nologin" {
		t.Errorf("Shell deveria ser nologin, obteve %s", store.Passwd[pIdx].Shell)
	}
	if !strings.HasPrefix(store.Shadow[sIdx].PasswordHash, "!") {
		t.Errorf("Password hash deveria ter prefixo !, obteve %s", store.Shadow[sIdx].PasswordHash)
	}

	// Reabilitar existinguser
	newExpire := DaysToShadowExpireDays(30)
	err = store.SetUserEnabled("existinguser", newExpire)
	if err != nil {
		t.Fatalf("Erro ao reabilitar usuário: %v", err)
	}

	if store.Passwd[pIdx].Shell != "/bin/false" {
		t.Errorf("Shell deveria ser /bin/false, obteve %s", store.Passwd[pIdx].Shell)
	}
	if strings.HasPrefix(store.Shadow[sIdx].PasswordHash, "!") {
		t.Errorf("Password hash não deveria ter prefixo !, obteve %s", store.Shadow[sIdx].PasswordHash)
	}
	if store.Shadow[sIdx].ExpireDays != newExpire {
		t.Errorf("Data de expiração incorreta: %s vs %s", store.Shadow[sIdx].ExpireDays, newExpire)
	}
}
