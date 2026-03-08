package handler

import "testing"

func TestValidateRegister(t *testing.T) {
	tests := []struct {
		name    string
		user    string
		email   string
		pass    string
		wantErr bool
	}{
		{name: "valid", user: "alice", email: "alice@example.com", pass: "password123", wantErr: false},
		{name: "empty username", user: "", email: "alice@example.com", pass: "password123", wantErr: true},
		{name: "invalid email", user: "alice", email: "bad", pass: "password123", wantErr: true},
		{name: "short password", user: "alice", email: "alice@example.com", pass: "short", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateRegister(tc.user, tc.email, tc.pass)
			if (err != nil) != tc.wantErr {
				t.Fatalf("wantErr=%v got err=%v", tc.wantErr, err)
			}
		})
	}
}

func TestValidateRoom(t *testing.T) {
	if err := validateRoom("general", "desc"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if err := validateRoom("", "desc"); err == nil {
		t.Fatal("expected error for empty room name")
	}
}

func TestValidateContent(t *testing.T) {
	if err := validateContent("hello"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if err := validateContent("   "); err == nil {
		t.Fatal("expected error for blank content")
	}
}
