package utils

import (
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
