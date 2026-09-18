// Package adminapi serves the admin panel: config versions, plans, users,
// cosmetics, live rooms and the audit log. Every mutation is audited.
package adminapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"monopsony/server/internal/auth"
	"monopsony/server/internal/cosmetics"
	"monopsony/server/internal/entitlement"
	"monopsony/server/internal/game"
	"monopsony/server/internal/ids"
	"monopsony/server/internal/lobby"
	"monopsony/server/internal/protocol"
	"monopsony/server/internal/store"
)

// API bundles dependencies.
type API struct {
	Auth  *auth.Service
	Store store.Store
	Lobby *lobby.Manager
	Ent   *entitlement.Resolver
	Log   *slog.Logger
	// Cosmetics validates manifests and owns the asset store.
	Cosmetics *cosmetics.Service
}

type ctxKey int

const adminKey ctxKey = 1

// Register mounts routes under /admin/api.
func (a *API) Register(mux *http.ServeMux) {
	h := a.requireAdmin
	mux.Handle("GET /admin/api/me", h(a.me))
	mux.Handle("GET /admin/api/configs", h(a.listConfigs))
	mux.Handle("GET /admin/api/configs/{id}", h(a.getConfig))
	mux.Handle("POST /admin/api/configs", h(a.createConfig))
	mux.Handle("POST /admin/api/configs/validate", h(a.validateConfig))
	mux.Handle("POST /admin/api/configs/{id}/publish", h(a.publishConfig))
	mux.Handle("GET /admin/api/plans", h(a.listPlans))
	mux.Handle("PUT /admin/api/plans/{tier}", h(a.savePlan))
	mux.Handle("GET /admin/api/users", h(a.listUsers))
	mux.Handle("PATCH /admin/api/users/{id}", h(a.updateUser))
	mux.Handle("POST /admin/api/users/{id}/grant", h(a.grantCosmetic))
	mux.Handle("GET /admin/api/cosmetics", h(a.listCosmetics))
	mux.Handle("PUT /admin/api/cosmetics/{id}", h(a.saveCosmetic))
	mux.Handle("POST /admin/api/cosmetics/validate", h(a.validateCosmetic))
	mux.Handle("GET /admin/api/assets", h(a.listAssets))
	mux.Handle("POST /admin/api/assets", h(a.uploadAsset))
	mux.Handle("DELETE /admin/api/assets/{id}", h(a.deleteAsset))
	mux.Handle("GET /admin/api/rooms", h(a.listRooms))
	mux.Handle("POST /admin/api/rooms/{id}/end", h(a.endRoom))
	mux.Handle("GET /admin/api/audit", h(a.listAudit))
}

// ---- plumbing ----------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": msg}})
}

func decode(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 2<<20)
	return json.NewDecoder(r.Body).Decode(v)
}

func adminFrom(r *http.Request) *store.User {
	u, _ := r.Context().Value(adminKey).(*store.User)
	return u
}

func (a *API) requireAdmin(next func(http.ResponseWriter, *http.Request)) http.Handler {
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
		if err != nil || u.Banned || u.Role != "admin" {
			writeErr(w, http.StatusForbidden, "forbidden", "admin role required")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), adminKey, u)))
	})
}

func (a *API) audit(ctx context.Context, admin *store.User, action, target string, payload any) {
	var raw json.RawMessage
	if payload != nil {
		raw, _ = json.Marshal(payload)
	}
	if err := a.Store.Audit(ctx, &store.AuditEntry{ID: ids.New(), AdminID: admin.ID, Action: action, Target: target, Payload: raw, At: time.Now()}); err != nil {
		a.Log.Error("audit", "err", err)
	}
}

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"user": adminFrom(r)})
}

// ---- configs ----------------------------------------------------------------------

func (a *API) listConfigs(w http.ResponseWriter, r *http.Request) {
	list, err := a.Store.ListConfigs(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	type row struct {
		ID        string    `json:"id"`
		Version   int       `json:"version"`
		Name      string    `json:"name"`
		Published bool      `json:"published"`
		CreatedBy string    `json:"createdBy"`
		CreatedAt time.Time `json:"createdAt"`
	}
	out := make([]row, 0, len(list))
	for _, c := range list {
		out = append(out, row{c.ID, c.Version, c.Name, c.Published, c.CreatedBy, c.CreatedAt})
	}
	writeJSON(w, http.StatusOK, map[string]any{"configs": out})
}

func (a *API) getConfig(w http.ResponseWriter, r *http.Request) {
	c, err := a.Store.GetConfig(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "no such config")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"config": c})
}

func (a *API) validateConfig(w http.ResponseWriter, r *http.Request) {
	var cfg game.Config
	if err := decode(r, &cfg); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "malformed config json")
		return
	}
	if err := cfg.Validate(); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"valid": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"valid": true})
}

// createConfig stores a new immutable version. Body: {name, config, publish}.
func (a *API) createConfig(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name    string      `json:"name"`
		Config  game.Config `json:"config"`
		Publish bool        `json:"publish"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "malformed body")
		return
	}
	if err := in.Config.Validate(); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid", err.Error())
		return
	}
	existing, err := a.Store.ListConfigs(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	version := 1
	for _, c := range existing {
		if c.Version >= version {
			version = c.Version + 1
		}
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = fmt.Sprintf("%s v%d", in.Config.Name, version)
	}
	rec := &store.ConfigRecord{ID: fmt.Sprintf("cfg-v%d-%s", version, ids.New()[:8]), Version: version, Name: name, Config: &in.Config, Published: in.Publish, CreatedBy: adminFrom(r).ID, CreatedAt: time.Now()}
	rec.Config.ID = rec.ID
	if err := a.Store.SaveConfig(r.Context(), rec); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	a.audit(r.Context(), adminFrom(r), "config.create", rec.ID, map[string]any{"version": version, "publish": in.Publish})
	writeJSON(w, http.StatusCreated, map[string]any{"config": rec})
}

func (a *API) publishConfig(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := a.Store.PublishConfig(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "not_found", "no such config")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	a.audit(r.Context(), adminFrom(r), "config.publish", id, nil)
	w.WriteHeader(http.StatusNoContent)
}

// ---- plans -------------------------------------------------------------------------

func (a *API) listPlans(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"plans": a.Ent.Plans()})
}

func (a *API) savePlan(w http.ResponseWriter, r *http.Request) {
	var p entitlement.Plan
	if err := decode(r, &p); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "malformed plan")
		return
	}
	p.Tier = r.PathValue("tier")
	if p.Tier != entitlement.TierFree && p.Tier != entitlement.TierPremium {
		writeErr(w, http.StatusBadRequest, "invalid", "unknown tier")
		return
	}
	if p.Caps.MaxPlayersPerRoom < game.MinPlayers || p.Caps.MaxPlayersPerRoom > game.MaxPlayers {
		writeErr(w, http.StatusBadRequest, "invalid", "maxPlayersPerRoom out of range")
		return
	}
	if err := a.Ent.Save(r.Context(), a.Store, p); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	a.audit(r.Context(), adminFrom(r), "plan.save", p.Tier, p)
	writeJSON(w, http.StatusOK, map[string]any{"plan": p})
}

// ---- users --------------------------------------------------------------------------

func (a *API) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := a.Store.ListUsers(r.Context(), r.URL.Query().Get("q"), 100)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if users == nil {
		users = []*store.User{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

func (a *API) updateUser(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Tier   *string `json:"tier"`
		Role   *string `json:"role"`
		Banned *bool   `json:"banned"`
		Name   *string `json:"name"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "malformed body")
		return
	}
	u, err := a.Store.GetUser(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "no such user")
		return
	}
	admin := adminFrom(r)
	if in.Tier != nil {
		if *in.Tier != entitlement.TierFree && *in.Tier != entitlement.TierPremium {
			writeErr(w, http.StatusBadRequest, "invalid", "unknown tier")
			return
		}
		u.Tier = *in.Tier
	}
	if in.Role != nil {
		if *in.Role != "player" && *in.Role != "admin" {
			writeErr(w, http.StatusBadRequest, "invalid", "unknown role")
			return
		}
		if u.ID == admin.ID && *in.Role != "admin" {
			writeErr(w, http.StatusBadRequest, "invalid", "you cannot demote yourself")
			return
		}
		u.Role = *in.Role
	}
	if in.Banned != nil {
		if u.ID == admin.ID && *in.Banned {
			writeErr(w, http.StatusBadRequest, "invalid", "you cannot ban yourself")
			return
		}
		u.Banned = *in.Banned
	}
	if in.Name != nil && strings.TrimSpace(*in.Name) != "" {
		u.Name = strings.TrimSpace(*in.Name)
	}
	if err := a.Store.UpdateUser(r.Context(), u); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	a.audit(r.Context(), admin, "user.update", u.ID, in)
	writeJSON(w, http.StatusOK, map[string]any{"user": u})
}

func (a *API) grantCosmetic(w http.ResponseWriter, r *http.Request) {
	var in struct{ CosmeticID string }
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "malformed body")
		return
	}
	if err := a.Store.GrantCosmetic(r.Context(), r.PathValue("id"), in.CosmeticID); err != nil {
		writeErr(w, http.StatusNotFound, "not_found", "no such cosmetic")
		return
	}
	a.audit(r.Context(), adminFrom(r), "user.grant", r.PathValue("id"), in)
	w.WriteHeader(http.StatusNoContent)
}

// ---- cosmetics -------------------------------------------------------------------------

func (a *API) listCosmetics(w http.ResponseWriter, r *http.Request) {
	items, err := a.Store.ListCosmetics(r.Context(), true)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if items == nil {
		items = []*store.Cosmetic{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *API) saveCosmetic(w http.ResponseWriter, r *http.Request) {
	var c store.Cosmetic
	if err := decode(r, &c); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "malformed cosmetic")
		return
	}
	c.ID = r.PathValue("id")
	if c.ID == "" || c.Name == "" {
		writeErr(w, http.StatusBadRequest, "invalid", "id and name are required")
		return
	}
	validSlot := false
	for _, s := range []string{"token", "board", "dice", "buildings", "cards"} {
		if c.Slot == s {
			validSlot = true
		}
	}
	if !validSlot {
		writeErr(w, http.StatusBadRequest, "invalid", "unknown slot")
		return
	}
	if c.PriceCents < 0 {
		writeErr(w, http.StatusBadRequest, "invalid", "negative price")
		return
	}
	if c.Currency == "" {
		c.Currency = "usd"
	}
	if len(c.Manifest) == 0 {
		c.Manifest = json.RawMessage(`{}`)
	}
	if err := a.validateManifest(c.Slot, c.Manifest); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid", err.Error())
		return
	}
	if existing, err := a.Store.GetCosmetic(r.Context(), c.ID); err == nil {
		c.CreatedAt = existing.CreatedAt
	} else {
		c.CreatedAt = time.Now()
	}
	if err := a.Store.SaveCosmetic(r.Context(), &c); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	a.audit(r.Context(), adminFrom(r), "cosmetic.save", c.ID, c)
	writeJSON(w, http.StatusOK, map[string]any{"item": c})
}

func (a *API) validateManifest(slot string, raw json.RawMessage) error {
	var assets *cosmetics.AssetStore
	if a.Cosmetics != nil {
		assets = a.Cosmetics.Assets
	}
	return cosmetics.ValidateManifest(slot, raw, assets)
}

// validateCosmetic checks a manifest without saving (the editor calls it live).
func (a *API) validateCosmetic(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Slot     string          `json:"slot"`
		Manifest json.RawMessage `json:"manifest"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "malformed body")
		return
	}
	if err := a.validateManifest(in.Slot, in.Manifest); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---- assets ------------------------------------------------------------------------------

func (a *API) assets(w http.ResponseWriter) *cosmetics.AssetStore {
	if a.Cosmetics == nil || a.Cosmetics.Assets == nil {
		writeErr(w, http.StatusNotImplemented, "assets_disabled", "set MONOPSONY_ASSET_DIR to enable uploads")
		return nil
	}
	return a.Cosmetics.Assets
}

func (a *API) listAssets(w http.ResponseWriter, _ *http.Request) {
	st := a.assets(w)
	if st == nil {
		return
	}
	list, err := st.List()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"assets": list})
}

// uploadAsset accepts multipart/form-data with a "file" part.
func (a *API) uploadAsset(w http.ResponseWriter, r *http.Request) {
	st := a.assets(w)
	if st == nil {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, cosmetics.MaxAssetSize+1<<20)
	f, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "multipart field 'file' is required")
		return
	}
	defer f.Close()
	asset, err := st.Put(hdr.Filename, f)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid", err.Error())
		return
	}
	a.audit(r.Context(), adminFrom(r), "asset.upload", asset.ID, asset)
	writeJSON(w, http.StatusCreated, map[string]any{"asset": asset})
}

func (a *API) deleteAsset(w http.ResponseWriter, r *http.Request) {
	st := a.assets(w)
	if st == nil {
		return
	}
	if err := st.Delete(r.PathValue("id")); err != nil {
		writeErr(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	a.audit(r.Context(), adminFrom(r), "asset.delete", r.PathValue("id"), nil)
	w.WriteHeader(http.StatusNoContent)
}

// ---- rooms ---------------------------------------------------------------------------------

func (a *API) listRooms(w http.ResponseWriter, _ *http.Request) {
	out := a.Lobby.ListAll()
	if out == nil {
		out = []protocol.Lobby{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"rooms": out})
}

func (a *API) endRoom(w http.ResponseWriter, r *http.Request) {
	room, ok := a.Lobby.Get(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "not_found", "no such room")
		return
	}
	if err := room.ForceEnd(); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	a.audit(r.Context(), adminFrom(r), "room.end", r.PathValue("id"), nil)
	w.WriteHeader(http.StatusNoContent)
}

// ---- audit ----------------------------------------------------------------------------------

func (a *API) listAudit(w http.ResponseWriter, r *http.Request) {
	entries, err := a.Store.ListAudit(r.Context(), 200)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if entries == nil {
		entries = []*store.AuditEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}
