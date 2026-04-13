package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/AGubenskiy/GoferMart/internal/auth"
	"github.com/AGubenskiy/GoferMart/internal/model"
	"github.com/AGubenskiy/GoferMart/internal/money"
	"github.com/AGubenskiy/GoferMart/internal/storage/postgres"
	"github.com/lib/pq"
)

func TestAuthServiceRegisterAndLoginFlow(t *testing.T) {
	store := openTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sessions := auth.NewSessionManager("test-secret", time.Hour)
	service := NewAuthService(store.Repositories().Users, sessions)

	token, err := service.Register(ctx, " alice ", "Strong-password1")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	createdUser, err := store.Repositories().Users.GetByLogin(ctx, "alice")
	if err != nil {
		t.Fatalf("GetByLogin() error = %v", err)
	}

	userID, err := sessions.Verify(token)
	if err != nil {
		t.Fatalf("Verify() registration token error = %v", err)
	}
	if userID != createdUser.ID {
		t.Fatalf("registration token user ID = %d, want %d", userID, createdUser.ID)
	}

	if _, err := service.Register(ctx, "alice", "Strong-password1"); !errors.Is(err, ErrLoginAlreadyTaken) {
		t.Fatalf("Register() duplicate error = %v, want %v", err, ErrLoginAlreadyTaken)
	}
	if _, err := service.Register(ctx, "", "Strong-password1"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Register() empty login error = %v, want %v", err, ErrInvalidInput)
	}
	if _, err := service.Register(ctx, "bob", "weak"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Register() weak password error = %v, want %v", err, ErrInvalidInput)
	}

	loginToken, err := service.Login(ctx, " alice ", "Strong-password1")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	userID, err = sessions.Verify(loginToken)
	if err != nil {
		t.Fatalf("Verify() login token error = %v", err)
	}
	if userID != createdUser.ID {
		t.Fatalf("login token user ID = %d, want %d", userID, createdUser.ID)
	}

	if _, err := service.Login(ctx, "alice", "wrong-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Login() wrong password error = %v, want %v", err, ErrInvalidCredentials)
	}
	if _, err := service.Login(ctx, "missing", "Strong-password1"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Login() missing user error = %v, want %v", err, ErrInvalidCredentials)
	}
	if _, err := service.Login(ctx, "", "Strong-password1"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Login() empty login error = %v, want %v", err, ErrInvalidInput)
	}
}

func TestLoyaltyServiceFlow(t *testing.T) {
	store := openTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	service := NewLoyaltyService(store)

	alice := createUser(t, ctx, store, "alice")
	bob := createUser(t, ctx, store, "bob")

	if _, err := service.UploadOrder(ctx, alice.ID, "invalid"); !errors.Is(err, ErrInvalidOrderNumber) {
		t.Fatalf("UploadOrder() invalid number error = %v, want %v", err, ErrInvalidOrderNumber)
	}

	result, err := service.UploadOrder(ctx, alice.ID, "12345678903")
	if err != nil {
		t.Fatalf("UploadOrder() accepted error = %v", err)
	}
	if !result.Accepted {
		t.Fatalf("UploadOrder() Accepted = false, want true")
	}

	result, err = service.UploadOrder(ctx, alice.ID, "12345678903")
	if err != nil {
		t.Fatalf("UploadOrder() repeat error = %v", err)
	}
	if result.Accepted {
		t.Fatalf("UploadOrder() repeat Accepted = true, want false")
	}

	if _, err := service.UploadOrder(ctx, bob.ID, "12345678903"); !errors.Is(err, ErrOrderConflict) {
		t.Fatalf("UploadOrder() conflict error = %v, want %v", err, ErrOrderConflict)
	}

	orders, err := service.ListOrders(ctx, alice.ID)
	if err != nil {
		t.Fatalf("ListOrders() error = %v", err)
	}
	if len(orders) != 1 {
		t.Fatalf("ListOrders() len = %d, want 1", len(orders))
	}
	if orders[0].Number != "12345678903" || orders[0].Status != model.OrderStatusNew {
		t.Fatalf("ListOrders() order = %+v, want NEW uploaded order", orders[0])
	}

	balance, err := service.GetBalance(ctx, alice.ID)
	if err != nil {
		t.Fatalf("GetBalance() initial error = %v", err)
	}
	if balance.Current != 0 || balance.Withdrawn != 0 {
		t.Fatalf("GetBalance() initial = %+v, want zero balance", balance)
	}

	if err := service.CreateWithdrawal(ctx, alice.ID, "bad", money.NewFromCents(100)); !errors.Is(err, ErrInvalidOrderNumber) {
		t.Fatalf("CreateWithdrawal() invalid order error = %v, want %v", err, ErrInvalidOrderNumber)
	}
	if err := service.CreateWithdrawal(ctx, alice.ID, "2377225624", 0); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("CreateWithdrawal() invalid sum error = %v, want %v", err, ErrInvalidInput)
	}
	if err := service.CreateWithdrawal(ctx, alice.ID, "2377225624", money.NewFromCents(10000)); !errors.Is(err, ErrInsufficientFunds) {
		t.Fatalf("CreateWithdrawal() insufficient funds error = %v, want %v", err, ErrInsufficientFunds)
	}

	accrual := money.NewFromCents(50050)
	if err := store.Repositories().Orders.UpdateStatus(ctx, "12345678903", model.OrderStatusProcessed, &accrual); err != nil {
		t.Fatalf("UpdateStatus() error = %v", err)
	}

	if err := service.CreateWithdrawal(ctx, alice.ID, "2377225624", money.NewFromCents(10000)); err != nil {
		t.Fatalf("CreateWithdrawal() success error = %v", err)
	}
	if err := service.CreateWithdrawal(ctx, alice.ID, "2377225624", money.NewFromCents(100)); !errors.Is(err, ErrOrderNumberUnavailable) {
		t.Fatalf("CreateWithdrawal() duplicate order error = %v, want %v", err, ErrOrderNumberUnavailable)
	}
	if _, err := service.UploadOrder(ctx, alice.ID, "2377225624"); !errors.Is(err, ErrOrderConflict) {
		t.Fatalf("UploadOrder() reserved withdrawal number error = %v, want %v", err, ErrOrderConflict)
	}

	withdrawals, err := service.ListWithdrawals(ctx, alice.ID)
	if err != nil {
		t.Fatalf("ListWithdrawals() error = %v", err)
	}
	if len(withdrawals) != 1 {
		t.Fatalf("ListWithdrawals() len = %d, want 1", len(withdrawals))
	}
	if withdrawals[0].OrderNumber != "2377225624" || withdrawals[0].Sum != money.NewFromCents(10000) {
		t.Fatalf("ListWithdrawals() = %+v, want withdrawal for 2377225624", withdrawals[0])
	}

	balance, err = service.GetBalance(ctx, alice.ID)
	if err != nil {
		t.Fatalf("GetBalance() final error = %v", err)
	}
	if balance.Current != money.NewFromCents(40050) || balance.Withdrawn != money.NewFromCents(10000) {
		t.Fatalf("GetBalance() final = %+v, want current=400.50 withdrawn=100.00", balance)
	}
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

func createUser(t *testing.T, ctx context.Context, store *postgres.Store, login string) model.User {
	t.Helper()

	user, err := store.Repositories().Users.Create(ctx, login, "hashed-password")
	if err != nil {
		t.Fatalf("Create(%q) error = %v", login, err)
	}

	return user
}

func createTemporaryDatabase(t *testing.T) (string, func()) {
	t.Helper()

	adminDSN, explicit := testAdminDSN()
	tempDBName := fmt.Sprintf("gophermart_service_test_%d", time.Now().UnixNano())

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
