package config

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"time"
)

const defaultRunAddress = ":8080"

type Config struct {
	RunAddress           string
	DatabaseURI          string
	AccrualSystemAddress string
	ShutdownTimeout      time.Duration
}

func Parse(args []string) (Config, error) {
	cfg := Config{
		RunAddress:           getEnv("RUN_ADDRESS", defaultRunAddress),
		DatabaseURI:          os.Getenv("DATABASE_URI"),
		AccrualSystemAddress: os.Getenv("ACCRUAL_SYSTEM_ADDRESS"),
		ShutdownTimeout:      10 * time.Second,
	}

	var stderr bytes.Buffer

	fs := flag.NewFlagSet("gophermart", flag.ContinueOnError)
	fs.SetOutput(&stderr)

	runAddress := fs.String("a", cfg.RunAddress, "HTTP server address")
	databaseURI := fs.String("d", cfg.DatabaseURI, "PostgreSQL connection string")
	accrualSystemAddress := fs.String("r", cfg.AccrualSystemAddress, "accrual system base URL")

	if err := fs.Parse(args); err != nil {
		return Config{}, fmt.Errorf("%w: %s", err, stderr.String())
	}

	cfg.RunAddress = *runAddress
	cfg.DatabaseURI = *databaseURI
	cfg.AccrualSystemAddress = *accrualSystemAddress

	if cfg.RunAddress == "" {
		return Config{}, fmt.Errorf("run address must not be empty")
	}

	if cfg.DatabaseURI == "" {
		return Config{}, fmt.Errorf("database URI must not be empty")
	}

	if cfg.AccrualSystemAddress == "" {
		return Config{}, fmt.Errorf("accrual system address must not be empty")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}
