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

func TestSSHUserTestRequest_Validate(t *testing.T) {
	tests := []struct {
		name     string
		username string
		password string
		time     int
		wantErr  bool
	}{
		{"valid 1 hour", "user_test", "senha123", 1, false},
		{"valid 24 hours", "user_test", "senha123", 24, false},
		{"valid 72 hours max", "user_test", "senha123", 72, false},
		{"invalid 0 hours", "user_test", "senha123", 0, true},
		{"invalid negative hours", "user_test", "senha123", -1, true},
		{"invalid 73 hours (exceeds max 72h)", "user_test", "senha123", 73, true},
		{"invalid 100 hours", "user_test", "senha123", 100, true},
		{"invalid username too short", "us", "senha123", 2, true},
		{"invalid password too short", "user_test", "123", 2, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := SSHUserTestRequest{
				Username: tt.username,
				Password: tt.password,
				Time:     tt.time,
			}
			err := req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() time = %d, err = %v, wantErr %v", tt.time, err, tt.wantErr)
			}
		})
	}
}
