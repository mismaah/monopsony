// Package cluster lets several server nodes share one game population.
// Every game is owned by exactly one node (its room actor runs there); the
// other nodes reach it through a small RPC layer over a pub/sub bus and
// forward the room's frames to their own WebSocket clients. Ownership and
// liveness live in a key-value space on the same bus, so when a node dies
// its games are adopted by whichever node next needs them (state is
// persisted after every command, so adoption is just a restore).
package cluster

import (
	"context"
	"sync"
	"time"
)

// Bus is the minimal pub/sub + key-value surface the cluster needs. Redis
// provides it in production; MemBus provides it in-process for tests.
type Bus interface {
	Publish(ctx context.Context, channel string, payload []byte) error
	// Subscribe delivers every payload on channel to fn, in order, from a
	// goroutine owned by the bus. The returned func cancels the subscription.
	Subscribe(ctx context.Context, channel string, fn func(payload []byte)) (cancel func(), err error)

	Set(ctx context.Context, key, value string, ttl time.Duration) error
	// SetNX sets key only if it is absent and reports whether it did.
	SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
	Get(ctx context.Context, key string) (value string, ok bool, err error)
	Del(ctx context.Context, keys ...string) error
	Close() error
}

// MemBus is an in-process Bus. Several nodes sharing one MemBus behave like
// several processes sharing one Redis.
type MemBus struct {
	mu   sync.Mutex
	subs map[string][]*memSub
	kv   map[string]kvEntry
	now  func() time.Time
}

type kvEntry struct {
	value   string
	expires time.Time // zero = never
}

type memSub struct {
	ch   chan []byte
	done chan struct{}
	once sync.Once
}

// NewMemBus creates an empty bus.
func NewMemBus() *MemBus {
	return &MemBus{subs: map[string][]*memSub{}, kv: map[string]kvEntry{}, now: time.Now}
}

func (b *MemBus) Publish(_ context.Context, channel string, payload []byte) error {
	b.mu.Lock()
	subs := append([]*memSub(nil), b.subs[channel]...)
	b.mu.Unlock()
	msg := append([]byte(nil), payload...)
	for _, s := range subs {
		select {
		case s.ch <- msg:
		case <-s.done:
		}
	}
	return nil
}

func (b *MemBus) Subscribe(_ context.Context, channel string, fn func([]byte)) (func(), error) {
	s := &memSub{ch: make(chan []byte, 4096), done: make(chan struct{})}
	b.mu.Lock()
	b.subs[channel] = append(b.subs[channel], s)
	b.mu.Unlock()
	go func() {
		for {
			select {
			case <-s.done:
				return
			case m := <-s.ch:
				fn(m)
			}
		}
	}()
	return func() {
		s.once.Do(func() { close(s.done) })
		b.mu.Lock()
		defer b.mu.Unlock()
		list := b.subs[channel]
		for i, x := range list {
			if x == s {
				b.subs[channel] = append(list[:i], list[i+1:]...)
				break
			}
		}
	}, nil
}

func (b *MemBus) live(key string) (kvEntry, bool) {
	e, ok := b.kv[key]
	if !ok {
		return kvEntry{}, false
	}
	if !e.expires.IsZero() && !b.now().Before(e.expires) {
		delete(b.kv, key)
		return kvEntry{}, false
	}
	return e, true
}

func (b *MemBus) entry(value string, ttl time.Duration) kvEntry {
	e := kvEntry{value: value}
	if ttl > 0 {
		e.expires = b.now().Add(ttl)
	}
	return e
}

func (b *MemBus) Set(_ context.Context, key, value string, ttl time.Duration) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.kv[key] = b.entry(value, ttl)
	return nil
}

func (b *MemBus) SetNX(_ context.Context, key, value string, ttl time.Duration) (bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.live(key); ok {
		return false, nil
	}
	b.kv[key] = b.entry(value, ttl)
	return true, nil
}

func (b *MemBus) Get(_ context.Context, key string) (string, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	e, ok := b.live(key)
	return e.value, ok, nil
}

func (b *MemBus) Del(_ context.Context, keys ...string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, k := range keys {
		delete(b.kv, k)
	}
	return nil
}

func (b *MemBus) Close() error { return nil }
