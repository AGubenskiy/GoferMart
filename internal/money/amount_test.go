package money

import (
	"database/sql/driver"
	"encoding/json"
	"strings"
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

func TestAmountValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   Amount
		want driver.Value
	}{
		{name: "zero", in: 0, want: "0"},
		{name: "positive", in: NewFromCents(1050), want: "10.5"},
		{name: "negative", in: NewFromCents(-10), want: "-0.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := tt.in.Value()
			if err != nil {
				t.Fatalf("Value() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("Value() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAmountScan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		src           any
		want          Amount
		wantErr       bool
		wantErrSubstr string
	}{
		{name: "string integer", src: "42", want: NewFromCents(4200)},
		{name: "string decimal", src: "10.25", want: NewFromCents(1025)},
		{name: "bytes decimal", src: []byte("500.5"), want: NewFromCents(50050)},
		{name: "int64 whole units", src: int64(7), want: NewFromCents(700)},
		{name: "float64 decimal", src: 0.1, want: NewFromCents(10)},
		{name: "nil source", src: nil, wantErr: true, wantErrSubstr: "amount is null"},
		{name: "invalid string", src: "abc", wantErr: true, wantErrSubstr: "invalid amount"},
		{name: "invalid bytes", src: []byte("1.005"), wantErr: true, wantErrSubstr: "at most two fractional digits"},
		{name: "unsupported type", src: true, wantErr: true, wantErrSubstr: "unsupported amount source bool"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got Amount
			err := got.Scan(tt.src)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Scan() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				if tt.wantErrSubstr != "" && !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Fatalf("Scan() error = %q, want substring %q", err.Error(), tt.wantErrSubstr)
				}
				return
			}
			if got != tt.want {
				t.Fatalf("Scan() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAmountScanNilReceiver(t *testing.T) {
	t.Parallel()

	var amount *Amount
	err := amount.Scan("1")
	if err == nil {
		t.Fatal("Scan() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "amount target is nil") {
		t.Fatalf("Scan() error = %q, want nil target error", err.Error())
	}
}
