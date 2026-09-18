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
		// A grant or purchase counts as owning the item even when the plan
		// would not include it, so admins can hand premium items to anyone.
		granted := ownedSet[c.ID]
		tierOK := c.TierRequired == "" || caps.Tier == c.TierRequired || u.Role == "admin"
		it := Item{Cosmetic: *c, Owned: granted || c.PriceCents == 0 && tierOK, Equipped: loadout[c.Slot] == c.ID}
		switch {
		case !tierOK && !granted:
			it.Locked = "tier"
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
	granted := false
	if c.TierRequired != "" || c.PriceCents > 0 {
		owned, _ := s.Store.OwnedCosmetics(ctx, u.ID)
		for _, id := range owned {
			if id == c.ID {
				granted = true
			}
		}
	}
	// Granted items skip the tier check (see Catalog); the slot check is a
	// plan capability and still applies.
	if c.TierRequired != "" && caps.Tier != c.TierRequired && u.Role != "admin" && !granted {
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
	if c.PriceCents > 0 && !granted {
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

// Seed inserts any built-in catalog item that is missing from the store.
// Existing rows are left alone so admin edits survive restarts.
func Seed(ctx context.Context, st store.Store) error {
	existing, err := st.ListCosmetics(ctx, true)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, c := range existing {
		have[c.ID] = true
	}
	m := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
	model := func(name string, metalness, roughness float64) json.RawMessage {
		return m(map[string]any{"model": map[string]any{"builtin": name}, "material": map[string]any{"metalness": metalness, "roughness": roughness}})
	}
	items := []store.Cosmetic{
		// Free tokens: every plan gets these.
		{ID: "token.pawn", Slot: "token", Name: "Pawn", Description: "The classic.", Manifest: m(map[string]any{"builtin": "token.pawn"}), SortOrder: 1},
		{ID: "token.cone", Slot: "token", Name: "Cone", Manifest: m(map[string]any{"builtin": "token.cone"}), SortOrder: 2},
		{ID: "token.sphere", Slot: "token", Name: "Marble", Manifest: m(map[string]any{"builtin": "token.sphere"}), SortOrder: 3},
		{ID: "token.cube", Slot: "token", Name: "Block", Manifest: m(map[string]any{"builtin": "token.cube"}), SortOrder: 4},
		{ID: "token.pyramid", Slot: "token", Name: "Pyramid", Description: "Four sides, no nonsense.", Manifest: m(map[string]any{"builtin": "token.pyramid"}), SortOrder: 5},
		// One-off purchases.
		{ID: "token.gem", Slot: "token", Name: "Gem", Description: "Faceted and shiny.", PriceCents: 199, Currency: "usd", Manifest: m(map[string]any{"builtin": "token.gem"}), SortOrder: 6},
		{ID: "token.ring", Slot: "token", Name: "Gold Ring", Description: "One ring to own them all.", PriceCents: 299, Currency: "usd", Manifest: m(map[string]any{"builtin": "token.ring"}), SortOrder: 7},
		{ID: "token.tophat", Slot: "token", Name: "Top Hat", Description: "A proper gentleman's token (glTF).", PriceCents: 249, Currency: "usd", Manifest: model("tophat", 0.1, 0.6), SortOrder: 8},
		// Premium tokens: included with the premium plan, or granted by an
		// admin. Models ship with the client (packages/board-assets).
		{ID: "token.rocket", Slot: "token", Name: "Rocket", Description: "To the moon (glTF).", TierRequired: entitlement.TierPremium, Manifest: model("rocket", 0.7, 0.3), SortOrder: 9},
		{ID: "token.crown", Slot: "token", Name: "Crown", Description: "Rule the table (glTF).", TierRequired: entitlement.TierPremium, Manifest: model("crown", 0.45, 0.35), SortOrder: 10},
		{ID: "token.trophy", Slot: "token", Name: "Trophy", Description: "For the reigning champion (glTF).", TierRequired: entitlement.TierPremium, Manifest: model("trophy", 0.5, 0.3), SortOrder: 11},
		{ID: "token.car", Slot: "token", Name: "Roadster", Description: "Zero to Go in one roll (glTF).", TierRequired: entitlement.TierPremium, Manifest: model("car", 0.5, 0.4), SortOrder: 12},
		{ID: "token.king", Slot: "token", Name: "King", Description: "Check. Mate. (glTF)", TierRequired: entitlement.TierPremium, Manifest: model("king", 0.3, 0.5), SortOrder: 13},
		{ID: "board.classic", Slot: "board", Name: "Harbourside Board", Description: "Sand tiles on a deep-water table.", Manifest: m(map[string]any{"builtin": "board.classic"}), SortOrder: 20},
		{ID: "board.midnight", Slot: "board", Name: "Midnight Board", Description: "Dark table, neon accents.", TierRequired: entitlement.TierPremium, Manifest: m(map[string]any{"builtin": "board.midnight"}), SortOrder: 21},
		{ID: "dice.ivory", Slot: "dice", Name: "Ivory Dice", Manifest: m(map[string]any{"builtin": "dice.ivory"}), SortOrder: 30},
		{ID: "dice.onyx", Slot: "dice", Name: "Onyx Dice", PriceCents: 149, Currency: "usd", Manifest: m(map[string]any{"builtin": "dice.onyx"}), SortOrder: 31},
		{ID: "dice.ruby", Slot: "dice", Name: "Ruby Dice", TierRequired: entitlement.TierPremium, Manifest: m(map[string]any{"builtin": "dice.ruby"}), SortOrder: 32},
	}
	for i := range items {
		if have[items[i].ID] {
			continue
		}
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
