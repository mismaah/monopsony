package store

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"monopsony/server/internal/game"
)

// Mem is an in-memory Store for development and tests.
type Mem struct {
	mu        sync.RWMutex
	users     map[string]*User
	byEmail   map[string]string
	tokens    map[string]RefreshToken
	games     map[string]*GameRecord
	events    map[string][]game.EventEnvelope
	configs   map[string]*ConfigRecord
	published string
	memMonetization
}

// NewMem returns an empty in-memory store.
func NewMem() *Mem {
	return &Mem{
		users:   map[string]*User{},
		byEmail: map[string]string{},
		tokens:  map[string]RefreshToken{},
		games:   map[string]*GameRecord{},
		events:  map[string][]game.EventEnvelope{},
		configs: map[string]*ConfigRecord{},

		memMonetization: newMemMonetization(),
	}
}

func clone[T any](v *T) *T {
	if v == nil {
		return nil
	}
	b, _ := json.Marshal(v)
	var out T
	_ = json.Unmarshal(b, &out)
	return &out
}

func (m *Mem) CreateUser(_ context.Context, u *User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.users[u.ID]; ok {
		return ErrConflict
	}
	if u.Email != "" {
		if _, ok := m.byEmail[strings.ToLower(u.Email)]; ok {
			return ErrConflict
		}
		m.byEmail[strings.ToLower(u.Email)] = u.ID
	}
	c := *u
	m.users[u.ID] = &c
	return nil
}

func (m *Mem) GetUser(_ context.Context, id string) (*User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	u, ok := m.users[id]
	if !ok {
		return nil, ErrNotFound
	}
	c := *u
	return &c, nil
}

func (m *Mem) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	m.mu.RLock()
	id, ok := m.byEmail[strings.ToLower(email)]
	m.mu.RUnlock()
	if !ok {
		return nil, ErrNotFound
	}
	return m.GetUser(ctx, id)
}

func (m *Mem) UpdateUser(_ context.Context, u *User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.users[u.ID]; !ok {
		return ErrNotFound
	}
	c := *u
	m.users[u.ID] = &c
	return nil
}

func (m *Mem) PutRefreshToken(_ context.Context, t RefreshToken) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tokens[t.Hash] = t
	return nil
}

func (m *Mem) GetRefreshToken(_ context.Context, hash string) (*RefreshToken, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tokens[hash]
	if !ok || time.Now().After(t.ExpiresAt) {
		return nil, ErrNotFound
	}
	return &t, nil
}

func (m *Mem) DeleteRefreshToken(_ context.Context, hash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.tokens, hash)
	return nil
}

func (m *Mem) SaveGame(_ context.Context, g *GameRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.games[g.ID] = clone(g)
	return nil
}

func (m *Mem) GetGame(_ context.Context, id string) (*GameRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	g, ok := m.games[id]
	if !ok {
		return nil, ErrNotFound
	}
	return clone(g), nil
}

func (m *Mem) ListGames(_ context.Context, status string, publicOnly bool, limit int) ([]*GameRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []*GameRecord
	for _, g := range m.games {
		if status != "" && g.Status != status {
			continue
		}
		if publicOnly && g.Visibility != "public" {
			continue
		}
		out = append(out, clone(g))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *Mem) ListGamesForUser(_ context.Context, userID string, limit int) ([]*GameRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []*GameRecord
	for _, g := range m.games {
		for _, s := range g.Seats {
			if s.UserID == userID {
				out = append(out, clone(g))
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *Mem) AppendEvents(_ context.Context, gameID string, events []game.EventEnvelope) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events[gameID] = append(m.events[gameID], events...)
	return nil
}

func (m *Mem) GetEvents(_ context.Context, gameID string, afterSeq int) ([]game.EventEnvelope, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []game.EventEnvelope
	for _, e := range m.events[gameID] {
		if e.Seq > afterSeq {
			out = append(out, e)
		}
	}
	return out, nil
}

func (m *Mem) SaveConfig(_ context.Context, c *ConfigRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.configs[c.ID]; ok {
		return ErrConflict
	}
	m.configs[c.ID] = clone(c)
	if c.Published {
		m.published = c.ID
	}
	return nil
}

func (m *Mem) GetConfig(_ context.Context, id string) (*ConfigRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.configs[id]
	if !ok {
		return nil, ErrNotFound
	}
	return clone(c), nil
}

func (m *Mem) GetPublishedConfig(_ context.Context) (*ConfigRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.configs[m.published]
	if !ok {
		return nil, ErrNotFound
	}
	return clone(c), nil
}

func (m *Mem) ListConfigs(_ context.Context) ([]*ConfigRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []*ConfigRecord
	for _, c := range m.configs {
		out = append(out, clone(c))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version > out[j].Version })
	return out, nil
}

func (m *Mem) PublishConfig(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.configs[id]; !ok {
		return ErrNotFound
	}
	for _, c := range m.configs {
		c.Published = c.ID == id
	}
	m.published = id
	return nil
}

func (m *Mem) Close() error { return nil }
