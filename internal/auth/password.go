package auth

import (
	"errors"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const (
	MinPasswordLength  = 8
	MaxPasswordBytes   = 72 //Bcrypt has a maximum password length of 72 bytes
	MinPasswordClasses = 3
)

var ErrWeakPassword = errors.New("password does not meet complexity requirements")

func HashPassword(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	return string(hash), nil
}

func CheckPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

func ValidatePassword(password string) error {
	if utf8.RuneCountInString(password) < MinPasswordLength || len(password) > MaxPasswordBytes {
		return ErrWeakPassword
	}

	if passwordClassCount(password) < MinPasswordClasses {
		return ErrWeakPassword
	}

	return nil
}

func passwordClassCount(password string) int {
	classes := map[string]bool{
		"lower":  false,
		"upper":  false,
		"digit":  false,
		"symbol": false,
	}

	for _, r := range password {
		switch {
		case unicode.IsLower(r):
			classes["lower"] = true
		case unicode.IsUpper(r):
			classes["upper"] = true
		case unicode.IsDigit(r):
			classes["digit"] = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			classes["symbol"] = true
		}
	}

	count := 0
	for _, ok := range classes {
		if ok {
			count++
		}
	}

	return count
}
