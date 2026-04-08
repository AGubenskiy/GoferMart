package validate

import (
	"strings"
	"testing"
)

func TestStringLengthAtMost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		limit int
		want  bool
	}{
		{
			name:  "ascii within limit",
			value: strings.Repeat("a", MaxVarcharLength),
			limit: MaxVarcharLength,
			want:  true,
		},
		{
			name:  "ascii exceeds limit",
			value: strings.Repeat("a", MaxVarcharLength+1),
			limit: MaxVarcharLength,
			want:  false,
		},
		{
			name:  "unicode within limit",
			value: strings.Repeat("ж", MaxVarcharLength),
			limit: MaxVarcharLength,
			want:  true,
		},
		{
			name:  "unicode exceeds limit",
			value: strings.Repeat("ж", MaxVarcharLength+1),
			limit: MaxVarcharLength,
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := StringLengthAtMost(tt.value, tt.limit); got != tt.want {
				t.Fatalf("StringLengthAtMost(len=%d, limit=%d) = %v, want %v", len([]rune(tt.value)), tt.limit, got, tt.want)
			}
		})
	}
}
