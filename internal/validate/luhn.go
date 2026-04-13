package validate

func OrderNumber(number string) bool {
	if number == "" {
		return false
	}

	sum := 0
	double := false

	for i := len(number) - 1; i >= 0; i-- {
		digit := number[i]
		if digit < '0' || digit > '9' {
			return false
		}

		value := int(digit - '0')
		if double {
			value *= 2
			if value > 9 {
				value -= 9
			}
		}

		sum += value
		double = !double
	}

	return sum%10 == 0
}
