package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Stripe talks to the Stripe REST API directly (Checkout Sessions, Billing
// Portal, signed webhooks). The surface is small enough that the official
// SDK is not worth its size.
type Stripe struct {
	SecretKey     string
	WebhookSecret string
	HTTP          *http.Client
	Now           func() time.Time
	// Tolerance for webhook timestamps (replay protection).
	Tolerance time.Duration
	apiBase   string
}

// NewStripe creates a provider.
func NewStripe(secretKey, webhookSecret string) *Stripe {
	return &Stripe{SecretKey: secretKey, WebhookSecret: webhookSecret, HTTP: &http.Client{Timeout: 15 * time.Second}, Now: time.Now, Tolerance: 5 * time.Minute, apiBase: "https://api.stripe.com"}
}

func (s *Stripe) Name() string { return "stripe" }

func (s *Stripe) post(ctx context.Context, path string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.apiBase+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.SetBasicAuth(s.SecretKey, "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := s.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 300 {
		var e struct {
			Error struct{ Message string } `json:"error"`
		}
		_ = json.Unmarshal(body, &e)
		return fmt.Errorf("stripe %s: %s (%d)", path, e.Error.Message, res.StatusCode)
	}
	return json.Unmarshal(body, out)
}

func (s *Stripe) CreateCheckout(ctx context.Context, req CheckoutRequest) (string, error) {
	form := url.Values{}
	form.Set("mode", string(req.Mode))
	form.Set("success_url", req.SuccessURL)
	form.Set("cancel_url", req.CancelURL)
	form.Set("client_reference_id", req.UserID)
	form.Set("metadata[user_id]", req.UserID)
	if req.Email != "" {
		form.Set("customer_email", req.Email)
	}
	switch req.Mode {
	case ModeSubscription:
		if req.PriceID == "" {
			return "", errors.New("no price id configured for this plan")
		}
		form.Set("line_items[0][price]", req.PriceID)
		form.Set("line_items[0][quantity]", "1")
		form.Set("metadata[tier]", req.Tier)
		form.Set("subscription_data[metadata][user_id]", req.UserID)
		form.Set("subscription_data[metadata][tier]", req.Tier)
	case ModePayment:
		cur := req.Currency
		if cur == "" {
			cur = "usd"
		}
		form.Set("line_items[0][price_data][currency]", cur)
		form.Set("line_items[0][price_data][unit_amount]", strconv.Itoa(req.AmountCent))
		form.Set("line_items[0][price_data][product_data][name]", req.Name)
		form.Set("line_items[0][quantity]", "1")
		form.Set("metadata[sku]", req.SKU)
	}
	var out struct {
		URL string `json:"url"`
	}
	if err := s.post(ctx, "/v1/checkout/sessions", form, &out); err != nil {
		return "", err
	}
	return out.URL, nil
}

func (s *Stripe) CreatePortal(ctx context.Context, customerID, returnURL string) (string, error) {
	form := url.Values{}
	form.Set("customer", customerID)
	form.Set("return_url", returnURL)
	var out struct {
		URL string `json:"url"`
	}
	if err := s.post(ctx, "/v1/billing_portal/sessions", form, &out); err != nil {
		return "", err
	}
	return out.URL, nil
}

// VerifySignature checks a Stripe-Signature header against the payload.
func (s *Stripe) VerifySignature(payload []byte, header string) error {
	var ts string
	var sigs []string
	for _, part := range strings.Split(header, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			ts = kv[1]
		case "v1":
			sigs = append(sigs, kv[1])
		}
	}
	if ts == "" || len(sigs) == 0 {
		return errors.New("malformed Stripe-Signature header")
	}
	t, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return errors.New("bad timestamp in signature")
	}
	if s.Tolerance > 0 {
		if d := s.Now().Sub(time.Unix(t, 0)); d > s.Tolerance || d < -s.Tolerance {
			return errors.New("webhook timestamp outside tolerance")
		}
	}
	mac := hmac.New(sha256.New, []byte(s.WebhookSecret))
	mac.Write([]byte(ts))
	mac.Write([]byte("."))
	mac.Write(payload)
	want := hex.EncodeToString(mac.Sum(nil))
	for _, sig := range sigs {
		if hmac.Equal([]byte(sig), []byte(want)) {
			return nil
		}
	}
	return errors.New("signature mismatch")
}

// Sign produces a Stripe-Signature header (used by tests and tooling).
func (s *Stripe) Sign(payload []byte, at time.Time) string {
	ts := strconv.FormatInt(at.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(s.WebhookSecret))
	mac.Write([]byte(ts + "."))
	mac.Write(payload)
	return "t=" + ts + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

type stripeEvent struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Data struct {
		Object json.RawMessage `json:"object"`
	} `json:"data"`
}

// ParseWebhook maps the Stripe events we care about onto billing Events:
//   - checkout.session.completed (subscription → active; payment → purchase)
//   - customer.subscription.updated / deleted (status changes)
func (s *Stripe) ParseWebhook(payload []byte, signature string) ([]Event, error) {
	if err := s.VerifySignature(payload, signature); err != nil {
		return nil, err
	}
	var ev stripeEvent
	if err := json.Unmarshal(payload, &ev); err != nil {
		return nil, err
	}
	switch ev.Type {
	case "checkout.session.completed":
		var cs struct {
			ID            string            `json:"id"`
			Mode          string            `json:"mode"`
			Customer      string            `json:"customer"`
			Subscription  string            `json:"subscription"`
			PaymentIntent string            `json:"payment_intent"`
			AmountTotal   int               `json:"amount_total"`
			Currency      string            `json:"currency"`
			ClientRef     string            `json:"client_reference_id"`
			Metadata      map[string]string `json:"metadata"`
		}
		if err := json.Unmarshal(ev.Data.Object, &cs); err != nil {
			return nil, err
		}
		userID := cs.Metadata["user_id"]
		if userID == "" {
			userID = cs.ClientRef
		}
		if cs.Mode == "subscription" {
			return []Event{{ID: ev.ID, Type: EventSubscriptionActive, UserID: userID, CustomerID: cs.Customer, SubscriptionID: cs.Subscription, Tier: cs.Metadata["tier"]}}, nil
		}
		return []Event{{ID: ev.ID, Type: EventPurchaseCompleted, UserID: userID, CustomerID: cs.Customer, SKU: cs.Metadata["sku"], ProviderRef: cs.PaymentIntent, AmountCents: cs.AmountTotal, Currency: cs.Currency}}, nil
	case "customer.subscription.updated", "customer.subscription.deleted":
		var sub struct {
			ID               string            `json:"id"`
			Customer         string            `json:"customer"`
			Status           string            `json:"status"`
			CurrentPeriodEnd int64             `json:"current_period_end"`
			Metadata         map[string]string `json:"metadata"`
		}
		if err := json.Unmarshal(ev.Data.Object, &sub); err != nil {
			return nil, err
		}
		e := Event{ID: ev.ID, UserID: sub.Metadata["user_id"], CustomerID: sub.Customer, SubscriptionID: sub.ID, Tier: sub.Metadata["tier"], PeriodEnd: time.Unix(sub.CurrentPeriodEnd, 0)}
		if ev.Type == "customer.subscription.deleted" || (sub.Status != "active" && sub.Status != "trialing") {
			e.Type = EventSubscriptionInactive
		} else {
			e.Type = EventSubscriptionActive
		}
		return []Event{e}, nil
	}
	return nil, nil // ignored event type
}
