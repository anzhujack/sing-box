package group

import (
	mathrand "math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"github.com/puzpuzpuz/xsync/v3"
	"github.com/sagernet/sing-box/adapter"
)

// Extended Smart algorithms + cross-cutting hysteresis + perf hooks.
//
// Why a second file: smart_algorithm.go houses the original five
// algorithms plus the dispatch contract. Keeping the new four here
// (round-robin, weighted-rr, p2c, latency-banded) along with the
// hysteresis layer makes the diff legible during review and lets the
// older code stay untouched.
//
// All four new algorithms are O(K) work where K = min(top-K cap, len)
// and zero allocations per dial. Hysteresis adds one xsync.MapOf load
// per dial; the cache line is shared with sticky-session.

const (
	smartAlgoRoundRobin        = "round-robin"
	smartAlgoWeightedRR        = "weighted-rr"
	smartAlgoP2C               = "p2c"
	smartAlgoLatencyBanded     = "latency-banded"
	smartAlgoConsistentHashing = "consistent-hashing"

	// p2cTopK is the candidate window from which we sample two for
	// comparison. 5 mirrors weightedRandomTopK so all "sample within
	// top set" algorithms share the same breadth.
	p2cTopK = 5

	// latencyBandDynamicMinMS / MaxMS bound latency-banded's dynamic
	// best+delta cohort. The old fixed buckets (<50 / 50-150 / >150)
	// treated 71 ms and 94 ms as equivalent "medium" nodes; dynamic
	// banding keeps only nodes close to the CURRENT best measured delay.
	latencyBandDynamicMinMS = 8.0
	latencyBandDynamicMaxMS = 25.0
	latencyBandDynamicRatio = 0.12
)

// shortRTTCacheTTL is how long a per-node ShortRTT lookup is cached in
// memory before the algorithm dispatcher re-queries the AtomicStatsRecord.
// Short enough that a recent latency spike still gets seen within a
// dial cycle, long enough that 1k QPS doesn't hammer the recordCache
// mutex on every selection. 250 ms × 1k QPS → 4 reads/sec/node.
const shortRTTCacheTTL = 250 * time.Millisecond

// shortRTTCacheEntry pairs a measurement with the wall-clock time it
// was captured. Stored by value (xsync.MapOf supports any comparable
// value type but we want amortised allocation-free reads via Load).
type shortRTTCacheEntry struct {
	rttMS    float64
	storedAt int64 // unix-nano
}

// roundRobinCounter is the atomic dial counter used by round-robin.
// Wrapped in a struct so future enhancements (per-target counters,
// metrics) have a place to live without breaking the API.
type roundRobinCounter struct{ n atomic.Uint64 }

func (r *roundRobinCounter) next(modulo int) int {
	if modulo <= 0 {
		return 0
	}
	return int(r.n.Add(1) % uint64(modulo))
}

// stickyKey is a struct value used as xsync.MapOf key for sticky &
// hysteresis lookups. Replaces the previous "target + |udp" string
// concat — saves an allocation per dial under load. xsync.MapOf
// requires comparable; struct-of-strings qualifies.
type stickyKey struct {
	target string
	isUDP  bool
}

// hysteresisEntry pairs the last-picked node tag with the time that
// pick became authoritative. Read on every algorithm pass; written on
// every dial-success funnel (rememberStickyChoice).
type hysteresisEntry struct {
	tag string
	at  int64 // unix-nano
}

// algoRandSource is a per-CPU rand source. math/rand/v2 NewPCG is
// lock-free per instance; we shard so concurrent reorderForAlgorithm
// callers across goroutines don't serialise on the global default
// source. Allocated once at init and indexed by a goroutine-stable
// counter.
//
// 16 shards is plenty: contention on math/rand at 1k QPS is already
// negligible after sharding even four-way. The extra shards leave
// headroom for the parallel-dial expansion.
const algoRandShards = 16

var algoRandShardsArr [algoRandShards]*mathrand.Rand
var algoRandIdx atomic.Uint32

func init() {
	for i := range algoRandShardsArr {
		algoRandShardsArr[i] = mathrand.New(mathrand.NewPCG(uint64(i)+1, 0xC4F5_2A1B_DEF0_1234))
	}
}

// pickRand returns a per-call rand source. The cursor is incremented
// atomically so concurrent callers fan out across the shards. Callers
// must NOT cache the returned pointer across goroutines.
func pickRand() *mathrand.Rand {
	idx := int(algoRandIdx.Add(1)) & (algoRandShards - 1)
	return algoRandShardsArr[idx]
}

// shortRTTCache is a global short-TTL memoiser for AtomicStatsRecord
// ShortRTT lookups. The recordCache itself already deduplicates the
// underlying bbolt fetch, but ShortRTT() takes the per-record mutex on
// every call — at 1k QPS that mutex shows up in profiles. The 250 ms
// TTL lets the algorithm see near-real-time data without paying the
// mutex cost more than ~4 times/second/node.
var shortRTTCache = xsync.NewMapOf[string, shortRTTCacheEntry]()

// cachedShortRTT returns ShortRTT for a tag, refreshing the cache
// when the entry is stale or missing. tag is namespaced with the
// group name so different Smart groups don't share entries (their
// underlying records are distinct).
func (s *Smart) cachedShortRTT(tag string) float64 {
	if tag == "" {
		return 0
	}
	key := s.Tag() + "|" + tag
	now := time.Now().UnixNano()
	if entry, ok := shortRTTCache.Load(key); ok {
		if now-entry.storedAt < int64(shortRTTCacheTTL) {
			return entry.rttMS
		}
	}
	rtt := s.shortRTTFor(tag)
	shortRTTCache.Store(key, shortRTTCacheEntry{rttMS: rtt, storedAt: now})
	return rtt
}

// candidateDelayMS returns the best available per-node delay signal in
// milliseconds. ShortRTT is the freshest real-traffic signal; URLTest
// history is the cold-start / health-check fallback. Zero means unknown.
func (s *Smart) candidateDelayMS(tag string) float64 {
	if tag == "" {
		return 0
	}
	if rtt := s.cachedShortRTT(tag); rtt > 0 {
		return rtt
	}
	if s.history != nil {
		if h := s.history.LoadURLTestHistory(tag); h != nil && h.Delay > 0 {
			return float64(h.Delay)
		}
	}
	return 0
}

func dynamicLatencyBandDeltaMS(best float64) float64 {
	if best <= 0 {
		return 0
	}
	delta := best * latencyBandDynamicRatio
	if delta < latencyBandDynamicMinMS {
		return latencyBandDynamicMinMS
	}
	if delta > latencyBandDynamicMaxMS {
		return latencyBandDynamicMaxMS
	}
	return delta
}

// reorderRoundRobin picks the next candidate by an atomic counter
// modulo the candidate count. O(1) work. Counter is per-Smart so two
// groups don't share rotation state.
func (s *Smart) reorderRoundRobin(candidates []adapter.Outbound) []adapter.Outbound {
	if s.rrCounter == nil {
		return candidates
	}
	idx := s.rrCounter.next(len(candidates))
	if idx > 0 {
		candidates[0], candidates[idx] = candidates[idx], candidates[0]
	}
	return candidates
}

// reorderWeightedRoundRobin assigns each top-K position a turn budget
// proportional to its rank (k turns for position 0, k-1 for position 1,
// ..., 1 for position k-1) and visits them in proportional rotation.
// Implemented as a per-Smart cursor + Σ-budget — same O(1) per dial as
// vanilla RR, just with a precomputed bucket map.
func (s *Smart) reorderWeightedRoundRobin(candidates []adapter.Outbound) []adapter.Outbound {
	if s.wrrCounter == nil {
		return candidates
	}
	k := p2cTopK
	if k > len(candidates) {
		k = len(candidates)
	}
	total := uint64(k * (k + 1) / 2)
	cursor := s.wrrCounter.next(int(total))
	// Map cursor ∈ [0, total) → bucket index ∈ [0, k).
	pick := uint64(cursor)
	idx := 0
	acc := uint64(0)
	for i := 0; i < k; i++ {
		acc += uint64(k - i)
		if pick < acc {
			idx = i
			break
		}
	}
	if idx > 0 {
		candidates[0], candidates[idx] = candidates[idx], candidates[0]
	}
	return candidates
}

// reorderP2C samples two distinct candidates from top-K and promotes
// the one with the lower cached ShortRTT (or, on RTT tie, the one
// with fewer active connections). Power-of-two-choices is provably
// load-balancing with O(1) overhead per pick.
//
// Falls back to strict-best when only one candidate exists or
// rand/source is unavailable.
func (s *Smart) reorderP2C(candidates []adapter.Outbound) []adapter.Outbound {
	if len(candidates) < 2 {
		return candidates
	}
	k := p2cTopK
	if k > len(candidates) {
		k = len(candidates)
	}
	r := pickRand()
	a := r.IntN(k)
	b := r.IntN(k - 1)
	if b >= a {
		b++ // ensures a != b without rejection sampling
	}
	winner := s.scoreP2C(candidates[a], candidates[b])
	pickIdx := a
	if winner == 1 {
		pickIdx = b
	}
	if pickIdx > 0 {
		candidates[0], candidates[pickIdx] = candidates[pickIdx], candidates[0]
	}
	return candidates
}

// scoreP2C decides which of two candidates wins under p2c rules:
// lower ShortRTT first; on RTT tie (or both 0), fewer active
// connections; final tiebreak is the original ordering (return 0 →
// keep candidates[0] as the winner).
func (s *Smart) scoreP2C(a, b adapter.Outbound) int {
	rttA := s.cachedShortRTT(a.Tag())
	rttB := s.cachedShortRTT(b.Tag())
	switch {
	case rttA > 0 && rttB > 0 && rttA != rttB:
		if rttA < rttB {
			return 0
		}
		return 1
	}
	if s.nodeLoad != nil {
		la := s.nodeLoad.get(a.Tag())
		lb := s.nodeLoad.get(b.Tag())
		if la < lb {
			return 0
		}
		if lb < la {
			return 1
		}
	}
	return 0
}

// reorderLatencyBanded promotes a uniformly-random pick from the
// dynamic best+delta cohort. This keeps load spread among truly
// comparable nodes while preventing the old fixed-bucket problem where
// 71 ms and 94 ms Asia nodes were both "medium" and randomly swapped.
//
// Allocation-free: per-call buckets are scratch indexes only.
func (s *Smart) reorderLatencyBanded(candidates []adapter.Outbound) []adapter.Outbound {
	if len(candidates) < 2 {
		return candidates
	}
	k := p2cTopK
	if k > len(candidates) {
		k = len(candidates)
	}
	var delays [p2cTopK]float64
	best := 0.0
	for i := 0; i < k; i++ {
		d := s.candidateDelayMS(candidates[i].Tag())
		delays[i] = d
		if d > 0 && (best == 0 || d < best) {
			best = d
		}
	}
	if best == 0 {
		return candidates
	}
	limit := best + dynamicLatencyBandDeltaMS(best)
	var cohort [p2cTopK]int
	nCohort := 0
	for i := 0; i < k; i++ {
		if d := delays[i]; d > 0 && d <= limit {
			cohort[nCohort] = i
			nCohort++
		}
	}
	if nCohort == 0 {
		return candidates
	}
	r := pickRand()
	pick := cohort[r.IntN(nCohort)]
	if pick > 0 {
		candidates[0], candidates[pick] = candidates[pick], candidates[0]
	}
	return candidates
}

// applyHysteresis is the cross-cutting anti-flap layer applied AFTER
// reorderForAlgorithm. `hysteresis` is interpreted as a quality delta
// (milliseconds), not a wall-clock pin duration: keep the previous pick
// only while it remains within threshold of the fresh best candidate.
//
// Why post-algorithm: we want algorithm scoring to see uncoloured
// candidates so its own decisions remain meaningful for new targets;
// hysteresis only kicks in once history exists.
func (s *Smart) applyHysteresis(candidates []adapter.Outbound, target string, isUDP bool) []adapter.Outbound {
	if s.hysteresisWindow <= 0 || s.hysteresisMemo == nil ||
		target == "" || len(candidates) <= 1 {
		return candidates
	}
	entry, ok := s.hysteresisMemo.Load(stickyKey{target, isUDP})
	if !ok || entry.tag == "" {
		return candidates
	}
	for i, ob := range candidates {
		if ob.Tag() == entry.tag {
			bestDelay := s.candidateDelayMS(candidates[0].Tag())
			prevDelay := s.candidateDelayMS(entry.tag)
			if bestDelay > 0 && prevDelay > 0 {
				delta := prevDelay - bestDelay
				if delta < 0 {
					delta = 0
				}
				if delta > float64(s.hysteresisWindow/time.Millisecond) {
					return candidates
				}
			}
			if i != 0 {
				candidates[0], candidates[i] = candidates[i], candidates[0]
			}
			return candidates
		}
	}
	// Previously-picked node has dropped from the candidate list
	// (e.g. went dead) — let the algorithm's choice stand.
	return candidates
}

func (s *Smart) selectionPreview(target string, isUDP bool) map[string]any {
	out := map[string]any{
		"algorithm":     s.CurrentAlgorithm(),
		"hysteresis_ms": float64(s.hysteresisWindow / time.Millisecond),
		"now":           s.Now(),
	}
	snap := s.state.Load()
	if snap == nil || len(snap.outbounds) == 0 {
		out["source"] = "empty"
		return out
	}
	candidates := make([]adapter.Outbound, len(snap.outbounds))
	copy(candidates, snap.outbounds)
	candidates = s.reorderForAlgorithm(candidates, target, isUDP)
	fresh := ""
	if len(candidates) > 0 {
		fresh = candidates[0].Tag()
		out["fresh_best"] = fresh
		if d := s.candidateDelayMS(fresh); d > 0 {
			out["fresh_best_delay_ms"] = d
		}
	}
	candidates = s.applyHysteresis(candidates, target, isUDP)
	if len(candidates) > 0 {
		recommended := candidates[0].Tag()
		out["recommended"] = recommended
		if d := s.candidateDelayMS(recommended); d > 0 {
			out["recommended_delay_ms"] = d
		}
		if recommended != fresh {
			out["reason"] = "kept_by_hysteresis_delta"
		} else {
			out["reason"] = "fresh_algorithm_choice"
		}
	}
	out["source"] = "algorithm-preview"
	return out
}

// rememberHysteresisChoice records the just-picked node for a target.
// Cheap (one xsync store) and safe to call from any dial-success
// funnel.
func (s *Smart) rememberHysteresisChoice(target, node string, isUDP bool) {
	if s.hysteresisMemo == nil || target == "" || node == "" {
		return
	}
	s.hysteresisMemo.Store(stickyKey{target, isUDP}, hysteresisEntry{
		tag: node,
		at:  time.Now().UnixNano(),
	})
}

// factorsPool recycles []float64 scratch slices used by
// reorderByPriority's pre-computed factor cache. Capacity tuned to
// smartMaxSelected (10) so the typical request avoids re-allocation
// and pool growth.
var factorsPool = sync.Pool{
	New: func() any {
		s := make([]float64, 0, 16)
		return &s
	},
}

func acquireFactorsSlice(n int) *[]float64 {
	p := factorsPool.Get().(*[]float64)
	if cap(*p) < n {
		*p = make([]float64, n)
	} else {
		*p = (*p)[:n]
	}
	return p
}

func releaseFactorsSlice(p *[]float64) {
	if p == nil {
		return
	}
	*p = (*p)[:0]
	factorsPool.Put(p)
}
