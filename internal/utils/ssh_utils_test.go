package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSanitizeUsername(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid lowercase", "joao", false},
		{"valid with numbers", "user123", false},
		{"valid with underscore", "test_user", false},
		{"valid with dash", "test-user", false},
		{"valid with dot", "test.user", false},
		{"valid mixed", "User_01.test-ok", false},
		{"empty", "", true},
		{"too long 33 chars", "abcdefghijklmnopqrstuvwxyz1234567", true},
		{"max 32 chars", "abcdefghijklmnopqrstuvwxyz123456", false},
		{"command injection semicolon", "user;rm -rf /", true},
		{"command injection backtick", "user`whoami`", true},
		{"command injection pipe", "user|cat /etc/passwd", true},
		{"command injection dollar", "user$(whoami)", true},
		{"command injection ampersand", "user&&echo pwned", true},
		{"spaces", "user name", true},
		{"slash", "user/name", true},
		{"single char", "a", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sanitizeUsername(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("sanitizeUsername(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestIsReservedUsername(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		reserved bool
	}{
		{"root", "root", true},
		{"admin", "admin", true},
		{"sshd", "sshd", true},
		{"www-data", "www-data", true},
		{"nobody", "nobody", true},
		{"ubuntu", "ubuntu", true},
		{"case insensitive ROOT", "ROOT", true},
		{"case insensitive Admin", "Admin", true},
		{"normal user", "joao", false},
		{"similar to reserved", "root2", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsReservedUsername(tt.input)
			if got != tt.reserved {
				t.Errorf("IsReservedUsername(%q) = %v, want %v", tt.input, got, tt.reserved)
			}
		})
	}
}

func TestGetUsersWithBinFalseShell(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "passwd_test_*")
	if err != nil {
		t.Fatalf("Erro ao criar tempDir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	passwdContent := `root:x:0:0:root:/root:/bin/bash
daemon:x:1:1:daemon:/usr/sbin:/usr/sbin/nologin
systemd-network:x:998:998:systemd Network Management:/:/usr/sbin/nologin
ubuntu:x:1000:1000:Ubuntu:/home/ubuntu:/bin/bash
admin_human:x:1001:1001:Admin:/home/admin_human:/bin/bash
vpn_user1:x:1002:1002::/home/vpn_user1:/bin/false
vpn_user2:x:1003:1003::/home/vpn_user2:/usr/bin/false
disabled_vpn:x:1004:1004::/home/disabled_vpn:/usr/sbin/nologin
sys_false:x:999:999::/nonexistent:/bin/false
`

	passwdFile := filepath.Join(tempDir, "passwd")
	if err := os.WriteFile(passwdFile, []byte(passwdContent), 0644); err != nil {
		t.Fatalf("Erro ao escrever passwd de teste: %v", err)
	}

	users, err := GetUsersWithBinFalseShell(passwdFile)
	if err != nil {
		t.Fatalf("GetUsersWithBinFalseShell retornou erro inesperado: %v", err)
	}

	// vpn_user1 deve estar presente
	if !users["vpn_user1"] {
		t.Errorf("vpn_user1 deveria estar presente no map de usuários /bin/false")
	}

	// vpn_user2 deve estar presente (/usr/bin/false)
	if !users["vpn_user2"] {
		t.Errorf("vpn_user2 deveria estar presente no map de usuários /bin/false")
	}

	// root NUNCA deve estar presente
	if users["root"] {
		t.Errorf("root NUNCA deve estar presente no map de usuários /bin/false")
	}

	// ubuntu (reservado / bash) não deve estar presente
	if users["ubuntu"] {
		t.Errorf("ubuntu não deve estar presente")
	}

	// admin_human (shell /bin/bash) não deve estar presente
	if users["admin_human"] {
		t.Errorf("admin_human (/bin/bash) não deve estar presente")
	}

	// disabled_vpn (/usr/sbin/nologin) não deve estar presente
	if users["disabled_vpn"] {
		t.Errorf("disabled_vpn (/usr/sbin/nologin) não deve estar presente")
	}

	// sys_false (UID 999 < 1000) não deve estar presente
	if users["sys_false"] {
		t.Errorf("sys_false (UID < 1000) não deve estar presente")
	}

	if len(users) != 2 {
		t.Errorf("Esperava exatamente 2 usuários válidos, obteve %d: %v", len(users), users)
	}

	// Testar arquivo inexistente
	nonExistentUsers, err := GetUsersWithBinFalseShell(filepath.Join(tempDir, "nonexistent"))
	if err == nil {
		t.Errorf("Esperava erro para arquivo inexistente")
	}
	if len(nonExistentUsers) != 0 {
		t.Errorf("Esperava map vazio para arquivo inexistente")
	}
}
