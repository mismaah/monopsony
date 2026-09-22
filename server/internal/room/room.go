// Package room hosts one live game. A Room is an actor: every operation is a
// closure executed on the room's single goroutine, so the engine state is
// never touched concurrently. Humans and bots feed the same command path.
package room

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"monopsony/server/internal/bot"
	"monopsony/server/internal/game"
	"monopsony/server/internal/metrics"
	"monopsony/server/internal/protocol"
	"monopsony/server/internal/store"
)

// Handle is the surface the gateway, REST and admin layers use. *Room
// implements it for games hosted on this node; the cluster package provides
// a proxy implementation for games owned by another node.
type Handle interface {
	ID() string
	Info() protocol.Lobby
	Record() *store.GameRecord
	Join(u *store.User) error
	Leave(userID string) error
	AddBot(byUserID, profile string) error
	Kick(byUserID, playerID string) error
	SetReady(userID string, ready bool)
	AssetsReady(userID string)
	StartGame(byUserID string) error
	Subscribe(sub Subscriber, lastSeq int)
	Unsubscribe(sub Subscriber)
	Command(userID string, cmd game.Command) error
	ForceEnd() error
	Chat(userID, text string)
}

// Subscriber is a connected client. Send must never block.
type Subscriber interface {
	UserID() string
	Send(env protocol.Envelope)
}

// Options tune timing. Zero values disable timers (useful in tests).
type Options struct {
	BotDelay    time.Duration // pause before a bot acts, for animation pacing
	TurnTimeout time.Duration // how long a human may take per decision
	// TradeTimeout is how long the recipient has to answer a trade offer. The
	// turn clock is paused meanwhile. Zero means the same as TurnTimeout.
	TradeTimeout time.Duration
	MaxTimeouts  int // consecutive timeouts before a bot takes the seat
	// AssetGrace caps how long StartGame holds the deal while players'
	// clients preload the table's 3D assets. Zero deals immediately.
	AssetGrace time.Duration
	Logger     *slog.Logger
	// Loadout resolves a user's equipped cosmetics; nil = none.
	Loadout func(userID string) map[string]string
	// Metrics receives command/event/bot counters; nil = the shared default.
	Metrics *metrics.Metrics
}

// Room is a lobby that becomes a game.
type Room struct {
	id    string
	rec   *store.GameRecord
	cfg   *game.Config
	st    store.Store
	opts  Options
	log   *slog.Logger
	inbox chan func()
	stop  chan struct{}
	once  sync.Once

	state     *game.State
	rng       game.RNG
	bots      map[string]*bot.Bot
	subs      map[Subscriber]struct{}
	connected map[string]int // playerID -> live connections
	ready     map[string]bool
	loaded    map[string]bool // playerID -> client has the table's assets
	timeouts  map[string]int
	deadline  time.Time
	timer     *time.Timer
	botTimer  *time.Timer
	// The deal waits for slow loaders: starting is set between StartGame and
	// the first hand, startAt is when the room deals regardless. Neither is
	// persisted - a room restored after a restart is a plain lobby again.
	starting   bool
	startAt    time.Time
	startTimer *time.Timer
	seqGen     int // bot seat counter

	// A pending trade pauses the turn clock: pausedTrade is the offer whose
	// deadline is armed, pausedTurn/pausedLeft what to restore afterwards.
	pausedTrade string
	pausedTurn  int
	pausedLeft  time.Duration
}

// New wraps a game record (lobby or in-progress) in a live room and starts
// its goroutine. In-progress games are rebuilt from the stored snapshot.
func New(rec *store.GameRecord, st store.Store, opts Options) (*Room, error) {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.MaxTimeouts <= 0 {
		opts.MaxTimeouts = 3
	}
	if opts.Metrics == nil {
		opts.Metrics = metrics.Default()
	}
	r := &Room{
		id: rec.ID, rec: rec, cfg: rec.Config, st: st, opts: opts,
		log:   opts.Logger.With("room", rec.ID),
		inbox: make(chan func(), 256), stop: make(chan struct{}),
		bots: map[string]*bot.Bot{}, subs: map[Subscriber]struct{}{},
		connected: map[string]int{}, ready: map[string]bool{}, loaded: map[string]bool{}, timeouts: map[string]int{},
		rng: game.NewSeededRNG(rand.Uint64()),
	}
	for _, s := range rec.Seats {
		if s.IsBot {
			r.bots[s.PlayerID] = bot.New(s.PlayerID, profile(s.BotProfile))
			r.seqGen++
		}
	}
	if rec.Status == store.StatusInProgress {
		if len(rec.State) == 0 {
			return nil, fmt.Errorf("room %s: in-progress game has no snapshot", rec.ID)
		}
		var s game.State
		if err := json.Unmarshal(rec.State, &s); err != nil {
			return nil, fmt.Errorf("room %s: decode snapshot: %w", rec.ID, err)
		}
		s.Bind(rec.Config)
		r.state = &s
	}
	go r.run()
	if r.state != nil {
		r.do(func() { r.afterChange(nil) })
	}
	return r, nil
}

func profile(name string) bot.Profile {
	switch name {
	case "cautious":
		return bot.Cautious
	case "aggressive":
		return bot.Aggressive
	default:
		return bot.Balanced
	}
}

var _ Handle = (*Room)(nil)

// ID returns the room/game id.
func (r *Room) ID() string { return r.id }

func (r *Room) run() {
	for {
		select {
		case f := <-r.inbox:
			f()
		case <-r.stop:
			return
		}
	}
}

// do posts work to the actor goroutine without waiting.
func (r *Room) do(f func()) {
	select {
	case r.inbox <- f:
	case <-r.stop:
	}
}

// call posts work and waits for its result.
func call[T any](r *Room, f func() (T, error)) (T, error) {
	type res struct {
		v   T
		err error
	}
	ch := make(chan res, 1)
	r.do(func() {
		v, err := f()
		ch <- res{v, err}
	})
	select {
	case out := <-ch:
		return out.v, out.err
	case <-r.stop:
		var zero T
		return zero, errors.New("room stopped")
	}
}

// Stop halts the actor. Pending work is dropped.
func (r *Room) Stop() {
	r.once.Do(func() {
		close(r.stop)
		if r.timer != nil {
			r.timer.Stop()
		}
		if r.botTimer != nil {
			r.botTimer.Stop()
		}
		if r.startTimer != nil {
			r.startTimer.Stop()
		}
	})
}

// ---- public API (thread-safe) ---------------------------------------------------

// Info returns the lobby view.
func (r *Room) Info() protocol.Lobby {
	v, _ := call(r, func() (protocol.Lobby, error) { return r.lobbyView(), nil })
	return v
}

// Record returns a copy of the persisted record.
func (r *Room) Record() *store.GameRecord {
	v, _ := call(r, func() (*store.GameRecord, error) {
		c := *r.rec
		c.Seats = append([]store.SeatRecord(nil), r.rec.Seats...)
		return &c, nil
	})
	return v
}

// Join seats a user in the lobby.
func (r *Room) Join(u *store.User) error {
	_, err := call(r, func() (struct{}, error) {
		if r.rec.Status != store.StatusLobby {
			return struct{}{}, errf("game_started", "the game has already started")
		}
		if r.starting {
			return struct{}{}, errf("game_starting", "the game is starting")
		}
		if r.seatOf(u.ID) != nil {
			return struct{}{}, nil
		}
		if len(r.rec.Seats) >= r.rec.MaxPlayers {
			return struct{}{}, errf("room_full", "the room is full")
		}
		r.rec.Seats = append(r.rec.Seats, store.SeatRecord{PlayerID: u.ID, UserID: u.ID, Name: u.Name, Loadout: r.loadoutFor(u.ID)})
		r.persist()
		r.broadcastLobby()
		return struct{}{}, nil
	})
	return err
}

// Leave removes a user from the lobby, or disconnects them mid-game.
func (r *Room) Leave(userID string) error {
	_, err := call(r, func() (struct{}, error) {
		seat := r.seatOf(userID)
		if seat == nil {
			return struct{}{}, nil
		}
		if r.rec.Status == store.StatusLobby {
			r.removeSeat(seat.PlayerID)
			if r.rec.HostID == userID {
				for _, s := range r.rec.Seats {
					if !s.IsBot {
						r.rec.HostID = s.UserID
						break
					}
				}
			}
			r.persist()
			if !r.dealIfLoaded() {
				r.broadcastLobby()
			}
		}
		return struct{}{}, nil
	})
	return err
}

// AddBot adds a bot seat (host only).
func (r *Room) AddBot(byUserID, prof string) error {
	_, err := call(r, func() (struct{}, error) {
		if err := r.requireHost(byUserID); err != nil {
			return struct{}{}, err
		}
		if r.rec.Status != store.StatusLobby {
			return struct{}{}, errf("game_started", "the game has already started")
		}
		if r.starting {
			return struct{}{}, errf("game_starting", "the game is starting")
		}
		if len(r.rec.Seats) >= r.rec.MaxPlayers {
			return struct{}{}, errf("room_full", "the room is full")
		}
		r.seqGen++
		p := profile(prof)
		id := fmt.Sprintf("bot-%d", r.seqGen)
		r.rec.Seats = append(r.rec.Seats, store.SeatRecord{PlayerID: id, Name: botName(p.Name, r.seqGen), IsBot: true, BotProfile: p.Name})
		r.bots[id] = bot.New(id, p)
		r.persist()
		r.broadcastLobby()
		return struct{}{}, nil
	})
	return err
}

func botName(profile string, n int) string {
	names := map[string][]string{
		"cautious":   {"Prudence", "Walter", "Ines"},
		"balanced":   {"Morgan", "Ada", "Kai"},
		"aggressive": {"Rex", "Vela", "Bruno"},
	}
	list := names[profile]
	return fmt.Sprintf("%s (bot)", list[n%len(list)])
}

// Kick removes a player (host only). In the lobby the seat is freed; during
// a game the seat is handed to a bot, exactly as after repeated timeouts, so
// the table stays balanced. The kicked user is detached from the seat and
// told so; they may keep watching but can no longer act or rejoin it.
func (r *Room) Kick(byUserID, playerID string) error {
	_, err := call(r, func() (struct{}, error) {
		if err := r.requireHost(byUserID); err != nil {
			return struct{}{}, err
		}
		if r.rec.Status == store.StatusFinished {
			return struct{}{}, errf("game_over", "the game is over")
		}
		seat := r.seatByPlayer(playerID)
		if seat == nil {
			return struct{}{}, errf("not_found", "no such seat")
		}
		if seat.UserID == byUserID {
			return struct{}{}, errf("invalid", "use leave (or surrender) to remove yourself")
		}
		if r.rec.Status == store.StatusLobby {
			r.removeSeat(playerID)
			r.persist()
			if !r.dealIfLoaded() {
				r.broadcastLobby()
			}
			return struct{}{}, nil
		}
		if seat.IsBot {
			return struct{}{}, errf("invalid", "that seat is already played by a bot")
		}
		kicked := seat.UserID
		seat.UserID = ""
		delete(r.connected, playerID)
		r.handToBot(seat)
		r.log.Info("player kicked; seat handed to bot", "player", playerID, "by", byUserID)
		for sub := range r.subs {
			if sub.UserID() == kicked {
				sub.Send(protocol.MustEncode(protocol.SKicked, "", protocol.Kicked{GameID: r.id, PlayerID: playerID}))
			}
		}
		// Re-issue everyone's view (the kicked user now has no actions) and
		// let the bot pick up the seat if it is being waited on.
		r.afterChange(nil)
		r.broadcastLobby()
		return struct{}{}, nil
	})
	return err
}

// handToBot puts a cautious bot in charge of a human seat. Only the seat
// record changes; the engine state is touched by commands alone.
func (r *Room) handToBot(seat *store.SeatRecord) {
	seat.IsBot = true
	seat.BotProfile = bot.Cautious.Name
	r.bots[seat.PlayerID] = bot.New(seat.PlayerID, bot.Cautious)
}

// SetReady toggles a lobby ready flag.
func (r *Room) SetReady(userID string, ready bool) {
	r.do(func() {
		if seat := r.seatOf(userID); seat != nil {
			r.ready[seat.PlayerID] = ready
			r.broadcastLobby()
		}
	})
}

// AssetsReady records that a player's client has finished preloading the
// table. Clients preload as soon as they sit down, so the host's start
// usually has nothing left to wait for; a report that arrives during the
// wait deals the game as soon as it is the last one outstanding.
func (r *Room) AssetsReady(userID string) {
	r.do(func() {
		seat := r.seatOf(userID)
		if seat == nil || r.rec.Status != store.StatusLobby || r.loaded[seat.PlayerID] {
			return
		}
		r.loaded[seat.PlayerID] = true
		if !r.dealIfLoaded() {
			r.broadcastLobby()
		}
	})
}

// StartGame begins play (host only). Dealing into a scene half the table
// cannot see yet is worse than a short pause, so the room first waits for
// every connected human's client to report its assets loaded - but only for
// Options.AssetGrace, after which it deals without the stragglers.
func (r *Room) StartGame(byUserID string) error {
	_, err := call(r, func() (struct{}, error) {
		if err := r.requireHost(byUserID); err != nil {
			return struct{}{}, err
		}
		if r.starting {
			return struct{}{}, errf("game_starting", "the game is already starting")
		}
		if r.rec.Status != store.StatusLobby {
			return struct{}{}, errf("game_started", "the game has already started")
		}
		if len(r.rec.Seats) < game.MinPlayers {
			return struct{}{}, errf("not_enough_players", "need at least %d players", game.MinPlayers)
		}
		if r.opts.AssetGrace <= 0 || len(r.stillLoading()) == 0 {
			return struct{}{}, r.deal()
		}
		grace := r.opts.AssetGrace
		r.starting = true
		r.startAt = time.Now().Add(grace)
		r.startTimer = time.AfterFunc(grace, func() { r.do(r.startGraceExpired) })
		r.log.Info("holding the deal for loading clients", "players", r.stillLoading(), "grace", grace)
		r.broadcastLobby()
		return struct{}{}, nil
	})
	return err
}

// stillLoading lists the seats the room would wait for: connected humans
// whose client has not reported in. Bots, empty seats and disconnected
// players are never waited on - nobody is watching those screens.
func (r *Room) stillLoading() []string {
	var out []string
	for _, s := range r.rec.Seats {
		if s.IsBot || s.UserID == "" || r.connected[s.PlayerID] == 0 || r.loaded[s.PlayerID] {
			continue
		}
		out = append(out, s.PlayerID)
	}
	return out
}

// dealIfLoaded deals once nobody is left to wait for, reporting whether it
// did. Every change to who is seated, connected or loaded runs through it.
func (r *Room) dealIfLoaded() bool {
	if !r.starting || len(r.stillLoading()) > 0 {
		return false
	}
	if err := r.deal(); err != nil {
		r.log.Error("start game", "err", err)
		return false
	}
	return true
}

// startGraceExpired deals without the clients that never reported: their
// scene fills in as the assets arrive, and one stuck client cannot hold the
// table hostage.
func (r *Room) startGraceExpired() {
	if !r.starting {
		return
	}
	if late := r.stillLoading(); len(late) > 0 {
		r.log.Info("dealing without fully loaded clients", "players", late)
	}
	if err := r.deal(); err != nil {
		r.log.Error("start game", "err", err)
		r.broadcastLobby()
	}
}

// stopWaiting leaves the waiting-for-clients phase.
func (r *Room) stopWaiting() {
	r.starting = false
	r.startAt = time.Time{}
	if r.startTimer != nil {
		r.startTimer.Stop()
		r.startTimer = nil
	}
}

// deal creates the game and pushes the first snapshot to every subscriber.
func (r *Room) deal() error {
	r.stopWaiting()
	// Belt and braces: whatever happened while we waited (an admin ending the
	// lobby, say), only a lobby is ever dealt.
	if r.rec.Status != store.StatusLobby {
		return errf("game_started", "the game has already started")
	}
	seats := make([]game.Seat, 0, len(r.rec.Seats))
	for _, s := range r.rec.Seats {
		seats = append(seats, game.Seat{ID: s.PlayerID, Name: s.Name, IsBot: s.IsBot})
	}
	st, evs, err := game.NewGame(r.cfg, seats, r.rng)
	if err != nil {
		return err
	}
	now := time.Now()
	r.state = st
	r.rec.Status = store.StatusInProgress
	r.rec.StartedAt = &now
	r.rec.Seq = 0
	r.afterChange(evs)
	r.broadcastLobby()
	for sub := range r.subs {
		r.sendSnapshot(sub)
	}
	return nil
}

// Subscribe attaches a connection and sends it the current view.
func (r *Room) Subscribe(sub Subscriber, lastSeq int) {
	r.do(func() {
		_, already := r.subs[sub]
		r.subs[sub] = struct{}{}
		if seat := r.seatOf(sub.UserID()); seat != nil {
			if !already { // a re-sent JoinGame must not inflate the presence count
				r.connected[seat.PlayerID]++
			}
			if lo := r.loadoutFor(sub.UserID()); lo != nil {
				seat.Loadout = lo
			}
		}
		if r.state == nil {
			sub.Send(protocol.MustEncode(protocol.SLobby, "", r.lobbyView()))
		} else if lastSeq > 0 && lastSeq < r.state.Seq {
			// Reconnect: replay the gap so the client can animate it, then snapshot.
			if evs, err := r.st.GetEvents(context.Background(), r.id, lastSeq); err == nil {
				for _, e := range evs {
					sub.Send(protocol.MustEncode(protocol.SEvent, "", protocol.Event{GameID: r.id, Seq: e.Seq, Type: e.Type, Payload: e.Payload}))
				}
			}
			r.sendSnapshot(sub)
		} else {
			r.sendSnapshot(sub)
		}
		r.broadcastLobby()
	})
}

// Unsubscribe detaches a connection.
func (r *Room) Unsubscribe(sub Subscriber) {
	r.do(func() {
		if _, ok := r.subs[sub]; !ok {
			return
		}
		delete(r.subs, sub)
		if seat := r.seatOf(sub.UserID()); seat != nil {
			if r.connected[seat.PlayerID]--; r.connected[seat.PlayerID] <= 0 {
				delete(r.connected, seat.PlayerID)
			}
		}
		if r.dealIfLoaded() {
			return
		}
		r.broadcastLobby()
	})
}

// Command applies a player's command.
func (r *Room) Command(userID string, cmd game.Command) error {
	_, err := call(r, func() (struct{}, error) {
		if r.state == nil {
			return struct{}{}, errf("not_started", "the game has not started")
		}
		seat := r.seatOf(userID)
		if seat == nil {
			return struct{}{}, errf("not_seated", "you are not playing in this game")
		}
		if seat.IsBot {
			return struct{}{}, errf("not_seated", "a bot controls this seat")
		}
		if cmd.Actor() != seat.PlayerID {
			return struct{}{}, errf("forbidden", "command is for another player")
		}
		evs, err := game.Apply(r.state, cmd, r.rng)
		if err != nil {
			r.opts.Metrics.Commands.WithLabelValues(cmd.CommandType(), "rejected").Inc()
			return struct{}{}, err
		}
		r.opts.Metrics.Commands.WithLabelValues(cmd.CommandType(), "ok").Inc()
		r.timeouts[seat.PlayerID] = 0
		r.afterChange(evs)
		return struct{}{}, nil
	})
	return err
}

// ForceEnd ends the game (admin action). Lobbies are simply closed.
func (r *Room) ForceEnd() error {
	_, err := call(r, func() (struct{}, error) {
		if r.state == nil {
			now := time.Now()
			r.stopWaiting() // a lobby closed mid-wait is never dealt
			r.rec.Status = store.StatusFinished
			r.rec.FinishedAt = &now
			r.persist()
			r.broadcastLobby()
			return struct{}{}, nil
		}
		evs := game.Abandon(r.state)
		if len(evs) > 0 {
			r.afterChange(evs)
		}
		return struct{}{}, nil
	})
	return err
}

// Chat relays a message to everyone in the room.
func (r *Room) Chat(userID, text string) {
	r.do(func() {
		seat := r.seatOf(userID)
		name := "spectator"
		pid := ""
		if seat != nil {
			name, pid = seat.Name, seat.PlayerID
		}
		if len(text) > 500 {
			text = text[:500]
		}
		r.broadcast(protocol.MustEncode(protocol.SChat, "", protocol.ChatMessage{GameID: r.id, PlayerID: pid, Name: name, Text: text, At: time.Now().UnixMilli()}))
	})
}

// ---- internals (actor goroutine only) ------------------------------------------------

type Error struct{ Code, Message string }

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func errf(code, format string, args ...any) error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

func (r *Room) loadoutFor(userID string) map[string]string {
	if r.opts.Loadout == nil || userID == "" {
		return nil
	}
	return r.opts.Loadout(userID)
}

func (r *Room) requireHost(userID string) error {
	if r.rec.HostID != userID {
		return errf("forbidden", "only the host can do that")
	}
	return nil
}

func (r *Room) seatOf(userID string) *store.SeatRecord {
	for i := range r.rec.Seats {
		if r.rec.Seats[i].UserID == userID && userID != "" {
			return &r.rec.Seats[i]
		}
	}
	return nil
}

func (r *Room) seatByPlayer(playerID string) *store.SeatRecord {
	for i := range r.rec.Seats {
		if r.rec.Seats[i].PlayerID == playerID {
			return &r.rec.Seats[i]
		}
	}
	return nil
}

func (r *Room) removeSeat(playerID string) {
	for i, s := range r.rec.Seats {
		if s.PlayerID == playerID {
			r.rec.Seats = append(r.rec.Seats[:i], r.rec.Seats[i+1:]...)
			delete(r.bots, playerID)
			delete(r.ready, playerID)
			delete(r.loaded, playerID)
			return
		}
	}
}

func (r *Room) seats() []protocol.SeatInfo {
	out := make([]protocol.SeatInfo, 0, len(r.rec.Seats))
	for _, s := range r.rec.Seats {
		out = append(out, protocol.SeatInfo{
			PlayerID: s.PlayerID, Name: s.Name, IsBot: s.IsBot,
			Connected: s.IsBot || r.connected[s.PlayerID] > 0,
			Ready:     r.ready[s.PlayerID], Loaded: s.IsBot || r.loaded[s.PlayerID],
			Host:    s.UserID != "" && s.UserID == r.rec.HostID,
			Loadout: s.Loadout,
		})
	}
	return out
}

func (r *Room) lobbyView() protocol.Lobby {
	v := protocol.Lobby{
		GameID: r.id, Status: r.rec.Status, Name: r.rec.Name, Visibility: r.rec.Visibility,
		InviteCode: r.rec.InviteCode, MaxPlayers: r.rec.MaxPlayers, ConfigID: r.rec.ConfigID,
		Rules: r.cfg.Rules, Seats: r.seats(), TurnSeconds: r.rec.TurnSeconds,
	}
	if r.starting {
		v.Starting, v.StartDeadline = true, r.startAt.UnixMilli()
	}
	return v
}

// LobbyFromRecord builds the lobby view of a game hosted elsewhere from its
// persisted record (used for cross-node listings, where live presence is
// unknown).
func LobbyFromRecord(rec *store.GameRecord) protocol.Lobby {
	seats := make([]protocol.SeatInfo, 0, len(rec.Seats))
	for _, s := range rec.Seats {
		seats = append(seats, protocol.SeatInfo{
			PlayerID: s.PlayerID, Name: s.Name, IsBot: s.IsBot, Connected: s.IsBot,
			Host: s.UserID != "" && s.UserID == rec.HostID, Loadout: s.Loadout,
		})
	}
	var rules game.Rules
	if rec.Config != nil {
		rules = rec.Config.Rules
	}
	return protocol.Lobby{
		GameID: rec.ID, Status: rec.Status, Name: rec.Name, Visibility: rec.Visibility,
		InviteCode: rec.InviteCode, MaxPlayers: rec.MaxPlayers, ConfigID: rec.ConfigID,
		Rules: rules, Seats: seats, TurnSeconds: rec.TurnSeconds,
	}
}

func (r *Room) deadlineMs() int64 {
	if r.deadline.IsZero() {
		return 0
	}
	return r.deadline.UnixMilli()
}

func (r *Room) sendSnapshot(sub Subscriber) {
	if r.state == nil {
		return
	}
	pid := ""
	if seat := r.seatOf(sub.UserID()); seat != nil {
		pid = seat.PlayerID
	}
	sub.Send(protocol.MustEncode(protocol.SSnapshot, "", protocol.Snapshot{
		GameID: r.id, Config: r.cfg, State: r.state, Seats: r.seats(),
		Legal: game.LegalActions(r.state, pid), WaitingOn: game.WaitingOn(r.state), Deadline: r.deadlineMs(),
	}))
}

func (r *Room) broadcast(env protocol.Envelope) {
	for sub := range r.subs {
		sub.Send(env)
	}
}

func (r *Room) broadcastLobby() {
	r.broadcast(protocol.MustEncode(protocol.SLobby, "", r.lobbyView()))
}

// afterChange persists, broadcasts events + legal actions, and arms timers.
// It runs after every successful command (and on restore with nil events).
func (r *Room) afterChange(evs []game.Event) {
	if len(evs) > 0 {
		envs := make([]game.EventEnvelope, 0, len(evs))
		base := r.state.Seq - len(evs)
		for i, ev := range evs {
			env, err := game.Wrap(base+i+1, ev)
			if err != nil {
				r.log.Error("encode event", "err", err)
				continue
			}
			envs = append(envs, env)
			r.opts.Metrics.Events.WithLabelValues(env.Type).Inc()
			r.broadcast(protocol.MustEncode(protocol.SEvent, "", protocol.Event{GameID: r.id, Seq: env.Seq, Type: env.Type, Payload: env.Payload}))
		}
		if err := r.st.AppendEvents(context.Background(), r.id, envs); err != nil {
			r.log.Error("append events", "err", err)
		}
	}
	if r.state.Turn.Phase == game.PhaseGameOver && r.rec.Status != store.StatusFinished {
		now := time.Now()
		r.rec.Status = store.StatusFinished
		r.rec.FinishedAt = &now
		r.rec.WinnerID = r.state.WinnerID
		r.deadline = time.Time{}
		if r.timer != nil {
			r.timer.Stop()
		}
	}
	r.persist()
	r.armTimer()
	waiting := game.WaitingOn(r.state)
	seats := r.seats()
	for sub := range r.subs {
		pid := ""
		if seat := r.seatOf(sub.UserID()); seat != nil {
			pid = seat.PlayerID
		}
		sub.Send(protocol.MustEncode(protocol.SUpdate, "", protocol.Update{
			GameID: r.id, Seq: r.state.Seq, State: r.state, Seats: seats, Actions: game.LegalActions(r.state, pid), WaitingOn: waiting, Deadline: r.deadlineMs(),
		}))
	}
	r.scheduleBots()
}

func (r *Room) persist() {
	r.rec.UpdatedAt = time.Now()
	if r.state != nil {
		b, err := json.Marshal(r.state)
		if err != nil {
			r.log.Error("encode snapshot", "err", err)
		} else {
			r.rec.State = b
			r.rec.Seq = r.state.Seq
		}
	}
	if err := r.st.SaveGame(context.Background(), r.rec); err != nil {
		r.log.Error("save game", "err", err)
	}
}

// scheduleBots arms a one-shot timer if any waiting player is a bot.
func (r *Room) scheduleBots() {
	if r.state == nil || r.state.Turn.Phase == game.PhaseGameOver {
		return
	}
	for _, pid := range game.WaitingOn(r.state) {
		if _, ok := r.bots[pid]; ok {
			if r.botTimer != nil {
				r.botTimer.Stop()
			}
			r.botTimer = time.AfterFunc(r.opts.BotDelay, func() { r.do(r.botStep) })
			return
		}
	}
}

// botStep lets one bot act, then re-schedules via afterChange.
func (r *Room) botStep() {
	if r.state == nil || r.state.Turn.Phase == game.PhaseGameOver {
		return
	}
	for _, pid := range game.WaitingOn(r.state) {
		b, ok := r.bots[pid]
		if !ok {
			continue
		}
		cmd := b.Decide(r.state)
		if cmd == nil {
			continue
		}
		evs, err := game.Apply(r.state, cmd, r.rng)
		if err != nil {
			r.log.Error("bot command rejected", "bot", pid, "cmd", cmd.CommandType(), "err", err)
			continue
		}
		r.opts.Metrics.BotActions.Inc()
		r.afterChange(evs)
		return
	}
}

// minResumeTime is the least a proposer gets back on their turn clock once a
// trade resolves, so an offer made at the buzzer still leaves room to act.
const minResumeTime = 5 * time.Second

// armTimer sets the human decision deadline. A pending trade gets its own
// TradeTimeout; the interrupted turn's remaining time is kept and restored
// when the trade resolves, so trades neither eat nor extend the turn.
func (r *Room) armTimer() {
	if r.timer != nil {
		r.timer.Stop()
	}
	now := time.Now()
	prev := r.deadline
	r.deadline = time.Time{}
	if r.state == nil || r.opts.TurnTimeout <= 0 || r.state.Turn.Phase == game.PhaseGameOver {
		r.pausedTrade = ""
		return
	}
	timeout := r.opts.TurnTimeout
	if t := r.state.Trade; t != nil {
		if r.pausedTrade != t.ID {
			// Freshly proposed: freeze the turn clock.
			r.pausedTrade, r.pausedTurn, r.pausedLeft = t.ID, r.state.Turn.Number, 0
			if !prev.IsZero() {
				r.pausedLeft = prev.Sub(now)
			}
		}
		timeout = r.opts.TradeTimeout
		if timeout <= 0 {
			timeout = r.opts.TurnTimeout
		}
	} else if r.pausedTrade != "" {
		// Trade resolved: resume the interrupted turn where it left off.
		if r.state.Turn.Number == r.pausedTurn && r.pausedLeft > 0 {
			timeout = max(r.pausedLeft, minResumeTime)
		}
		r.pausedTrade = ""
	}
	humanWaiting := false
	for _, pid := range game.WaitingOn(r.state) {
		if _, isBot := r.bots[pid]; !isBot {
			humanWaiting = true
		}
	}
	if !humanWaiting {
		return
	}
	r.deadline = now.Add(timeout)
	r.timer = time.AfterFunc(timeout, func() { r.do(r.onTimeout) })
}

// onTimeout applies the safe default for every waiting human and, after
// repeated timeouts, hands the seat to a bot.
func (r *Room) onTimeout() {
	if r.state == nil || r.state.Turn.Phase == game.PhaseGameOver {
		return
	}
	for _, pid := range game.WaitingOn(r.state) {
		if _, isBot := r.bots[pid]; isBot {
			continue
		}
		// Letting a trade offer lapse just declines it; it is not a sign
		// the player is away, so it does not count toward the bot takeover.
		if r.state.Trade == nil {
			r.timeouts[pid]++
		}
		r.opts.Metrics.Timeouts.Inc()
		if r.timeouts[pid] >= r.opts.MaxTimeouts {
			if seat := r.seatByPlayer(pid); seat != nil {
				r.handToBot(seat)
				r.log.Info("seat handed to bot after timeouts", "player", pid)
				r.broadcastLobby()
			}
			continue
		}
		cmd := bot.SafeDefault(r.state, pid)
		if cmd == nil {
			continue
		}
		if evs, err := game.Apply(r.state, cmd, r.rng); err == nil {
			r.afterChange(evs)
			return
		}
	}
	// Nothing applied (e.g. seat converted): let bots/timers re-evaluate.
	r.afterChange(nil)
}
