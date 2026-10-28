package ratelimit

import (
	"testing"
	"time"
)

func TestWindowLimitAndReset(t *testing.T) {
	limiter := New(2, time.Minute)
	start := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	if allowed, _ := limiter.allow("login:ip", start); !allowed {
		t.Fatal("first request was unexpectedly denied")
	}
	if allowed, _ := limiter.allow("login:ip", start.Add(time.Second)); !allowed {
		t.Fatal("second request was unexpectedly denied")
	}
	if allowed, retry := limiter.allow("login:ip", start.Add(2*time.Second)); allowed || retry < 1 {
		t.Fatalf("third request result = allowed:%v retry:%d, want denied with retry", allowed, retry)
	}
	if allowed, _ := limiter.allow("login:ip", start.Add(time.Minute)); !allowed {
		t.Fatal("request after window reset was unexpectedly denied")
	}
}
