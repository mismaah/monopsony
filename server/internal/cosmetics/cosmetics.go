// Package cosmetics manages the shop catalog, ownership and per-user
// loadouts. Items are data; the client renders whatever the manifest says.
package cosmetics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"monopsony/server/internal/entitlement"
	"monopsony/server/internal/store"
)

// Slots a loadout may fill.
var Slots = []string{"token", "board", "dice", "buildings", "cards"}

// Service is the cosmetics facade.
type Service struct {
	Store store.Store
	Ent   *entitlement.Resolver
	// Assets holds uploaded models/textures; nil when uploads are disabled.
	Assets *AssetStore
}

// Manifests returns id -> manifest for every enabled item, which is what
// the game client needs to render any loadout it sees at the table.
func (s *Service) Manifests(ctx context.Context) (map[string]json.RawMessage, error) {
	all, err := s.Store.ListCosmetics(ctx, false)
	if err != nil {
		return nil, err
	}
	out := make(map[string]json.RawMessage, len(all))
	for _, c := range all {
		out[c.ID] = c.Manifest
	}
	return out, nil
}

// Item is a catalog entry decorated with the viewer's relationship to it.
type Item struct {
	store.Cosmetic
	Owned    bool `json:"owned"`
	Equipped bool `json:"equipped"`
	// Locked explains why the viewer cannot equip it ("" = can).
	Locked string `json:"locked,omitempty"`
}

// Catalog returns enabled items with ownership for a user.
func (s *Service) Catalog(ctx context.Context, u *store.User) ([]Item, error) {
	all, err := s.Store.ListCosmetics(ctx, false)
	if err != nil {
		return nil, err
	}
	owned, _ := s.Store.OwnedCosmetics(ctx, u.ID)
	ownedSet := map[string]bool{}
	for _, id := range owned {
		ownedSet[id] = true
	}
	loadout, _ := s.Store.GetLoadout(ctx, u.ID)
	caps := s.Ent.Resolve(ctx, u)
	slotOK := map[string]bool{}
	for _, sl := range caps.CosmeticSlots {
		slotOK[sl] = true
	}
	out := make([]Item, 0, len(all))
	for _, c := range all {
		it := Item{Cosmetic: *c, Owned: ownedSet[c.ID] || c.PriceCents == 0 && c.TierRequired == "", Equipped: loadout[c.Slot] == c.ID}
		switch {
		case c.TierRequired != "" && caps.Tier != c.TierRequired && u.Role != "admin":
			it.Locked = "tier"
			it.Owned = false
		case !slotOK[c.Slot]:
			it.Locked = "slot"
		case !it.Owned:
			it.Locked = "purchase"
		}
		out = append(out, it)
	}
	return out, nil
}

// canUse reports whether a user may equip an item right now.
func (s *Service) canUse(ctx context.Context, u *store.User, c *store.Cosmetic) error {
	if !c.Enabled {
		return errors.New("item is not available")
	}
	caps := s.Ent.Resolve(ctx, u)
	if c.TierRequired != "" && caps.Tier != c.TierRequired && u.Role != "admin" {
		return fmt.Errorf("%s requires the %s plan", c.Name, c.TierRequired)
	}
	slotOK := false
	for _, sl := range caps.CosmeticSlots {
		if sl == c.Slot {
			slotOK = true
		}
	}
	if !slotOK {
		return fmt.Errorf("your plan cannot customise the %s slot", c.Slot)
	}
	if c.PriceCents > 0 {
		owned, _ := s.Store.OwnedCosmetics(ctx, u.ID)
		for _, id := range owned {
			if id == c.ID {
				return nil
			}
		}
		return fmt.Errorf("you do not own %s", c.Name)
	}
	return nil
}

// Equip sets a slot in the user's loadout (empty id clears it).
func (s *Service) Equip(ctx context.Context, u *store.User, slot, id string) (map[string]string, error) {
	lo, err := s.Store.GetLoadout(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	if id == "" {
		delete(lo, slot)
		return lo, s.Store.SetLoadout(ctx, u.ID, lo)
	}
	c, err := s.Store.GetCosmetic(ctx, id)
	if err != nil {
		return nil, errors.New("no such item")
	}
	if c.Slot != slot {
		return nil, fmt.Errorf("%s is a %s item, not %s", c.Name, c.Slot, slot)
	}
	if err := s.canUse(ctx, u, c); err != nil {
		return nil, err
	}
	lo[slot] = id
	return lo, s.Store.SetLoadout(ctx, u.ID, lo)
}

// EffectiveLoadout is what the room broadcasts: the stored loadout minus
// anything the user is no longer entitled to (e.g. after a downgrade).
func (s *Service) EffectiveLoadout(ctx context.Context, userID string) map[string]string {
	u, err := s.Store.GetUser(ctx, userID)
	if err != nil {
		return nil
	}
	lo, err := s.Store.GetLoadout(ctx, userID)
	if err != nil {
		return nil
	}
	out := map[string]string{}
	for slot, id := range lo {
		c, err := s.Store.GetCosmetic(ctx, id)
		if err != nil || s.canUse(ctx, u, c) != nil {
			continue
		}
		out[slot] = id
	}
	return out
}

// Seed inserts the built-in catalog when the store is empty.
func Seed(ctx context.Context, st store.Store) error {
	existing, err := st.ListCosmetics(ctx, true)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}
	m := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
	items := []store.Cosmetic{
		{ID: "token.pawn", Slot: "token", Name: "Pawn", Description: "The classic.", Manifest: m(map[string]any{"builtin": "token.pawn"}), SortOrder: 1},
		{ID: "token.cone", Slot: "token", Name: "Cone", Manifest: m(map[string]any{"builtin": "token.cone"}), SortOrder: 2},
		{ID: "token.sphere", Slot: "token", Name: "Marble", Manifest: m(map[string]any{"builtin": "token.sphere"}), SortOrder: 3},
		{ID: "token.cube", Slot: "token", Name: "Block", Manifest: m(map[string]any{"builtin": "token.cube"}), SortOrder: 4},
		{ID: "token.gem", Slot: "token", Name: "Gem", Description: "Faceted and shiny.", PriceCents: 199, Currency: "usd", Manifest: m(map[string]any{"builtin": "token.gem"}), SortOrder: 5},
		{ID: "token.ring", Slot: "token", Name: "Gold Ring", Description: "One ring to own them all.", PriceCents: 299, Currency: "usd", Manifest: m(map[string]any{"builtin": "token.ring"}), SortOrder: 6},
		// glTF-backed items: models ship with the client (packages/board-assets).
		{ID: "token.tophat", Slot: "token", Name: "Top Hat", Description: "A proper gentleman's token (glTF).", PriceCents: 249, Currency: "usd", Manifest: m(map[string]any{"model": map[string]any{"builtin": "tophat"}, "material": map[string]any{"metalness": 0.1, "roughness": 0.6}}), SortOrder: 7},
		{ID: "token.rocket", Slot: "token", Name: "Rocket", Description: "To the moon (glTF).", TierRequired: entitlement.TierPremium, Manifest: m(map[string]any{"model": map[string]any{"builtin": "rocket"}, "material": map[string]any{"metalness": 0.7, "roughness": 0.3}}), SortOrder: 8},
		{ID: "board.classic", Slot: "board", Name: "Classic Board", Manifest: m(map[string]any{"builtin": "board.classic"}), SortOrder: 10},
		{ID: "board.midnight", Slot: "board", Name: "Midnight Board", Description: "Dark table, neon accents.", TierRequired: entitlement.TierPremium, Manifest: m(map[string]any{"builtin": "board.midnight"}), SortOrder: 11},
		{ID: "dice.ivory", Slot: "dice", Name: "Ivory Dice", Manifest: m(map[string]any{"builtin": "dice.ivory"}), SortOrder: 20},
		{ID: "dice.onyx", Slot: "dice", Name: "Onyx Dice", PriceCents: 149, Currency: "usd", Manifest: m(map[string]any{"builtin": "dice.onyx"}), SortOrder: 21},
		{ID: "dice.ruby", Slot: "dice", Name: "Ruby Dice", TierRequired: entitlement.TierPremium, Manifest: m(map[string]any{"builtin": "dice.ruby"}), SortOrder: 22},
	}
	for i := range items {
		items[i].Enabled = true
		items[i].CreatedAt = time.Now()
		if items[i].Currency == "" {
			items[i].Currency = "usd"
		}
		if err := st.SaveCosmetic(ctx, &items[i]); err != nil {
			return err
		}
	}
	return nil
}
