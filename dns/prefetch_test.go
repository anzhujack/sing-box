package dns

import (
	"testing"
	"time"

	"github.com/miekg/dns"
)

func TestPrefetchManager_ShouldPrefetch(t *testing.T) {
	pm := NewPrefetchManager(100, 10)
	now := time.Now()
	key := dnsCacheKey{
		Question:     dns.Question{Name: "example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET},
		transportTag: "default",
	}

	// 未记录的 key 不应预刷新
	if pm.ShouldPrefetch(key, 300, now) {
		t.Error("未记录的 key 不应预刷新")
	}

	// 记录访问后立即应该可以预刷新
	pm.RecordAccess(key, 300, now)
	if !pm.ShouldPrefetch(key, 300, now) {
		t.Error("记录访问后应该可以预刷新")
	}

	// 再次访问会更新预刷新时间
	pm.RecordAccess(key, 300, now.Add(100*time.Second))
	// 此时预刷新时间应该被推后，不应该立即预刷新
	if pm.ShouldPrefetch(key, 300, now.Add(100*time.Second)) {
		t.Error("更新访问后预刷新时间应该被推后")
	}
}

func TestPrefetchManager_Backoff(t *testing.T) {
	pm := NewPrefetchManager(100, 10)
	now := time.Now()
	key := dnsCacheKey{
		Question:     dns.Question{Name: "example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET},
		transportTag: "default",
	}

	pm.RecordAccess(key, 300, now)

	// 记录失败
	pm.RecordFailure(key, now)

	// 失败后立即不应预刷新
	if pm.ShouldPrefetch(key, 300, now) {
		t.Error("失败后立即不应预刷新")
	}

	// 退避时间后应该可以预刷新
	if !pm.ShouldPrefetch(key, 300, now.Add(2*time.Second)) {
		t.Error("退避时间后应该可以预刷新")
	}

	// 成功后重置失败计数
	pm.RecordSuccess(key)
	if !pm.ShouldPrefetch(key, 300, now.Add(2*time.Second)) {
		t.Error("成功后应该重置失败计数，应该可以立即预刷新")
	}
}

func TestPrefetchManager_Eviction(t *testing.T) {
	pm := NewPrefetchManager(2, 10)
	now := time.Now()

	key1 := dnsCacheKey{
		Question:     dns.Question{Name: "example1.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET},
		transportTag: "default",
	}
	key2 := dnsCacheKey{
		Question:     dns.Question{Name: "example2.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET},
		transportTag: "default",
	}
	key3 := dnsCacheKey{
		Question:     dns.Question{Name: "example3.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET},
		transportTag: "default",
	}

	pm.RecordAccess(key1, 300, now)
	pm.RecordAccess(key2, 300, now.Add(1*time.Second))
	pm.RecordAccess(key3, 300, now.Add(2*time.Second))

	// 应该淘汰最久未访问的 key1
	pm.mu.RLock()
	if _, exists := pm.metadata[key1]; exists {
		t.Error("应该淘汰最久未访问的 key")
	}
	pm.mu.RUnlock()
}

func TestPrefetchManager_Cleanup(t *testing.T) {
	pm := NewPrefetchManager(100, 10)
	now := time.Now()

	key1 := dnsCacheKey{
		Question:     dns.Question{Name: "example1.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET},
		transportTag: "default",
	}
	key2 := dnsCacheKey{
		Question:     dns.Question{Name: "example2.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET},
		transportTag: "default",
	}

	pm.RecordAccess(key1, 300, now)
	pm.RecordAccess(key2, 300, now.Add(45*time.Minute))

	// 清理 30 分钟未访问的，当前时间为 now + 1 小时
	pm.Cleanup(now.Add(1*time.Hour), 30*time.Minute)

	pm.mu.RLock()
	if _, exists := pm.metadata[key1]; exists {
		t.Error("应该清理过期的元数据")
	}
	if _, exists := pm.metadata[key2]; !exists {
		t.Error("不应该清理最近访问的元数据")
	}
	pm.mu.RUnlock()
}
