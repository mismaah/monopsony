package entitlement

import (
	"context"
	"encoding/json"
	"testing"

	"monopsony/server/internal/store"
)

// A plan row saved before cosmeticSlots existed must not lock every slot.
func TestLoadKeepsDefaultsForMissingFields(t *testing.T) {
	ctx := context.Background()
	st := store.NewMem()
	old := json.RawMessage(`{"maxPlayersPerRoom":6,"privateRooms":true,"houseRules":true,"showAds":false,"statsHistoryDays":0,"maxConcurrentGames":5}`)
	if err := st.SavePlan(ctx, &store.PlanRecord{Tier: TierPremium, Caps: old}); err != nil {
		t.Fatal(err)
	}
	// An explicitly empty list is a real choice and must survive.
	if err := st.SavePlan(ctx, &store.PlanRecord{Tier: TierFree, Caps: json.RawMessage(`{"cosmeticSlots":[]}`)}); err != nil {
		t.Fatal(err)
	}
	r := NewResolver()
	if err := r.Load(ctx, st); err != nil {
		t.Fatal(err)
	}
	prem := r.Resolve(ctx, &store.User{Tier: TierPremium})
	if prem.MaxPlayersPerRoom != 6 {
		t.Fatalf("stored field should override the default: %+v", prem)
	}
	if len(prem.CosmeticSlots) != len(DefaultPlans()[TierPremium].Caps.CosmeticSlots) {
		t.Fatalf("missing cosmeticSlots should fall back to the default, got %v", prem.CosmeticSlots)
	}
	free := r.Resolve(ctx, &store.User{Tier: TierFree})
	if len(free.CosmeticSlots) != 0 || free.MaxPlayersPerRoom != 4 {
		t.Fatalf("explicit empty list should stay empty, other fields default: %+v", free)
	}
}
