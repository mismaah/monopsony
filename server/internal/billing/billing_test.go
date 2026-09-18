package billing

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"monopsony/server/internal/entitlement"
	"monopsony/server/internal/store"
)

func setup(t *testing.T, p Provider) (*Service, *store.User) {
	t.Helper()
	st := store.NewMem()
	u := &store.User{ID: "u1", Name: "Buyer", Role: "player", Tier: "free", CreatedAt: time.Now()}
	_ = st.CreateUser(context.Background(), u)
	_ = st.SaveCosmetic(context.Background(), &store.Cosmetic{ID: "token.gold", Slot: "token", Name: "Gold", PriceCents: 299, Currency: "usd", Enabled: true})
	ent := entitlement.NewResolver()
	ent.SetPlan(entitlement.Plan{Tier: "premium", PriceID: "price_123", Caps: entitlement.DefaultPlans()["premium"].Caps})
	return &Service{Provider: p, Store: st, Ent: ent, Log: slog.Default(), BaseURL: "http://app.test"}, u
}

func TestFakeSubscriptionAndPurchase(t *testing.T) {
	fake := NewFake("http://api.test")
	svc, u := setup(t, fake)
	ctx := context.Background()

	url, err := svc.StartSubscription(ctx, u, "premium")
	if err != nil {
		t.Fatal(err)
	}
	tok := url[len("http://api.test/api/billing/fake/complete?token="):]
	ev, next, err := fake.Complete(tok)
	if err != nil || next != "http://app.test/account?upgraded=1" {
		t.Fatalf("complete: %v %s", err, next)
	}
	if err := svc.Apply(ctx, ev); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Store.GetUser(ctx, u.ID)
	if got.Tier != "premium" {
		t.Fatalf("tier=%s", got.Tier)
	}
	if _, _, err := fake.Complete(tok); err == nil {
		t.Fatal("token should be single-use")
	}

	c, _ := svc.Store.GetCosmetic(ctx, "token.gold")
	url, err = svc.BuyCosmetic(ctx, u, c)
	if err != nil {
		t.Fatal(err)
	}
	ev, _, _ = fake.Complete(url[len("http://api.test/api/billing/fake/complete?token="):])
	if err := svc.Apply(ctx, ev); err != nil {
		t.Fatal(err)
	}
	owned, _ := svc.Store.OwnedCosmetics(ctx, u.ID)
	if len(owned) != 1 || owned[0] != "token.gold" {
		t.Fatalf("owned=%v", owned)
	}

	// Webhook path is idempotent per event id.
	body, _ := json.Marshal(map[string]any{"events": []Event{{ID: "evt-1", Type: EventSubscriptionInactive, UserID: u.ID}}})
	if err := svc.HandleWebhook(ctx, body, ""); err != nil {
		t.Fatal(err)
	}
	got, _ = svc.Store.GetUser(ctx, u.ID)
	if got.Tier != "free" {
		t.Fatalf("tier after cancel=%s", got.Tier)
	}
	// Re-upgrade directly, then replay the old cancel event: must be ignored.
	_ = svc.Apply(ctx, Event{ID: "x", Type: EventSubscriptionActive, UserID: u.ID, Tier: "premium"})
	if err := svc.HandleWebhook(ctx, body, ""); err != nil {
		t.Fatal(err)
	}
	if got, _ = svc.Store.GetUser(ctx, u.ID); got.Tier != "premium" {
		t.Fatal("replayed webhook must not be applied twice")
	}
}

func TestStripeWebhookSignatureAndMapping(t *testing.T) {
	s := NewStripe("sk_test", "whsec_test")
	now := time.Now()
	s.Now = func() time.Time { return now }
	payload := []byte(`{"id":"evt_1","type":"checkout.session.completed","data":{"object":{"id":"cs_1","mode":"subscription","customer":"cus_1","subscription":"sub_1","client_reference_id":"u1","metadata":{"user_id":"u1","tier":"premium"}}}}`)

	if _, err := s.ParseWebhook(payload, "t=1,v1=deadbeef"); err == nil {
		t.Fatal("bad signature accepted")
	}
	stale := s.Sign(payload, now.Add(-time.Hour))
	if _, err := s.ParseWebhook(payload, stale); err == nil {
		t.Fatal("stale signature accepted")
	}
	evs, err := s.ParseWebhook(payload, s.Sign(payload, now))
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || evs[0].Type != EventSubscriptionActive || evs[0].UserID != "u1" || evs[0].SubscriptionID != "sub_1" || evs[0].Tier != "premium" {
		t.Fatalf("mapped: %+v", evs)
	}

	pay := []byte(`{"id":"evt_2","type":"checkout.session.completed","data":{"object":{"id":"cs_2","mode":"payment","customer":"cus_1","payment_intent":"pi_1","amount_total":299,"currency":"usd","metadata":{"user_id":"u1","sku":"token.gold"}}}}`)
	evs, _ = s.ParseWebhook(pay, s.Sign(pay, now))
	if len(evs) != 1 || evs[0].Type != EventPurchaseCompleted || evs[0].SKU != "token.gold" || evs[0].AmountCents != 299 {
		t.Fatalf("purchase mapped: %+v", evs)
	}

	del := []byte(`{"id":"evt_3","type":"customer.subscription.deleted","data":{"object":{"id":"sub_1","customer":"cus_1","status":"canceled","current_period_end":1700000000,"metadata":{"user_id":"u1"}}}}`)
	evs, _ = s.ParseWebhook(del, s.Sign(del, now))
	if len(evs) != 1 || evs[0].Type != EventSubscriptionInactive {
		t.Fatalf("cancel mapped: %+v", evs)
	}

	other := []byte(`{"id":"evt_4","type":"invoice.created","data":{"object":{}}}`)
	if evs, err := s.ParseWebhook(other, s.Sign(other, now)); err != nil || len(evs) != 0 {
		t.Fatalf("unrelated event: %v %v", err, evs)
	}
}
