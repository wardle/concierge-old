package empi

import (
	"strings"
	"unicode"
)

// IsValidNHSNumber validates an NHS number
// Note: This does not check for repeated (and supposedly invalid) NHS numbers such as 4444444444 and 6666666666
// This is only an issue for NHS number generation and not the validation we have here.
//
func IsValidNHSNumber(nnn string) bool {
	var err error
	if nnn == "" || len(nnn) != 10 {
		return false
	}
	nni := make([]int, 10)
	sum, cd := 0, 0
	for i, c := range nnn {
		if unicode.IsDigit(c) == false {
			return false
		}
		nni[i] = int(c - '0')
		if err != nil {
			return false
		}
		if i < 9 {
			sum += nni[i] * (10 - i)
		}
	}
	cd = 11 - (sum % 11)
	if cd == 11 {
		cd = 0
	}
	return cd != 10 && cd == nni[9]
}

// FormatNHSNumber returns a formatted NHS number with spaces
// e.g. 0123456789 -> 012 345 6789
func FormatNHSNumber(nnn string) string {
	if nnn == "" || len(nnn) != 10 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(nnn[0:3])
	sb.WriteString(" ")
	sb.WriteString(nnn[3:6])
	sb.WriteString(" ")
	sb.WriteString(nnn[6:])
	return sb.String()
}
