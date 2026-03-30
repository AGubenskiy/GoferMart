package auth

import (
	"testing"
	"time"
)

func TestSessionManagerIssueAndVerify(t *testing.T) {
	t.Parallel()

	manager := NewSessionManager("test-secret", time.Hour)

	token, err := manager.Issue(42)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	userID, err := manager.Verify(token)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}

	if userID != 42 {
		t.Fatalf("Verify() userID = %d, want 42", userID)
	}
}

func TestSessionManagerRejectsTamperedToken(t *testing.T) {
	t.Parallel()

	manager := NewSessionManager("test-secret", time.Hour)

	token, err := manager.Issue(42)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	token += "tampered"

	if _, err := manager.Verify(token); err == nil {
		t.Fatal("Verify() error = nil, want token validation error")
	}
}

func TestSessionManagerRejectsExpiredToken(t *testing.T) {
	t.Parallel()

	manager := NewSessionManager("test-secret", -time.Second)

	token, err := manager.Issue(42)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	if _, err := manager.Verify(token); err == nil {
		t.Fatal("Verify() error = nil, want expiration error")
	}
}
