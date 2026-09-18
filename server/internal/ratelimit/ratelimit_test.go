package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBucketRefills(t *testing.T) {
	l := New(Rate{PerMinute: 60, Burst: 2})
	now := time.Unix(0, 0)
	l.now = func() time.Time { return now }
	if !l.Allow("a") || !l.Allow("a") {
		t.Fatal("burst should allow two")
	}
	if l.Allow("a") {
		t.Fatal("third should be limited")
	}
	if !l.Allow("b") {
		t.Fatal("other keys are independent")
	}
	now = now.Add(time.Second) // 1 token/s
	if !l.Allow("a") {
		t.Fatal("should refill one token per second")
	}
	if l.Allow("a") {
		t.Fatal("only one token refilled")
	}
	now = now.Add(2 * time.Minute)
	l.Allow("c") // triggers a sweep: a and b are full again
	if l.Len() != 1 {
		t.Fatalf("sweep should drop idle buckets, have %d", l.Len())
	}
}

func TestDisabled(t *testing.T) {
	l := New(Rate{})
	for i := 0; i < 1000; i++ {
		if !l.Allow("x") {
			t.Fatal("zero rate must allow everything")
		}
	}
}

func TestMiddleware(t *testing.T) {
	l := New(Rate{PerMinute: 60, Burst: 1})
	var scope string
	h := l.Middleware("api", true, func(s string) { scope = s }, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) }))
	do := func(ip string) int {
		req := httptest.NewRequest("GET", "/x", nil)
		req.Header.Set("X-Forwarded-For", ip+", 10.0.0.1")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if do("1.1.1.1") != 204 || do("1.1.1.1") != 429 || do("2.2.2.2") != 204 {
		t.Fatal("per-ip limiting broken")
	}
	if scope != "api" {
		t.Fatalf("rejected callback scope = %q", scope)
	}
	req := httptest.NewRequest("GET", "/x", nil)
	req.RemoteAddr = "3.3.3.3:1234"
	if ClientIP(req, false) != "3.3.3.3" {
		t.Fatal("remote addr host")
	}
	req.Header.Set("X-Forwarded-For", "9.9.9.9")
	if ClientIP(req, false) != "3.3.3.3" || ClientIP(req, true) != "9.9.9.9" {
		t.Fatal("proxy trust")
	}
}
