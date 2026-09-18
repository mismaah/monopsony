package billing

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"monopsony/server/internal/ids"
)

// Fake is a development provider: checkout "pages" are links back into the
// server that fulfil the order immediately. Webhooks accept raw JSON Events
// so tests can drive every path without a network.
type Fake struct {
	BaseURL string // where /api/billing/fake/complete lives
	mu      sync.Mutex
	pending map[string]CheckoutRequest
	OnEvent func(ctx context.Context, ev Event) error // set by Mount
}

// NewFake returns a fake provider.
func NewFake(baseURL string) *Fake {
	return &Fake{BaseURL: baseURL, pending: map[string]CheckoutRequest{}}
}

func (f *Fake) Name() string { return "fake" }

func (f *Fake) CreateCheckout(_ context.Context, req CheckoutRequest) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	tok := ids.Token()
	f.pending[tok] = req
	return f.BaseURL + "/api/billing/fake/complete?token=" + tok, nil
}

func (f *Fake) CreatePortal(_ context.Context, _ string, returnURL string) (string, error) {
	return returnURL + "?portal=fake", nil
}

// ParseWebhook accepts {"events":[Event...]} with no signature check.
func (f *Fake) ParseWebhook(payload []byte, _ string) ([]Event, error) {
	var in struct {
		Events []Event `json:"events"`
	}
	if err := json.Unmarshal(payload, &in); err != nil {
		return nil, err
	}
	return in.Events, nil
}

// Complete fulfils a pending checkout and returns the event it produced.
func (f *Fake) Complete(token string) (Event, string, error) {
	f.mu.Lock()
	req, ok := f.pending[token]
	delete(f.pending, token)
	f.mu.Unlock()
	if !ok {
		return Event{}, "", errors.New("unknown or already completed checkout")
	}
	ev := Event{ID: "fake-" + ids.New(), UserID: req.UserID, CustomerID: "cus_fake_" + req.UserID, Currency: req.Currency}
	if req.Mode == ModeSubscription {
		ev.Type = EventSubscriptionActive
		ev.SubscriptionID = "sub_fake_" + ids.New()
		ev.Tier = req.Tier
		ev.PeriodEnd = time.Now().Add(30 * 24 * time.Hour)
	} else {
		ev.Type = EventPurchaseCompleted
		ev.SKU = req.SKU
		ev.AmountCents = req.AmountCent
		ev.ProviderRef = "pi_fake_" + ids.New()
	}
	return ev, req.SuccessURL, nil
}

// Mount adds the fake's completion endpoint.
func (f *Fake) Mount(mux *http.ServeMux, apply func(ctx context.Context, ev Event) error) {
	mux.HandleFunc("GET /api/billing/fake/complete", func(w http.ResponseWriter, r *http.Request) {
		ev, next, err := f.Complete(r.URL.Query().Get("token"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := apply(r.Context(), ev); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, next, http.StatusFound)
	})
}
