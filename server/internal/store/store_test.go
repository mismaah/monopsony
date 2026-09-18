package store

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"monopsony/server/internal/game"
)

// TestConformance runs the same scenario against every backend. Postgres is
// exercised when MONOPSONY_TEST_DATABASE_URL is set (see deploy/docker-compose.yml).
func TestConformance(t *testing.T) {
	t.Run("mem", func(t *testing.T) { conformance(t, NewMem()) })
	if url := os.Getenv("MONOPSONY_TEST_DATABASE_URL"); url != "" {
		pg, err := OpenPG(context.Background(), url)
		if err != nil {
			t.Fatal(err)
		}
		defer pg.Close()
		t.Run("postgres", func(t *testing.T) { conformance(t, pg) })
	}
}

func conformance(t *testing.T, st Store) {
	ctx := context.Background()
	suffix := time.Now().Format("150405.000000")

	// Users
	u := &User{ID: "u-" + suffix, Email: "user" + suffix + "@example.com", Name: "Test", Role: "player", Tier: "free", CreatedAt: time.Now()}
	if err := st.CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateUser(ctx, &User{ID: "u2-" + suffix, Email: u.Email, Name: "Dup", Role: "player", Tier: "free", CreatedAt: time.Now()}); err != ErrConflict {
		t.Fatalf("expected conflict on duplicate email, got %v", err)
	}
	got, err := st.GetUserByEmail(ctx, "USER"+suffix+"@EXAMPLE.COM")
	if err != nil || got.ID != u.ID {
		t.Fatalf("email lookup is not case-insensitive: %v %v", err, got)
	}
	u.Tier = "premium"
	if err := st.UpdateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	if got, _ = st.GetUser(ctx, u.ID); got.Tier != "premium" {
		t.Fatal("update not applied")
	}
	if _, err := st.GetUser(ctx, "nope"); err != ErrNotFound {
		t.Fatalf("expected not found, got %v", err)
	}

	// Tokens
	tok := RefreshToken{Hash: "h-" + suffix, UserID: u.ID, ExpiresAt: time.Now().Add(time.Hour)}
	if err := st.PutRefreshToken(ctx, tok); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetRefreshToken(ctx, tok.Hash); err != nil {
		t.Fatal(err)
	}
	_ = st.PutRefreshToken(ctx, RefreshToken{Hash: "old-" + suffix, UserID: u.ID, ExpiresAt: time.Now().Add(-time.Hour)})
	if _, err := st.GetRefreshToken(ctx, "old-"+suffix); err != ErrNotFound {
		t.Fatal("expired token should not be returned")
	}
	_ = st.DeleteRefreshToken(ctx, tok.Hash)
	if _, err := st.GetRefreshToken(ctx, tok.Hash); err != ErrNotFound {
		t.Fatal("deleted token should not be returned")
	}

	// Configs
	cfg := game.ClassicConfig()
	cfg.ID = "cfg-" + suffix
	if err := st.SaveConfig(ctx, &ConfigRecord{ID: cfg.ID, Version: 1, Name: "v1", Config: cfg, Published: true, CreatedBy: u.ID, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	pub, err := st.GetPublishedConfig(ctx)
	if err != nil || pub.ID != cfg.ID || len(pub.Config.Spaces) != 40 {
		t.Fatalf("published config: %v %v", err, pub)
	}

	// Games
	state, _, _ := game.NewGame(cfg, []game.Seat{{ID: u.ID, Name: "Test"}, {ID: "bot-1", Name: "Bot", IsBot: true}}, game.NewSeededRNG(1))
	snap, _ := json.Marshal(state)
	g := &GameRecord{
		ID: "g-" + suffix, Name: "test", Status: StatusInProgress, HostID: u.ID, Visibility: "private", InviteCode: "AB12-CD34",
		MaxPlayers: 4, TurnSeconds: 60, ConfigID: cfg.ID, Config: cfg,
		Seats: []SeatRecord{{PlayerID: u.ID, UserID: u.ID, Name: "Test"}, {PlayerID: "bot-1", Name: "Bot", IsBot: true, BotProfile: "balanced"}},
		State: snap, Seq: state.Seq, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := st.SaveGame(ctx, g); err != nil {
		t.Fatal(err)
	}
	g.Status = StatusFinished
	g.WinnerID = u.ID
	if err := st.SaveGame(ctx, g); err != nil { // upsert
		t.Fatal(err)
	}
	back, err := st.GetGame(ctx, g.ID)
	if err != nil || back.Status != StatusFinished || back.WinnerID != u.ID || back.InviteCode != "AB12-CD34" || len(back.Seats) != 2 || back.Config.Spaces[39].Name != "Boardwalk" {
		t.Fatalf("game round-trip: %v %+v", err, back)
	}
	var restored game.State
	if err := json.Unmarshal(back.State, &restored); err != nil || restored.Players[0].Cash != 1500 {
		t.Fatalf("state round-trip: %v", err)
	}
	mine, err := st.ListGamesForUser(ctx, u.ID, 10)
	if err != nil || len(mine) != 1 || mine[0].ID != g.ID {
		t.Fatalf("games for user: %v %d", err, len(mine))
	}
	none, _ := st.ListGamesForUser(ctx, "nobody", 10)
	if len(none) != 0 {
		t.Fatal("expected no games for unknown user")
	}

	// Events
	var envs []game.EventEnvelope
	for i := 1; i <= 3; i++ {
		env, _ := game.Wrap(i, game.PhaseChanged{Phase: game.PhasePreRoll})
		envs = append(envs, env)
	}
	if err := st.AppendEvents(ctx, g.ID, envs); err != nil {
		t.Fatal(err)
	}
	if err := st.AppendEvents(ctx, g.ID, envs[:1]); err != nil { // duplicate seq is ignored
		t.Fatal(err)
	}
	evs, err := st.GetEvents(ctx, g.ID, 1)
	if err != nil || len(evs) != 2 || evs[0].Seq != 2 || evs[1].Type != "PhaseChanged" {
		t.Fatalf("events after seq 1: %v %+v", err, evs)
	}
}

func TestMonetizationConformance(t *testing.T) {
	t.Run("mem", func(t *testing.T) { monetization(t, NewMem()) })
	if url := os.Getenv("MONOPSONY_TEST_DATABASE_URL"); url != "" {
		pg, err := OpenPG(context.Background(), url)
		if err != nil {
			t.Fatal(err)
		}
		defer pg.Close()
		t.Run("postgres", func(t *testing.T) { monetization(t, pg) })
	}
}

func monetization(t *testing.T, st Store) {
	ctx := context.Background()
	suffix := time.Now().Format("150405.000000")
	u := &User{ID: "mu-" + suffix, Name: "Buyer", Role: "player", Tier: "free", CreatedAt: time.Now()}
	if err := st.CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}

	// Subscriptions upsert
	if _, err := st.GetSubscription(ctx, u.ID); err != ErrNotFound {
		t.Fatalf("expected not found, got %v", err)
	}
	sub := &Subscription{UserID: u.ID, Provider: "fake", CustomerID: "c1", SubscriptionID: "s1", Status: "active", Tier: "premium", PeriodEnd: time.Now().Add(720 * time.Hour)}
	if err := st.SaveSubscription(ctx, sub); err != nil {
		t.Fatal(err)
	}
	sub.Status = "canceled"
	if err := st.SaveSubscription(ctx, sub); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.GetSubscription(ctx, u.ID); got.Status != "canceled" || got.PeriodEnd.IsZero() {
		t.Fatalf("subscription: %+v", got)
	}

	// Webhook idempotency
	if first, _ := st.MarkWebhook(ctx, "fake", "evt-"+suffix); !first {
		t.Fatal("first delivery should be new")
	}
	if again, _ := st.MarkWebhook(ctx, "fake", "evt-"+suffix); again {
		t.Fatal("duplicate delivery should be detected")
	}

	// Cosmetics, ownership, loadout
	c := &Cosmetic{ID: "cos-" + suffix, Slot: "token", Name: "Gold Pawn", PriceCents: 299, Currency: "usd", Manifest: []byte(`{"shape":"pawn","color":"#ffd700"}`), Enabled: true, CreatedAt: time.Now()}
	if err := st.SaveCosmetic(ctx, c); err != nil {
		t.Fatal(err)
	}
	c.Enabled = false
	if err := st.SaveCosmetic(ctx, c); err != nil { // upsert
		t.Fatal(err)
	}
	all, _ := st.ListCosmetics(ctx, true)
	enabled, _ := st.ListCosmetics(ctx, false)
	found := false
	for _, x := range all {
		if x.ID == c.ID {
			found = true
		}
	}
	for _, x := range enabled {
		if x.ID == c.ID {
			t.Fatal("disabled cosmetic listed as enabled")
		}
	}
	if !found {
		t.Fatal("cosmetic not listed")
	}
	if err := st.GrantCosmetic(ctx, u.ID, "nope"); err != ErrNotFound {
		t.Fatalf("granting unknown cosmetic: %v", err)
	}
	if err := st.GrantCosmetic(ctx, u.ID, c.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.GrantCosmetic(ctx, u.ID, c.ID); err != nil { // idempotent
		t.Fatal(err)
	}
	owned, _ := st.OwnedCosmetics(ctx, u.ID)
	if len(owned) != 1 || owned[0] != c.ID {
		t.Fatalf("owned: %v", owned)
	}
	if lo, _ := st.GetLoadout(ctx, u.ID); len(lo) != 0 {
		t.Fatal("expected empty loadout")
	}
	if err := st.SetLoadout(ctx, u.ID, map[string]string{"token": c.ID}); err != nil {
		t.Fatal(err)
	}
	if lo, _ := st.GetLoadout(ctx, u.ID); lo["token"] != c.ID {
		t.Fatalf("loadout: %v", lo)
	}

	// Purchases
	if err := st.SavePurchase(ctx, &Purchase{ID: "p-" + suffix, UserID: u.ID, Provider: "fake", SKU: c.ID, AmountCents: 299, Currency: "usd", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if ps, _ := st.ListPurchases(ctx, u.ID); len(ps) != 1 || ps[0].SKU != c.ID {
		t.Fatalf("purchases: %v", ps)
	}

	// Plans
	if err := st.SavePlan(ctx, &PlanRecord{Tier: "premium", Caps: []byte(`{"maxPlayersPerRoom":8}`), PriceID: "price_x"}); err != nil {
		t.Fatal(err)
	}
	plans, _ := st.ListPlans(ctx)
	ok := false
	for _, p := range plans {
		if p.Tier == "premium" && p.PriceID == "price_x" {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("plans: %v", plans)
	}

	// Users search + audit
	users, _ := st.ListUsers(ctx, "buyer", 10)
	ok = false
	for _, x := range users {
		if x.ID == u.ID {
			ok = true
		}
	}
	if !ok {
		t.Fatal("user search failed")
	}
	if err := st.Audit(ctx, &AuditEntry{ID: "a-" + suffix, AdminID: "admin", Action: "user.tier", Target: u.ID, Payload: []byte(`{"tier":"premium"}`), At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if entries, _ := st.ListAudit(ctx, 5); len(entries) == 0 || entries[0].Action == "" {
		t.Fatal("audit log empty")
	}
}
