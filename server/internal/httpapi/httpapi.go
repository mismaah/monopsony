// Package httpapi serves the REST endpoints: auth, lobby management and
// account info. Gameplay itself goes over the WebSocket (package ws).
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"monopsony/server/internal/auth"
	"monopsony/server/internal/entitlement"
	"monopsony/server/internal/lobby"
	"monopsony/server/internal/room"
	"monopsony/server/internal/store"
)

// API bundles the dependencies of the handlers.
type API struct {
	Auth   *auth.Service
	Store  store.Store
	Lobby  *lobby.Manager
	Ent    *entitlement.Resolver
	Log    *slog.Logger
	Secure bool // set Secure on cookies (prod)
	// Ads is handed to the client so the ad provider can change without a
	// rebuild.
	Ads AdsConfig
}

// AdsConfig selects the client-side ad provider for the free tier.
// Provider is "none" (nothing rendered), "house" (built-in upgrade promo)
// or "adsense" (Google AdSense; Client and Slot required).
type AdsConfig struct {
	Provider string `json:"provider"`
	Client   string `json:"client,omitempty"`
	Slot     string `json:"slot,omitempty"`
	// NonPersonalized requests non-personalised ads (consent-less regions).
	NonPersonalized bool `json:"nonPersonalized,omitempty"`
}

const refreshCookie = "mp_refresh"

type ctxKey int

const userKey ctxKey = 1

// Register mounts the routes on mux.
func (a *API) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("GET /api/config", a.clientConfig)

	mux.HandleFunc("POST /api/auth/register", a.register)
	mux.HandleFunc("POST /api/auth/login", a.login)
	mux.HandleFunc("POST /api/auth/guest", a.guest)
	mux.HandleFunc("POST /api/auth/refresh", a.refresh)
	mux.HandleFunc("POST /api/auth/logout", a.logout)
	mux.HandleFunc("GET /api/auth/providers", a.providers)
	mux.HandleFunc("GET /api/auth/oauth/{provider}/start", a.oauthStart)
	mux.HandleFunc("GET /api/auth/oauth/{provider}/callback", a.oauthCallback)

	mux.Handle("GET /api/me", a.requireUser(a.me))
	mux.Handle("GET /api/games", a.requireUser(a.listGames))
	mux.Handle("GET /api/games/mine", a.requireUser(a.myGames))
	mux.Handle("POST /api/games", a.requireUser(a.createGame))
	mux.Handle("POST /api/games/join-code", a.requireUser(a.joinByCode))
	mux.Handle("GET /api/games/{id}", a.requireUser(a.getGame))
	mux.Handle("POST /api/games/{id}/join", a.requireUser(a.joinGame))
	mux.Handle("POST /api/games/{id}/leave", a.requireUser(a.leaveGame))
	mux.Handle("POST /api/games/{id}/bots", a.requireUser(a.addBot))
	mux.Handle("DELETE /api/games/{id}/seats/{playerId}", a.requireUser(a.kick))
	mux.Handle("POST /api/games/{id}/start", a.requireUser(a.startGame))
}

// ---- helpers ------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{"error": apiError{Code: code, Message: msg}})
}

func decode(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 64*1024)
	return json.NewDecoder(r.Body).Decode(v)
}

func userFrom(r *http.Request) *store.User {
	u, _ := r.Context().Value(userKey).(*store.User)
	return u
}

func (a *API) requireUser(next func(http.ResponseWriter, *http.Request)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "missing bearer token")
			return
		}
		claims, err := a.Auth.Verify(strings.TrimPrefix(h, "Bearer "))
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "invalid or expired token")
			return
		}
		u, err := a.Store.GetUser(r.Context(), claims.Subject)
		if err != nil || u.Banned {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "unknown user")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	})
}

func (a *API) writeSession(w http.ResponseWriter, s *auth.Session) {
	http.SetCookie(w, &http.Cookie{
		Name: refreshCookie, Value: s.RefreshToken, Path: "/api/auth", HttpOnly: true, Secure: a.Secure,
		SameSite: http.SameSiteLaxMode, Expires: time.Now().Add(a.Auth.RefreshTTL()),
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"accessToken": s.AccessToken, "expiresAt": s.ExpiresAt.UnixMilli(), "user": s.User,
	})
}

func (a *API) authErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials), errors.Is(err, auth.ErrInvalidToken):
		writeErr(w, http.StatusUnauthorized, "unauthorized", err.Error())
	case errors.Is(err, auth.ErrEmailTaken):
		writeErr(w, http.StatusConflict, "email_taken", err.Error())
	case errors.Is(err, auth.ErrBanned):
		writeErr(w, http.StatusForbidden, "banned", err.Error())
	default:
		writeErr(w, http.StatusBadRequest, "invalid", err.Error())
	}
}

func (a *API) lobbyErr(w http.ResponseWriter, err error) {
	var le *lobby.Error
	if errors.As(err, &le) {
		status := http.StatusBadRequest
		if le.Code == "upgrade_required" {
			status = http.StatusPaymentRequired
		}
		writeErr(w, status, le.Code, le.Message)
		return
	}
	var re *room.Error
	if errors.As(err, &re) {
		status := http.StatusBadRequest
		if re.Code == "forbidden" {
			status = http.StatusForbidden
		}
		writeErr(w, status, re.Code, re.Message)
		return
	}
	a.Log.Error("lobby error", "err", err)
	writeErr(w, http.StatusInternalServerError, "internal", "something went wrong")
}

// ---- auth ---------------------------------------------------------------------------

func (a *API) register(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email, Password, Name string }
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "malformed body")
		return
	}
	s, err := a.Auth.Register(r.Context(), in.Email, in.Password, in.Name)
	if err != nil {
		a.authErr(w, err)
		return
	}
	a.writeSession(w, s)
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email, Password string }
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "malformed body")
		return
	}
	s, err := a.Auth.Login(r.Context(), in.Email, in.Password)
	if err != nil {
		a.authErr(w, err)
		return
	}
	a.writeSession(w, s)
}

func (a *API) guest(w http.ResponseWriter, r *http.Request) {
	var in struct{ Name string }
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "malformed body")
		return
	}
	s, err := a.Auth.Guest(r.Context(), in.Name)
	if err != nil {
		a.authErr(w, err)
		return
	}
	a.writeSession(w, s)
}

func (a *API) refresh(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(refreshCookie)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "no refresh token")
		return
	}
	s, err := a.Auth.Refresh(r.Context(), c.Value)
	if err != nil {
		a.authErr(w, err)
		return
	}
	a.writeSession(w, s)
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(refreshCookie); err == nil {
		_ = a.Auth.Logout(r.Context(), c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: refreshCookie, Value: "", Path: "/api/auth", HttpOnly: true, MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) providers(w http.ResponseWriter, _ *http.Request) {
	var names []string
	for _, n := range []string{"google", "discord"} {
		if _, ok := a.Auth.Provider(n); ok {
			names = append(names, n)
		}
	}
	if names == nil {
		names = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"providers": names})
}

func (a *API) oauthStart(w http.ResponseWriter, r *http.Request) {
	p, ok := a.Auth.Provider(r.PathValue("provider"))
	if !ok {
		writeErr(w, http.StatusNotFound, "not_found", "unknown provider")
		return
	}
	state := auth.NewState()
	http.SetCookie(w, &http.Cookie{Name: "mp_oauth_state", Value: state, Path: "/api/auth/oauth", HttpOnly: true, Secure: a.Secure, SameSite: http.SameSiteLaxMode, MaxAge: 600})
	http.Redirect(w, r, p.AuthURL(state), http.StatusFound)
}

func (a *API) oauthCallback(w http.ResponseWriter, r *http.Request) {
	p, ok := a.Auth.Provider(r.PathValue("provider"))
	if !ok {
		writeErr(w, http.StatusNotFound, "not_found", "unknown provider")
		return
	}
	c, err := r.Cookie("mp_oauth_state")
	if err != nil || c.Value == "" || c.Value != r.URL.Query().Get("state") {
		writeErr(w, http.StatusBadRequest, "bad_state", "oauth state mismatch")
		return
	}
	id, err := p.Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		writeErr(w, http.StatusBadGateway, "oauth_failed", err.Error())
		return
	}
	s, err := a.Auth.LoginWithIdentity(r.Context(), id)
	if err != nil {
		a.authErr(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: refreshCookie, Value: s.RefreshToken, Path: "/api/auth", HttpOnly: true, Secure: a.Secure,
		SameSite: http.SameSiteLaxMode, Expires: time.Now().Add(a.Auth.RefreshTTL()),
	})
	// The SPA finishes login by calling /api/auth/refresh.
	http.Redirect(w, r, "/oauth/done", http.StatusFound)
}

// clientConfig is public, non-secret configuration the SPA reads at boot.
func (a *API) clientConfig(w http.ResponseWriter, _ *http.Request) {
	ads := a.Ads
	if ads.Provider == "" || (ads.Provider == "adsense" && (ads.Client == "" || ads.Slot == "")) {
		ads = AdsConfig{Provider: "house"}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ads": ads})
}

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	writeJSON(w, http.StatusOK, map[string]any{"user": u, "caps": a.Ent.Resolve(r.Context(), u)})
}

// ---- lobby -------------------------------------------------------------------------

func (a *API) listGames(w http.ResponseWriter, _ *http.Request) {
	list := a.Lobby.ListPublic()
	if list == nil {
		list = nil
	}
	writeJSON(w, http.StatusOK, map[string]any{"games": list})
}

func (a *API) myGames(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	recs, err := a.Store.ListGamesForUser(r.Context(), u.ID, 50)
	if err != nil {
		a.lobbyErr(w, err)
		return
	}
	// Free tier only sees recent finished games; live games are always listed.
	if days := a.Ent.Resolve(r.Context(), u).StatsHistoryDays; days > 0 {
		cutoff := time.Now().AddDate(0, 0, -days)
		kept := recs[:0]
		for _, g := range recs {
			if g.Status != store.StatusFinished || g.UpdatedAt.After(cutoff) {
				kept = append(kept, g)
			}
		}
		recs = kept
	}
	type row struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Status   string `json:"status"`
		WinnerID string `json:"winnerId,omitempty"`
		Players  int    `json:"players"`
		Updated  int64  `json:"updatedAt"`
	}
	out := make([]row, 0, len(recs))
	for _, g := range recs {
		out = append(out, row{ID: g.ID, Name: g.Name, Status: g.Status, WinnerID: g.WinnerID, Players: len(g.Seats), Updated: g.UpdatedAt.UnixMilli()})
	}
	writeJSON(w, http.StatusOK, map[string]any{"games": out})
}

func (a *API) createGame(w http.ResponseWriter, r *http.Request) {
	var opts lobby.CreateOptions
	if err := decode(r, &opts); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "malformed body")
		return
	}
	room, err := a.Lobby.Create(r.Context(), userFrom(r), opts)
	if err != nil {
		a.lobbyErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"game": room.Info()})
}

func (a *API) getGame(w http.ResponseWriter, r *http.Request) {
	room, ok := a.Lobby.Get(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "not_found", "no such game")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"game": room.Info()})
}

func (a *API) joinGame(w http.ResponseWriter, r *http.Request) {
	room, ok := a.Lobby.Get(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "not_found", "no such game")
		return
	}
	if err := a.Lobby.Join(r.Context(), room, userFrom(r)); err != nil {
		a.lobbyErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"game": room.Info()})
}

func (a *API) joinByCode(w http.ResponseWriter, r *http.Request) {
	var in struct{ Code string }
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "malformed body")
		return
	}
	room, ok := a.Lobby.ByInvite(in.Code)
	if !ok {
		writeErr(w, http.StatusNotFound, "not_found", "no game with that code")
		return
	}
	if err := a.Lobby.Join(r.Context(), room, userFrom(r)); err != nil {
		a.lobbyErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"game": room.Info()})
}

func (a *API) leaveGame(w http.ResponseWriter, r *http.Request) {
	room, ok := a.Lobby.Get(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "not_found", "no such game")
		return
	}
	if err := room.Leave(userFrom(r).ID); err != nil {
		a.lobbyErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) addBot(w http.ResponseWriter, r *http.Request) {
	room, ok := a.Lobby.Get(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "not_found", "no such game")
		return
	}
	var in struct{ Profile string }
	_ = decode(r, &in)
	if err := room.AddBot(userFrom(r).ID, in.Profile); err != nil {
		a.lobbyErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"game": room.Info()})
}

func (a *API) kick(w http.ResponseWriter, r *http.Request) {
	room, ok := a.Lobby.Get(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "not_found", "no such game")
		return
	}
	if err := room.Kick(userFrom(r).ID, r.PathValue("playerId")); err != nil {
		a.lobbyErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"game": room.Info()})
}

func (a *API) startGame(w http.ResponseWriter, r *http.Request) {
	room, ok := a.Lobby.Get(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "not_found", "no such game")
		return
	}
	if err := room.StartGame(userFrom(r).ID); err != nil {
		a.lobbyErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"game": room.Info()})
}

// CORS allows the dev front-ends (Vite) to call the API from another origin.
func CORS(origins []string, next http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, o := range origins {
		allowed[o] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if o := r.Header.Get("Origin"); o != "" && allowed[o] {
			w.Header().Set("Access-Control-Allow-Origin", o)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
