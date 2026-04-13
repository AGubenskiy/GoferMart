package auth

import (
	"encoding/base64"
	"strings"
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

func TestSessionManagerIssuesJWT(t *testing.T) {
	t.Parallel()

	manager := NewSessionManager("test-secret", time.Hour)

	token, err := manager.Issue(42)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d parts, want 3", len(parts))
	}

	var header jwtHeader
	if err := decodeJWTPart(parts[0], &header); err != nil {
		t.Fatalf("decode JWT header: %v", err)
	}

	if header.Algorithm != jwtAlgorithm || header.Type != "JWT" {
		t.Fatalf("JWT header = %+v, want alg=%s typ=JWT", header, jwtAlgorithm)
	}

	var claims sessionClaims
	if err := decodeJWTPart(parts[1], &claims); err != nil {
		t.Fatalf("decode JWT claims: %v", err)
	}

	if claims.Subject != "42" || claims.ExpiresAt == 0 || claims.IssuedAt == 0 {
		t.Fatalf("JWT claims = %+v, want subject and timestamps", claims)
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

func TestSessionManagerRejectsUnexpectedJWTAlgorithm(t *testing.T) {
	t.Parallel()

	manager := NewSessionManager("test-secret", time.Hour)
	now := time.Now()

	encodedHeader, err := encodeJWTPart(jwtHeader{Algorithm: "none", Type: "JWT"})
	if err != nil {
		t.Fatalf("encode JWT header: %v", err)
	}

	encodedClaims, err := encodeJWTPart(sessionClaims{
		Subject:   "42",
		ExpiresAt: now.Add(time.Hour).Unix(),
		IssuedAt:  now.Unix(),
	})
	if err != nil {
		t.Fatalf("encode JWT claims: %v", err)
	}

	signingInput := encodedHeader + "." + encodedClaims
	token := signingInput + "." + base64.RawURLEncoding.EncodeToString(manager.sign(signingInput))

	if _, err := manager.Verify(token); err == nil {
		t.Fatal("Verify() error = nil, want algorithm validation error")
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
