package dns

import (
	"context"
	"net"
	"sync"
	"testing"

	mDNS "github.com/miekg/dns"
	"github.com/sagernet/sing-box/adapter"
	"github.com/stretchr/testify/require"
)

func TestClientRecordsEachSyncAndAsyncExchangeOnce(t *testing.T) {
	recorder := new(countingQueryRecorder)
	SetQueryRecorder(recorder)
	t.Cleanup(func() { SetQueryRecorder(nil) })

	client := NewClient(ClientOptions{Context: context.Background()})
	client.Start()
	transport := newStatsCountingTransport()

	response, err := client.Exchange(context.Background(), transport, newStatsQuery(), adapter.DNSQueryOptions{}, nil)
	require.NoError(t, err)
	require.NotNil(t, response)

	response, err = client.Exchange(context.Background(), transport, newStatsQuery(), adapter.DNSQueryOptions{}, nil)
	require.NoError(t, err)
	require.NotNil(t, response)

	client.ClearCache()
	type asyncResult struct {
		response *mDNS.Msg
		err      error
	}
	callbackResult := make(chan asyncResult, 1)
	client.ExchangeAsync(context.Background(), transport, newStatsQuery(), adapter.DNSQueryOptions{}, nil, func(response *mDNS.Msg, err error) {
		callbackResult <- asyncResult{response: response, err: err}
	})
	result := <-callbackResult
	require.NoError(t, result.err)
	require.NotNil(t, result.response)

	require.Equal(t, 3, recorder.Count())
	require.Equal(t, 2, transport.Count())
}

type statsCountingTransport struct {
	TransportAdapter
	access sync.Mutex
	count  int
}

func newStatsCountingTransport() *statsCountingTransport {
	return &statsCountingTransport{TransportAdapter: NewTransportAdapter("test", "test", nil)}
}

func (t *statsCountingTransport) Start(adapter.StartStage) error { return nil }
func (t *statsCountingTransport) Close() error                   { return nil }
func (t *statsCountingTransport) Reset()                         {}

func (t *statsCountingTransport) Exchange(_ context.Context, message *mDNS.Msg) (*mDNS.Msg, error) {
	t.access.Lock()
	t.count++
	t.access.Unlock()

	response := new(mDNS.Msg)
	response.SetReply(message)
	response.Answer = []mDNS.RR{&mDNS.A{
		Hdr: mDNS.RR_Header{
			Name:   message.Question[0].Name,
			Rrtype: mDNS.TypeA,
			Class:  mDNS.ClassINET,
			Ttl:    60,
		},
		A: net.IPv4(192, 0, 2, 1),
	}}
	return response, nil
}

func (t *statsCountingTransport) ExchangeAsync(ctx context.Context, message *mDNS.Msg, callback func(response *mDNS.Msg, err error)) {
	callback(t.Exchange(ctx, message))
}

func (t *statsCountingTransport) Count() int {
	t.access.Lock()
	defer t.access.Unlock()
	return t.count
}

func newStatsQuery() *mDNS.Msg {
	message := new(mDNS.Msg)
	message.SetQuestion("example.com.", mDNS.TypeA)
	return message
}

type countingQueryRecorder struct {
	access sync.Mutex
	count  int
}

func (r *countingQueryRecorder) Record(_ string, _ uint16, _ int, _ string, _ int64, _ string) {
	r.access.Lock()
	r.count++
	r.access.Unlock()
}

func (r *countingQueryRecorder) Count() int {
	r.access.Lock()
	defer r.access.Unlock()
	return r.count
}
