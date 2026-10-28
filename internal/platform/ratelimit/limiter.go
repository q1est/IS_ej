package ratelimit

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"school-journal/internal/platform/httpx"
)

type bucket struct {
	started time.Time
	count   int
}

type Limiter struct {
	mu      sync.Mutex
	buckets map[string]bucket
	limit   int
	window  time.Duration
}

func New(limit int, window time.Duration) *Limiter {
	return &Limiter{buckets: make(map[string]bucket), limit: limit, window: window}
}

func (l *Limiter) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil || net.ParseIP(ip) == nil {
			ip = "unknown"
		}
		route := r.Pattern
		if route == "" {
			route = r.URL.Path
		}
		key := r.Method + ":" + route + ":" + ip
		now := time.Now()
		allowed, retryAfter := l.allow(key, now)
		if !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			httpx.Error(w, http.StatusTooManyRequests, "rate_limited", "Too many attempts; try again later")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (l *Limiter) allow(key string, now time.Time) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.buckets) > 4096 {
		for candidate, value := range l.buckets {
			if now.Sub(value.started) >= l.window {
				delete(l.buckets, candidate)
			}
		}
	}
	if len(l.buckets) >= 8192 {
		parts := strings.SplitN(key, ":", 3)
		key = "overflow:" + parts[0] + ":" + parts[1]
	}
	value := l.buckets[key]
	if now.Sub(value.started) >= l.window || value.started.IsZero() {
		value = bucket{started: now}
	}
	if value.count >= l.limit {
		l.buckets[key] = value
		remaining := int((l.window - now.Sub(value.started) + time.Second - 1) / time.Second)
		if remaining < 1 {
			remaining = 1
		}
		return false, remaining
	}
	value.count++
	l.buckets[key] = value
	return true, 0
}
