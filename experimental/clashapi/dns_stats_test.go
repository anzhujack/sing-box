package clashapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/miekg/dns"
)

func TestDNSStatsRouterNilManagerReturnsServiceUnavailable(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/summary", nil)
	recorder := httptest.NewRecorder()

	dnsStatsRouter(nil).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, recorder.Code)
	}
}

func TestDNSStatsManagerRecordsSummary(t *testing.T) {
	manager := NewDNSStatsManager()
	manager.Record("example.com", dns.TypeA, dns.RcodeSuccess, "direct", 12, "")

	summary := manager.aggregator.GetSummary()
	if summary.TotalQueries != 1 {
		t.Fatalf("expected 1 query, got %d", summary.TotalQueries)
	}
	if len(summary.TopDomains) != 1 || summary.TopDomains[0].Domain != "example.com" || summary.TopDomains[0].Count != 1 {
		t.Fatalf("unexpected top domains: %#v", summary.TopDomains)
	}
}
