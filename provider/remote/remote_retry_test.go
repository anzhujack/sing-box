package remote

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/sagernet/sing-box/log"
	"github.com/stretchr/testify/require"
)

func TestProviderUpdateDelays(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	updateInterval := time.Hour

	require.Equal(t, providerInitialRetryInterval, initialProviderUpdateDelay(time.Time{}, updateInterval, now))
	require.Equal(t, 30*time.Minute, initialProviderUpdateDelay(now.Add(-30*time.Minute), updateInterval, now))
	require.Equal(t, time.Nanosecond, initialProviderUpdateDelay(now.Add(-2*time.Hour), updateInterval, now))
	require.Equal(t, providerInitialRetryInterval, providerRetryDelay(time.Time{}, updateInterval))
	require.Equal(t, updateInterval, providerRetryDelay(now, updateInterval))
}

func TestOverdueProviderUpdateDelayIsTickerSafe(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	for _, testCase := range []struct {
		name        string
		lastUpdated time.Time
	}{
		{name: "due now", lastUpdated: now.Add(-time.Hour)},
		{name: "overdue", lastUpdated: now.Add(-2 * time.Hour)},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			ticker := time.NewTicker(time.Hour)
			defer ticker.Stop()

			require.NotPanics(t, func() {
				ticker.Reset(initialProviderUpdateDelay(testCase.lastUpdated, time.Hour, now))
			})
		})
	}
}

func TestReadProviderResponseLimits(t *testing.T) {
	content, err := readProviderResponseLimited(strings.NewReader("abcd"), 4)
	require.NoError(t, err)
	require.Equal(t, []byte("abcd"), content)

	_, err = readProviderResponseLimited(strings.NewReader("abcde"), 4)
	require.ErrorContains(t, err, "exceeds size limit")

	_, err = readProviderResponseLimited(strings.NewReader(""), 4)
	require.ErrorContains(t, err, "empty response body")
}

func TestProviderFetchBounds(t *testing.T) {
	require.Equal(t, 60*time.Second, providerFetchTimeout)
	require.EqualValues(t, 50*1024*1024, providerMaxResponseBytes)
}

type roundTripFunc func(request *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type closeTrackingBody struct {
	io.Reader
	closed bool
}

func (b *closeTrackingBody) Close() error {
	b.closed = true
	return nil
}

func TestFetchClosesResponseBodyOnNonContentResponses(t *testing.T) {
	for _, statusCode := range []int{http.StatusNotModified, http.StatusInternalServerError} {
		t.Run(http.StatusText(statusCode), func(t *testing.T) {
			body := &closeTrackingBody{Reader: strings.NewReader("")}
			provider := &ProviderRemote{
				ctx:    context.Background(),
				logger: log.NewNOPFactory().NewLogger("provider"),
				url:    "https://provider.example/subscription",
				httpClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: statusCode,
						Status:     http.StatusText(statusCode),
						Header:     make(http.Header),
						Body:       body,
						Request:    request,
					}, nil
				})},
			}
			err := provider.fetch(context.Background(), true)
			if statusCode == http.StatusNotModified {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			require.True(t, body.closed)
		})
	}
}
