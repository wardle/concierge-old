package empi

import "testing"

func TestValidation(t *testing.T) {
	valid := []string{
		"1111111111",
		"6328797966",
		"6148595893",
		"4865447040",
		"4823917286",
		"482 391 7286",
	}
	invalid := []string{
		"",
		" ",
		"4865447041",
		"a4785",
		"1234567890",
		"          ",
	}
	for _, nnn := range valid {
		if IsValidNHSNumber(nnn) == false {
			t.Errorf("%s reported as invalid", nnn)
		}
	}
	for _, nnn := range invalid {
		if IsValidNHSNumber(nnn) == true {
			t.Errorf("%s reported as valid", nnn)
		}
	}
}

func TestFormatting(t *testing.T) {
	tests := map[string]string{
		"1111111111": "111 111 1111",
		"6328797966": "632 879 7966",
		"a4785":      "",
	}
	for k, v := range tests {
		if FormatNHSNumber(k) != v {
			t.Errorf("failed for format NHS number. expected: %s. got: %s", v, FormatNHSNumber(k))
		}
	}
}
