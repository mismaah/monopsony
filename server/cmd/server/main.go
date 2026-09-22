// Command server runs the Monopsony API: REST for auth/lobby, WebSocket for
// gameplay, the admin API and (optionally) the built SPAs. Configuration
// comes from environment variables prefixed MONOPSONY_.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"monopsony/server/internal/app"
	"monopsony/server/internal/auth"
	"monopsony/server/internal/billing"
	"monopsony/server/internal/cluster"
	"monopsony/server/internal/cosmetics"
	"monopsony/server/internal/httpapi"
	"monopsony/server/internal/metrics"
	"monopsony/server/internal/ratelimit"
	"monopsony/server/internal/room"
	"monopsony/server/internal/store"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil {
		return v
	}
	return def
}

func envBool(key string) bool {
	switch strings.ToLower(os.Getenv(key)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// envRate reads "per-minute[/burst]" (e.g. "300/60"); "0" disables.
func envRate(key string, def ratelimit.Rate) ratelimit.Rate {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	per, burst, _ := strings.Cut(v, "/")
	r := ratelimit.Rate{}
	r.PerMinute, _ = strconv.Atoi(strings.TrimSpace(per))
	r.Burst, _ = strconv.Atoi(strings.TrimSpace(burst))
	return r
}

func newLogger() *slog.Logger {
	level := slog.LevelInfo
	switch strings.ToLower(env("MONOPSONY_LOG_LEVEL", "info")) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	opts := &slog.HandlerOptions{Level: level}
	if env("MONOPSONY_LOG_FORMAT", "text") == "json" {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}

func main() {
	// Dev convenience: pick up the repo-root .env (or one in the cwd) so
	// running the binary directly (air, go run) matches `task server`.
	// Variables already set in the environment win; missing files are ignored.
	_ = godotenv.Load("../.env", ".env")

	log := newLogger()
	slog.SetDefault(log)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	addr := env("MONOPSONY_ADDR", ":8080")
	dbURL := os.Getenv("MONOPSONY_DATABASE_URL")
	origins := strings.Split(env("MONOPSONY_CORS_ORIGINS", "http://localhost:5173,http://localhost:5174"), ",")

	var st store.Store
	if dbURL != "" {
		pg, err := store.OpenPG(ctx, dbURL)
		if err != nil {
			log.Error("postgres", "err", err)
			os.Exit(1)
		}
		st = pg
		log.Info("store: postgres")
	} else {
		st = store.NewMem()
		log.Warn("store: in-memory (set MONOPSONY_DATABASE_URL for persistence)")
	}
	defer st.Close()

	var provider billing.Provider // nil = fake
	if env("MONOPSONY_BILLING_PROVIDER", "fake") == "stripe" {
		provider = billing.NewStripe(os.Getenv("STRIPE_SECRET_KEY"), os.Getenv("STRIPE_WEBHOOK_SECRET"))
		log.Info("billing: stripe")
	} else {
		log.Warn("billing: fake provider (purchases complete instantly)")
	}

	publicURL := env("MONOPSONY_PUBLIC_URL", "http://localhost:5173")
	var oauth []auth.Provider
	if id := os.Getenv("GOOGLE_CLIENT_ID"); id != "" {
		oauth = append(oauth, auth.NewGoogle(id, os.Getenv("GOOGLE_CLIENT_SECRET"), publicURL+"/api/auth/oauth/google/callback"))
	}
	if id := os.Getenv("DISCORD_CLIENT_ID"); id != "" {
		oauth = append(oauth, auth.NewDiscord(id, os.Getenv("DISCORD_CLIENT_SECRET"), publicURL+"/api/auth/oauth/discord/callback"))
	}

	// Multi-node: every server sharing MONOPSONY_REDIS_URL (and the same
	// Postgres) forms one cluster; games are reachable from any node.
	var bus cluster.Bus
	if redisURL := os.Getenv("MONOPSONY_REDIS_URL"); redisURL != "" {
		rb, err := cluster.OpenRedis(ctx, redisURL)
		if err != nil {
			log.Error("redis", "err", err)
			os.Exit(1)
		}
		defer rb.Close()
		bus = rb
		if dbURL == "" {
			log.Warn("cluster: redis without postgres — nodes will not see each other's games")
		}
		log.Info("cluster: redis")
	}

	var assets *cosmetics.AssetStore
	if dir := env("MONOPSONY_ASSET_DIR", "./data/assets"); dir != "" && dir != "off" {
		a, err := cosmetics.NewAssetStore(dir, "/media")
		if err != nil {
			log.Error("asset dir", "dir", dir, "err", err)
			os.Exit(1)
		}
		assets = a
	}

	rl := app.DefaultRateLimits
	rl.Auth = envRate("MONOPSONY_RATE_AUTH", rl.Auth)
	rl.API = envRate("MONOPSONY_RATE_API", rl.API)
	rl.WS = envRate("MONOPSONY_RATE_WS", rl.WS)
	rl.WSFrames = envRate("MONOPSONY_RATE_WS_FRAMES", rl.WSFrames)
	rl.WSChat = envRate("MONOPSONY_RATE_WS_CHAT", rl.WSChat)
	rl.TrustProxy = envBool("MONOPSONY_TRUST_PROXY")

	met := metrics.New()
	a, err := app.New(ctx, st, app.Config{
		JWTSecret:   env("MONOPSONY_JWT_SECRET", "dev-secret-change-me"),
		Origins:     origins,
		Secure:      env("MONOPSONY_ENV", "dev") == "prod",
		PublicURL:   publicURL,
		OAuth:       oauth,
		Billing:     provider,
		AdminEmails: strings.Split(os.Getenv("MONOPSONY_ADMIN_EMAILS"), ","),
		RoomOptions: room.Options{
			BotDelay:    time.Duration(envInt("MONOPSONY_BOT_DELAY_MS", 900)) * time.Millisecond,
			AssetGrace:  time.Duration(envInt("MONOPSONY_ASSET_GRACE_MS", 10000)) * time.Millisecond,
			MaxTimeouts: 3,
		},
		Logger:     log,
		RateLimits: rl,
		Metrics:    met,
		Cluster:    bus,
		NodeID:     os.Getenv("MONOPSONY_NODE_ID"),
		Assets:     assets,
		Ads: httpapi.AdsConfig{
			Provider:        env("MONOPSONY_ADS_PROVIDER", "house"),
			Client:          os.Getenv("MONOPSONY_ADSENSE_CLIENT"),
			Slot:            os.Getenv("MONOPSONY_ADSENSE_SLOT"),
			NonPersonalized: envBool("MONOPSONY_ADS_NON_PERSONALIZED"),
		},
	})
	if err != nil {
		log.Error("startup", "err", err)
		os.Exit(1)
	}
	go func() {
		t := time.NewTicker(10 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				a.Lobby.Sweep(30 * time.Minute)
			}
		}
	}()

	handler := a.Handler
	if dir := os.Getenv("MONOPSONY_STATIC_DIR"); dir != "" {
		api := handler
		static := spaHandler(dir)
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := r.URL.Path
			if strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/admin/api/") || strings.HasPrefix(p, "/media/") || p == "/ws" || p == "/healthz" {
				api.ServeHTTP(w, r)
				return
			}
			static.ServeHTTP(w, r)
		})
	}

	// Metrics are served on their own listener so they are never exposed
	// through the public port; scrape it from inside the network.
	if maddr := env("MONOPSONY_METRICS_ADDR", ":9100"); maddr != "" && maddr != "off" {
		mmux := http.NewServeMux()
		mmux.Handle("GET /metrics", met.Handler())
		mmux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
		msrv := &http.Server{Addr: maddr, Handler: mmux, ReadHeaderTimeout: 5 * time.Second}
		go func() {
			log.Info("metrics listening", "addr", maddr)
			if err := msrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Error("metrics server", "err", err)
			}
		}()
		defer msrv.Close()
	}

	srv := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		log.Info("listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server", "err", err)
			os.Exit(1)
		}
	}()
	<-ctx.Done()
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Stop accepting, drain the rooms (they persist after every command, so
	// the next boot or another cluster node picks them up), then close.
	_ = srv.Shutdown(shutdownCtx)
	a.Shutdown(shutdownCtx)
}

// spaHandler serves the player app from dir and the admin app from dir/admin,
// falling back to each app's index.html for client-side routes.
func spaHandler(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		index := dir + "/index.html"
		if p == "admin" || strings.HasPrefix(p, "admin/") {
			index = dir + "/admin/index.html"
		}
		if p == "" || p == "admin" || p == "admin/" {
			http.ServeFile(w, r, index)
			return
		}
		if st, err := os.Stat(dir + "/" + p); err != nil || st.IsDir() {
			http.ServeFile(w, r, index)
			return
		}
		fs.ServeHTTP(w, r)
	})
}
