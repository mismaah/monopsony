// Package lobby owns the set of live rooms: creation (with entitlement
// checks), lookup by id or invite code, public listing, and restoring
// in-progress games after a restart.
package lobby

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"monopsony/server/internal/cluster"
	"monopsony/server/internal/entitlement"
	"monopsony/server/internal/game"
	"monopsony/server/internal/ids"
	"monopsony/server/internal/protocol"
	"monopsony/server/internal/room"
	"monopsony/server/internal/store"
)

// CreateOptions are the host's choices.
type CreateOptions struct {
	Name        string      `json:"name"`
	Visibility  string      `json:"visibility"` // "public" | "private"
	MaxPlayers  int         `json:"maxPlayers"`
	TurnSeconds int         `json:"turnSeconds"`
	Rules       *game.Rules `json:"rules,omitempty"` // house-rule overrides (premium)
}

// Manager is the room registry.
type Manager struct {
	st       store.Store
	ent      *entitlement.Resolver
	roomOpts room.Options
	log      *slog.Logger

	// node is set when several servers share one game population; nil means
	// every room lives in this process.
	node *cluster.Node

	mu     sync.RWMutex
	rooms  map[string]*room.Room
	invite map[string]string // code -> room id
}

// New creates a manager.
func New(st store.Store, ent *entitlement.Resolver, opts room.Options, log *slog.Logger) *Manager {
	if log == nil {
		log = slog.Default()
	}
	return &Manager{st: st, ent: ent, roomOpts: opts, log: log, rooms: map[string]*room.Room{}, invite: map[string]string{}}
}

// UseCluster joins the manager to a cluster node. Call before Restore.
func (m *Manager) UseCluster(n *cluster.Node) {
	m.node = n
	n.Attach(func(id string) (room.Handle, bool) {
		m.mu.RLock()
		defer m.mu.RUnlock()
		r, ok := m.rooms[id]
		return r, ok
	}, func() []string {
		m.mu.RLock()
		defer m.mu.RUnlock()
		ids := make([]string, 0, len(m.rooms))
		for id := range m.rooms {
			ids = append(ids, id)
		}
		return ids
	})
}

// Restore reopens every lobby / in-progress game from the store. In a
// cluster each game is claimed first, so nodes booting together split the
// population instead of all hosting everything.
func (m *Manager) Restore(ctx context.Context) error {
	skipped := 0
	for _, status := range []string{store.StatusLobby, store.StatusInProgress} {
		recs, err := m.st.ListGames(ctx, status, false, 1000)
		if err != nil {
			return err
		}
		for _, rec := range recs {
			if m.node != nil && !m.node.Claim(ctx, rec.ID) {
				skipped++
				continue
			}
			r, err := m.open(rec, m.roomOpts)
			if err != nil {
				m.log.Error("restore room", "id", rec.ID, "err", err)
				continue
			}
			m.add(r, rec.InviteCode)
		}
	}
	m.log.Info("rooms restored", "count", len(m.rooms), "owned_elsewhere", skipped)
	return nil
}

// open starts a room for rec with the record's turn timeout applied.
func (m *Manager) open(rec *store.GameRecord, opts room.Options) (*room.Room, error) {
	if rec.TurnSeconds > 0 {
		opts.TurnTimeout = time.Duration(rec.TurnSeconds) * time.Second
	}
	return room.New(rec, m.st, opts)
}

// Counts reports live local rooms per status (metrics).
func (m *Manager) Counts() map[string]int {
	out := map[string]int{}
	for _, r := range m.Local() {
		if rec := r.Record(); rec != nil {
			out[rec.Status]++
		}
	}
	return out
}

func (m *Manager) add(r *room.Room, code string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rooms[r.ID()] = r
	if code != "" {
		m.invite[strings.ToUpper(code)] = r.ID()
	}
	if m.node != nil {
		m.node.SetInvite(context.Background(), code, r.ID())
	}
}

// Error is a user-facing lobby error.
type Error struct{ Code, Message string }

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func errf(code, format string, args ...any) error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Create validates the host's entitlements and opens a room.
func (m *Manager) Create(ctx context.Context, host *store.User, opts CreateOptions) (room.Handle, error) {
	caps := m.ent.Resolve(ctx, host)
	if opts.MaxPlayers == 0 {
		opts.MaxPlayers = caps.MaxPlayersPerRoom
	}
	if opts.MaxPlayers < game.MinPlayers || opts.MaxPlayers > game.MaxPlayers {
		return nil, errf("invalid", "maxPlayers must be %d-%d", game.MinPlayers, game.MaxPlayers)
	}
	if opts.MaxPlayers > caps.MaxPlayersPerRoom {
		return nil, errf("upgrade_required", "your plan allows up to %d players per room", caps.MaxPlayersPerRoom)
	}
	if opts.Visibility == "" {
		opts.Visibility = "public"
	}
	if opts.Visibility != "public" && opts.Visibility != "private" {
		return nil, errf("invalid", "visibility must be public or private")
	}
	if opts.Visibility == "private" && !caps.PrivateRooms {
		return nil, errf("upgrade_required", "private rooms require a premium plan")
	}
	if opts.TurnSeconds < 0 || opts.TurnSeconds > 600 {
		return nil, errf("invalid", "turnSeconds must be 0-600")
	}
	if opts.TurnSeconds == 0 {
		opts.TurnSeconds = 60
	}
	if caps.MaxConcurrentGames > 0 {
		active, err := m.activeGamesFor(ctx, host.ID)
		if err != nil {
			return nil, err
		}
		if active >= caps.MaxConcurrentGames {
			return nil, errf("upgrade_required", "your plan allows %d active game(s) at a time", caps.MaxConcurrentGames)
		}
	}

	cfgRec, err := m.st.GetPublishedConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("no published config: %w", err)
	}
	cfg := *cfgRec.Config
	cfg.Spaces = append([]game.SpaceDef(nil), cfg.Spaces...)
	if opts.Rules != nil {
		if !caps.HouseRules {
			return nil, errf("upgrade_required", "custom rules require a premium plan")
		}
		cfg.Rules = mergeRules(cfg.Rules, *opts.Rules)
		if err := cfg.Validate(); err != nil {
			return nil, errf("invalid", "rules: %v", err)
		}
	}
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = host.Name + "'s game"
	}
	if len(name) > 40 {
		name = name[:40]
	}
	rec := &store.GameRecord{
		ID: ids.New(), Name: name, Status: store.StatusLobby, HostID: host.ID, Visibility: opts.Visibility,
		MaxPlayers: opts.MaxPlayers, TurnSeconds: opts.TurnSeconds, ConfigID: cfgRec.ID, Config: &cfg,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if opts.Visibility == "private" {
		rec.InviteCode = ids.Code()
	}
	if err := m.st.SaveGame(ctx, rec); err != nil {
		return nil, err
	}
	if m.node != nil && !m.node.Claim(ctx, rec.ID) {
		return nil, fmt.Errorf("could not claim new game %s", rec.ID)
	}
	r, err := m.open(rec, m.roomOpts)
	if err != nil {
		return nil, err
	}
	m.add(r, rec.InviteCode)
	if err := r.Join(host); err != nil {
		return nil, err
	}
	return r, nil
}

// mergeRules applies only the house-rule toggles from o onto base; the
// structural numbers (supply, start cash) stay as published.
func mergeRules(base, o game.Rules) game.Rules {
	base.AuctionsEnabled = o.AuctionsEnabled
	base.FreeParkingJackpot = o.FreeParkingJackpot
	base.DoubleSalaryOnGoLanding = o.DoubleSalaryOnGoLanding
	base.NoRentInJail = o.NoRentInJail
	if o.TurnLimit >= 0 {
		base.TurnLimit = o.TurnLimit
	}
	if o.StartCash > 0 {
		base.StartCash = o.StartCash
	}
	return base
}

func (m *Manager) activeGamesFor(ctx context.Context, userID string) (int, error) {
	recs, err := m.st.ListGamesForUser(ctx, userID, 100)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, g := range recs {
		if g.Status != store.StatusFinished {
			n++
		}
	}
	return n, nil
}

// Get returns a handle for a live room: the local actor, a proxy to the
// node that owns it, or, when no live node owns it, a room adopted from
// the store.
func (m *Manager) Get(id string) (room.Handle, bool) {
	m.mu.RLock()
	r, ok := m.rooms[id]
	m.mu.RUnlock()
	if ok {
		return r, true
	}
	if m.node == nil {
		return nil, false
	}
	ctx := context.Background()
	if owner, ok := m.node.Owner(ctx, id); ok && owner != m.node.ID {
		return m.node.Proxy(id, owner), true
	}
	return m.adopt(ctx, id)
}

// adopt restores an unowned game from the store onto this node.
func (m *Manager) adopt(ctx context.Context, id string) (room.Handle, bool) {
	rec, err := m.st.GetGame(ctx, id)
	if err != nil || rec.Status == store.StatusFinished {
		return nil, false
	}
	if !m.node.Claim(ctx, id) {
		// Lost the race: whoever won is the owner now.
		if owner, ok := m.node.Owner(ctx, id); ok && owner != m.node.ID {
			return m.node.Proxy(id, owner), true
		}
		return nil, false
	}
	m.mu.RLock()
	r, ok := m.rooms[id]
	m.mu.RUnlock()
	if ok {
		return r, true
	}
	r, err = m.open(rec, m.roomOpts)
	if err != nil {
		m.log.Error("adopt room", "id", id, "err", err)
		return nil, false
	}
	m.add(r, rec.InviteCode)
	m.log.Info("adopted game from a departed node", "id", id, "status", rec.Status)
	return r, true
}

// ByInvite resolves an invite code.
func (m *Manager) ByInvite(code string) (room.Handle, bool) {
	code = strings.ToUpper(strings.TrimSpace(code))
	m.mu.RLock()
	id, ok := m.invite[code]
	m.mu.RUnlock()
	if !ok && m.node != nil {
		id, ok = m.node.LookupInvite(context.Background(), code)
	}
	if !ok {
		return nil, false
	}
	return m.Get(id)
}

// Join seats a user, enforcing the joiner's own plan limits too (a free user
// cannot sit in a 6-player room).
func (m *Manager) Join(ctx context.Context, r room.Handle, u *store.User) error {
	caps := m.ent.Resolve(ctx, u)
	info := r.Info()
	if info.MaxPlayers > caps.MaxPlayersPerRoom {
		return errf("upgrade_required", "your plan allows rooms of up to %d players", caps.MaxPlayersPerRoom)
	}
	if info.Visibility == "private" && !caps.PrivateRooms {
		return errf("upgrade_required", "private rooms require a premium plan")
	}
	if caps.MaxConcurrentGames > 0 {
		active, err := m.activeGamesFor(ctx, u.ID)
		if err != nil {
			return err
		}
		already := false
		for _, s := range info.Seats {
			if s.PlayerID == u.ID {
				already = true
			}
		}
		if !already && active >= caps.MaxConcurrentGames {
			return errf("upgrade_required", "your plan allows %d active game(s) at a time", caps.MaxConcurrentGames)
		}
	}
	return r.Join(u)
}

// ListPublic returns joinable public lobbies. Cluster-wide listings come
// from the store; rooms on this node contribute their live view.
func (m *Manager) ListPublic() []protocol.Lobby {
	var out []protocol.Lobby
	for _, info := range m.list(store.StatusLobby, true) {
		if info.Status == store.StatusLobby && info.Visibility == "public" {
			out = append(out, info)
		}
	}
	return out
}

// ListAll returns every lobby and in-progress game (admin listing), with
// the node hosting each one when clustered.
func (m *Manager) ListAll() []protocol.Lobby {
	out := m.list(store.StatusLobby, false)
	return append(out, m.list(store.StatusInProgress, false)...)
}

func (m *Manager) list(status string, publicOnly bool) []protocol.Lobby {
	local := map[string]*room.Room{}
	for _, r := range m.Local() {
		local[r.ID()] = r
	}
	var out []protocol.Lobby
	if m.node == nil {
		for _, r := range local {
			info := r.Info()
			if info.Status == status && (!publicOnly || info.Visibility == "public") {
				out = append(out, info)
			}
		}
		return out
	}
	ctx := context.Background()
	recs, err := m.st.ListGames(ctx, status, publicOnly, 500)
	if err != nil {
		m.log.Error("list games", "err", err)
		return nil
	}
	for _, rec := range recs {
		if r, ok := local[rec.ID]; ok {
			info := r.Info()
			info.Node = m.node.ID
			out = append(out, info)
			continue
		}
		info := room.LobbyFromRecord(rec)
		if owner, ok := m.node.Owner(ctx, rec.ID); ok {
			info.Node = owner
		} else {
			info.Node = "unowned"
		}
		out = append(out, info)
	}
	return out
}

// Local returns the rooms hosted by this process.
func (m *Manager) Local() []*room.Room {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*room.Room, 0, len(m.rooms))
	for _, r := range m.rooms {
		out = append(out, r)
	}
	return out
}

// Shutdown stops every local room and, when clustered, hands the games
// back so another node can pick them up as soon as clients reconnect.
func (m *Manager) Shutdown(ctx context.Context) {
	if m.node != nil {
		m.node.Leave(ctx)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, r := range m.rooms {
		r.Stop()
		delete(m.rooms, id)
	}
}

// Sweep stops finished rooms that have been idle, freeing memory.
func (m *Manager) Sweep(idle time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, r := range m.rooms {
		rec := r.Record()
		if rec == nil {
			delete(m.rooms, id)
			continue
		}
		if rec.Status == store.StatusFinished && time.Since(rec.UpdatedAt) > idle {
			r.Stop()
			delete(m.rooms, id)
			if rec.InviteCode != "" {
				delete(m.invite, strings.ToUpper(rec.InviteCode))
			}
			if m.node != nil {
				m.node.Release(context.Background(), id, rec.InviteCode)
			}
		}
	}
}

// EnsurePublishedConfig seeds the classic config when nothing is published.
func EnsurePublishedConfig(ctx context.Context, st store.Store) error {
	_, err := st.GetPublishedConfig(ctx)
	if err == nil {
		return nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	cfg := game.ClassicConfig()
	return st.SaveConfig(ctx, &store.ConfigRecord{ID: cfg.ID, Version: 1, Name: cfg.Name, Config: cfg, Published: true, CreatedBy: "system", CreatedAt: time.Now()})
}
