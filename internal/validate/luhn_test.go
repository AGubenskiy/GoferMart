package validate

import "testing"

func TestOrderNumber(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		number string
		valid  bool
	}{
		{name: "valid specification example", number: "12345678903", valid: true},
		{name: "valid withdrawal example", number: "2377225624", valid: true},
		{name: "empty", number: "", valid: false},
		{name: "alpha chars", number: "12a45", valid: false},
		{name: "invalid checksum", number: "12345678904", valid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := OrderNumber(tt.number); got != tt.valid {
				t.Fatalf("OrderNumber(%q) = %v, want %v", tt.number, got, tt.valid)
			}
		})
	}
}
