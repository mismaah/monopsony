// Package app assembles the HTTP handler from its parts so the binary and
// the integration tests build the exact same server.
package app

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"monopsony/server/internal/adminapi"
	"monopsony/server/internal/auth"
	"monopsony/server/internal/billing"
	"monopsony/server/internal/cluster"
	"monopsony/server/internal/cosmetics"
	"monopsony/server/internal/entitlement"
	"monopsony/server/internal/httpapi"
	"monopsony/server/internal/lobby"
	"monopsony/server/internal/metrics"
	"monopsony/server/internal/ratelimit"
	"monopsony/server/internal/room"
	"monopsony/server/internal/store"
	"monopsony/server/internal/ws"
)

// RateLimits are the per-client-IP budgets. Zero rates disable a limiter.
type RateLimits struct {
	Auth ratelimit.Rate // login/register/guest/refresh
	API  ratelimit.Rate // every other REST route
	WS   ratelimit.Rate // WebSocket upgrades
	// WSFrames caps inbound frames per connection; WSChat caps chat lines.
	WSFrames ratelimit.Rate
	WSChat   ratelimit.Rate
	// TrustProxy honours X-Forwarded-For (only behind a proxy you control).
	TrustProxy bool
}

// DefaultRateLimits are sensible production budgets.
var DefaultRateLimits = RateLimits{
	Auth:     ratelimit.Rate{PerMinute: 20, Burst: 10},
	API:      ratelimit.Rate{PerMinute: 300, Burst: 60},
	WS:       ratelimit.Rate{PerMinute: 30, Burst: 10},
	WSFrames: ratelimit.Rate{PerMinute: 600, Burst: 60},
	WSChat:   ratelimit.Rate{PerMinute: 30, Burst: 5},
}

// Config are the runtime settings.
type Config struct {
	JWTSecret   string
	Origins     []string // allowed CORS/WS origins, full form ("http://localhost:5173")
	Secure      bool
	RoomOptions room.Options
	Logger      *slog.Logger
	// PublicURL is where browsers reach the app (redirect targets for billing).
	PublicURL string
	// Billing is the payment provider; nil selects the in-process fake.
	Billing billing.Provider
	// AdminEmails get the admin role automatically.
	AdminEmails []string
	// OAuth providers to register (nil entries are skipped).
	OAuth []auth.Provider
	// RateLimits; the zero value disables limiting (tests).
	RateLimits RateLimits
	// Metrics registry; nil creates a fresh one.
	Metrics *metrics.Metrics
	// Cluster bus; nil runs as a single node. NodeID is optional.
	Cluster cluster.Bus
	NodeID  string
	// Ads is the client-side ad provider configuration served at /api/config.
	Ads httpapi.AdsConfig
	// Assets stores admin-uploaded cosmetic files; nil disables uploads.
	Assets *cosmetics.AssetStore
}

// App is the assembled server.
type App struct {
	Handler   http.Handler
	Mux       *http.ServeMux
	Auth      *auth.Service
	Lobby     *lobby.Manager
	Ent       *entitlement.Resolver
	Store     store.Store
	Billing   *billing.Service
	Cosmetics *cosmetics.Service
	Metrics   *metrics.Metrics
	Node      *cluster.Node // nil when not clustered
}

// New wires everything together, seeding the classic config if needed and
// restoring live rooms from the store.
func New(ctx context.Context, st store.Store, cfg Config) (*App, error) {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Metrics == nil {
		cfg.Metrics = metrics.New()
	}
	if cfg.RoomOptions.Logger == nil {
		cfg.RoomOptions.Logger = cfg.Logger
	}
	cfg.RoomOptions.Metrics = cfg.Metrics
	if cfg.PublicURL == "" {
		cfg.PublicURL = "http://localhost:5173"
	}
	if err := lobby.EnsurePublishedConfig(ctx, st); err != nil {
		return nil, err
	}
	if err := cosmetics.Seed(ctx, st); err != nil {
		return nil, err
	}
	authSvc := auth.New(st, cfg.JWTSecret, 15*time.Minute, 30*24*time.Hour)
	for _, e := range cfg.AdminEmails {
		if e = strings.ToLower(strings.TrimSpace(e)); e != "" {
			authSvc.AdminEmails[e] = true
			if u, err := st.GetUserByEmail(ctx, e); err == nil && u.Role != "admin" {
				u.Role = "admin"
				_ = st.UpdateUser(ctx, u)
			}
		}
	}
	for _, p := range cfg.OAuth {
		if p != nil {
			authSvc.RegisterProvider(p)
		}
	}
	ent := entitlement.NewResolver()
	if err := ent.Load(ctx, st); err != nil {
		return nil, err
	}
	cos := &cosmetics.Service{Store: st, Ent: ent, Assets: cfg.Assets}
	provider := cfg.Billing
	if provider == nil {
		provider = billing.NewFake(cfg.PublicURL)
	}
	bill := &billing.Service{Provider: provider, Store: st, Ent: ent, Log: cfg.Logger, BaseURL: cfg.PublicURL}

	roomOpts := cfg.RoomOptions
	roomOpts.Loadout = func(userID string) map[string]string { return cos.EffectiveLoadout(context.Background(), userID) }
	mgr := lobby.New(st, ent, roomOpts, cfg.Logger)
	var node *cluster.Node
	if cfg.Cluster != nil {
		n, err := cluster.Join(ctx, cfg.Cluster, cluster.Options{ID: cfg.NodeID, Logger: cfg.Logger, Metrics: cfg.Metrics})
		if err != nil {
			return nil, err
		}
		node = n
		mgr.UseCluster(n)
	}
	if err := mgr.Restore(ctx); err != nil {
		return nil, err
	}
	cfg.Metrics.RegisterRooms(mgr.Counts)

	mux := http.NewServeMux()
	api := &httpapi.API{Auth: authSvc, Store: st, Lobby: mgr, Ent: ent, Log: cfg.Logger, Secure: cfg.Secure, Ads: cfg.Ads}
	api.Register(mux)
	(&httpapi.ShopAPI{API: api, Billing: bill, Cosmetics: cos}).Register(mux)
	(&adminapi.API{Auth: authSvc, Store: st, Lobby: mgr, Ent: ent, Log: cfg.Logger, Cosmetics: cos}).Register(mux)
	mux.Handle("GET /ws", &ws.Handler{
		Auth: authSvc, Lobby: mgr, Origins: hostPatterns(cfg.Origins), Log: cfg.Logger, Metrics: cfg.Metrics,
		FrameRate: cfg.RateLimits.WSFrames, ChatRate: cfg.RateLimits.WSChat,
	})

	handler := httpapi.CORS(cfg.Origins, cfg.Metrics.HTTP(limited(cfg.RateLimits, cfg.Metrics, mux)))
	return &App{
		Handler: handler, Mux: mux, Auth: authSvc, Lobby: mgr, Ent: ent, Store: st, Billing: bill, Cosmetics: cos,
		Metrics: cfg.Metrics, Node: node,
	}, nil
}

// Shutdown drains the rooms (and leaves the cluster) so games move to
// another node or restore cleanly on the next boot.
func (a *App) Shutdown(ctx context.Context) {
	a.Lobby.Shutdown(ctx)
}

// limited applies the per-IP budgets by path family: auth endpoints are
// tight, the WebSocket upgrade is modest, everything else under /api shares
// the general budget. Billing webhooks are exempt (they are signed).
func limited(rl RateLimits, met *metrics.Metrics, mux http.Handler) http.Handler {
	rejected := func(scope string) { met.RateLimited.WithLabelValues(scope).Inc() }
	authH := ratelimit.New(rl.Auth).Middleware("auth", rl.TrustProxy, rejected, mux)
	apiH := ratelimit.New(rl.API).Middleware("api", rl.TrustProxy, rejected, mux)
	wsH := ratelimit.New(rl.WS).Middleware("ws", rl.TrustProxy, rejected, mux)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case p == "/api/billing/webhook":
			mux.ServeHTTP(w, r)
		case strings.HasPrefix(p, "/api/auth/") && !strings.HasPrefix(p, "/api/auth/providers"):
			authH.ServeHTTP(w, r)
		case p == "/ws":
			wsH.ServeHTTP(w, r)
		case strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/admin/api/"):
			apiH.ServeHTTP(w, r)
		default:
			mux.ServeHTTP(w, r)
		}
	})
}

func hostPatterns(origins []string) []string {
	var out []string
	for _, o := range origins {
		o = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(o), "http://"), "https://")
		if o != "" {
			out = append(out, o)
		}
	}
	return out
}
