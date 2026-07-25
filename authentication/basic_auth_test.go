package authentication

import "testing"

func TestNewBaseAuth(t *testing.T) {
	got := NewBaseAuth("me", "secret")
	if got.Username != "me" || got.Password != "secret" {
		t.Errorf("NewBaseAuth = %+v", got)
	}
}

func TestNewBaseAuthFromMap(t *testing.T) {
	tests := []struct {
		name         string
		in           map[string]string
		wantUser     string
		wantPassword string
	}{
		{
			name:         "both fields",
			in:           map[string]string{"username": "me", "password": "secret"},
			wantUser:     "me",
			wantPassword: "secret",
		},
		{
			name:     "username only",
			in:       map[string]string{"username": "me"},
			wantUser: "me",
		},
		{
			name: "nil map yields empty credentials",
			in:   nil,
		},
		{
			name: "unrelated keys are ignored",
			in:   map[string]string{"user": "me"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewBaseAuthFromMap(tt.in)
			if got.Username != tt.wantUser {
				t.Errorf("Username = %q, want %q", got.Username, tt.wantUser)
			}
			if got.Password != tt.wantPassword {
				t.Errorf("Password = %q, want %q", got.Password, tt.wantPassword)
			}
		})
	}
}
