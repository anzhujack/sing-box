package build_shared

import "testing"

func TestNormalizeTag(t *testing.T) {
	testCases := map[string]string{
		"v1.14.0-alpha.48-reF1nd": "v1.14.0-alpha.48-xiaobaf14g",
		"v1.14.0-alpha.48":        "v1.14.0-alpha.48",
	}
	for input, expected := range testCases {
		if actual := normalizeTag(input); actual != expected {
			t.Fatalf("normalizeTag(%q) = %q, want %q", input, actual, expected)
		}
	}
}
