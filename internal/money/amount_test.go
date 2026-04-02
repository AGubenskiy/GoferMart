package money

import (
	"encoding/json"
	"testing"
)

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    Amount
		wantErr bool
	}{
		{name: "integer", input: "42", want: NewFromCents(4200)},
		{name: "one decimal", input: "500.5", want: NewFromCents(50050)},
		{name: "two decimals", input: "10.25", want: NewFromCents(1025)},
		{name: "negative", input: "-0.10", want: NewFromCents(-10)},
		{name: "too many decimals", input: "1.005", wantErr: true},
		{name: "invalid chars", input: "abc", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := Parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}

			if err == nil && got != tt.want {
				t.Fatalf("Parse() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAmountJSON(t *testing.T) {
	t.Parallel()

	type payload struct {
		Sum Amount `json:"sum"`
	}

	var decoded payload
	if err := json.Unmarshal([]byte(`{"sum":0.1}`), &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if decoded.Sum != NewFromCents(10) {
		t.Fatalf("decoded sum = %v, want 0.1", decoded.Sum)
	}

	encoded, err := json.Marshal(payload{Sum: NewFromCents(1050)})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	if string(encoded) != `{"sum":10.5}` {
		t.Fatalf("Marshal() = %s, want %s", string(encoded), `{"sum":10.5}`)
	}
}
