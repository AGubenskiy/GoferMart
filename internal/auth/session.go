package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const CookieName = "gophermart_session"

const jwtAlgorithm = "HS256"

// нужно ли переходить на jwt/v5 ?
type jwtHeader struct {
	Algorithm string `json:"alg"`
	Type      string `json:"typ"`
}

type sessionClaims struct {
	Subject   string `json:"sub"`
	ExpiresAt int64  `json:"exp"`
	IssuedAt  int64  `json:"iat"`
}

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

	now := time.Now()
	header := jwtHeader{
		Algorithm: jwtAlgorithm,
		Type:      "JWT",
	}
	claims := sessionClaims{
		Subject:   strconv.FormatInt(userID, 10),
		ExpiresAt: now.Add(m.ttl).Unix(),
		IssuedAt:  now.Unix(),
	}

	encodedHeader, err := encodeJWTPart(header)
	if err != nil {
		return "", fmt.Errorf("encode JWT header: %w", err)
	}

	encodedClaims, err := encodeJWTPart(claims)
	if err != nil {
		return "", fmt.Errorf("encode JWT claims: %w", err)
	}

	signingInput := encodedHeader + "." + encodedClaims
	signature := m.sign(signingInput)

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (m *SessionManager) Verify(token string) (int64, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid JWT format")
	}

	var header jwtHeader
	if err := decodeJWTPart(parts[0], &header); err != nil {
		return 0, fmt.Errorf("decode JWT header: %w", err)
	}

	if header.Algorithm != jwtAlgorithm {
		return 0, fmt.Errorf("unexpected JWT algorithm: %s", header.Algorithm)
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return 0, fmt.Errorf("decode JWT signature: %w", err)
	}

	signingInput := parts[0] + "." + parts[1]
	expectedSignature := m.sign(signingInput)
	if !hmac.Equal(signature, expectedSignature) {
		return 0, fmt.Errorf("invalid JWT signature")
	}

	var claims sessionClaims
	if err := decodeJWTPart(parts[1], &claims); err != nil {
		return 0, fmt.Errorf("decode JWT claims: %w", err)
	}

	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse JWT subject: %w", err)
	}
	if userID <= 0 {
		return 0, fmt.Errorf("invalid JWT subject: %d", userID)
	}

	if time.Now().Unix() >= claims.ExpiresAt {
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

func encodeJWTPart(value any) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeJWTPart(encoded string, target any) error {
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return err
	}

	return json.Unmarshal(payload, target)
}
