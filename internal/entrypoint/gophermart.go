package entrypoint

import (
	"context"
	"fmt"
	"io"
	"os/signal"
	"syscall"

	"github.com/AGubenskiy/GoferMart/internal/app"
	"github.com/AGubenskiy/GoferMart/internal/config"
	"github.com/AGubenskiy/GoferMart/internal/logger"
)

func RunGophermart(args []string, stderr io.Writer) int {
	cfg, err := config.Parse(args)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "config error: %v\n", err)
		return 1
	}

	log := logger.New()
	application, err := app.New(context.Background(), cfg, log)
	if err != nil {
		log.Error("failed to initialize application", "error", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := application.Run(ctx); err != nil {
		log.Error("application stopped with error", "error", err)
		return 1
	}

	return 0
}
