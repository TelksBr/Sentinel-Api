package models

import (
	"testing"
)

func TestSSHUser_ValidateUsernames(t *testing.T) {
	tests := []struct {
		name     string
		username string
		wantErr  bool
	}{
		{"alphanumeric", "usuario123", false},
		{"with underscore", "user_0001_1hia", false},
		{"with dash", "user-0002-test", false},
		{"with dot", "user.0003.test", false},
		{"with mixed allowed chars", "User_01.test-ok", false},
		{"with colon (invalid delimiter)", "user:name", true},
		{"with spaces (invalid)", "user name", true},
		{"with semicolon (invalid injection)", "user;rm -rf /", true},
		{"too short", "ab", true},
		{"too long", "abcdefghijklmnopqrstuvwxyz1234567", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := SSHUser{
				Username:     tt.username,
				Password:     "senha12345",
				ValidateDays: 30,
			}
			err := user.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() username = %s, err = %v, wantErr %v", tt.username, err, tt.wantErr)
			}
		})
	}
}
