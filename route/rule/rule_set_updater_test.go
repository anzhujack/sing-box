package rule

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestInitialRuleSetUpdateDelay(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	updateInterval := time.Hour

	require.Equal(t, ruleSetInitialRetryInterval, initialRuleSetUpdateDelay(time.Time{}, updateInterval, now))
	require.Equal(t, 30*time.Minute, initialRuleSetUpdateDelay(now.Add(-30*time.Minute), updateInterval, now))
	require.Zero(t, initialRuleSetUpdateDelay(now.Add(-2*time.Hour), updateInterval, now))
}

func TestReadRuleSetResponseBounds(t *testing.T) {
	content, err := readRuleSetResponse(strings.NewReader("abcd"), 4)
	require.NoError(t, err)
	require.Equal(t, []byte("abcd"), content)

	_, err = readRuleSetResponse(strings.NewReader("abcde"), 4)
	require.ErrorContains(t, err, "exceeds")

	_, err = readRuleSetResponse(strings.NewReader(""), 4)
	require.ErrorContains(t, err, "empty")
}
