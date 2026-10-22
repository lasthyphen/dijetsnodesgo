package txservice

import (
	"sync"
	"time"
)

// RateLimit allows Burst requests at once, refilled at PerMinute per minute.
type RateLimit struct {
	PerMinute float64 `json:"perMinute"`
	Burst     float64 `json:"burst"`
}

type bucket struct {
	tokens float64
	last   time.Time
}

// rateLimiter is a token bucket per key.
type rateLimiter struct {
	lock    sync.Mutex
	limit   RateLimit
	buckets map[string]*bucket
	now     func() time.Time
}

func newRateLimiter(limit RateLimit, now func() time.Time) *rateLimiter {
	return &rateLimiter{
		limit:   limit,
		buckets: make(map[string]*bucket),
		now:     now,
	}
}

// Allow takes a token for [key] and reports whether one was available.
func (r *rateLimiter) Allow(key string) bool {
	if r.limit.PerMinute <= 0 {
		return true
	}
	r.lock.Lock()
	defer r.lock.Unlock()

	now := r.now()
	b, ok := r.buckets[key]
	if !ok {
		b = &bucket{tokens: r.limit.Burst, last: now}
		r.buckets[key] = b
	}
	b.tokens += now.Sub(b.last).Minutes() * r.limit.PerMinute
	if b.tokens > r.limit.Burst {
		b.tokens = r.limit.Burst
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
