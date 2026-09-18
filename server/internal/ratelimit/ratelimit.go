// Package ratelimit is a small token-bucket limiter keyed by an arbitrary
// string (client IP, user id, connection). Buckets that have been idle long
// enough to refill are swept so the map does not grow without bound.
package ratelimit

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Rate allows Burst requests at once, then PerMinute sustained. A zero Rate
// disables the limiter.
type Rate struct {
	PerMinute int
	Burst     int
}

// Enabled reports whether the rate limits anything.
func (r Rate) Enabled() bool { return r.PerMinute > 0 }

type bucket struct {
	tokens float64
	last   time.Time
}

// Limiter holds one bucket per key.
type Limiter struct {
	rate Rate
	now  func() time.Time

	mu      sync.Mutex
	buckets map[string]*bucket
	swept   time.Time
}

// New creates a limiter; a zero rate allows everything.
func New(rate Rate) *Limiter {
	if rate.Burst <= 0 {
		rate.Burst = rate.PerMinute
	}
	return &Limiter{rate: rate, now: time.Now, buckets: map[string]*bucket{}}
}

// Allow consumes one token for key and reports whether it was available.
func (l *Limiter) Allow(key string) bool {
	ok, _ := l.take(key)
	return ok
}

// take returns whether a token was consumed and, if not, how long until one is.
func (l *Limiter) take(key string) (bool, time.Duration) {
	if !l.rate.Enabled() {
		return true, 0
	}
	perSec := float64(l.rate.PerMinute) / 60
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.swept) > time.Minute {
		l.sweep(now)
	}
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: float64(l.rate.Burst), last: now}
		l.buckets[key] = b
	}
	b.tokens += now.Sub(b.last).Seconds() * perSec
	if b.tokens > float64(l.rate.Burst) {
		b.tokens = float64(l.rate.Burst)
	}
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	return false, time.Duration((1-b.tokens)/perSec*float64(time.Second)) + time.Millisecond
}

// sweep drops buckets that are full again (idle long enough to refill).
func (l *Limiter) sweep(now time.Time) {
	l.swept = now
	perSec := float64(l.rate.PerMinute) / 60
	for k, b := range l.buckets {
		if b.tokens+now.Sub(b.last).Seconds()*perSec >= float64(l.rate.Burst) {
			delete(l.buckets, k)
		}
	}
}

// Len returns the number of tracked keys (tests).
func (l *Limiter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}

// ClientIP extracts the caller's address, honouring X-Forwarded-For only
// when the deployment says a trusted proxy sets it.
func ClientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if i := strings.IndexByte(xff, ','); i >= 0 {
				xff = xff[:i]
			}
			return strings.TrimSpace(xff)
		}
		if rip := r.Header.Get("X-Real-IP"); rip != "" {
			return strings.TrimSpace(rip)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Rejected is called when a request is dropped (for metrics).
type Rejected func(scope string)

// Middleware limits by client IP and answers 429 with a JSON error body in
// the API's error shape plus a Retry-After header.
func (l *Limiter) Middleware(scope string, trustProxy bool, rejected Rejected, next http.Handler) http.Handler {
	if !l.rate.Enabled() {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok, wait := l.take(ClientIP(r, trustProxy))
		if !ok {
			if rejected != nil {
				rejected(scope)
			}
			w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "rate_limited", "message": "too many requests, slow down"}})
			return
		}
		next.ServeHTTP(w, r)
	})
}
