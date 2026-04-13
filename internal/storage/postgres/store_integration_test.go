package postgres

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

	"github.com/AGubenskiy/GoferMart/internal/model"
	"github.com/AGubenskiy/GoferMart/internal/money"
	"github.com/lib/pq"
)

func TestOpenRejectsEmptyDSN(t *testing.T) {
	t.Parallel()

	store, err := Open(context.Background(), "")
	if store != nil {
		t.Fatalf("Open() store = %v, want nil", store)
	}
	if err == nil || !strings.Contains(err.Error(), "database URI is empty") {
		t.Fatalf("Open() error = %v, want database URI error", err)
	}
}

func TestOpenRunsMigrationsAndWithTx(t *testing.T) {
	store := openTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var appliedMigrations int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&appliedMigrations); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if appliedMigrations != 2 {
		t.Fatalf("applied migrations = %d, want 2", appliedMigrations)
	}

	if err := runMigrations(ctx, store.db); err != nil {
		t.Fatalf("runMigrations() error = %v", err)
	}

	if err := store.WithTx(ctx, func(repos *Repositories) error {
		_, err := repos.Users.Create(ctx, "alice", "hash")
		return err
	}); err != nil {
		t.Fatalf("WithTx() commit path error = %v", err)
	}

	if _, err := store.Repositories().Users.GetByLogin(ctx, "alice"); err != nil {
		t.Fatalf("GetByLogin() after commit error = %v", err)
	}

	sentinel := errors.New("rollback me")
	err := store.WithTx(ctx, func(repos *Repositories) error {
		if _, createErr := repos.Users.Create(ctx, "bob", "hash"); createErr != nil {
			return createErr
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("WithTx() rollback error = %v, want %v", err, sentinel)
	}

	if _, err := store.Repositories().Users.GetByLogin(ctx, "bob"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("GetByLogin() after rollback error = %v, want %v", err, ErrUserNotFound)
	}
}

func TestStoreCloseHandlesNilReceiver(t *testing.T) {
	t.Parallel()

	var store *Store
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}
}

func TestUserRepositoryCreateAndLookup(t *testing.T) {
	store := openTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	users := store.Repositories().Users

	created, err := users.Create(ctx, "alice", "hash-1")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.ID <= 0 {
		t.Fatalf("Create() ID = %d, want positive", created.ID)
	}

	if _, err := users.Create(ctx, "alice", "hash-2"); !errors.Is(err, ErrDuplicateLogin) {
		t.Fatalf("Create() duplicate error = %v, want %v", err, ErrDuplicateLogin)
	}

	gotByID, err := users.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if gotByID.Login != created.Login || gotByID.PasswordHash != created.PasswordHash {
		t.Fatalf("GetByID() = %+v, want login=%q password hash=%q", gotByID, created.Login, created.PasswordHash)
	}

	gotByLogin, err := users.GetByLogin(ctx, created.Login)
	if err != nil {
		t.Fatalf("GetByLogin() error = %v", err)
	}
	if gotByLogin.ID != created.ID {
		t.Fatalf("GetByLogin() ID = %d, want %d", gotByLogin.ID, created.ID)
	}

	if err := users.LockByID(ctx, created.ID); err != nil {
		t.Fatalf("LockByID() error = %v", err)
	}

	if _, err := users.GetByID(ctx, created.ID+999); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("GetByID() missing error = %v, want %v", err, ErrUserNotFound)
	}
	if _, err := users.GetByLogin(ctx, "missing"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("GetByLogin() missing error = %v, want %v", err, ErrUserNotFound)
	}
	if err := users.LockByID(ctx, created.ID+999); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("LockByID() missing error = %v, want %v", err, ErrUserNotFound)
	}
}

func TestOrderNumberRepositoryReserveAndGet(t *testing.T) {
	store := openTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	user := createTestUser(t, ctx, store, "alice")
	repo := store.Repositories().OrderNumbers

	if err := repo.Reserve(ctx, user.ID, "12345678903", OrderNumberKindUpload); err != nil {
		t.Fatalf("Reserve() error = %v", err)
	}

	reservation, err := repo.Get(ctx, "12345678903")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if reservation.Number != "12345678903" || reservation.UserID != user.ID || reservation.Kind != OrderNumberKindUpload {
		t.Fatalf("Get() = %+v, want upload reservation", reservation)
	}

	if err := repo.Reserve(ctx, user.ID, "12345678903", OrderNumberKindWithdrawal); !errors.Is(err, ErrOrderNumberAlreadyReserved) {
		t.Fatalf("Reserve() duplicate error = %v, want %v", err, ErrOrderNumberAlreadyReserved)
	}

	if _, err := repo.Get(ctx, "missing"); !errors.Is(err, ErrOrderNumberReservationMissed) {
		t.Fatalf("Get() missing error = %v, want %v", err, ErrOrderNumberReservationMissed)
	}
}

func TestOrderRepositoryLifecycle(t *testing.T) {
	store := openTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	alice := createTestUser(t, ctx, store, "alice")
	bob := createTestUser(t, ctx, store, "bob")
	repo := store.Repositories().Orders

	if err := repo.Add(ctx, alice.ID, "12345678903"); err != nil {
		t.Fatalf("Add() first error = %v", err)
	}
	if err := repo.Add(ctx, alice.ID, "2377225624"); err != nil {
		t.Fatalf("Add() second error = %v", err)
	}
	if err := repo.Add(ctx, alice.ID, "49927398716"); err != nil {
		t.Fatalf("Add() third error = %v", err)
	}

	setOrderUploadedAt(t, ctx, store, "12345678903", time.Date(2026, 4, 10, 9, 0, 0, 0, time.UTC))
	setOrderUploadedAt(t, ctx, store, "2377225624", time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC))
	setOrderUploadedAt(t, ctx, store, "49927398716", time.Date(2026, 4, 10, 11, 0, 0, 0, time.UTC))

	order, err := repo.GetByNumber(ctx, "12345678903")
	if err != nil {
		t.Fatalf("GetByNumber() error = %v", err)
	}
	if order.Status != model.OrderStatusNew || order.Accrual != nil {
		t.Fatalf("GetByNumber() = %+v, want NEW order without accrual", order)
	}

	if err := repo.Add(ctx, alice.ID, "12345678903"); !errors.Is(err, ErrOrderAlreadyUploadedByUser) {
		t.Fatalf("Add() same user duplicate error = %v, want %v", err, ErrOrderAlreadyUploadedByUser)
	}
	if err := repo.Add(ctx, bob.ID, "12345678903"); !errors.Is(err, ErrOrderUploadedByAnotherUser) {
		t.Fatalf("Add() other user duplicate error = %v, want %v", err, ErrOrderUploadedByAnotherUser)
	}

	orders, err := repo.ListByUser(ctx, alice.ID)
	if err != nil {
		t.Fatalf("ListByUser() error = %v", err)
	}
	if len(orders) != 3 {
		t.Fatalf("ListByUser() len = %d, want 3", len(orders))
	}
	if orders[0].Number != "49927398716" || orders[1].Number != "2377225624" || orders[2].Number != "12345678903" {
		t.Fatalf("ListByUser() order numbers = [%s %s %s], want newest first", orders[0].Number, orders[1].Number, orders[2].Number)
	}

	accrual := money.NewFromCents(50050)
	if err := repo.UpdateStatus(ctx, "12345678903", model.OrderStatusProcessed, &accrual); err != nil {
		t.Fatalf("UpdateStatus() processed error = %v", err)
	}
	if err := repo.UpdateStatus(ctx, "2377225624", model.OrderStatusProcessing, nil); err != nil {
		t.Fatalf("UpdateStatus() processing error = %v", err)
	}

	updated, err := repo.GetByNumber(ctx, "12345678903")
	if err != nil {
		t.Fatalf("GetByNumber() updated error = %v", err)
	}
	if updated.Status != model.OrderStatusProcessed {
		t.Fatalf("updated status = %s, want %s", updated.Status, model.OrderStatusProcessed)
	}
	if updated.Accrual == nil || *updated.Accrual != accrual {
		t.Fatalf("updated accrual = %v, want %v", updated.Accrual, accrual)
	}

	syncOrders, err := repo.ListForAccrualSync(ctx, 2)
	if err != nil {
		t.Fatalf("ListForAccrualSync() error = %v", err)
	}
	if len(syncOrders) != 2 {
		t.Fatalf("ListForAccrualSync() len = %d, want 2", len(syncOrders))
	}
	if syncOrders[0].Number != "2377225624" || syncOrders[1].Number != "49927398716" {
		t.Fatalf("ListForAccrualSync() = [%s %s], want oldest eligible first", syncOrders[0].Number, syncOrders[1].Number)
	}

	if err := repo.UpdateStatus(ctx, "missing", model.OrderStatusProcessed, nil); !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("UpdateStatus() missing error = %v, want %v", err, ErrOrderNotFound)
	}
	if _, err := repo.GetByNumber(ctx, "missing"); !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("GetByNumber() missing error = %v, want %v", err, ErrOrderNotFound)
	}
}

func TestWithdrawalRepositoryCreateListAndBalance(t *testing.T) {
	store := openTestStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	user := createTestUser(t, ctx, store, "alice")
	orders := store.Repositories().Orders
	withdrawals := store.Repositories().Withdrawals

	initialBalance, err := withdrawals.GetBalance(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetBalance() initial error = %v", err)
	}
	if initialBalance.Current != 0 || initialBalance.Withdrawn != 0 {
		t.Fatalf("initial balance = %+v, want zero balance", initialBalance)
	}

	if err := orders.Add(ctx, user.ID, "12345678903"); err != nil {
		t.Fatalf("Add() order 1 error = %v", err)
	}
	if err := orders.Add(ctx, user.ID, "2377225624"); err != nil {
		t.Fatalf("Add() order 2 error = %v", err)
	}

	firstAccrual := money.NewFromCents(50050)
	secondAccrual := money.NewFromCents(10000)
	if err := orders.UpdateStatus(ctx, "12345678903", model.OrderStatusProcessed, &firstAccrual); err != nil {
		t.Fatalf("UpdateStatus() order 1 error = %v", err)
	}
	if err := orders.UpdateStatus(ctx, "2377225624", model.OrderStatusProcessed, &secondAccrual); err != nil {
		t.Fatalf("UpdateStatus() order 2 error = %v", err)
	}

	if err := withdrawals.Create(ctx, user.ID, "49927398716", money.NewFromCents(10000)); err != nil {
		t.Fatalf("Create() first withdrawal error = %v", err)
	}
	if err := withdrawals.Create(ctx, user.ID, "79927398713", money.NewFromCents(5050)); err != nil {
		t.Fatalf("Create() second withdrawal error = %v", err)
	}

	setWithdrawalProcessedAt(t, ctx, store, "49927398716", time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC))
	setWithdrawalProcessedAt(t, ctx, store, "79927398713", time.Date(2026, 4, 10, 13, 0, 0, 0, time.UTC))

	if err := withdrawals.Create(ctx, user.ID, "49927398716", money.NewFromCents(100)); !errors.Is(err, ErrWithdrawalOrderAlreadyExists) {
		t.Fatalf("Create() duplicate error = %v, want %v", err, ErrWithdrawalOrderAlreadyExists)
	}

	list, err := withdrawals.ListByUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListByUser() error = %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("ListByUser() len = %d, want 2", len(list))
	}
	if list[0].OrderNumber != "79927398713" || list[1].OrderNumber != "49927398716" {
		t.Fatalf("ListByUser() order numbers = [%s %s], want newest first", list[0].OrderNumber, list[1].OrderNumber)
	}

	balance, err := withdrawals.GetBalance(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetBalance() error = %v", err)
	}

	wantWithdrawn := money.NewFromCents(15050)
	wantCurrent := money.NewFromCents(45000)
	if balance.Withdrawn != wantWithdrawn || balance.Current != wantCurrent {
		t.Fatalf("balance = %+v, want current=%v withdrawn=%v", balance, wantCurrent, wantWithdrawn)
	}
}

func TestParseMigrationVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{name: "valid", input: "000001_init.up.sql", want: 1},
		{name: "missing underscore", input: "000001init.up.sql", wantErr: true},
		{name: "non numeric prefix", input: "abc_init.up.sql", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseMigrationVersion(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseMigrationVersion() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Fatalf("parseMigrationVersion() = %d, want %d", got, tt.want)
			}
		})
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()

	dsn, cleanup := createTemporaryDatabase(t)
	t.Cleanup(cleanup)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	store, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	return store
}

func createTestUser(t *testing.T, ctx context.Context, store *Store, login string) model.User {
	t.Helper()

	user, err := store.Repositories().Users.Create(ctx, login, "hashed-password")
	if err != nil {
		t.Fatalf("Create(%q) error = %v", login, err)
	}

	return user
}

func setOrderUploadedAt(t *testing.T, ctx context.Context, store *Store, number string, uploadedAt time.Time) {
	t.Helper()

	if _, err := store.db.ExecContext(
		ctx,
		`UPDATE orders SET uploaded_at = $2, updated_at = $2 WHERE number = $1`,
		number,
		uploadedAt,
	); err != nil {
		t.Fatalf("set order uploaded_at for %q: %v", number, err)
	}
}

func setWithdrawalProcessedAt(t *testing.T, ctx context.Context, store *Store, orderNumber string, processedAt time.Time) {
	t.Helper()

	if _, err := store.db.ExecContext(
		ctx,
		`UPDATE withdrawals SET processed_at = $2 WHERE order_number = $1`,
		orderNumber,
		processedAt,
	); err != nil {
		t.Fatalf("set withdrawal processed_at for %q: %v", orderNumber, err)
	}
}

func createTemporaryDatabase(t *testing.T) (string, func()) {
	t.Helper()

	adminDSN, explicit := testAdminDSN()
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
