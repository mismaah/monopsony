// Package billing turns money into entitlements. A Provider (Stripe, or the
// in-process fake) produces Checkout/Portal URLs and parses webhooks into a
// small set of normalised Events; the Service applies those events to the
// store (tier changes, cosmetic grants) idempotently.
package billing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"monopsony/server/internal/entitlement"
	"monopsony/server/internal/ids"
	"monopsony/server/internal/store"
)

// CheckoutMode selects a subscription or a one-off payment.
type CheckoutMode string

const (
	ModeSubscription CheckoutMode = "subscription"
	ModePayment      CheckoutMode = "payment"
)

// CheckoutRequest describes what to sell.
type CheckoutRequest struct {
	UserID     string
	Email      string
	Mode       CheckoutMode
	PriceID    string // provider price id (subscriptions)
	Name       string // one-off item name
	AmountCent int    // one-off price
	Currency   string
	SKU        string // cosmetic id for one-off purchases
	Tier       string // tier granted by a subscription
	SuccessURL string
	CancelURL  string
}

// EventType is the normalised webhook event kind.
type EventType string

const (
	EventSubscriptionActive   EventType = "subscription_active"
	EventSubscriptionInactive EventType = "subscription_inactive"
	EventPurchaseCompleted    EventType = "purchase_completed"
)

// Event is a provider-agnostic billing event.
type Event struct {
	ID             string
	Type           EventType
	UserID         string
	CustomerID     string
	SubscriptionID string
	Tier           string
	PeriodEnd      time.Time
	SKU            string
	ProviderRef    string
	AmountCents    int
	Currency       string
}

// Provider is implemented per payment processor.
type Provider interface {
	Name() string
	CreateCheckout(ctx context.Context, req CheckoutRequest) (url string, err error)
	CreatePortal(ctx context.Context, customerID, returnURL string) (url string, err error)
	// ParseWebhook verifies the signature and normalises the payload.
	ParseWebhook(payload []byte, signature string) ([]Event, error)
}

var ErrNoCustomer = errors.New("no billing customer for user")

// Service applies billing events.
type Service struct {
	Provider Provider
	Store    store.Store
	Ent      *entitlement.Resolver
	Log      *slog.Logger
	BaseURL  string // public origin for success/cancel redirects
}

// StartSubscription returns a Checkout URL for the premium plan.
func (s *Service) StartSubscription(ctx context.Context, u *store.User, tier string) (string, error) {
	priceID := ""
	for _, p := range s.Ent.Plans() {
		if p.Tier == tier {
			priceID = p.PriceID
		}
	}
	if tier == entitlement.TierFree {
		return "", errors.New("cannot subscribe to the free tier")
	}
	return s.Provider.CreateCheckout(ctx, CheckoutRequest{
		UserID: u.ID, Email: u.Email, Mode: ModeSubscription, PriceID: priceID, Tier: tier,
		SuccessURL: s.BaseURL + "/account?upgraded=1", CancelURL: s.BaseURL + "/account",
	})
}

// BuyCosmetic returns a Checkout URL for a one-off cosmetic purchase.
func (s *Service) BuyCosmetic(ctx context.Context, u *store.User, c *store.Cosmetic) (string, error) {
	if c.PriceCents <= 0 {
		return "", errors.New("item is free")
	}
	return s.Provider.CreateCheckout(ctx, CheckoutRequest{
		UserID: u.ID, Email: u.Email, Mode: ModePayment, Name: c.Name, AmountCent: c.PriceCents, Currency: c.Currency, SKU: c.ID,
		SuccessURL: s.BaseURL + "/shop?purchased=" + c.ID, CancelURL: s.BaseURL + "/shop",
	})
}

// Portal returns the provider's self-service portal for the user.
func (s *Service) Portal(ctx context.Context, u *store.User) (string, error) {
	sub, err := s.Store.GetSubscription(ctx, u.ID)
	if err != nil || sub.CustomerID == "" {
		return "", ErrNoCustomer
	}
	return s.Provider.CreatePortal(ctx, sub.CustomerID, s.BaseURL+"/account")
}

// HandleWebhook verifies, deduplicates and applies provider events.
func (s *Service) HandleWebhook(ctx context.Context, payload []byte, signature string) error {
	events, err := s.Provider.ParseWebhook(payload, signature)
	if err != nil {
		return err
	}
	for _, ev := range events {
		fresh, err := s.Store.MarkWebhook(ctx, s.Provider.Name(), ev.ID)
		if err != nil {
			return err
		}
		if !fresh {
			continue
		}
		if err := s.Apply(ctx, ev); err != nil {
			s.Log.Error("apply billing event", "id", ev.ID, "type", ev.Type, "err", err)
			return err
		}
	}
	return nil
}

// Apply updates the store for one event.
func (s *Service) Apply(ctx context.Context, ev Event) error {
	if ev.UserID == "" {
		return fmt.Errorf("event %s has no user", ev.ID)
	}
	u, err := s.Store.GetUser(ctx, ev.UserID)
	if err != nil {
		return fmt.Errorf("event %s: user %s: %w", ev.ID, ev.UserID, err)
	}
	switch ev.Type {
	case EventSubscriptionActive:
		tier := ev.Tier
		if tier == "" {
			tier = entitlement.TierPremium
		}
		if err := s.Store.SaveSubscription(ctx, &store.Subscription{
			UserID: u.ID, Provider: s.Provider.Name(), CustomerID: ev.CustomerID, SubscriptionID: ev.SubscriptionID,
			Status: "active", Tier: tier, PeriodEnd: ev.PeriodEnd,
		}); err != nil {
			return err
		}
		u.Tier = tier
		return s.Store.UpdateUser(ctx, u)
	case EventSubscriptionInactive:
		sub, err := s.Store.GetSubscription(ctx, u.ID)
		if err == nil {
			sub.Status = "canceled"
			sub.Tier = entitlement.TierFree
			_ = s.Store.SaveSubscription(ctx, sub)
		}
		if u.Role != "admin" {
			u.Tier = entitlement.TierFree
		}
		return s.Store.UpdateUser(ctx, u)
	case EventPurchaseCompleted:
		if err := s.Store.SavePurchase(ctx, &store.Purchase{
			ID: ids.New(), UserID: u.ID, Provider: s.Provider.Name(), ProviderRef: ev.ProviderRef, SKU: ev.SKU,
			AmountCents: ev.AmountCents, Currency: ev.Currency, CreatedAt: time.Now(),
		}); err != nil {
			return err
		}
		return s.Store.GrantCosmetic(ctx, u.ID, ev.SKU)
	}
	return fmt.Errorf("unknown event type %q", ev.Type)
}
