package integration

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
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AGubenskiy/GoferMart/internal/accrual"
	"github.com/AGubenskiy/GoferMart/internal/auth"
	httphandler "github.com/AGubenskiy/GoferMart/internal/http/handler"
	"github.com/AGubenskiy/GoferMart/internal/http/middleware"
	"github.com/AGubenskiy/GoferMart/internal/money"
	"github.com/AGubenskiy/GoferMart/internal/service"
	"github.com/AGubenskiy/GoferMart/internal/storage/postgres"
	"github.com/AGubenskiy/GoferMart/internal/worker"
	"github.com/lib/pq"
)

const (
	uploadedOrderNumber   = "12345678903"
	withdrawalOrderNumber = "2377225624"
)

type orderPayload struct {
	Number     string        `json:"number"`
	Status     string        `json:"status"`
	Accrual    *money.Amount `json:"accrual,omitempty"`
	UploadedAt string        `json:"uploaded_at"`
}

type balancePayload struct {
	Current   money.Amount `json:"current"`
	Withdrawn money.Amount `json:"withdrawn"`
}

type withdrawalPayload struct {
	Order       string       `json:"order"`
	Sum         money.Amount `json:"sum"`
	ProcessedAt string       `json:"processed_at"`
}

func TestGophermartMainFlow(t *testing.T) {
	store := openIntegrationStore(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	accrualServer := newFakeAccrualServer(t)
	accrualClient, err := accrual.NewClient(accrualServer.URL)
	if err != nil {
		t.Fatalf("create accrual client: %v", err)
	}

	sessions := auth.NewSessionManager("integration-test-secret", time.Hour)
	authService := service.NewAuthService(store.Repositories().Users, sessions)
	loyaltyService := service.NewLoyaltyService(store)

	apiServer := httptest.NewServer(httphandler.NewRouter(log, httphandler.Dependencies{
		UserHandler:    httphandler.NewUserHandler(authService, sessions),
		AccountHandler: httphandler.NewAccountHandler(loyaltyService),
		AuthMiddleware: middleware.AuthRequired(sessions),
	}))
	t.Cleanup(apiServer.Close)

	client := newTestHTTPClient(t)
	otherClient := newTestHTTPClient(t)

	doRequest(t, client, http.MethodGet, apiServer.URL+"/api/user/balance", "", nil, http.StatusUnauthorized)

	registerUser(t, client, apiServer.URL, "alice", "strong-password", http.StatusOK)
	registerUser(t, otherClient, apiServer.URL, "alice", "other-password", http.StatusConflict)
	loginUser(t, otherClient, apiServer.URL, "alice", "wrong-password", http.StatusUnauthorized)
	loginUser(t, client, apiServer.URL, "alice", "strong-password", http.StatusOK)

	uploadOrder(t, client, apiServer.URL, "12345678904", http.StatusUnprocessableEntity)
	uploadOrder(t, client, apiServer.URL, uploadedOrderNumber, http.StatusAccepted)
	uploadOrder(t, client, apiServer.URL, uploadedOrderNumber, http.StatusOK)

	registerUser(t, otherClient, apiServer.URL, "bob", "another-password", http.StatusOK)
	uploadOrder(t, otherClient, apiServer.URL, uploadedOrderNumber, http.StatusConflict)
	doRequest(t, otherClient, http.MethodGet, apiServer.URL+"/api/user/orders", "", nil, http.StatusNoContent)
	doRequest(t, otherClient, http.MethodGet, apiServer.URL+"/api/user/withdrawals", "", nil, http.StatusNoContent)

	orders := listOrders(t, client, apiServer.URL, http.StatusOK)
	if len(orders) != 1 {
		t.Fatalf("orders len = %d, want 1", len(orders))
	}
	assertOrder(t, orders[0], uploadedOrderNumber, "NEW")
	assertOrderWithoutAccrual(t, orders[0])

	workerCtx, stopWorker := context.WithCancel(context.Background())
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		worker.NewAccrualWorker(log, accrualClient, store.Repositories().Orders).Run(workerCtx)
	}()
	t.Cleanup(func() {
		stopWorker()
		<-workerDone
	})

	waitForBalance(t, client, apiServer.URL, balancePayload{
		Current:   money.NewFromCents(50050),
		Withdrawn: 0,
	})

	orders = listOrders(t, client, apiServer.URL, http.StatusOK)
	if len(orders) != 1 {
		t.Fatalf("orders len = %d, want 1", len(orders))
	}
	assertOrder(t, orders[0], uploadedOrderNumber, "PROCESSED")
	assertOrderAccrual(t, orders[0], money.NewFromCents(50050))

	withdraw(t, client, apiServer.URL, "49927398716", money.NewFromCents(100000), http.StatusPaymentRequired)
	withdraw(t, client, apiServer.URL, "49927398717", money.NewFromCents(10000), http.StatusUnprocessableEntity)
	withdraw(t, client, apiServer.URL, withdrawalOrderNumber, money.NewFromCents(10000), http.StatusOK)
	withdraw(t, client, apiServer.URL, withdrawalOrderNumber, money.NewFromCents(1000), http.StatusUnprocessableEntity)

	balance := getBalance(t, client, apiServer.URL, http.StatusOK)
	if balance.Current != money.NewFromCents(40050) || balance.Withdrawn != money.NewFromCents(10000) {
		t.Fatalf("balance = %+v, want current=400.5 withdrawn=100", balance)
	}

	withdrawals := listWithdrawals(t, client, apiServer.URL, http.StatusOK)
	if len(withdrawals) != 1 {
		t.Fatalf("withdrawals len = %d, want 1", len(withdrawals))
	}
	assertWithdrawal(t, withdrawals[0], withdrawalOrderNumber, money.NewFromCents(10000))
}

func openIntegrationStore(t *testing.T) *postgres.Store {
	t.Helper()

	dsn, cleanup := createTemporaryDatabase(t)
	t.Cleanup(cleanup)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	store, err := postgres.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open integration store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close integration store: %v", err)
		}
	})

	return store
}

func createTemporaryDatabase(t *testing.T) (string, func()) {
	t.Helper()

	adminDSN, explicit := integrationAdminDSN()
	tempDBName := fmt.Sprintf("gophermart_test_%d", time.Now().UnixNano())

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

func integrationAdminDSN() (string, bool) {
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

func newFakeAccrualServer(t *testing.T) *httptest.Server {
	t.Helper()

	var (
		mu    sync.Mutex
		calls = make(map[string]int)
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/api/orders/") {
			http.NotFound(w, r)
			return
		}

		number := path.Base(r.URL.Path)
		if number != uploadedOrderNumber {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		mu.Lock()
		calls[number]++
		callNumber := calls[number]
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if callNumber == 1 {
			_, _ = w.Write([]byte(`{"order":"` + uploadedOrderNumber + `","status":"PROCESSING"}`))
			return
		}

		_, _ = w.Write([]byte(`{"order":"` + uploadedOrderNumber + `","status":"PROCESSED","accrual":500.5}`))
	}))
	t.Cleanup(server.Close)

	return server
}

func newTestHTTPClient(t *testing.T) *http.Client {
	t.Helper()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("create cookie jar: %v", err)
	}

	return &http.Client{
		Jar:     jar,
		Timeout: 5 * time.Second,
	}
}

func registerUser(t *testing.T, client *http.Client, baseURL string, login string, password string, wantStatus int) {
	t.Helper()

	body := []byte(fmt.Sprintf(`{"login":%q,"password":%q}`, login, password))
	doRequest(t, client, http.MethodPost, baseURL+"/api/user/register", "application/json", body, wantStatus)
}

func loginUser(t *testing.T, client *http.Client, baseURL string, login string, password string, wantStatus int) {
	t.Helper()

	body := []byte(fmt.Sprintf(`{"login":%q,"password":%q}`, login, password))
	doRequest(t, client, http.MethodPost, baseURL+"/api/user/login", "application/json", body, wantStatus)
}

func uploadOrder(t *testing.T, client *http.Client, baseURL string, number string, wantStatus int) {
	t.Helper()

	doRequest(t, client, http.MethodPost, baseURL+"/api/user/orders", "text/plain", []byte(number), wantStatus)
}

func withdraw(t *testing.T, client *http.Client, baseURL string, orderNumber string, sum money.Amount, wantStatus int) {
	t.Helper()

	body := []byte(fmt.Sprintf(`{"order":%q,"sum":%s}`, orderNumber, sum.String()))
	doRequest(t, client, http.MethodPost, baseURL+"/api/user/balance/withdraw", "application/json", body, wantStatus)
}

func listOrders(t *testing.T, client *http.Client, baseURL string, wantStatus int) []orderPayload {
	t.Helper()

	body := doRequest(t, client, http.MethodGet, baseURL+"/api/user/orders", "", nil, wantStatus)
	if wantStatus == http.StatusNoContent {
		return nil
	}

	var orders []orderPayload
	if err := json.Unmarshal(body, &orders); err != nil {
		t.Fatalf("decode orders response: %v; body=%s", err, string(body))
	}

	return orders
}

func getBalance(t *testing.T, client *http.Client, baseURL string, wantStatus int) balancePayload {
	t.Helper()

	body := doRequest(t, client, http.MethodGet, baseURL+"/api/user/balance", "", nil, wantStatus)

	var balance balancePayload
	if err := json.Unmarshal(body, &balance); err != nil {
		t.Fatalf("decode balance response: %v; body=%s", err, string(body))
	}

	return balance
}

func listWithdrawals(t *testing.T, client *http.Client, baseURL string, wantStatus int) []withdrawalPayload {
	t.Helper()

	body := doRequest(t, client, http.MethodGet, baseURL+"/api/user/withdrawals", "", nil, wantStatus)
	if wantStatus == http.StatusNoContent {
		return nil
	}

	var withdrawals []withdrawalPayload
	if err := json.Unmarshal(body, &withdrawals); err != nil {
		t.Fatalf("decode withdrawals response: %v; body=%s", err, string(body))
	}

	return withdrawals
}

func doRequest(
	t *testing.T,
	client *http.Client,
	method string,
	requestURL string,
	contentType string,
	body []byte,
	wantStatus int,
) []byte {
	t.Helper()

	request, err := http.NewRequest(method, requestURL, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create request %s %s: %v", method, requestURL, err)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}

	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("perform request %s %s: %v", method, requestURL, err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}

	if response.StatusCode != wantStatus {
		t.Fatalf("%s %s status = %d, want %d; body=%s", method, requestURL, response.StatusCode, wantStatus, string(responseBody))
	}

	return responseBody
}

func waitForBalance(t *testing.T, client *http.Client, baseURL string, want balancePayload) {
	t.Helper()

	deadline := time.Now().Add(7 * time.Second)
	var last balancePayload

	for time.Now().Before(deadline) {
		last = getBalance(t, client, baseURL, http.StatusOK)
		if last == want {
			return
		}

		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("balance did not reach %+v before timeout; last=%+v", want, last)
}

func assertOrder(t *testing.T, order orderPayload, number string, status string) {
	t.Helper()

	if order.Number != number || order.Status != status {
		t.Fatalf("order = %+v, want number=%s status=%s", order, number, status)
	}
	if _, err := time.Parse(time.RFC3339, order.UploadedAt); err != nil {
		t.Fatalf("order uploaded_at is not RFC3339: %q", order.UploadedAt)
	}
}

func assertOrderWithoutAccrual(t *testing.T, order orderPayload) {
	t.Helper()

	if order.Accrual != nil {
		t.Fatalf("order accrual = %v, want nil", *order.Accrual)
	}
}

func assertOrderAccrual(t *testing.T, order orderPayload, want money.Amount) {
	t.Helper()

	if order.Accrual == nil {
		t.Fatalf("order accrual = nil, want %v", want)
	}
	if *order.Accrual != want {
		t.Fatalf("order accrual = %v, want %v", *order.Accrual, want)
	}
}

func assertWithdrawal(t *testing.T, withdrawal withdrawalPayload, orderNumber string, sum money.Amount) {
	t.Helper()

	if withdrawal.Order != orderNumber || withdrawal.Sum != sum {
		t.Fatalf("withdrawal = %+v, want order=%s sum=%s", withdrawal, orderNumber, sum)
	}
	if _, err := time.Parse(time.RFC3339, withdrawal.ProcessedAt); err != nil {
		t.Fatalf("withdrawal processed_at is not RFC3339: %q", withdrawal.ProcessedAt)
	}
}
