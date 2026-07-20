package dns

import (
	"sync"
	"time"

	"github.com/miekg/dns"
)

// QueryStats 单条查询统计
type QueryStats struct {
	Domain    string    `json:"domain"`
	QType     string    `json:"qtype"`
	Rcode     string    `json:"rcode"`
	Transport string    `json:"transport"`
	Latency   int64     `json:"latency_ms"`
	Timestamp time.Time `json:"timestamp"`
	ClientIP  string    `json:"client_ip,omitempty"`
}

// QueryRecorder 用于接入外部统计写入（如 clashapi）
type QueryRecorder interface {
	Record(domain string, qType uint16, rcode int, transport string, latency int64, clientIP string)
}

var (
	globalQueryRecorderAccess sync.RWMutex
	globalQueryRecorder       QueryRecorder
)

// SetQueryRecorder 设置全局 DNS 查询记录器
func SetQueryRecorder(recorder QueryRecorder) {
	globalQueryRecorderAccess.Lock()
	globalQueryRecorder = recorder
	globalQueryRecorderAccess.Unlock()
}

func recordExternalQuery(domain string, qType uint16, rcode int, transport string, latency int64, clientIP string) {
	globalQueryRecorderAccess.RLock()
	recorder := globalQueryRecorder
	globalQueryRecorderAccess.RUnlock()
	if recorder == nil {
		return
	}
	recorder.Record(domain, qType, rcode, transport, latency, clientIP)
}

// StatsAggregator DNS 查询统计聚合器（环形缓冲）
type StatsAggregator struct {
	mu      sync.RWMutex
	queries []QueryStats
	maxSize int
	index   int
	full    bool
	startAt time.Time
}

// NewStatsAggregator 创建统计聚合器（maxSize 条记录）
func NewStatsAggregator(maxSize int) *StatsAggregator {
	if maxSize < 100 {
		maxSize = 100
	}
	if maxSize > 100000 {
		maxSize = 100000
	}
	return &StatsAggregator{
		queries: make([]QueryStats, maxSize),
		maxSize: maxSize,
		startAt: time.Now(),
	}
}

// Record 记录一条查询
func (sa *StatsAggregator) Record(domain string, qType uint16, rcode int, transport string, latency time.Duration, clientIP string) {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	sa.queries[sa.index] = QueryStats{
		Domain:    domain,
		QType:     dns.TypeToString[qType],
		Rcode:     dns.RcodeToString[rcode],
		Transport: transport,
		Latency:   latency.Milliseconds(),
		Timestamp: time.Now(),
		ClientIP:  clientIP,
	}
	sa.index++
	if sa.index >= sa.maxSize {
		sa.index = 0
		sa.full = true
	}
}

// GetStats 获取所有统计（按时间倒序）
func (sa *StatsAggregator) GetStats() []QueryStats {
	sa.mu.RLock()
	defer sa.mu.RUnlock()

	var result []QueryStats
	if sa.full {
		// 环形缓冲已满，从 index 开始倒序
		for i := 0; i < sa.maxSize; i++ {
			idx := (sa.index - 1 - i + sa.maxSize) % sa.maxSize
			result = append(result, sa.queries[idx])
		}
	} else {
		// 未满，从 index-1 倒序到 0
		for i := sa.index - 1; i >= 0; i-- {
			result = append(result, sa.queries[i])
		}
	}
	return result
}

// GetStatsSince 获取指定时间之后的统计（按时间倒序）
func (sa *StatsAggregator) GetStatsSince(since time.Time) []QueryStats {
	all := sa.GetStats()
	if since.IsZero() {
		return all
	}
	result := make([]QueryStats, 0, len(all))
	for _, q := range all {
		if q.Timestamp.After(since) || q.Timestamp.Equal(since) {
			result = append(result, q)
		}
	}
	return result
}

// Summary 统计摘要
type Summary struct {
	TotalQueries   int64            `json:"total_queries"`
	SuccessQueries int64            `json:"success_queries"`
	FailedQueries  int64            `json:"failed_queries"`
	AvgLatency     int64            `json:"avg_latency_ms"`
	TopDomains     []DomainStat     `json:"top_domains"`
	TopTransports  []TransportStat  `json:"top_transports"`
	RcodeStats     map[string]int64 `json:"rcode_stats"`
	QTypeStats     map[string]int64 `json:"qtype_stats"`
	UpTime         int64            `json:"uptime_seconds"`
}

type DomainStat struct {
	Domain string `json:"domain"`
	Count  int64  `json:"count"`
}

type TransportStat struct {
	Transport string `json:"transport"`
	Count     int64  `json:"count"`
}

// GetSummary 获取统计摘要
func (sa *StatsAggregator) GetSummary() Summary {
	sa.mu.RLock()
	defer sa.mu.RUnlock()

	summary := Summary{
		RcodeStats: make(map[string]int64),
		QTypeStats: make(map[string]int64),
		UpTime:     int64(time.Since(sa.startAt).Seconds()),
	}

	domainMap := make(map[string]int64)
	transportMap := make(map[string]int64)
	var totalLatency int64

	var count int
	if sa.full {
		count = sa.maxSize
	} else {
		count = sa.index
	}

	for i := 0; i < count; i++ {
		q := sa.queries[i]
		summary.TotalQueries++
		if q.Rcode == "NOERROR" {
			summary.SuccessQueries++
		} else if q.Rcode != "" {
			summary.FailedQueries++
		}
		totalLatency += q.Latency
		domainMap[q.Domain]++
		transportMap[q.Transport]++
		summary.RcodeStats[q.Rcode]++
		summary.QTypeStats[q.QType]++
	}

	if summary.TotalQueries > 0 {
		summary.AvgLatency = totalLatency / summary.TotalQueries
	}

	// Top 10 domains
	topDomains := topN(domainMap, 10)
	for _, d := range topDomains {
		summary.TopDomains = append(summary.TopDomains, DomainStat{Domain: d.key, Count: d.count})
	}

	// Top 5 transports
	topTransports := topN(transportMap, 5)
	for _, t := range topTransports {
		summary.TopTransports = append(summary.TopTransports, TransportStat{Transport: t.key, Count: t.count})
	}

	return summary
}

type kv struct {
	key   string
	count int64
}

func topN(m map[string]int64, n int) []kv {
	var items []kv
	for k, v := range m {
		items = append(items, kv{k, v})
	}
	// 简单排序（实际可用 sort.Slice）
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if items[j].count > items[i].count {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
	if len(items) > n {
		items = items[:n]
	}
	return items
}

// Clear 清空统计
func (sa *StatsAggregator) Clear() {
	sa.mu.Lock()
	defer sa.mu.Unlock()
	sa.queries = make([]QueryStats, sa.maxSize)
	sa.index = 0
	sa.full = false
	sa.startAt = time.Now()
}
