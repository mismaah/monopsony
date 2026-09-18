// Package store is the persistence boundary. Two implementations exist: an
// in-memory store for development and tests, and Postgres for production.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"monopsony/server/internal/game"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)

// User is an account. Guests have no email and cannot log in again once
// their refresh token expires.
type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email,omitempty"`
	Name         string    `json:"name"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"` // "player" | "admin"
	Tier         string    `json:"tier"` // "free" | "premium"
	Guest        bool      `json:"guest"`
	Banned       bool      `json:"banned"`
	CreatedAt    time.Time `json:"createdAt"`
}

// RefreshToken is stored hashed; the raw token lives only in the client cookie.
type RefreshToken struct {
	Hash      string
	UserID    string
	ExpiresAt time.Time
}

// SeatRecord is a seat as persisted with the game.
type SeatRecord struct {
	PlayerID   string            `json:"playerId"`
	UserID     string            `json:"userId,omitempty"` // empty for bots
	Name       string            `json:"name"`
	IsBot      bool              `json:"isBot"`
	BotProfile string            `json:"botProfile,omitempty"`
	Loadout    map[string]string `json:"loadout,omitempty"` // cosmetic slot -> item id, refreshed on join/subscribe
}

// Game statuses.
const (
	StatusLobby      = "lobby"
	StatusInProgress = "in_progress"
	StatusFinished   = "finished"
)

// GameRecord is everything needed to rebuild a room after a restart.
type GameRecord struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Status      string          `json:"status"`
	HostID      string          `json:"hostId"`
	Visibility  string          `json:"visibility"` // "public" | "private"
	InviteCode  string          `json:"inviteCode,omitempty"`
	MaxPlayers  int             `json:"maxPlayers"`
	TurnSeconds int             `json:"turnSeconds"`
	ConfigID    string          `json:"configId"`
	Config      *game.Config    `json:"config"` // pinned copy
	Seats       []SeatRecord    `json:"seats"`
	State       json.RawMessage `json:"state,omitempty"` // latest game.State snapshot
	Seq         int             `json:"seq"`
	WinnerID    string          `json:"winnerId,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
	StartedAt   *time.Time      `json:"startedAt,omitempty"`
	FinishedAt  *time.Time      `json:"finishedAt,omitempty"`
}

// ConfigRecord is a versioned, immutable game config.
type ConfigRecord struct {
	ID        string       `json:"id"`
	Version   int          `json:"version"`
	Name      string       `json:"name"`
	Config    *game.Config `json:"config"`
	Published bool         `json:"published"`
	CreatedBy string       `json:"createdBy"`
	CreatedAt time.Time    `json:"createdAt"`
}

// Store is implemented by mem and pg.
type Store interface {
	// Users
	CreateUser(ctx context.Context, u *User) error
	GetUser(ctx context.Context, id string) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	UpdateUser(ctx context.Context, u *User) error

	// Sessions
	PutRefreshToken(ctx context.Context, t RefreshToken) error
	GetRefreshToken(ctx context.Context, hash string) (*RefreshToken, error)
	DeleteRefreshToken(ctx context.Context, hash string) error

	// Games
	SaveGame(ctx context.Context, g *GameRecord) error
	GetGame(ctx context.Context, id string) (*GameRecord, error)
	ListGames(ctx context.Context, status string, publicOnly bool, limit int) ([]*GameRecord, error)
	ListGamesForUser(ctx context.Context, userID string, limit int) ([]*GameRecord, error)
	AppendEvents(ctx context.Context, gameID string, events []game.EventEnvelope) error
	GetEvents(ctx context.Context, gameID string, afterSeq int) ([]game.EventEnvelope, error)

	// Configs
	SaveConfig(ctx context.Context, c *ConfigRecord) error
	GetConfig(ctx context.Context, id string) (*ConfigRecord, error)
	GetPublishedConfig(ctx context.Context) (*ConfigRecord, error)
	ListConfigs(ctx context.Context) ([]*ConfigRecord, error)
	PublishConfig(ctx context.Context, id string) error

	MonetizationStore

	Close() error
}

// ---- monetization -------------------------------------------------------------

// Subscription is the user's current plan as reported by the payment provider.
type Subscription struct {
	UserID         string    `json:"userId"`
	Provider       string    `json:"provider"`
	CustomerID     string    `json:"customerId"`
	SubscriptionID string    `json:"subscriptionId"`
	Status         string    `json:"status"` // active | canceled | past_due | ...
	Tier           string    `json:"tier"`
	PeriodEnd      time.Time `json:"periodEnd"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// Purchase is a one-off cosmetic purchase.
type Purchase struct {
	ID          string    `json:"id"`
	UserID      string    `json:"userId"`
	Provider    string    `json:"provider"`
	ProviderRef string    `json:"providerRef"`
	SKU         string    `json:"sku"`
	AmountCents int       `json:"amountCents"`
	Currency    string    `json:"currency"`
	CreatedAt   time.Time `json:"createdAt"`
}

// Cosmetic is a shop item. Manifest is whatever the client renderer needs
// (built-in ids, or glTF/texture URLs for asset-backed items).
type Cosmetic struct {
	ID           string          `json:"id"`
	Slot         string          `json:"slot"` // token | board | dice | buildings | cards
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	PriceCents   int             `json:"priceCents"` // 0 = free
	Currency     string          `json:"currency"`
	TierRequired string          `json:"tierRequired,omitempty"` // "" = any
	Manifest     json.RawMessage `json:"manifest"`
	Enabled      bool            `json:"enabled"`
	SortOrder    int             `json:"sortOrder"`
	CreatedAt    time.Time       `json:"createdAt"`
}

// PlanRecord is an admin-editable tier definition (caps as JSON).
type PlanRecord struct {
	Tier      string          `json:"tier"`
	Caps      json.RawMessage `json:"caps"`
	PriceID   string          `json:"priceId,omitempty"` // provider price id for subscriptions
	UpdatedAt time.Time       `json:"updatedAt"`
}

// AuditEntry records an admin action.
type AuditEntry struct {
	ID      string          `json:"id"`
	AdminID string          `json:"adminId"`
	Action  string          `json:"action"`
	Target  string          `json:"target"`
	Payload json.RawMessage `json:"payload,omitempty"`
	At      time.Time       `json:"at"`
}

// MonetizationStore is the second half of Store (kept separate for readability).
type MonetizationStore interface {
	SaveSubscription(ctx context.Context, s *Subscription) error
	GetSubscription(ctx context.Context, userID string) (*Subscription, error)
	SavePurchase(ctx context.Context, p *Purchase) error
	ListPurchases(ctx context.Context, userID string) ([]*Purchase, error)
	// MarkWebhook returns false if the event id was already processed.
	MarkWebhook(ctx context.Context, provider, eventID string) (bool, error)

	SaveCosmetic(ctx context.Context, c *Cosmetic) error
	GetCosmetic(ctx context.Context, id string) (*Cosmetic, error)
	ListCosmetics(ctx context.Context, includeDisabled bool) ([]*Cosmetic, error)
	GrantCosmetic(ctx context.Context, userID, cosmeticID string) error
	OwnedCosmetics(ctx context.Context, userID string) ([]string, error)
	SetLoadout(ctx context.Context, userID string, loadout map[string]string) error
	GetLoadout(ctx context.Context, userID string) (map[string]string, error)

	SavePlan(ctx context.Context, p *PlanRecord) error
	ListPlans(ctx context.Context) ([]*PlanRecord, error)

	ListUsers(ctx context.Context, query string, limit int) ([]*User, error)
	Audit(ctx context.Context, e *AuditEntry) error
	ListAudit(ctx context.Context, limit int) ([]*AuditEntry, error)
}
