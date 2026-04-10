package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/AGubenskiy/GoferMart/internal/auth"
	"github.com/AGubenskiy/GoferMart/internal/http/middleware"
	"github.com/AGubenskiy/GoferMart/internal/money"
	"github.com/AGubenskiy/GoferMart/internal/service"
	"github.com/AGubenskiy/GoferMart/internal/storage/postgres"
	"github.com/lib/pq"
)

type testOrderResponse struct {
	Number     string        `json:"number"`
	Status     string        `json:"status"`
	Accrual    *money.Amount `json:"accrual,omitempty"`
	UploadedAt string        `json:"uploaded_at"`
}

type testBalanceResponse struct {
	Current   money.Amount `json:"current"`
	Withdrawn money.Amount `json:"withdrawn"`
}

type testWithdrawalResponse struct {
	Order       string       `json:"order"`
	Sum         money.Amount `json:"sum"`
	ProcessedAt string       `json:"processed_at"`
}

func TestRouterWithoutDependenciesReturnsInternalServerError(t *testing.T) {
	t.Parallel()

	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), Dependencies{})

	tests := []struct {
		method string
		path   string
	}{
		{method: http.MethodPost, path: "/api/user/register"},
		{method: http.MethodPost, path: "/api/user/login"},
		{method: http.MethodPost, path: "/api/user/orders"},
		{method: http.MethodGet, path: "/api/user/balance"},
	}

	for _, tt := range tests {
		request := httptest.NewRequest(tt.method, tt.path, nil)
		recorder := httptest.NewRecorder()

		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("%s %s status = %d, want %d", tt.method, tt.path, recorder.Code, http.StatusInternalServerError)
		}
	}
}

func TestUserRoutesFollowSpecification(t *testing.T) {
	store := openTestStore(t)
	server, _ := newTestServer(t, store)

	client := newTestHTTPClient(t)

	doRequest(t, client, http.MethodPost, server.URL+"/api/user/register", "text/plain", []byte("bad"), http.StatusBadRequest)

	registerBody := []byte(`{"login":"alice","password":"Strong-password1"}`)
	response := doRequest(t, client, http.MethodPost, server.URL+"/api/user/register", "application/json", registerBody, http.StatusOK)
	if len(response) != 0 {
		t.Fatalf("register response body length = %d, want 0", len(response))
	}
	if len(client.Jar.Cookies(mustParseURL(t, server.URL))) == 0 {
		t.Fatal("register did not set session cookie")
	}

	doRequest(t, client, http.MethodPost, server.URL+"/api/user/register", "application/json", registerBody, http.StatusConflict)
	doRequest(t, client, http.MethodPost, server.URL+"/api/user/register", "application/json", []byte(`{"login":"bob","password":"weak"}`), http.StatusBadRequest)

	doRequest(t, client, http.MethodPost, server.URL+"/api/user/login", "text/plain", []byte("bad"), http.StatusBadRequest)
	doRequest(t, client, http.MethodPost, server.URL+"/api/user/login", "application/json", []byte(`{"login":"alice","password":"wrong-password"}`), http.StatusUnauthorized)
	doRequest(t, client, http.MethodPost, server.URL+"/api/user/login", "application/json", registerBody, http.StatusOK)
}

func TestAccountRoutesFollowSpecification(t *testing.T) {
	store := openTestStore(t)
	server, sessions := newTestServer(t, store)

	anonClient := newTestHTTPClient(t)
	userClient := newTestHTTPClient(t)
	otherClient := newTestHTTPClient(t)

	doRequest(t, anonClient, http.MethodGet, server.URL+"/api/user/balance", "", nil, http.StatusUnauthorized)

	registerAndLogin(t, userClient, server.URL, "alice", "Strong-password1", http.StatusOK)
	registerAndLogin(t, otherClient, server.URL, "bob", "Another-password1", http.StatusOK)

	doRequest(t, userClient, http.MethodGet, server.URL+"/api/user/orders", "", nil, http.StatusNoContent)
	doRequest(t, userClient, http.MethodGet, server.URL+"/api/user/withdrawals", "", nil, http.StatusNoContent)

	doRequest(t, userClient, http.MethodPost, server.URL+"/api/user/orders", "application/json", []byte(`{"number":"12345678903"}`), http.StatusBadRequest)
	doRequest(t, userClient, http.MethodPost, server.URL+"/api/user/orders", "text/plain", []byte("12345678904"), http.StatusUnprocessableEntity)
	doRequest(t, userClient, http.MethodPost, server.URL+"/api/user/orders", "text/plain", []byte("12345678903"), http.StatusAccepted)
	doRequest(t, userClient, http.MethodPost, server.URL+"/api/user/orders", "text/plain", []byte("12345678903"), http.StatusOK)
	doRequest(t, otherClient, http.MethodPost, server.URL+"/api/user/orders", "text/plain", []byte("12345678903"), http.StatusConflict)

	ordersBody := doRequest(t, userClient, http.MethodGet, server.URL+"/api/user/orders", "", nil, http.StatusOK)
	var orders []testOrderResponse
	if err := json.Unmarshal(ordersBody, &orders); err != nil {
		t.Fatalf("decode orders response: %v; body=%s", err, string(ordersBody))
	}
	if len(orders) != 1 {
		t.Fatalf("orders len = %d, want 1", len(orders))
	}
	if orders[0].Number != "12345678903" || orders[0].Status != "NEW" || orders[0].Accrual != nil {
		t.Fatalf("orders[0] = %+v, want NEW order without accrual", orders[0])
	}
	if _, err := time.Parse(time.RFC3339, orders[0].UploadedAt); err != nil {
		t.Fatalf("orders[0].UploadedAt = %q, want RFC3339", orders[0].UploadedAt)
	}

	balanceBody := doRequest(t, userClient, http.MethodGet, server.URL+"/api/user/balance", "", nil, http.StatusOK)
	var balance testBalanceResponse
	if err := json.Unmarshal(balanceBody, &balance); err != nil {
		t.Fatalf("decode balance response: %v; body=%s", err, string(balanceBody))
	}
	if balance.Current != 0 || balance.Withdrawn != 0 {
		t.Fatalf("balance = %+v, want zero balance", balance)
	}

	doRequest(t, userClient, http.MethodPost, server.URL+"/api/user/balance/withdraw", "text/plain", []byte("bad"), http.StatusBadRequest)
	doRequest(t, userClient, http.MethodPost, server.URL+"/api/user/balance/withdraw", "application/json", []byte(`{"order":"123","sum":100}`), http.StatusUnprocessableEntity)
	doRequest(t, userClient, http.MethodPost, server.URL+"/api/user/balance/withdraw", "application/json", []byte(`{"order":"2377225624","sum":100}`), http.StatusPaymentRequired)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	accrual := money.NewFromCents(50050)
	if err := store.Repositories().Orders.UpdateStatus(ctx, "12345678903", "PROCESSED", &accrual); err != nil {
		t.Fatalf("UpdateStatus() error = %v", err)
	}

	balanceBody = doRequest(t, userClient, http.MethodGet, server.URL+"/api/user/balance", "", nil, http.StatusOK)
	if err := json.Unmarshal(balanceBody, &balance); err != nil {
		t.Fatalf("decode updated balance response: %v; body=%s", err, string(balanceBody))
	}
	if balance.Current != money.NewFromCents(50050) || balance.Withdrawn != 0 {
		t.Fatalf("updated balance = %+v, want current=500.50 withdrawn=0", balance)
	}

	doRequest(t, userClient, http.MethodPost, server.URL+"/api/user/balance/withdraw", "application/json", []byte(`{"order":"2377225624","sum":100}`), http.StatusOK)
	doRequest(t, userClient, http.MethodPost, server.URL+"/api/user/balance/withdraw", "application/json", []byte(`{"order":"2377225624","sum":1}`), http.StatusUnprocessableEntity)

	balanceBody = doRequest(t, userClient, http.MethodGet, server.URL+"/api/user/balance", "", nil, http.StatusOK)
	if err := json.Unmarshal(balanceBody, &balance); err != nil {
		t.Fatalf("decode final balance response: %v; body=%s", err, string(balanceBody))
	}
	if balance.Current != money.NewFromCents(40050) || balance.Withdrawn != money.NewFromCents(10000) {
		t.Fatalf("final balance = %+v, want current=400.50 withdrawn=100.00", balance)
	}

	ordersBody = doRequest(t, userClient, http.MethodGet, server.URL+"/api/user/orders", "", nil, http.StatusOK)
	if err := json.Unmarshal(ordersBody, &orders); err != nil {
		t.Fatalf("decode processed orders response: %v; body=%s", err, string(ordersBody))
	}
	if len(orders) != 1 || orders[0].Accrual == nil || *orders[0].Accrual != money.NewFromCents(50050) || orders[0].Status != "PROCESSED" {
		t.Fatalf("processed orders = %+v, want PROCESSED order with accrual", orders)
	}

	withdrawalsBody := doRequest(t, userClient, http.MethodGet, server.URL+"/api/user/withdrawals", "", nil, http.StatusOK)
	var withdrawals []testWithdrawalResponse
	if err := json.Unmarshal(withdrawalsBody, &withdrawals); err != nil {
		t.Fatalf("decode withdrawals response: %v; body=%s", err, string(withdrawalsBody))
	}
	if len(withdrawals) != 1 {
		t.Fatalf("withdrawals len = %d, want 1", len(withdrawals))
	}
	if withdrawals[0].Order != "2377225624" || withdrawals[0].Sum != money.NewFromCents(10000) {
		t.Fatalf("withdrawals[0] = %+v, want successful withdrawal", withdrawals[0])
	}
	if _, err := time.Parse(time.RFC3339, withdrawals[0].ProcessedAt); err != nil {
		t.Fatalf("withdrawals[0].ProcessedAt = %q, want RFC3339", withdrawals[0].ProcessedAt)
	}

	token, err := sessions.Issue(999)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/user/balance", nil)
	request.AddCookie(sessions.BuildCookie(token))
	recorder := httptest.NewRecorder()
	NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), Dependencies{
		UserHandler:    NewUserHandler(service.NewAuthService(store.Repositories().Users, sessions), sessions),
		AccountHandler: NewAccountHandler(service.NewLoyaltyService(store)),
		AuthMiddleware: middleware.AuthRequired(sessions),
	}).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /api/user/balance with missing user status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func newTestServer(t *testing.T, store *postgres.Store) (*httptest.Server, *auth.SessionManager) {
	t.Helper()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	sessions := auth.NewSessionManager("handler-test-secret", time.Hour)

	authService := service.NewAuthService(store.Repositories().Users, sessions)
	loyaltyService := service.NewLoyaltyService(store)

	server := httptest.NewServer(NewRouter(log, Dependencies{
		UserHandler:    NewUserHandler(authService, sessions),
		AccountHandler: NewAccountHandler(loyaltyService),
		AuthMiddleware: middleware.AuthRequired(sessions),
	}))
	t.Cleanup(server.Close)

	return server, sessions
}

func registerAndLogin(t *testing.T, client *http.Client, baseURL, login, password string, wantStatus int) {
	t.Helper()

	body := []byte(fmt.Sprintf(`{"login":%q,"password":%q}`, login, password))
	doRequest(t, client, http.MethodPost, baseURL+"/api/user/register", "application/json", body, wantStatus)
	doRequest(t, client, http.MethodPost, baseURL+"/api/user/login", "application/json", body, http.StatusOK)
}

func doRequest(t *testing.T, client *http.Client, method, requestURL, contentType string, body []byte, wantStatus int) []byte {
	t.Helper()

	request, err := http.NewRequest(method, requestURL, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest(%s, %s) error = %v", method, requestURL, err)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}

	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("Do(%s, %s) error = %v", method, requestURL, err)
	}
	defer response.Body.Close()

	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("ReadAll(%s, %s) error = %v", method, requestURL, err)
	}

	if response.StatusCode != wantStatus {
		t.Fatalf("%s %s status = %d, want %d; body=%s", method, requestURL, response.StatusCode, wantStatus, string(payload))
	}

	return payload
}

func newTestHTTPClient(t *testing.T) *http.Client {
	t.Helper()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New() error = %v", err)
	}

	return &http.Client{
		Jar:     jar,
		Timeout: 5 * time.Second,
	}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()

	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse(%q) error = %v", raw, err)
	}

	return parsed
}

func openTestStore(t *testing.T) *postgres.Store {
	t.Helper()

	dsn, cleanup := createTemporaryDatabase(t)
	t.Cleanup(cleanup)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	store, err := postgres.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("postgres.Open() error = %v", err)
	}

	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("store.Close() error = %v", err)
		}
	})

	return store
}

func createTemporaryDatabase(t *testing.T) (string, func()) {
	t.Helper()

	adminDSN, explicit := testAdminDSN()
	tempDBName := fmt.Sprintf("gophermart_handler_test_%d", time.Now().UnixNano())

	adminDB, err := sql.Open("postgres", adminDSN)
	if err != nil {
		t.Fatalf("open admin database handle: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := adminDB.PingContext(ctx); err != nil {
		_ = adminDB.Close()
		if explicit {
			t.Fatalf("ping test database %q: %v", adminDSN, err)
		}
		t.Skipf("PostgreSQL is not available for integration tests: %v", err)
	}

	if _, err := adminDB.ExecContext(ctx, "CREATE DATABASE "+pq.QuoteIdentifier(tempDBName)); err != nil {
		_ = adminDB.Close()
		t.Fatalf("create temporary database %q: %v", tempDBName, err)
	}

	tempDSN, err := replaceDatabaseName(adminDSN, tempDBName)
	if err != nil {
		dropTemporaryDatabase(t, adminDB, tempDBName)
		_ = adminDB.Close()
		t.Fatalf("build temporary database DSN: %v", err)
	}

	cleanup := func() {
		dropTemporaryDatabase(t, adminDB, tempDBName)
		if err := adminDB.Close(); err != nil {
			t.Fatalf("close admin database handle: %v", err)
		}
	}

	return tempDSN, cleanup
}

func testAdminDSN() (string, bool) {
	if dsn := strings.TrimSpace(os.Getenv("GOFERMART_TEST_DATABASE_URI")); dsn != "" {
		return dsn, true
	}
	if dsn := strings.TrimSpace(os.Getenv("DATABASE_URI")); dsn != "" {
		return dsn, true
	}

	return "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable", false
}

func replaceDatabaseName(dsn string, databaseName string) (string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}

	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return "", fmt.Errorf("only postgres:// and postgresql:// DSNs are supported")
	}

	parsed.Path = "/" + databaseName
	return parsed.String(), nil
}

func dropTemporaryDatabase(t *testing.T, adminDB *sql.DB, databaseName string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, _ = adminDB.ExecContext(
		ctx,
		`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1`,
		databaseName,
	)
	if _, err := adminDB.ExecContext(ctx, "DROP DATABASE IF EXISTS "+pq.QuoteIdentifier(databaseName)); err != nil {
		t.Fatalf("drop temporary database %q: %v", databaseName, err)
	}
}
