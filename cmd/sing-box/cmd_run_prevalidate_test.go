package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPreValidateJSONSyntaxAcceptsCommentsAndEscapes(t *testing.T) {
	content := []byte("{\n  // comment\n  \"value\": \"escaped \\\" quote and { bracket\",\n  # another comment\n  \"items\": [1, 2, 3] /* tail */\n}\n")
	require.NoError(t, preValidateJSONSyntax(content))
}

func TestPreValidateJSONSyntaxRejectsMalformedInput(t *testing.T) {
	testCases := []struct {
		name    string
		content string
		message string
	}{
		{name: "unterminated string", content: "{\"server\": \"dns_hosts\n}", message: "unterminated string literal"},
		{name: "unterminated comment", content: "{/* comment", message: "unterminated block comment"},
		{name: "extra brace", content: "{}}", message: "unmatched '}'"},
		{name: "missing bracket", content: "{\"items\": [1, 2}", message: "unclosed '['"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := preValidateJSONSyntax([]byte(testCase.content))
			require.ErrorContains(t, err, testCase.message)
		})
	}
}
