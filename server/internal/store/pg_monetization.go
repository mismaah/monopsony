package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (p *PG) SaveSubscription(ctx context.Context, s *Subscription) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO subscriptions(user_id,provider,customer_id,subscription_id,status,tier,period_end,updated_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,now())
		ON CONFLICT (user_id) DO UPDATE SET provider=EXCLUDED.provider,customer_id=EXCLUDED.customer_id,subscription_id=EXCLUDED.subscription_id,
		status=EXCLUDED.status,tier=EXCLUDED.tier,period_end=EXCLUDED.period_end,updated_at=now()`,
		s.UserID, s.Provider, s.CustomerID, s.SubscriptionID, s.Status, s.Tier, nullTime(s.PeriodEnd))
	return err
}

func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

func (p *PG) GetSubscription(ctx context.Context, userID string) (*Subscription, error) {
	var s Subscription
	var periodEnd *time.Time
	err := p.pool.QueryRow(ctx, `SELECT user_id,provider,customer_id,subscription_id,status,tier,period_end,updated_at FROM subscriptions WHERE user_id=$1`, userID).
		Scan(&s.UserID, &s.Provider, &s.CustomerID, &s.SubscriptionID, &s.Status, &s.Tier, &periodEnd, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if periodEnd != nil {
		s.PeriodEnd = *periodEnd
	}
	return &s, nil
}

func (p *PG) SavePurchase(ctx context.Context, pu *Purchase) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO purchases(id,user_id,provider,provider_ref,sku,amount_cents,currency,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT (id) DO NOTHING`,
		pu.ID, pu.UserID, pu.Provider, pu.ProviderRef, pu.SKU, pu.AmountCents, pu.Currency, pu.CreatedAt)
	return err
}

func (p *PG) ListPurchases(ctx context.Context, userID string) ([]*Purchase, error) {
	rows, err := p.pool.Query(ctx, `SELECT id,user_id,provider,provider_ref,sku,amount_cents,currency,created_at FROM purchases WHERE user_id=$1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Purchase
	for rows.Next() {
		var pu Purchase
		if err := rows.Scan(&pu.ID, &pu.UserID, &pu.Provider, &pu.ProviderRef, &pu.SKU, &pu.AmountCents, &pu.Currency, &pu.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &pu)
	}
	return out, rows.Err()
}

func (p *PG) MarkWebhook(ctx context.Context, provider, eventID string) (bool, error) {
	tag, err := p.pool.Exec(ctx, `INSERT INTO webhook_events(provider,event_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, provider, eventID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

const cosmeticCols = `id,slot,name,description,price_cents,currency,tier_required,manifest,enabled,sort_order,created_at`

func scanCosmetic(row pgx.Row) (*Cosmetic, error) {
	var c Cosmetic
	var manifest []byte
	if err := row.Scan(&c.ID, &c.Slot, &c.Name, &c.Description, &c.PriceCents, &c.Currency, &c.TierRequired, &manifest, &c.Enabled, &c.SortOrder, &c.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	c.Manifest = json.RawMessage(manifest)
	return &c, nil
}

func (p *PG) SaveCosmetic(ctx context.Context, c *Cosmetic) error {
	manifest := c.Manifest
	if len(manifest) == 0 {
		manifest = json.RawMessage(`{}`)
	}
	_, err := p.pool.Exec(ctx, `INSERT INTO cosmetics(`+cosmeticCols+`) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (id) DO UPDATE SET slot=EXCLUDED.slot,name=EXCLUDED.name,description=EXCLUDED.description,price_cents=EXCLUDED.price_cents,
		currency=EXCLUDED.currency,tier_required=EXCLUDED.tier_required,manifest=EXCLUDED.manifest,enabled=EXCLUDED.enabled,sort_order=EXCLUDED.sort_order`,
		c.ID, c.Slot, c.Name, c.Description, c.PriceCents, c.Currency, c.TierRequired, []byte(manifest), c.Enabled, c.SortOrder, c.CreatedAt)
	return err
}

func (p *PG) GetCosmetic(ctx context.Context, id string) (*Cosmetic, error) {
	return scanCosmetic(p.pool.QueryRow(ctx, `SELECT `+cosmeticCols+` FROM cosmetics WHERE id=$1`, id))
}

func (p *PG) ListCosmetics(ctx context.Context, includeDisabled bool) ([]*Cosmetic, error) {
	rows, err := p.pool.Query(ctx, `SELECT `+cosmeticCols+` FROM cosmetics WHERE enabled OR $1 ORDER BY sort_order, id`, includeDisabled)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Cosmetic
	for rows.Next() {
		c, err := scanCosmetic(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (p *PG) GrantCosmetic(ctx context.Context, userID, cosmeticID string) error {
	tag, err := p.pool.Exec(ctx, `INSERT INTO user_cosmetics(user_id,cosmetic_id) SELECT $1,$2 WHERE EXISTS(SELECT 1 FROM cosmetics WHERE id=$2) ON CONFLICT DO NOTHING`, userID, cosmeticID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		_ = p.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM cosmetics WHERE id=$1)`, cosmeticID).Scan(&exists)
		if !exists {
			return ErrNotFound
		}
	}
	return nil
}

func (p *PG) OwnedCosmetics(ctx context.Context, userID string) ([]string, error) {
	rows, err := p.pool.Query(ctx, `SELECT cosmetic_id FROM user_cosmetics WHERE user_id=$1 ORDER BY cosmetic_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (p *PG) SetLoadout(ctx context.Context, userID string, loadout map[string]string) error {
	b, _ := json.Marshal(loadout)
	_, err := p.pool.Exec(ctx, `INSERT INTO user_loadouts(user_id,loadout) VALUES($1,$2) ON CONFLICT (user_id) DO UPDATE SET loadout=EXCLUDED.loadout`, userID, b)
	return err
}

func (p *PG) GetLoadout(ctx context.Context, userID string) (map[string]string, error) {
	var b []byte
	err := p.pool.QueryRow(ctx, `SELECT loadout FROM user_loadouts WHERE user_id=$1`, userID).Scan(&b)
	if errors.Is(err, pgx.ErrNoRows) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	return out, json.Unmarshal(b, &out)
}

func (p *PG) SavePlan(ctx context.Context, pl *PlanRecord) error {
	_, err := p.pool.Exec(ctx, `INSERT INTO plans(tier,caps,price_id,updated_at) VALUES($1,$2,$3,now()) ON CONFLICT (tier) DO UPDATE SET caps=EXCLUDED.caps,price_id=EXCLUDED.price_id,updated_at=now()`,
		pl.Tier, []byte(pl.Caps), pl.PriceID)
	return err
}

func (p *PG) ListPlans(ctx context.Context) ([]*PlanRecord, error) {
	rows, err := p.pool.Query(ctx, `SELECT tier,caps,price_id,updated_at FROM plans ORDER BY tier`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*PlanRecord
	for rows.Next() {
		var pl PlanRecord
		var caps []byte
		if err := rows.Scan(&pl.Tier, &caps, &pl.PriceID, &pl.UpdatedAt); err != nil {
			return nil, err
		}
		pl.Caps = json.RawMessage(caps)
		out = append(out, &pl)
	}
	return out, rows.Err()
}

func (p *PG) ListUsers(ctx context.Context, query string, limit int) ([]*User, error) {
	if limit <= 0 {
		limit = 50
	}
	q := "%" + strings.ToLower(strings.TrimSpace(query)) + "%"
	rows, err := p.pool.Query(ctx, `SELECT `+userCols+` FROM users WHERE $1='%%' OR lower(name) LIKE $1 OR lower(COALESCE(email,'')) LIKE $1 OR id=$2 ORDER BY created_at DESC LIMIT $3`, q, strings.TrimSpace(query), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (p *PG) Audit(ctx context.Context, e *AuditEntry) error {
	var payload any
	if len(e.Payload) > 0 {
		payload = []byte(e.Payload)
	}
	_, err := p.pool.Exec(ctx, `INSERT INTO audit_log(id,admin_id,action,target,payload,at) VALUES($1,$2,$3,$4,$5,$6)`, e.ID, e.AdminID, e.Action, e.Target, payload, e.At)
	return err
}

func (p *PG) ListAudit(ctx context.Context, limit int) ([]*AuditEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := p.pool.Query(ctx, `SELECT id,admin_id,action,target,payload,at FROM audit_log ORDER BY at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*AuditEntry
	for rows.Next() {
		var e AuditEntry
		var payload []byte
		if err := rows.Scan(&e.ID, &e.AdminID, &e.Action, &e.Target, &payload, &e.At); err != nil {
			return nil, err
		}
		if payload != nil {
			e.Payload = json.RawMessage(payload)
		}
		out = append(out, &e)
	}
	return out, rows.Err()
}
