package clashapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/sagernet/sing-box/dns"
)

// DNSStatsManager 管理 DNS 统计
type DNSStatsManager struct {
	aggregator *dns.StatsAggregator
}

// NewDNSStatsManager 创建 DNS 统计管理器
func NewDNSStatsManager() *DNSStatsManager {
	return &DNSStatsManager{
		aggregator: dns.NewStatsAggregator(5000),
	}
}

// Record 记录一条 DNS 查询
func (m *DNSStatsManager) Record(domain string, qType uint16, rcode int, transport string, latency int64, clientIP string) {
	if m.aggregator == nil {
		return
	}
	// latency 参数单位是毫秒
	m.aggregator.Record(domain, qType, rcode, transport, time.Duration(latency)*time.Millisecond, clientIP)
}

// dnsStatsRouter 创建 DNS 统计路由
func dnsStatsRouter(statsManager *DNSStatsManager) http.Handler {
	r := chi.NewRouter()
	r.Get("/summary", getSummary(statsManager))
	r.Get("/queries", getQueries(statsManager))
	r.Delete("/", clearStats(statsManager))
	return r
}

// getSummary 获取统计摘要
func getSummary(statsManager *DNSStatsManager) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if statsManager.aggregator == nil {
			render.Status(r, http.StatusServiceUnavailable)
			render.JSON(w, r, newError("stats not available"))
			return
		}
		summary := statsManager.aggregator.GetSummary()
		render.JSON(w, r, summary)
	}
}

// getQueries 获取查询列表
func getQueries(statsManager *DNSStatsManager) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if statsManager.aggregator == nil {
			render.Status(r, http.StatusServiceUnavailable)
			render.JSON(w, r, newError("stats not available"))
			return
		}
		limit := 100
		if l := r.URL.Query().Get("limit"); l != "" {
			if err := parseIntParam(l, &limit); err != nil {
				render.Status(r, http.StatusBadRequest)
				render.JSON(w, r, newError("invalid limit"))
				return
			}
		}
		queries := statsManager.aggregator.GetStats()
		if len(queries) > limit {
			queries = queries[:limit]
		}
		render.JSON(w, r, render.M{"queries": queries})
	}
}

// clearStats 清空统计
func clearStats(statsManager *DNSStatsManager) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if statsManager.aggregator == nil {
			render.Status(r, http.StatusServiceUnavailable)
			render.JSON(w, r, newError("stats not available"))
			return
		}
		statsManager.aggregator.Clear()
		render.JSON(w, r, render.M{"message": "stats cleared"})
	}
}

// parseIntParam 解析整数参数
func parseIntParam(s string, dest *int) error {
	val, err := strconv.Atoi(s)
	if err != nil {
		return err
	}
	*dest = val
	return nil
}
