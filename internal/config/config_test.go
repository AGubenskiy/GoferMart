package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseUsesEnvironmentAndFlags(t *testing.T) {
	unsetEnv(t, "RUN_ADDRESS")

	t.Setenv("DATABASE_URI", "postgres://env-db")
	t.Setenv("ACCRUAL_SYSTEM_ADDRESS", "http://env-accrual")

	cfg, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if cfg.RunAddress != defaultRunAddress {
		t.Fatalf("RunAddress = %q, want %q", cfg.RunAddress, defaultRunAddress)
	}
	if cfg.DatabaseURI != "postgres://env-db" {
		t.Fatalf("DatabaseURI = %q, want env value", cfg.DatabaseURI)
	}
	if cfg.AccrualSystemAddress != "http://env-accrual" {
		t.Fatalf("AccrualSystemAddress = %q, want env value", cfg.AccrualSystemAddress)
	}
	if cfg.ShutdownTimeout != 10*time.Second {
		t.Fatalf("ShutdownTimeout = %s, want 10s", cfg.ShutdownTimeout)
	}

	cfg, err = Parse([]string{
		"-a", ":9090",
		"-d", "postgres://flag-db",
		"-r", "http://flag-accrual",
	})
	if err != nil {
		t.Fatalf("Parse() with flags error = %v", err)
	}

	if cfg.RunAddress != ":9090" {
		t.Fatalf("RunAddress = %q, want %q", cfg.RunAddress, ":9090")
	}
	if cfg.DatabaseURI != "postgres://flag-db" {
		t.Fatalf("DatabaseURI = %q, want flag value", cfg.DatabaseURI)
	}
	if cfg.AccrualSystemAddress != "http://flag-accrual" {
		t.Fatalf("AccrualSystemAddress = %q, want flag value", cfg.AccrualSystemAddress)
	}
}

func TestParseRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "empty run address",
			args: []string{"-a", "", "-d", "postgres://db", "-r", "http://accrual"},
			want: "run address must not be empty",
		},
		{
			name: "empty database uri",
			args: []string{"-d", "", "-r", "http://accrual"},
			want: "database URI must not be empty",
		},
		{
			name: "empty accrual address",
			args: []string{"-d", "postgres://db", "-r", ""},
			want: "accrual system address must not be empty",
		},
		{
			name: "unknown flag",
			args: []string{"-unknown"},
			want: "flag provided but not defined",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("RUN_ADDRESS", ":8081")
			t.Setenv("DATABASE_URI", "postgres://env-db")
			t.Setenv("ACCRUAL_SYSTEM_ADDRESS", "http://env-accrual")

			_, err := Parse(tt.args)
			if err == nil {
				t.Fatal("Parse() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Parse() error = %q, want substring %q", err.Error(), tt.want)
			}
		})
	}
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()

	value, ok := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("Unsetenv(%q) error = %v", key, err)
	}

	t.Cleanup(func() {
		if !ok {
			_ = os.Unsetenv(key)
			return
		}

		_ = os.Setenv(key, value)
	})
}
