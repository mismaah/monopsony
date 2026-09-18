package store

import (
	"context"
	"sort"
	"strings"
	"time"
)

type memMonetization struct {
	subs      map[string]*Subscription
	purchases map[string][]*Purchase
	webhooks  map[string]bool
	cosmetics map[string]*Cosmetic
	owned     map[string]map[string]bool
	loadouts  map[string]map[string]string
	plans     map[string]*PlanRecord
	audit     []*AuditEntry
}

func newMemMonetization() memMonetization {
	return memMonetization{
		subs: map[string]*Subscription{}, purchases: map[string][]*Purchase{}, webhooks: map[string]bool{},
		cosmetics: map[string]*Cosmetic{}, owned: map[string]map[string]bool{}, loadouts: map[string]map[string]string{},
		plans: map[string]*PlanRecord{},
	}
}

func (m *Mem) SaveSubscription(_ context.Context, s *Subscription) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subs[s.UserID] = clone(s)
	return nil
}

func (m *Mem) GetSubscription(_ context.Context, userID string) (*Subscription, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.subs[userID]
	if !ok {
		return nil, ErrNotFound
	}
	return clone(s), nil
}

func (m *Mem) SavePurchase(_ context.Context, p *Purchase) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.purchases[p.UserID] = append(m.purchases[p.UserID], clone(p))
	return nil
}

func (m *Mem) ListPurchases(_ context.Context, userID string) ([]*Purchase, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Purchase, 0, len(m.purchases[userID]))
	for _, p := range m.purchases[userID] {
		out = append(out, clone(p))
	}
	return out, nil
}

func (m *Mem) MarkWebhook(_ context.Context, provider, eventID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := provider + ":" + eventID
	if m.webhooks[k] {
		return false, nil
	}
	m.webhooks[k] = true
	return true, nil
}

func (m *Mem) SaveCosmetic(_ context.Context, c *Cosmetic) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cosmetics[c.ID] = clone(c)
	return nil
}

func (m *Mem) GetCosmetic(_ context.Context, id string) (*Cosmetic, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.cosmetics[id]
	if !ok {
		return nil, ErrNotFound
	}
	return clone(c), nil
}

func (m *Mem) ListCosmetics(_ context.Context, includeDisabled bool) ([]*Cosmetic, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []*Cosmetic
	for _, c := range m.cosmetics {
		if c.Enabled || includeDisabled {
			out = append(out, clone(c))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (m *Mem) GrantCosmetic(_ context.Context, userID, cosmeticID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.cosmetics[cosmeticID]; !ok {
		return ErrNotFound
	}
	if m.owned[userID] == nil {
		m.owned[userID] = map[string]bool{}
	}
	m.owned[userID][cosmeticID] = true
	return nil
}

func (m *Mem) OwnedCosmetics(_ context.Context, userID string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []string
	for id := range m.owned[userID] {
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}

func (m *Mem) SetLoadout(_ context.Context, userID string, loadout map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := map[string]string{}
	for k, v := range loadout {
		c[k] = v
	}
	m.loadouts[userID] = c
	return nil
}

func (m *Mem) GetLoadout(_ context.Context, userID string) (map[string]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := map[string]string{}
	for k, v := range m.loadouts[userID] {
		out[k] = v
	}
	return out, nil
}

func (m *Mem) SavePlan(_ context.Context, p *PlanRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p.UpdatedAt = time.Now()
	m.plans[p.Tier] = clone(p)
	return nil
}

func (m *Mem) ListPlans(_ context.Context) ([]*PlanRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []*PlanRecord
	for _, p := range m.plans {
		out = append(out, clone(p))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Tier < out[j].Tier })
	return out, nil
}

func (m *Mem) ListUsers(_ context.Context, query string, limit int) ([]*User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	q := strings.ToLower(strings.TrimSpace(query))
	var out []*User
	for _, u := range m.users {
		if q == "" || strings.Contains(strings.ToLower(u.Name), q) || strings.Contains(strings.ToLower(u.Email), q) || u.ID == q {
			c := *u
			out = append(out, &c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *Mem) Audit(_ context.Context, e *AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.audit = append(m.audit, clone(e))
	return nil
}

func (m *Mem) ListAudit(_ context.Context, limit int) ([]*AuditEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n := len(m.audit)
	if limit > 0 && n > limit {
		n = limit
	}
	out := make([]*AuditEntry, 0, n)
	for i := len(m.audit) - 1; i >= 0 && len(out) < n; i-- {
		out = append(out, clone(m.audit[i]))
	}
	return out, nil
}
