package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestValidatePassword(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{
			name:     "lowercase symbol and digit",
			password: "strong-password1",
		},
		{
			name:     "uppercase lowercase and digit",
			password: "Password1",
		},
		{
			name:     "only two classes",
			password: "strong-password",
			wantErr:  true,
		},
		{
			name:     "too short",
			password: "a123-",
			wantErr:  true,
		},
		{
			name:     "only single",
			password: "password",
			wantErr:  true,
		},
		{
			name:     "bcrypt byte limit",
			password: strings.Repeat("A", MaxPasswordBytes-1) + "1!",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidatePassword(tt.password)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidatePassword() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHashPasswordRejectsWeakPassword(t *testing.T) {
	t.Parallel()

	if _, err := HashPassword("password"); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("HashPassword() error = %v, want %v", err, ErrWeakPassword)
	}
}
