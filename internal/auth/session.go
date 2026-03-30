package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const CookieName = "gophermart_session"

type SessionManager struct {
	secret []byte
	ttl    time.Duration
}

func NewSessionManager(secret string, ttl time.Duration) *SessionManager {
	return &SessionManager{
		secret: []byte(secret),
		ttl:    ttl,
	}
}

func (m *SessionManager) Issue(userID int64) (string, error) {
	if userID <= 0 {
		return "", fmt.Errorf("invalid user ID: %d", userID)
	}

	expiresAt := time.Now().Add(m.ttl).Unix()
	payload := strconv.FormatInt(userID, 10) + ":" + strconv.FormatInt(expiresAt, 10)
	signature := m.sign(payload)

	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (m *SessionManager) Verify(token string) (int64, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid token format")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return 0, fmt.Errorf("decode token payload: %w", err)
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0, fmt.Errorf("decode token signature: %w", err)
	}

	payload := string(payloadBytes)
	expectedSignature := m.sign(payload)
	if !hmac.Equal(signature, expectedSignature) {
		return 0, fmt.Errorf("invalid token signature")
	}

	fields := strings.Split(payload, ":")
	if len(fields) != 2 {
		return 0, fmt.Errorf("invalid token payload")
	}

	userID, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse token user ID: %w", err)
	}

	expiresAtUnix, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse token expiration: %w", err)
	}

	if time.Now().After(time.Unix(expiresAtUnix, 0)) {
		return 0, fmt.Errorf("token expired")
	}

	return userID, nil
}

func (m *SessionManager) BuildCookie(token string) *http.Cookie {
	return &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(m.ttl.Seconds()),
	}
}

func (m *SessionManager) ExpireCookie() *http.Cookie {
	return &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	}
}

func (m *SessionManager) sign(payload string) []byte {
	mac := hmac.New(sha256.New, m.secret)
	mac.Write([]byte(payload))
	return mac.Sum(nil)
}
