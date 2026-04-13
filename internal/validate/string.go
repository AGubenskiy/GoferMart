package validate

import "unicode/utf8"

const MaxVarcharLength = 200

func StringLengthAtMost(value string, limit int) bool {
	return utf8.RuneCountInString(value) <= limit
}
