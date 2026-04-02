package money

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Amount stores values in cents to avoid floating-point drift.
type Amount int64

func NewFromCents(cents int64) Amount {
	return Amount(cents)
}

func Parse(input string) (Amount, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return 0, fmt.Errorf("amount is empty")
	}

	sign := int64(1)
	if value[0] == '-' {
		sign = -1
		value = value[1:]
	} else if value[0] == '+' {
		value = value[1:]
	}

	if value == "" {
		return 0, fmt.Errorf("amount is empty")
	}

	parts := strings.Split(value, ".")
	if len(parts) > 2 {
		return 0, fmt.Errorf("invalid amount %q", input)
	}

	wholePart := parts[0]
	if wholePart == "" {
		wholePart = "0"
	}

	whole, err := parseDigits(wholePart)
	if err != nil {
		return 0, err
	}

	var fraction int64
	if len(parts) == 2 {
		switch frac := parts[1]; len(frac) {
		case 0:
			fraction = 0
		case 1:
			digit, err := parseDigits(frac)
			if err != nil {
				return 0, err
			}

			fraction = digit * 10
		case 2:
			digits, err := parseDigits(frac)
			if err != nil {
				return 0, err
			}

			fraction = digits
		default:
			return 0, fmt.Errorf("amount must have at most two fractional digits")
		}
	}

	return Amount(sign * (whole*100 + fraction)), nil
}

func (a Amount) IsPositive() bool {
	return a > 0
}

func (a Amount) String() string {
	cents := int64(a)
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}

	whole := cents / 100
	fraction := cents % 100

	switch {
	case fraction == 0:
		return sign + strconv.FormatInt(whole, 10)
	case fraction%10 == 0:
		return sign + strconv.FormatInt(whole, 10) + "." + strconv.FormatInt(fraction/10, 10)
	default:
		return sign + strconv.FormatInt(whole, 10) + "." + fmt.Sprintf("%02d", fraction)
	}
}

func (a Amount) MarshalJSON() ([]byte, error) {
	return []byte(a.String()), nil
}

func (a *Amount) UnmarshalJSON(data []byte) error {
	if a == nil {
		return fmt.Errorf("amount target is nil")
	}

	if string(data) == "null" {
		return fmt.Errorf("amount must not be null")
	}

	var raw string
	if len(data) > 0 && data[0] == '"' {
		if err := json.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("decode amount string: %w", err)
		}
	} else {
		raw = string(data)
	}

	parsed, err := Parse(raw)
	if err != nil {
		return err
	}

	*a = parsed
	return nil
}

func (a Amount) Value() (driver.Value, error) {
	return a.String(), nil
}

func (a *Amount) Scan(src any) error {
	if a == nil {
		return fmt.Errorf("amount target is nil")
	}

	switch value := src.(type) {
	case nil:
		return fmt.Errorf("amount is null")
	case string:
		parsed, err := Parse(value)
		if err != nil {
			return err
		}

		*a = parsed
		return nil
	case []byte:
		parsed, err := Parse(string(value))
		if err != nil {
			return err
		}

		*a = parsed
		return nil
	case int64:
		*a = NewFromCents(value * 100)
		return nil
	case float64:
		parsed, err := Parse(strconv.FormatFloat(value, 'f', -1, 64))
		if err != nil {
			return err
		}

		*a = parsed
		return nil
	default:
		return fmt.Errorf("unsupported amount source %T", src)
	}
}

func parseDigits(input string) (int64, error) {
	if input == "" {
		return 0, fmt.Errorf("invalid amount")
	}

	for _, r := range input {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("invalid amount")
		}
	}

	value, err := strconv.ParseInt(input, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse amount: %w", err)
	}

	return value, nil
}
