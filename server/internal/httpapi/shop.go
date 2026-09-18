package httpapi

import (
	"errors"
	"io"
	"net/http"

	"monopsony/server/internal/billing"
	"monopsony/server/internal/cosmetics"
	"monopsony/server/internal/store"
)

// ShopAPI serves the catalog, loadouts, subscriptions and billing webhooks.
type ShopAPI struct {
	*API
	Billing   *billing.Service
	Cosmetics *cosmetics.Service
}

// Register mounts the shop routes.
func (s *ShopAPI) Register(mux *http.ServeMux) {
	mux.Handle("GET /api/shop/catalog", s.requireUser(s.catalog))
	mux.Handle("GET /api/cosmetics/manifests", s.requireUser(s.manifests))
	if s.Cosmetics.Assets != nil {
		mux.Handle("GET "+s.Cosmetics.Assets.BaseURL+"/{id}", s.Cosmetics.Assets.Handler())
	}
	mux.Handle("POST /api/shop/checkout", s.requireUser(s.checkout))
	mux.Handle("PUT /api/me/loadout", s.requireUser(s.equip))
	mux.Handle("GET /api/me/billing", s.requireUser(s.billingInfo))
	mux.Handle("POST /api/billing/subscribe", s.requireUser(s.subscribe))
	mux.Handle("POST /api/billing/portal", s.requireUser(s.portal))
	mux.HandleFunc("POST /api/billing/webhook", s.webhook)
	if f, ok := s.Billing.Provider.(*billing.Fake); ok {
		f.Mount(mux, s.Billing.Apply)
	}
}

func (s *ShopAPI) catalog(w http.ResponseWriter, r *http.Request) {
	items, err := s.Cosmetics.Catalog(r.Context(), userFrom(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	loadout, _ := s.Store.GetLoadout(r.Context(), userFrom(r).ID)
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "loadout": loadout, "slots": cosmetics.Slots})
}

func (s *ShopAPI) manifests(w http.ResponseWriter, r *http.Request) {
	items, err := s.Cosmetics.Manifests(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=60")
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *ShopAPI) checkout(w http.ResponseWriter, r *http.Request) {
	var in struct{ ItemID string }
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "malformed body")
		return
	}
	c, err := s.Store.GetCosmetic(r.Context(), in.ItemID)
	if err != nil || !c.Enabled {
		writeErr(w, http.StatusNotFound, "not_found", "no such item")
		return
	}
	u := userFrom(r)
	if u.Guest {
		writeErr(w, http.StatusForbidden, "guest", "create an account to make purchases")
		return
	}
	url, err := s.Billing.BuyCosmetic(r.Context(), u, c)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"url": url})
}

func (s *ShopAPI) equip(w http.ResponseWriter, r *http.Request) {
	var in struct{ Slot, ItemID string }
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "malformed body")
		return
	}
	lo, err := s.Cosmetics.Equip(r.Context(), userFrom(r), in.Slot, in.ItemID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"loadout": lo})
}

func (s *ShopAPI) billingInfo(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	sub, err := s.Store.GetSubscription(r.Context(), u.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	purchases, _ := s.Store.ListPurchases(r.Context(), u.ID)
	if purchases == nil {
		purchases = []*store.Purchase{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"subscription": sub, "purchases": purchases, "plans": s.Ent.Plans(), "provider": s.Billing.Provider.Name(),
	})
}

func (s *ShopAPI) subscribe(w http.ResponseWriter, r *http.Request) {
	var in struct{ Tier string }
	_ = decode(r, &in)
	if in.Tier == "" {
		in.Tier = "premium"
	}
	u := userFrom(r)
	if u.Guest {
		writeErr(w, http.StatusForbidden, "guest", "create an account to subscribe")
		return
	}
	url, err := s.Billing.StartSubscription(r.Context(), u, in.Tier)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"url": url})
}

func (s *ShopAPI) portal(w http.ResponseWriter, r *http.Request) {
	url, err := s.Billing.Portal(r.Context(), userFrom(r))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"url": url})
}

func (s *ShopAPI) webhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "unreadable body")
		return
	}
	if err := s.Billing.HandleWebhook(r.Context(), body, r.Header.Get("Stripe-Signature")); err != nil {
		s.Log.Warn("webhook rejected", "err", err)
		writeErr(w, http.StatusBadRequest, "webhook", err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}
