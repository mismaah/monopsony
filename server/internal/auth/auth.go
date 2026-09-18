// Package auth issues and verifies sessions. Access tokens are short-lived
// JWTs; refresh tokens are opaque, stored hashed, and rotated on use.
//
// Identity providers (email/password, guest, OAuth) all end in the same
// place: a store.User and a session.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/argon2"

	"monopsony/server/internal/ids"
	"monopsony/server/internal/store"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrEmailTaken         = errors.New("email already registered")
	ErrInvalidToken       = errors.New("invalid token")
	ErrBanned             = errors.New("account banned")
)

// Claims is the JWT payload.
type Claims struct {
	Name  string `json:"name"`
	Role  string `json:"role"`
	Guest bool   `json:"guest,omitempty"`
	jwt.RegisteredClaims
}

// Session is what a login returns.
type Session struct {
	User         *store.User
	AccessToken  string
	RefreshToken string // raw; only ever sent to the client once
	ExpiresAt    time.Time
}

// Service is the auth facade.
type Service struct {
	st         store.Store
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
	providers  map[string]Provider
	// AdminEmails are promoted to the admin role on registration/login.
	AdminEmails map[string]bool
}

// Provider is an external identity source (OAuth). Implementations live in
// their own files and are registered by name ("google", "discord").
type Provider interface {
	Name() string
	// AuthURL returns where to send the browser; state is opaque CSRF data.
	AuthURL(state string) string
	// Exchange turns the callback code into a verified identity.
	Exchange(ctx context.Context, code string) (Identity, error)
}

// Identity is what a Provider returns.
type Identity struct {
	Provider string
	Subject  string
	Email    string
	Name     string
}

// New creates the service.
func New(st store.Store, secret string, accessTTL, refreshTTL time.Duration) *Service {
	if accessTTL <= 0 {
		accessTTL = 15 * time.Minute
	}
	if refreshTTL <= 0 {
		refreshTTL = 30 * 24 * time.Hour
	}
	return &Service{st: st, secret: []byte(secret), accessTTL: accessTTL, refreshTTL: refreshTTL, providers: map[string]Provider{}, AdminEmails: map[string]bool{}}
}

func (s *Service) roleFor(email string) string {
	if s.AdminEmails[strings.ToLower(email)] {
		return "admin"
	}
	return "player"
}

// RegisterProvider adds an OAuth provider.
func (s *Service) RegisterProvider(p Provider) { s.providers[p.Name()] = p }

// Provider looks up a registered provider.
func (s *Service) Provider(name string) (Provider, bool) {
	p, ok := s.providers[name]
	return p, ok
}

// ---- password hashing (argon2id) ----------------------------------------------

func hashPassword(pw string) string {
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	key := argon2.IDKey([]byte(pw), salt, 2, 64*1024, 1, 32)
	return "argon2id$" + hex.EncodeToString(salt) + "$" + hex.EncodeToString(key)
}

func checkPassword(hash, pw string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 3 || parts[0] != "argon2id" {
		return false
	}
	salt, err := hex.DecodeString(parts[1])
	if err != nil {
		return false
	}
	want, err := hex.DecodeString(parts[2])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(pw), salt, 2, 64*1024, 1, 32)
	return subtle.ConstantTimeCompare(got, want) == 1
}

// ---- flows -----------------------------------------------------------------------

func validName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if len(name) < 2 || len(name) > 24 {
		return "", errors.New("name must be 2-24 characters")
	}
	return name, nil
}

// Register creates an email/password account.
func (s *Service) Register(ctx context.Context, email, password, name string) (*Session, error) {
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return nil, errors.New("invalid email")
	}
	if len(password) < 8 {
		return nil, errors.New("password must be at least 8 characters")
	}
	name, err = validName(name)
	if err != nil {
		return nil, err
	}
	u := &store.User{ID: ids.New(), Email: strings.ToLower(email), Name: name, PasswordHash: hashPassword(password), Role: s.roleFor(email), Tier: "free", CreatedAt: time.Now()}
	if err := s.st.CreateUser(ctx, u); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return nil, ErrEmailTaken
		}
		return nil, err
	}
	return s.issue(ctx, u)
}

// Login checks email/password.
func (s *Service) Login(ctx context.Context, email, password string) (*Session, error) {
	u, err := s.st.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	if !checkPassword(u.PasswordHash, password) {
		return nil, ErrInvalidCredentials
	}
	if u.Banned {
		return nil, ErrBanned
	}
	if s.roleFor(u.Email) == "admin" && u.Role != "admin" {
		u.Role = "admin"
		_ = s.st.UpdateUser(ctx, u)
	}
	return s.issue(ctx, u)
}

// Guest creates a throwaway account.
func (s *Service) Guest(ctx context.Context, name string) (*Session, error) {
	name, err := validName(name)
	if err != nil {
		return nil, err
	}
	u := &store.User{ID: ids.New(), Name: name, Role: "player", Tier: "free", Guest: true, CreatedAt: time.Now()}
	if err := s.st.CreateUser(ctx, u); err != nil {
		return nil, err
	}
	return s.issue(ctx, u)
}

// LoginWithIdentity finds or creates the user for an external identity.
func (s *Service) LoginWithIdentity(ctx context.Context, id Identity) (*Session, error) {
	if id.Email == "" {
		return nil, errors.New("provider returned no email")
	}
	u, err := s.st.GetUserByEmail(ctx, id.Email)
	if errors.Is(err, store.ErrNotFound) {
		name := id.Name
		if name == "" {
			name = strings.SplitN(id.Email, "@", 2)[0]
		}
		if len(name) > 24 {
			name = name[:24]
		}
		u = &store.User{ID: ids.New(), Email: strings.ToLower(id.Email), Name: name, Role: s.roleFor(id.Email), Tier: "free", CreatedAt: time.Now()}
		if err := s.st.CreateUser(ctx, u); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	if u.Banned {
		return nil, ErrBanned
	}
	return s.issue(ctx, u)
}

// Refresh rotates a refresh token and returns a new session.
func (s *Service) Refresh(ctx context.Context, raw string) (*Session, error) {
	h := tokenHash(raw)
	t, err := s.st.GetRefreshToken(ctx, h)
	if err != nil {
		return nil, ErrInvalidToken
	}
	_ = s.st.DeleteRefreshToken(ctx, h)
	u, err := s.st.GetUser(ctx, t.UserID)
	if err != nil {
		return nil, ErrInvalidToken
	}
	if u.Banned {
		return nil, ErrBanned
	}
	return s.issue(ctx, u)
}

// Logout revokes a refresh token.
func (s *Service) Logout(ctx context.Context, raw string) error {
	if raw == "" {
		return nil
	}
	return s.st.DeleteRefreshToken(ctx, tokenHash(raw))
}

func tokenHash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (s *Service) issue(ctx context.Context, u *store.User) (*Session, error) {
	now := time.Now()
	exp := now.Add(s.accessTTL)
	claims := Claims{
		Name: u.Name, Role: u.Role, Guest: u.Guest,
		RegisteredClaims: jwt.RegisteredClaims{Subject: u.ID, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(exp)},
	}
	access, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
	if err != nil {
		return nil, err
	}
	raw := ids.Token()
	if err := s.st.PutRefreshToken(ctx, store.RefreshToken{Hash: tokenHash(raw), UserID: u.ID, ExpiresAt: now.Add(s.refreshTTL)}); err != nil {
		return nil, err
	}
	return &Session{User: u, AccessToken: access, RefreshToken: raw, ExpiresAt: exp}, nil
}

// Verify parses an access token.
func (s *Service) Verify(token string) (*Claims, error) {
	var claims Claims
	t, err := jwt.ParseWithClaims(token, &claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method %v", t.Header["alg"])
		}
		return s.secret, nil
	}, jwt.WithExpirationRequired())
	if err != nil || !t.Valid {
		return nil, ErrInvalidToken
	}
	return &claims, nil
}

// RefreshTTL exposes the cookie lifetime.
func (s *Service) RefreshTTL() time.Duration { return s.refreshTTL }

// NewState returns an opaque CSRF token for OAuth round-trips.
func NewState() string { return ids.Token() }
