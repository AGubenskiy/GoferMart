package entrypoint

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/signal"
	"syscall"

	"github.com/AGubenskiy/GoferMart/internal/app"
	"github.com/AGubenskiy/GoferMart/internal/config"
	"github.com/AGubenskiy/GoferMart/internal/logger"
)

func RunGophermart(args []string, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Parse(args)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "config error: %v\n", err)
		return 1
	}

	log := logger.New()
	application, err := app.New(ctx, cfg, log)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			log.Info("application initialization canceled")
			return 0
		}

		log.Error("failed to initialize application", "error", err)
		return 1
	}

	if err := application.Run(ctx); err != nil {
		log.Error("application stopped with error", "error", err)
		return 1
	}

	return 0
}
