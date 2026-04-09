package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/AGubenskiy/GoferMart/internal/accrual"
	"github.com/AGubenskiy/GoferMart/internal/auth"
	"github.com/AGubenskiy/GoferMart/internal/config"
	"github.com/AGubenskiy/GoferMart/internal/http/handler"
	"github.com/AGubenskiy/GoferMart/internal/http/middleware"
	"github.com/AGubenskiy/GoferMart/internal/service"
	"github.com/AGubenskiy/GoferMart/internal/storage/postgres"
	"github.com/AGubenskiy/GoferMart/internal/worker"
)

type App struct {
	cfg    config.Config
	log    *slog.Logger
	server *http.Server
	store  *postgres.Store
	worker *worker.AccrualWorker
}

func New(ctx context.Context, cfg config.Config, log *slog.Logger) (*App, error) {
	if cfg.DatabaseURI == "" {
		return nil, fmt.Errorf("database URI is required")
	}

	if cfg.AccrualSystemAddress == "" {
		return nil, fmt.Errorf("accrual system address is required")
	}

	store, err := postgres.Open(ctx, cfg.DatabaseURI)
	if err != nil {
		return nil, fmt.Errorf("initialize postgres store: %w", err)
	}

	sessions := auth.NewSessionManager("gofermart-local-secret", 24*time.Hour) //Нужно ли убрать его в конфиг/env ?

	authService := service.NewAuthService(store.Repositories().Users, sessions)
	loyaltyService := service.NewLoyaltyService(store)

	accrualClient, err := accrual.NewClient(cfg.AccrualSystemAddress)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("initialize accrual client: %w", err)
	}

	accrualWorker, err := worker.NewAccrualWorker(log, accrualClient, store.Repositories().Orders)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("initialize accrual worker: %w", err)
	}

	router := handler.NewRouter(log, handler.Dependencies{
		UserHandler:    handler.NewUserHandler(authService, sessions),
		AccountHandler: handler.NewAccountHandler(loyaltyService),
		AuthMiddleware: middleware.AuthRequired(sessions),
	})

	return &App{
		cfg:    cfg,
		log:    log,
		store:  store,
		worker: accrualWorker,
		server: &http.Server{
			Addr:              cfg.RunAddress,
			Handler:           router,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	serverErr := make(chan error, 1)
	workerDone := a.startWorker(runCtx)

	go func() {
		a.log.Info("starting HTTP server", "address", a.cfg.RunAddress)

		if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- fmt.Errorf("listen and serve: %w", err)
			return
		}

		close(serverErr)
	}()

	select {
	case <-runCtx.Done():
		cancel()
		return a.shutdown(workerDone)
	case err, ok := <-serverErr:
		stopCtx, stopCancel := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
		defer stopCancel()

		if !ok {
			cancel()
			_ = waitDone(stopCtx, workerDone)
			return nil
		}

		cancel()
		_ = waitDone(stopCtx, workerDone)
		return err
	}
}

func (a *App) startWorker(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	if a.worker == nil {
		close(done)
		return done
	}

	go func() {
		defer close(done)
		a.worker.Run(ctx)
	}()

	return done
}

func (a *App) shutdown(workerDone <-chan struct{}) error {
	ctx, cancel := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
	defer cancel()

	a.log.Info("shutting down HTTP server", "timeout", a.cfg.ShutdownTimeout.String())

	if err := a.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown server: %w", err)
	}

	if err := waitDone(ctx, workerDone); err != nil {
		return fmt.Errorf("stop accrual worker: %w", err)
	}

	if err := a.store.Close(); err != nil {
		return fmt.Errorf("close postgres store: %w", err)
	}

	return nil
}

func waitDone(ctx context.Context, done <-chan struct{}) error {
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
