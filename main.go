package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/AGubenskiy/GoferMart/internal/app"
	"github.com/AGubenskiy/GoferMart/internal/config"
	"github.com/AGubenskiy/GoferMart/internal/logger"
)

func main() {
	cfg, err := config.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	log := logger.New()
	application, err := app.New(context.Background(), cfg, log)
	if err != nil {
		log.Error("failed to initialize application", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := application.Run(ctx); err != nil {
		log.Error("application stopped with error", "error", err)
		os.Exit(1)
	}
}
