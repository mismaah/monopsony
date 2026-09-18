package cluster

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"
)

// busSuite is run against MemBus always and against Redis when
// MONOPSONY_TEST_REDIS_URL is set, so both behave the same.
func busSuite(t *testing.T, bus Bus) {
	ctx := context.Background()
	prefix := "mp:test:" + time.Now().Format("150405.000") + ":"

	// Keys: set / get / setnx / ttl / del.
	if err := bus.Set(ctx, prefix+"k", "v", 0); err != nil {
		t.Fatal(err)
	}
	if v, ok, _ := bus.Get(ctx, prefix+"k"); !ok || v != "v" {
		t.Fatalf("get = %q %v", v, ok)
	}
	if ok, _ := bus.SetNX(ctx, prefix+"k", "w", 0); ok {
		t.Fatal("setnx on an existing key must fail")
	}
	if ok, _ := bus.SetNX(ctx, prefix+"n", "w", 0); !ok {
		t.Fatal("setnx on a new key must succeed")
	}
	if err := bus.Set(ctx, prefix+"ttl", "x", 300*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := bus.Get(ctx, prefix+"ttl"); !ok {
		t.Fatal("ttl key should exist right away")
	}
	time.Sleep(600 * time.Millisecond)
	if _, ok, _ := bus.Get(ctx, prefix+"ttl"); ok {
		t.Fatal("ttl key should have expired")
	}
	_ = bus.Del(ctx, prefix+"k", prefix+"n")
	if _, ok, _ := bus.Get(ctx, prefix+"k"); ok {
		t.Fatal("del")
	}

	// Pub/sub: ordered delivery to every subscriber, none after cancel.
	var mu sync.Mutex
	var got1, got2 []string
	c1, err := bus.Subscribe(ctx, prefix+"ch", func(p []byte) { mu.Lock(); got1 = append(got1, string(p)); mu.Unlock() })
	if err != nil {
		t.Fatal(err)
	}
	c2, err := bus.Subscribe(ctx, prefix+"ch", func(p []byte) { mu.Lock(); got2 = append(got2, string(p)); mu.Unlock() })
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{"a", "b", "c"} {
		if err := bus.Publish(ctx, prefix+"ch", []byte(m)); err != nil {
			t.Fatal(err)
		}
	}
	waitFor(t, "3 messages on both subscribers", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got1) == 3 && len(got2) == 3
	})
	mu.Lock()
	if got1[0] != "a" || got1[1] != "b" || got1[2] != "c" {
		t.Fatalf("order: %v", got1)
	}
	mu.Unlock()
	c2()
	_ = bus.Publish(ctx, prefix+"ch", []byte("d"))
	waitFor(t, "4th message on the live subscriber", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got1) == 4
	})
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	if len(got2) != 3 {
		t.Fatalf("cancelled subscriber received %d", len(got2))
	}
	mu.Unlock()
	c1()
}

func TestMemBus(t *testing.T) { busSuite(t, NewMemBus()) }

func TestRedisBus(t *testing.T) {
	url := os.Getenv("MONOPSONY_TEST_REDIS_URL")
	if url == "" {
		t.Skip("set MONOPSONY_TEST_REDIS_URL to run the Redis conformance test")
	}
	bus, err := OpenRedis(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Close()
	busSuite(t, bus)

	// Two nodes on a real Redis: the full RPC path.
	a, b, fr := twoNodes(t, bus, time.Second)
	p := b.Proxy("g1", a.ID)
	if p.Info().Name != "fake" {
		t.Fatal("info over redis")
	}
	sub := &recSub{}
	p.Subscribe(sub, 0)
	waitFor(t, "lobby frame over redis", func() bool { return sub.find("Lobby") != nil })
	p.Unsubscribe(sub)
	waitFor(t, "unsubscribe over redis", func() bool { return fr.subCount() == 0 })
}
