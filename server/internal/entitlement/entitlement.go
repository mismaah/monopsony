// Package entitlement is the single place that turns a user's tier into
// concrete capabilities. Everything that gates free vs paid consults Resolve;
// nothing else hard-codes tier rules.
package entitlement

import (
	"context"
	"encoding/json"
	"sync"

	"monopsony/server/internal/store"
)

// Tier names.
const (
	TierFree    = "free"
	TierPremium = "premium"
)

// Capabilities are the concrete limits applied to a user.
type Capabilities struct {
	Tier               string   `json:"tier"`
	MaxPlayersPerRoom  int      `json:"maxPlayersPerRoom"`
	PrivateRooms       bool     `json:"privateRooms"`
	HouseRules         bool     `json:"houseRules"`
	ShowAds            bool     `json:"showAds"`
	StatsHistoryDays   int      `json:"statsHistoryDays"` // 0 = unlimited
	MaxConcurrentGames int      `json:"maxConcurrentGames"`
	CosmeticSlots      []string `json:"cosmeticSlots"`
}

// Plan is an admin-editable tier definition.
type Plan struct {
	Tier    string       `json:"tier"`
	PriceID string       `json:"priceId,omitempty"` // payment-provider price for this tier
	Caps    Capabilities `json:"caps"`
}

// DefaultPlans are used until an admin edits them.
func DefaultPlans() map[string]Plan {
	return map[string]Plan{
		TierFree: {Tier: TierFree, Caps: Capabilities{
			Tier: TierFree, MaxPlayersPerRoom: 4, PrivateRooms: false, HouseRules: false,
			ShowAds: true, StatsHistoryDays: 7, MaxConcurrentGames: 1,
			CosmeticSlots: []string{"token"},
		}},
		TierPremium: {Tier: TierPremium, Caps: Capabilities{
			Tier: TierPremium, MaxPlayersPerRoom: 8, PrivateRooms: true, HouseRules: true,
			ShowAds: false, StatsHistoryDays: 0, MaxConcurrentGames: 5,
			CosmeticSlots: []string{"token", "board", "dice", "buildings", "cards"},
		}},
	}
}

// Resolver maps users to capabilities.
type Resolver struct {
	mu    sync.RWMutex
	plans map[string]Plan
}

// NewResolver starts with the default plans.
func NewResolver() *Resolver { return &Resolver{plans: DefaultPlans()} }

// SetPlan replaces a tier definition (admin API).
func (r *Resolver) SetPlan(p Plan) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p.Caps.Tier = p.Tier
	r.plans[p.Tier] = p
}

// Load replaces the defaults with plans persisted by the admin, if any.
func (r *Resolver) Load(ctx context.Context, st store.Store) error {
	recs, err := st.ListPlans(ctx)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rec := range recs {
		var caps Capabilities
		if err := json.Unmarshal(rec.Caps, &caps); err != nil {
			return err
		}
		caps.Tier = rec.Tier
		r.plans[rec.Tier] = Plan{Tier: rec.Tier, PriceID: rec.PriceID, Caps: caps}
	}
	return nil
}

// Save persists a plan and applies it.
func (r *Resolver) Save(ctx context.Context, st store.Store, p Plan) error {
	caps, err := json.Marshal(p.Caps)
	if err != nil {
		return err
	}
	if err := st.SavePlan(ctx, &store.PlanRecord{Tier: p.Tier, Caps: caps, PriceID: p.PriceID}); err != nil {
		return err
	}
	r.SetPlan(p)
	return nil
}

// Plans lists the current plans.
func (r *Resolver) Plans() []Plan {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Plan, 0, len(r.plans))
	for _, p := range r.plans {
		out = append(out, p)
	}
	return out
}

// Resolve returns a user's capabilities. Admins get premium capabilities.
func (r *Resolver) Resolve(_ context.Context, u *store.User) Capabilities {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tier := u.Tier
	if u.Role == "admin" {
		tier = TierPremium
	}
	if p, ok := r.plans[tier]; ok {
		return p.Caps
	}
	return r.plans[TierFree].Caps
}
