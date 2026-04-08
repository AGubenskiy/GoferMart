package postgres

import (
	"strings"
	"testing"
)

func TestValidateMigrationName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		migrationName string
		wantErr       bool
	}{
		{
			name:          "within limit",
			migrationName: strings.Repeat("a", 200),
			wantErr:       false,
		},
		{
			name:          "exceeds limit",
			migrationName: strings.Repeat("a", 201),
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateMigrationName(tt.migrationName)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateMigrationName() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
